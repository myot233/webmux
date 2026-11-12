package p2p

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const (
	frameTextFinal   byte = 0x01
	frameTextCont    byte = 0x02
	frameBinaryFinal byte = 0x03
	frameBinaryCont  byte = 0x04
	frameClose       byte = 0x05

	maxChunkPayload     = 32 * 1024
	defaultReadLimit    = 32 * 1024
	bufferLowThreshold  = 256 * 1024
	maxBufferedAmount   = 1024 * 1024
	closeReasonMaxBytes = 123
)

// CloseError reports an application-level close frame received from the peer.
type CloseError struct {
	Code   uint16
	Reason string
}

func (e *CloseError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("p2p stream closed (%d)", e.Code)
	}
	return fmt.Sprintf("p2p stream closed (%d): %s", e.Code, e.Reason)
}

// rawChannel abstracts a WebRTC DataChannel so framing, reassembly, close
// codes, read limits, and BufferedAmount backpressure can be unit-tested
// deterministically as well as driven by pion/webrtc.
type rawChannel interface {
	Send(data []byte) error
	Close() error
	BufferedAmount() uint64
	SetBufferedAmountLowThreshold(th uint64)
	OnBufferedAmountLow(f func())
}

type inboundMsg struct {
	typ  MessageType
	data []byte
	err  error
}

// FramedConn adapts a rawChannel into a StreamConn with 32 KiB fragmentation,
// application-level status close frames, read-size enforcement, and
// BufferedAmount slow-client backpressure.
type FramedConn struct {
	raw       rawChannel
	readLimit atomic.Int64

	writeMu   sync.Mutex
	lowNotify chan struct{}

	inCh      chan inboundMsg
	closedCh  chan struct{}
	closeOnce sync.Once
	onDone    func()

	fragTyp MessageType
	fragBuf []byte
}

func newFramedConn(raw rawChannel, onDone func()) *FramedConn {
	fc := &FramedConn{
		raw:       raw,
		lowNotify: make(chan struct{}, 1),
		inCh:      make(chan inboundMsg, 64),
		closedCh:  make(chan struct{}),
		onDone:    onDone,
	}
	fc.readLimit.Store(defaultReadLimit)
	raw.SetBufferedAmountLowThreshold(bufferLowThreshold)
	raw.OnBufferedAmountLow(func() {
		select {
		case fc.lowNotify <- struct{}{}:
		default:
		}
	})
	return fc
}

// FeedRaw processes one raw binary packet delivered by DataChannel.OnMessage.
func (c *FramedConn) FeedRaw(packet []byte) {
	if len(packet) == 0 {
		return
	}
	select {
	case <-c.closedCh:
		return
	default:
	}

	op := packet[0]
	body := packet[1:]
	limit := c.readLimit.Load()

	switch op {
	case frameClose:
		var code uint16 = 1000
		reason := ""
		if len(body) >= 2 {
			code = binary.BigEndian.Uint16(body[:2])
			reason = string(body[2:])
		}
		c.deliver(inboundMsg{err: &CloseError{Code: code, Reason: reason}})
		_ = c.CloseNow()
		return

	case frameTextCont, frameTextFinal, frameBinaryCont, frameBinaryFinal:
		msgTyp := MessageBinary
		if op == frameTextCont || op == frameTextFinal {
			msgTyp = MessageText
		}
		final := op == frameTextFinal || op == frameBinaryFinal

		if len(c.fragBuf) == 0 {
			c.fragTyp = msgTyp
		} else if c.fragTyp != msgTyp {
			c.deliver(inboundMsg{err: errors.New("p2p stream: mismatched fragment message type")})
			_ = c.CloseNow()
			return
		}

		if limit > 0 && int64(len(c.fragBuf)+len(body)) > limit {
			c.fragBuf = nil
			c.deliver(inboundMsg{err: fmt.Errorf("p2p stream: message exceeds read limit (%d bytes)", limit)})
			_ = c.CloseNow()
			return
		}

		if !final {
			c.fragBuf = append(c.fragBuf, body...)
			return
		}

		var complete []byte
		if len(c.fragBuf) == 0 {
			complete = append([]byte(nil), body...)
		} else {
			c.fragBuf = append(c.fragBuf, body...)
			complete = c.fragBuf
			c.fragBuf = nil
		}
		c.deliver(inboundMsg{typ: msgTyp, data: complete})

	default:
		c.deliver(inboundMsg{err: fmt.Errorf("p2p stream: unknown frame opcode 0x%02x", op)})
		_ = c.CloseNow()
	}
}

