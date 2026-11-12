package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRawPair struct {
	a, b *fakeRawChannel
}

type fakeRawChannel struct {
	mu       sync.Mutex
	peer     *fakeRawChannel
	onRecv   func([]byte)
	onClose  func()
	lowCb    func()
	lowTh    uint64
	buffered atomic.Uint64
	closed   bool
}

func newFakeRawPair() (*FramedConn, *FramedConn, *fakeRawPair) {
	aRaw := &fakeRawChannel{}
	bRaw := &fakeRawChannel{}
	aRaw.peer = bRaw
	bRaw.peer = aRaw
	aConn := newFramedConn(aRaw, nil)
	bConn := newFramedConn(bRaw, nil)
	aRaw.onRecv = bConn.FeedRaw
	aRaw.onClose = bConn.HandlePeerClose
	bRaw.onRecv = aConn.FeedRaw
	bRaw.onClose = aConn.HandlePeerClose
	return aConn, bConn, &fakeRawPair{a: aRaw, b: bRaw}
}

func (f *fakeRawChannel) Send(data []byte) error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return errors.New("closed")
	}
	onRecv := f.onRecv
	f.mu.Unlock()
	if onRecv != nil {
		cp := append([]byte(nil), data...)
		onRecv(cp)
	}
	return nil
}

func (f *fakeRawChannel) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	onClose := f.onClose
	f.mu.Unlock()
	if onClose != nil {
		go onClose()
	}
	return nil
}

func (f *fakeRawChannel) BufferedAmount() uint64 {
	return f.buffered.Load()
}

func (f *fakeRawChannel) SetBufferedAmountLowThreshold(th uint64) {
	f.mu.Lock()
	f.lowTh = th
	f.mu.Unlock()
}

func (f *fakeRawChannel) OnBufferedAmountLow(cb func()) {
	f.mu.Lock()
	f.lowCb = cb
	f.mu.Unlock()
}

func (f *fakeRawChannel) setBuffered(v uint64) {
	prev := f.buffered.Swap(v)
	f.mu.Lock()
	th := f.lowTh
	cb := f.lowCb
	f.mu.Unlock()
	if prev > th && v <= th && cb != nil {
		cb()
	}
}

func TestFramedConnLargeFragmentationAndCloseCodes(t *testing.T) {
	server, client, _ := newFakeRawPair()
	defer server.CloseNow()
	defer client.CloseNow()

	// 256 KiB binary snapshot (> 64 KiB SCTP single-chunk ceiling)
	client.SetReadLimit(1024 * 1024)
	large := make([]byte, 256*1024+137)
	if _, err := rand.Read(large); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := server.Write(ctx, MessageBinary, large); err != nil {
		t.Fatal(err)
	}
	typ, got, err := client.Read(ctx)
	if err != nil || typ != MessageBinary || !bytes.Equal(got, large) {
		t.Fatalf("large binary read typ=%v len=%d err=%v", typ, len(got), err)
	}

	// Empty text message roundtrip
	if err := client.Write(ctx, MessageText, []byte{}); err != nil {
		t.Fatal(err)
	}
	typ, got, err = server.Read(ctx)
	if err != nil || typ != MessageText || len(got) != 0 {
		t.Fatalf("empty text read typ=%v got=%q err=%v", typ, got, err)
	}

	// Application-level close code (4410 session exited)
	if err := server.Close(4410, "session exited"); err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Read(ctx)
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 4410 || ce.Reason != "session exited" {
		t.Fatalf("expected CloseError(4410, session exited), got %v", err)
	}
}

func TestFramedConnReadLimitAndBackpressure(t *testing.T) {
	server, client, pair := newFakeRawPair()
	defer server.CloseNow()
	defer client.CloseNow()

	// Exceeding readLimit rejects the stream.
	client.SetReadLimit(1024)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Write(ctx, MessageBinary, make([]byte, 2048))
	if _, _, err := client.Read(ctx); err == nil {
		t.Fatal("expected read limit error")
	}

	// Backpressure: bufferedAmount above maxBufferedAmount blocks until low or timeout.
	s2, c2, p2 := newFakeRawPair()
	defer s2.CloseNow()
	defer c2.CloseNow()

	p2.a.setBuffered(maxBufferedAmount + 100)
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer shortCancel()
	err := s2.Write(shortCtx, MessageText, []byte("blocked"))
	if !errors.Is(err, ErrSlowClient) {
		t.Fatalf("expected ErrSlowClient on stalled buffer, got %v", err)
	}

	// Draining buffer unblocks write.
	go func() {
		time.Sleep(20 * time.Millisecond)
		p2.a.setBuffered(bufferLowThreshold)
	}()
	okCtx, okCancel := context.WithTimeout(context.Background(), time.Second)
	defer okCancel()
	if err := s2.Write(okCtx, MessageText, []byte("unblocked")); err != nil {
		t.Fatalf("expected write to succeed once buffer drained, got %v", err)
	}
	_, got, err := c2.Read(okCtx)
	if err != nil || string(got) != "unblocked" {
		t.Fatalf("read after drain = %q, %v", got, err)
	}
	_ = pair
}

func TestFramedConnCloseReadCancelsContext(t *testing.T) {
	server, client, _ := newFakeRawPair()
	defer server.CloseNow()

	readCtx := server.CloseRead(context.Background())
	select {
	case <-readCtx.Done():
		t.Fatal("CloseRead canceled prematurely")
	default:
	}

	_ = client.Close(4409, "session moved")
	select {
	case <-readCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("CloseRead did not cancel after peer close")
	}
}
