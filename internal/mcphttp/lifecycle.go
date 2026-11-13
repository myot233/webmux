package mcphttp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/mcpprotocol"
)

const (
	maxSessions     = 128
	maxGlobalTools  = 8
	maxSessionTools = 4
	readyTimeout    = 60 * time.Second
	idleTimeout     = 10 * time.Minute
	hardTimeout     = time.Hour
	callTimeout     = 185 * time.Second
)

type inflight struct {
	key       string
	cancel    context.CancelFunc
	terminal  bool
	cancelled bool
	tool      bool
}

type entry struct {
	id           string
	epoch        [32]byte
	protocol     *mcpprotocol.Session
	created      time.Time
	lastActivity time.Time
	calls        map[string]*inflight
	tools        int
	wg           sync.WaitGroup
}

// Handler owns one process-local, authenticated-owner MCP session namespace.
// It must be closed before its backing registry/client dependencies are closed.
type Handler struct {
	ctx      context.Context
	cancel   context.CancelFunc
	registry *mcpprotocol.Registry
	now      func() time.Time
	epoch    [32]byte
	mu       sync.Mutex
	sessions map[string]*entry
	tools    int
	closed   bool
	wg       sync.WaitGroup
	requests sync.WaitGroup
	done     chan struct{}
}

// New serves the JSON-only initialize-era Streamable HTTP profile. The service
// context, not an HTTP request context, owns accepted operations.
func New(ctx context.Context, registry *mcpprotocol.Registry) *Handler {
	ticker := time.NewTicker(time.Second)
	return newHandler(ctx, registry, time.Now, ticker.C, ticker.Stop)
}

// newHandler permits deterministic expiry tests without sleeping through TTLs.
func newHandler(ctx context.Context, registry *mcpprotocol.Registry, now func() time.Time, ticks <-chan time.Time, stop func()) *Handler {
	ctx, cancel := context.WithCancel(ctx)
	h := &Handler{ctx: ctx, cancel: cancel, registry: registry, now: now,
		sessions: make(map[string]*entry), done: make(chan struct{})}
	if _, err := rand.Read(h.epoch[:]); err != nil || registry == nil {
		h.closed = true
		cancel()
	}
	go h.scan(ticks, stop)
	return h
}

// Close cancels all accepted work and joins cleanup. It is safe to call more
// than once, including concurrently with requests and service cancellation.
func (h *Handler) Close() {
	h.cancel()
	<-h.done
}

func (h *Handler) scan(ticks <-chan time.Time, stop func()) {
	defer close(h.done)
	defer stop()
	for {
		select {
		case <-h.ctx.Done():
			h.mu.Lock()
			h.closed = true
			entries, cancels := h.expireLocked(true)
			h.mu.Unlock()
			cancelAll(cancels)
			joinEntries(entries)
			h.wg.Wait()
			h.requests.Wait()
			return
		case <-ticks:
			h.mu.Lock()
			entries, cancels := h.expireLocked(false)
			h.mu.Unlock()
			cancelAll(cancels)
			joinEntries(entries)
		}
	}
}

func (h *Handler) expired(e *entry, now time.Time) bool {
	return now.Sub(e.created) >= hardTimeout ||
		(!e.protocol.Ready() && now.Sub(e.created) >= readyTimeout) ||
		(len(e.calls) == 0 && now.Sub(e.lastActivity) >= idleTimeout)
}

// removeLocked commits cancellation under the same short lock as completion.
// Context cancellation itself runs outside that lock.
func (h *Handler) removeLocked(e *entry) []context.CancelFunc {
	delete(h.sessions, e.id)
	var cancels []context.CancelFunc
	for _, call := range e.calls {
		if !call.terminal {
			call.terminal, call.cancelled = true, true
			cancels = append(cancels, call.cancel)
		}
	}
	return cancels
}