// HandlePeerClose notifies readers when the underlying DataChannel closes.
func (c *FramedConn) HandlePeerClose() {
	c.deliver(inboundMsg{err: io.EOF})
	_ = c.CloseNow()
}

func (c *FramedConn) deliver(m inboundMsg) {
	select {
	case <-c.closedCh:
		return
	default:
	}
	select {
	case c.inCh <- m:
	case <-c.closedCh:
	}
}

func (c *FramedConn) Read(ctx context.Context) (MessageType, []byte, error) {
	select {
	case m := <-c.inCh:
		if m.err != nil {
			return 0, nil, m.err
		}
		return m.typ, m.data, nil
	case <-c.closedCh:
		select {
		case m := <-c.inCh:
			if m.err != nil {
				return 0, nil, m.err
			}
			return m.typ, m.data, nil
		default:
			return 0, nil, io.EOF
		}
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

func (c *FramedConn) Write(ctx context.Context, typ MessageType, p []byte) error {
	var contOp, finalOp byte
	switch typ {
	case MessageText:
		contOp, finalOp = frameTextCont, frameTextFinal
	case MessageBinary:
		contOp, finalOp = frameBinaryCont, frameBinaryFinal
	default:
		return fmt.Errorf("unsupported message type %v", typ)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if len(p) == 0 {
		return c.sendPacketLocked(ctx, []byte{finalOp})
	}

	for offset := 0; offset < len(p); offset += maxChunkPayload {
		end := offset + maxChunkPayload
		op := contOp
		if end >= len(p) {
			end = len(p)
			op = finalOp
		}
		chunk := make([]byte, 1+(end-offset))
		chunk[0] = op
		copy(chunk[1:], p[offset:end])
		if err := c.sendPacketLocked(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *FramedConn) sendPacketLocked(ctx context.Context, pkt []byte) error {
	for c.raw.BufferedAmount() > maxBufferedAmount {
		select {
		case <-c.closedCh:
			return io.ErrClosedPipe
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrSlowClient, ctx.Err())
		case <-c.lowNotify:
		}
	}
	select {
	case <-c.closedCh:
		return io.ErrClosedPipe
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return c.raw.Send(pkt)
}

func (c *FramedConn) Close(code uint16, reason string) error {
	if len(reason) > closeReasonMaxBytes {
		reason = reason[:closeReasonMaxBytes]
	}
	pkt := make([]byte, 3+len(reason))
	pkt[0] = frameClose
	binary.BigEndian.PutUint16(pkt[1:3], code)
	copy(pkt[3:], reason)

	c.writeMu.Lock()
	select {
	case <-c.closedCh:
	default:
		_ = c.raw.Send(pkt)
	}
	c.writeMu.Unlock()
	return c.CloseNow()
}

func (c *FramedConn) CloseNow() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closedCh)
		err = c.raw.Close()
		if c.onDone != nil {
			c.onDone()
		}
	})
	return err
}

func (c *FramedConn) SetReadLimit(n int64) {
	c.readLimit.Store(n)
}

func (c *FramedConn) CloseRead(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		for {
			if _, _, err := c.Read(ctx); err != nil {
				return
			}
		}
	}()
	return ctx
}
