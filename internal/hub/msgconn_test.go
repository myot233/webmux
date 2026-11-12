package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/voice"
)

type recordedFrame struct {
	typ  MessageType
	data []byte
}

type fakeMessageConn struct {
	mu          sync.Mutex
	inbound     chan recordedFrame
	peerClosed  chan struct{}
	closeNowCh  chan struct{}
	closeNowCnt int
	closed      bool
	closeCode   StatusCode
	closeReason string
	readLimit   int64
	written     []recordedFrame
	writeHook   func(ctx context.Context, typ MessageType, p []byte) error
}

var _ MessageConn = (*fakeMessageConn)(nil)

func newFakeMessageConn() *fakeMessageConn {
	return &fakeMessageConn{
		inbound:    make(chan recordedFrame, 16),
		peerClosed: make(chan struct{}),
		closeNowCh: make(chan struct{}),
	}
}

func (f *fakeMessageConn) sendFromPeer(typ MessageType, data []byte) {
	f.inbound <- recordedFrame{typ: typ, data: bytes.Clone(data)}
}

func (f *fakeMessageConn) closeFromPeer() {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.peerClosed:
	default:
		close(f.peerClosed)
	}
}

func (f *fakeMessageConn) Read(ctx context.Context) (MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case <-f.closeNowCh:
		return 0, nil, errors.New("connection closed")
	case frame := <-f.inbound:
		f.mu.Lock()
		limit := f.readLimit
		f.mu.Unlock()
		if limit > 0 && int64(len(frame.data)) > limit {
			return 0, nil, fmt.Errorf("frame length %d exceeds limit %d", len(frame.data), limit)
		}
		return frame.typ, frame.data, nil
	case <-f.peerClosed:
		return 0, nil, errors.New("EOF")
	}
}

func (f *fakeMessageConn) Write(ctx context.Context, typ MessageType, p []byte) error {
	f.mu.Lock()
	hook := f.writeHook
	f.mu.Unlock()
	if hook != nil {
		if err := hook(ctx, typ, p); err != nil {
			return err
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.closeNowCh:
		return errors.New("connection closed")
	default:
	}
	f.mu.Lock()
	f.written = append(f.written, recordedFrame{typ: typ, data: bytes.Clone(p)})
	f.mu.Unlock()
	return nil
}

func (f *fakeMessageConn) Close(code StatusCode, reason string) error {
	f.mu.Lock()
	if !f.closed {
		f.closed = true
		f.closeCode = code
		f.closeReason = reason
	}
	f.mu.Unlock()
	return f.CloseNow()
}

func (f *fakeMessageConn) CloseNow() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeNowCnt++
	select {
	case <-f.closeNowCh:
	default:
		close(f.closeNowCh)
	}
	return nil
}

func (f *fakeMessageConn) SetReadLimit(n int64) {
	f.mu.Lock()
	f.readLimit = n
	f.mu.Unlock()
}

func (f *fakeMessageConn) CloseRead(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		select {
		case <-ctx.Done():
		case <-f.closeNowCh:
		case <-f.peerClosed:
		case <-f.inbound:
		}
	}()
	return ctx
}

func (f *fakeMessageConn) status() (closed bool, code StatusCode, reason string, limit int64, closeNows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed, f.closeCode, f.closeReason, f.readLimit, f.closeNowCnt
}

func (f *fakeMessageConn) frames() []recordedFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedFrame, len(f.written))
	copy(out, f.written)
	return out
}

func TestNewWebSocketConnNil(t *testing.T) {
	if got := NewWebSocketConn(nil); got != nil {
		t.Fatalf("NewWebSocketConn(nil) = %#v, want nil", got)
	}
	if StatusNotFound != 4404 {
		t.Fatalf("StatusNotFound = %d, want 4404", StatusNotFound)
	}
}

