package hybrid

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"
)

const (
	// dedupeEntries and dedupeTTL bound the remembered client request IDs
	// of one session.
	dedupeEntries = 256
	dedupeTTL     = 30 * time.Minute
	// commandTimeout bounds one command, including any agent call it makes.
	// The command runs to completion even if its request ends, so a retry
	// learns the real outcome.
	commandTimeout = 30 * time.Second
)

// requestIDPattern is a client request ID: the client's own random ID.
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// dedupe remembers the outcome of recent commands by client request ID, so
// a retried request gets the first outcome (success or error) without
// running again. It has its own lock: commands wait on it outside the actor.
type dedupe struct {
	mu      sync.Mutex
	entries map[string]*dedupeEntry
	// order holds the entries oldest first.
	order []*dedupeEntry
}

type dedupeEntry struct {
	key     string
	command string
	at      time.Time
	done    chan struct{}
	value   any
	err     error
}

// once runs fn for the first request with key and returns its outcome to
// every request with that key within dedupeTTL. A key reused for another
// command is invalid. fn gets a context that outlives the request.
func once[T any](ctx context.Context, d *dedupe, key, command string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if !requestIDPattern.MatchString(key) {
		return zero, fmt.Errorf("%w: clientRequestId 必须是 8 到 64 个字母、数字、- 或 _", ErrInvalid)
	}
	e, first := d.claim(key, command, time.Now())
	if e.command != command {
		return zero, fmt.Errorf("%w: clientRequestId 已用于另一个请求", ErrInvalid)
	}
	if first {
		go func() {
			run, cancel := context.WithTimeout(context.WithoutCancel(ctx), commandTimeout)
			defer cancel()
			v, err := fn(run)
			e.value, e.err = v, err
			close(e.done)
		}()
	}
	select {
	case <-e.done:
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	if e.err != nil {
		return zero, e.err
	}
	v, _ := e.value.(T)
	return v, nil
}

// claim returns the live entry for key, or registers a new one.
func (d *dedupe) claim(key, command string, now time.Time) (*dedupeEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries == nil {
		d.entries = make(map[string]*dedupeEntry)
	}
	for len(d.order) > 0 && (len(d.order) >= dedupeEntries || now.Sub(d.order[0].at) > dedupeTTL) {
		old := d.order[0]
		d.order[0] = nil
		d.order = d.order[1:]
		if d.entries[old.key] == old {
			delete(d.entries, old.key)
		}
	}
	if e, ok := d.entries[key]; ok {
		return e, false
	}
	e := &dedupeEntry{key: key, command: command, at: now, done: make(chan struct{})}
	d.entries[key] = e
	d.order = append(d.order, e)
	return e, true
}