func (h *Handler) expireLocked(all bool) ([]*entry, []context.CancelFunc) {
	now := h.now()
	var entries []*entry
	var cancels []context.CancelFunc
	for _, e := range h.sessions {
		if all || h.expired(e, now) {
			entries = append(entries, e)
			cancels = append(cancels, h.removeLocked(e)...)
		}
	}
	return entries, cancels
}

func cancelAll(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}

func joinEntries(entries []*entry) {
	for _, e := range entries {
		e.wg.Wait()
	}
}

func (h *Handler) allocate() (*entry, int) {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, 503
	}
	id := base64.RawURLEncoding.EncodeToString(random[:])
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.ctx.Err() != nil {
		return nil, 503
	}
	if len(h.sessions) >= maxSessions {
		return nil, 429
	}
	if h.sessions[id] != nil {
		return nil, 503
	}
	now := h.now()
	e := &entry{id: id, epoch: h.epoch, protocol: h.registry.NewSession(),
		created: now, lastActivity: now, calls: make(map[string]*inflight)}
	h.sessions[id] = e
	return e, 0
}

func (h *Handler) lookup(id, version string) (*entry, int) {
	h.mu.Lock()
	if h.closed || h.ctx.Err() != nil {
		h.mu.Unlock()
		return nil, 503
	}
	e := h.sessions[id]
	if e == nil || e.epoch != h.epoch {
		h.mu.Unlock()
		return nil, 404
	}
	if h.expired(e, h.now()) {
		cancels := h.removeLocked(e)
		h.mu.Unlock()
		cancelAll(cancels)
		e.wg.Wait()
		return nil, 404
	}
	if version != "" && version != e.protocol.ProtocolVersion() {
		h.mu.Unlock()
		return nil, 400
	}
	h.mu.Unlock()
	return e, 0
}

func (h *Handler) begin(e *entry, key string, tool bool) (context.Context, *inflight, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.ctx.Err() != nil {
		return nil, nil, 503
	}
	if h.sessions[e.id] != e || h.expired(e, h.now()) {
		return nil, nil, 404
	}
	if e.calls[key] != nil {
		return nil, nil, 409
	}
	if tool && (h.tools >= maxGlobalTools || e.tools >= maxSessionTools) {
		return nil, nil, 429
	}
	// Existing calls cannot outlive the entry's hard deadline, even while the
	// single scanner is joining cleanup from another expired entry.
	timeout := min(callTimeout, hardTimeout-h.now().Sub(e.created))
	ctx, cancel := context.WithTimeout(h.ctx, timeout)
	call := &inflight{key: key, cancel: cancel, tool: tool}
	e.calls[key] = call
	if tool {
		h.tools++
		e.tools++
	}
	h.wg.Add(1)
	e.wg.Add(1)
	return ctx, call, 0
}

// finish commits the completion/cancellation winner but retains the reservation
// and tool slot until response encoding and delivery have finished.
func (h *Handler) finish(e *entry, call *inflight) bool {
	h.mu.Lock()
	// Shutdown and the hard deadline may reach a handler before the scanner.
	if !call.terminal && (h.ctx.Err() != nil || h.now().Sub(e.created) >= hardTimeout) {
		call.cancelled = true
	}
	call.terminal = true
	cancelled := call.cancelled
	h.mu.Unlock()
	call.cancel()
	return cancelled
}

func (h *Handler) release(e *entry, call *inflight) {
	h.mu.Lock()
	delete(e.calls, call.key)
	if call.tool {
		h.tools--
		e.tools--
	}
	e.lastActivity = h.now()
	h.mu.Unlock()
	e.wg.Done()
	h.wg.Done()
}

func (h *Handler) cancelRequest(e *entry, key string) bool {
	h.mu.Lock()
	if h.sessions[e.id] != e || h.expired(e, h.now()) {
		h.mu.Unlock()
		return false
	}
	call := e.calls[key]
	var cancel context.CancelFunc
	if call != nil && !call.terminal {
		call.terminal, call.cancelled = true, true
		cancel = call.cancel
	}
	e.lastActivity = h.now()
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}