func TestMessageConnWriteMessagesCloseCodesAndDeadline(t *testing.T) {
	movedSide := string(hybrid.SideACP)
	exitZero := 0

	tests := []struct {
		name       string
		giveMsg    *session.Message
		giveSlow   bool
		failWrite  bool
		wantCode   StatusCode
		wantReason string
	}{
		{
			name:       "slow-subscriber",
			giveSlow:   true,
			wantCode:   StatusSlowClient,
			wantReason: "slow client",
		},
		{
			name:       "write-timeout-10s-deadline",
			giveMsg:    &session.Message{Frame: []byte{contract.FrameOutput, 'x'}},
			failWrite:  true,
			wantCode:   StatusSlowClient,
			wantReason: "write timeout",
		},
		{
			name:       "moved-to-chat",
			giveMsg:    &session.Message{Moved: &movedSide},
			wantCode:   contract.CloseMoved,
			wantReason: "session moved: chat",
		},
		{
			name:       "invalid-message",
			giveMsg:    &session.Message{},
			wantCode:   StatusInternalError,
			wantReason: "invalid session message",
		},
		{
			name:       "session-exited",
			giveMsg:    &session.Message{Exit: &exitZero},
			wantCode:   StatusSessionExited,
			wantReason: "session exited",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newFakeMessageConn()
			var sawDeadline bool
			if tt.failWrite {
				conn.writeHook = func(ctx context.Context, _ MessageType, _ []byte) error {
					deadline, ok := ctx.Deadline()
					remaining := time.Until(deadline)
					sawDeadline = ok && remaining > 5*time.Second && remaining <= 10*time.Second
					return context.DeadlineExceeded
				}
			}
			sub := &session.Subscriber{
				Out:  make(chan session.Message, 2),
				Slow: make(chan struct{}),
			}
			if tt.giveSlow {
				close(sub.Slow)
			}
			if tt.giveMsg != nil {
				sub.Out <- *tt.giveMsg
			}

			writeMessages(t.Context(), conn, sub, false)

			closed, code, reason, _, _ := conn.status()
			if !closed || code != tt.wantCode || reason != tt.wantReason {
				t.Fatalf("close = (%v, %d, %q), want (true, %d, %q)", closed, code, reason, tt.wantCode, tt.wantReason)
			}
			if tt.failWrite && !sawDeadline {
				t.Fatal("write did not attach ~10s context deadline")
			}
		})
	}
}

func TestMessageConnAttachReadLimitAndCleanDetach(t *testing.T) {
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	s, err := m.Create(session.CreateInput{Cmd: []string{"cat"}})
	if err != nil {
		t.Fatal(err)
	}

	conn := newFakeMessageConn()
	// Oversized frame (> 1 MiB) causes Read to fail and Attach to exit cleanly
	// without terminating the underlying persistent cat session.
	oversized := bytes.Repeat([]byte{'a'}, 1024*1024+1)
	conn.sendFromPeer(MessageBinary, oversized)

	done := make(chan struct{})
	go func() {
		defer close(done)
		Attach(t.Context(), s, conn, session.AttachOptions{}, false)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Attach did not exit after oversized frame error")
	}

	_, _, _, limit, closeNows := conn.status()
	if limit != 1024*1024 {
		t.Fatalf("readLimit = %d, want %d", limit, 1024*1024)
	}
	if closeNows == 0 {
		t.Fatal("Attach did not call CloseNow on exit")
	}
	if got := s.Info().Status; got == session.StatusExited {
		t.Fatalf("session status = %s, want running persistent session", got)
	}
}

