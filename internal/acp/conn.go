package acp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
)

const (
	// writeQueueSize absorbs requests, notifications and answers issued
	// together (a cancel plus permission answers) while the single writer
	// is busy; callers block beyond it, bounded by their context.
	writeQueueSize = 16
	// inboxSize lets the reader run ahead of a slow handler by a burst of
	// streamed updates. Beyond it the reader stops reading, and the agent
	// blocks on its stdout pipe rather than the client buffering without
	// bound.
	inboxSize = 256
	// readBufferSize is the bufio buffer for agent output; longer lines are
	// assembled from several fills up to MaxLineBytes.
	readBufferSize = 64 << 10
	// retainedLineBuffer is the largest line buffer kept between reads, so
	// one huge tool output does not pin its memory for the connection's
	// lifetime.
	retainedLineBuffer = 1 << 20
)

// Handler receives what the agent sends on its own initiative.
//
// SessionUpdate is called for each session/update notification, in arrival
// order, on one goroutine; it must not wait for a reply from this Conn.
// Every update the agent sent before a response is handled before the
// matching call returns, so updates replayed by session/load are seen before
// LoadSession returns.
//
// RequestPermission and CreateElicitation each run on their own goroutine,
// started in arrival order relative to updates. ctx ends when the agent
// sends $/cancel_request for the request or the connection ends; the
// handler must then return promptly. A returned *RPCError is sent as is;
// a context error is sent as CodeRequestCancelled and any other error as
// CodeInternalError without its text.
type Handler interface {
	SessionUpdate(n SessionNotification)
	RequestPermission(ctx context.Context, req RequestPermissionRequest) (RequestPermissionResponse, error)
	CreateElicitation(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error)
}

// Options configures a Conn.
type Options struct {
	// Handler receives updates and agent requests. Nil suits short-lived
	// probes such as session/list: updates are dropped and agent requests
	// are answered with CodeMethodNotFound.
	Handler Handler
	// Logger receives protocol error kinds and line lengths, malformed
	// update kinds and write failures; never message content. Nil discards.
	Logger *slog.Logger
}

// Conn is a JSON-RPC 2.0 connection to one ACP agent over its stdout (r)
// and stdin (w). The caller owns the process: Conn never starts or kills
// it. One goroutine reads lines, one dispatches them in order, and one
// writes; the reader runs until r ends.
type Conn struct {
	w   io.WriteCloser
	h   Handler
	log *slog.Logger

	nextID atomic.Int64
	out    chan []byte
	inbox  chan wireMessage

	mu       sync.Mutex
	pending  map[int64]chan wireMessage
	incoming map[string]context.CancelFunc
	err      error
	readErr  error // set by the reader before it closes inbox

	dead     chan struct{}
	deadOnce sync.Once
	base     context.Context
	stop     context.CancelFunc
	loops    sync.WaitGroup
	requests sync.WaitGroup
	done     chan struct{}
}

// NewConn starts a connection. w must allow Close while a Write is blocked
// (os pipes and io.Pipe do), so a stalled agent cannot wedge Close.
func NewConn(r io.Reader, w io.WriteCloser, o Options) *Conn {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	h := o.Handler
	if h == nil {
		h = noHandler{}
	}
	base, stop := context.WithCancel(context.Background())
	c := &Conn{
		w:        w,
		h:        h,
		log:      log,
		out:      make(chan []byte, writeQueueSize),
		inbox:    make(chan wireMessage, inboxSize),
		pending:  make(map[int64]chan wireMessage),
		incoming: make(map[string]context.CancelFunc),
		dead:     make(chan struct{}),
		base:     base,
		stop:     stop,
		done:     make(chan struct{}),
	}
	c.loops.Add(3)
	go c.readLoop(r)
	go c.dispatchLoop()
	go c.writeLoop()
	go func() {
		c.loops.Wait()
		c.requests.Wait()
		close(c.done)
	}()
	return c
}

// Close ends the connection: pending calls fail with ErrClosed and the
// agent's stdin is closed. Output still arriving is read and discarded
// until the agent's stdout ends, so the agent can exit.
func (c *Conn) Close() error {
	c.fail(ErrClosed)
	return nil
}

// Done is closed once the agent's stdout has ended (the process exited or
// closed it) and the dispatcher, writer and every request handler have
// returned. A caller waits for Done before reaping the process.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err reports why the connection ended: ErrClosed, an error wrapping
// ErrDisconnected, or a *ProtocolError. It is nil while the connection is
// open.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Conn) fail(err error) {
	c.deadOnce.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.dead)
		c.stop()
		var perr *ProtocolError
		if errors.As(err, &perr) {
			c.log.Warn("acp protocol error; disconnecting", "reason", perr.Reason, "bytes", perr.Bytes)
		}
		// Closing stdin tells the agent to exit and unblocks a stalled
		// write; the error adds nothing once the connection is over.
		_ = c.w.Close()
	})
}

// readLoop parses agent output until it ends, breaks the protocol or the
// connection dies, then keeps reading and discarding it until EOF so an
// agent whose stdin was closed can finish writing and exit instead of
// blocking on a full pipe.
func (c *Conn) readLoop(r io.Reader) {
	defer c.loops.Done()
	br := bufio.NewReaderSize(r, readBufferSize)
	c.setReadErr(c.readMessages(br))
	close(c.inbox)
	_, _ = io.Copy(io.Discard, br) // read errors only end the drain
}

func (c *Conn) readMessages(br *bufio.Reader) error {
	var buf []byte
	for {
		line, err := readLine(br, buf)
		if err != nil {
			return err
		}
		if cap(line) <= retainedLineBuffer {
			buf = line
		} else {
			buf = nil
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		m, err := parseMessage(line)
		if err != nil {
			return err
		}
		select {
		case c.inbox <- m:
		case <-c.dead:
			return ErrClosed
		}
	}
}

func (c *Conn) setReadErr(err error) {
	var perr *ProtocolError
	if !errors.As(err, &perr) && !errors.Is(err, ErrClosed) {
		if errors.Is(err, io.EOF) {
			err = ErrDisconnected
		} else {
			err = fmt.Errorf("%w: read: %w", ErrDisconnected, err)
		}
	}
	c.mu.Lock()
	c.readErr = err
	c.mu.Unlock()
}

// dispatchLoop handles messages in arrival order. Responses are delivered
// here rather than by the reader so that a call never returns before the
// updates the agent sent ahead of its response have been handled.
func (c *Conn) dispatchLoop() {
	defer c.loops.Done()
	for m := range c.inbox {
		switch {
		case m.isRequest():
			c.startRequest(m)
		case m.isNotification():
			c.notification(m)
		default:
			c.deliver(m)
		}
	}
	c.mu.Lock()
	err := c.readErr
	c.mu.Unlock()
	c.fail(err)
}

func (c *Conn) writeLoop() {
	defer c.loops.Done()
	for {
		select {
		case b := <-c.out:
			if _, err := c.w.Write(b); err != nil {
				c.log.Warn("acp write failed", "error", err)
				c.fail(fmt.Errorf("%w: write: %w", ErrDisconnected, err))
				return
			}
		case <-c.dead:
			return
		}
	}
}

// enqueue hands an encoded message to the writer.
func (c *Conn) enqueue(ctx context.Context, b []byte) error {
	select {
	case <-c.dead:
		return c.Err()
	default:
	}
	select {
	case c.out <- b:
		return nil
	case <-c.dead:
		return c.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Conn) deliver(m wireMessage) {
	id, err := strconv.ParseInt(string(m.ID), 10, 64)
	if err != nil {
		return // not an id this client issued
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- m
	}
}

func (c *Conn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}