func TestMessageConnCloseReadHalfCloseOnWriteOnlyStreams(t *testing.T) {
	t.Run("List", func(t *testing.T) {
		m := session.NewManager(session.Options{})
		defer m.Shutdown(context.Background())

		conn := newFakeMessageConn()
		done := make(chan struct{})
		go func() {
			defer close(done)
			List(t.Context(), m, nil, conn)
		}()

		deadline := time.After(5 * time.Second)
		for len(conn.frames()) == 0 {
			select {
			case <-deadline:
				t.Fatal("List did not write initial snapshot")
			default:
				time.Sleep(time.Millisecond)
			}
		}
		frames := conn.frames()
		if frames[0].typ != MessageText {
			t.Fatalf("snapshot message type = %v, want MessageText", frames[0].typ)
		}

		// Sending unexpected inbound data on a write-only stream triggers CloseRead cancellation.
		conn.sendFromPeer(MessageText, []byte("unexpected"))
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("List did not exit after CloseRead cancelled")
		}
	})

	t.Run("SettingsTryAgainLaterAndHalfClose", func(t *testing.T) {
		events := make(chan settings.Event)
		close(events)
		conn := newFakeMessageConn()
		Settings(t.Context(), &settings.Subscription{Events: events}, conn)

		closed, code, reason, _, _ := conn.status()
		if !closed || code != StatusTryAgainLater || reason != "settings changed; reconnect" {
			t.Fatalf("Settings close = (%v, %d, %q), want (true, %d, %q)", closed, code, reason, StatusTryAgainLater, "settings changed; reconnect")
		}

		openEvents := make(chan settings.Event)
		conn2 := newFakeMessageConn()
		done := make(chan struct{})
		go func() {
			defer close(done)
			Settings(t.Context(), &settings.Subscription{Events: openEvents}, conn2)
		}()
		conn2.closeFromPeer()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Settings did not exit after peer closed")
		}
	})
}

func TestMessageConnCloseChatCodes(t *testing.T) {
	tests := []struct {
		name       string
		give       error
		wantCode   StatusCode
		wantReason string
	}{
		{
			name:       "slow",
			give:       hybrid.ErrSlow,
			wantCode:   StatusSlowClient,
			wantReason: "slow client",
		},
		{
			name:       "reset",
			give:       hybrid.ErrReset,
			wantCode:   StatusTryAgainLater,
			wantReason: "chat log reset; reconnect",
		},
		{
			name:       "ended",
			give:       hybrid.ErrEnded,
			wantCode:   StatusSessionExited,
			wantReason: "session ended",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newFakeMessageConn()
			closeChat(conn, tt.give)
			closed, code, reason, _, _ := conn.status()
			if !closed || code != tt.wantCode || reason != tt.wantReason {
				t.Fatalf("closeChat(%v) = (%v, %d, %q), want (true, %d, %q)", tt.give, closed, code, reason, tt.wantCode, tt.wantReason)
			}
		})
	}
}

func TestMessageConnVoiceNormalClosureAndReadLimit(t *testing.T) {
	svc := voice.New(voice.Options{
		Cmd:           []string{"sh", "-c", `cat >/dev/null; printf '{"type":"final","text":"hello"}\n'`},
		Cwd:           t.TempDir(),
		ReadyDelay:    5 * time.Millisecond,
		FinishTimeout: 2 * time.Second,
	})
	sess, err := svc.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	conn := newFakeMessageConn()
	conn.sendFromPeer(MessageBinary, []byte{0, 1, 2, 3})
	stopMsg, err := json.Marshal(contract.VoiceClientMessage{Type: "stop"})
	if err != nil {
		t.Fatal(err)
	}
	conn.sendFromPeer(MessageText, stopMsg)

	Voice(t.Context(), sess, svc.FinishTimeout(), conn)

	closed, code, reason, limit, _ := conn.status()
	if limit != 65536 {
		t.Fatalf("voice readLimit = %d, want 65536", limit)
	}
	if !closed || code != StatusNormalClosure || reason != "done" {
		t.Fatalf("voice close = (%v, %d, %q), want (true, %d, %q)", closed, code, reason, StatusNormalClosure, "done")
	}
	frames := conn.frames()
	if len(frames) == 0 {
		t.Fatal("expected voice events to be written")
	}
	var sawFinal bool
	for _, f := range frames {
		if f.typ != MessageText {
			t.Fatalf("unexpected voice frame type %v", f.typ)
		}
		var ev contract.VoiceServerEvent
		if err := json.Unmarshal(f.data, &ev); err != nil {
			t.Fatalf("unmarshal voice event: %v", err)
		}
		if ev.Type == "final" && ev.Text == "hello" {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Fatalf("did not observe final voice event in %d frames", len(frames))
	}
}
