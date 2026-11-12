package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// cancelMode selects what a call does when its context ends first.
type cancelMode int

const (
	// cancelRequest sends $/cancel_request and returns the context error.
	cancelRequest cancelMode = iota
	// cancelTurn sends session/cancel for the session and keeps waiting:
	// the agent must still answer the prompt, with stopReason cancelled.
	cancelTurn
)

type callOptions struct {
	mode      cancelMode
	sessionID string
}

// call sends a request and decodes its result into result (when non-nil).
func (c *Conn) call(ctx context.Context, method string, params, result any, o callOptions) error {
	id := c.nextID.Add(1)
	b, err := encodeRequest(id, method, params)
	if err != nil {
		return err
	}
	ch := make(chan wireMessage, 1)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return fmt.Errorf("acp: %s: %w", method, err)
	}
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.enqueue(ctx, b); err != nil {
		c.forget(id)
		return fmt.Errorf("acp: %s: %w", method, err)
	}
	var m wireMessage
	select {
	case m = <-ch:
	case <-c.dead:
		return c.afterDeath(method, id, ch, result)
	case <-ctx.Done():
		if o.mode == cancelTurn {
			if err := c.notify(context.Background(), MethodSessionCancel, CancelNotification{SessionID: o.sessionID}); err != nil {
				c.forget(id)
				return fmt.Errorf("acp: %s: %w", method, err)
			}
			select {
			case m = <-ch:
			case <-c.dead:
				return c.afterDeath(method, id, ch, result)
			}
			break
		}
		c.forget(id)
		select {
		case m = <-ch:
			// Answered while cancelling; the answer wins.
		default:
			// Best effort: the call has already failed for the caller.
			_ = c.notify(context.Background(), MethodCancelRequest, CancelRequestNotification{RequestID: json.RawMessage(strconv.FormatInt(id, 10))})
			return fmt.Errorf("acp: %s: %w", method, ctx.Err())
		}
	}
	return decodeResult(method, m, result)
}

// afterDeath prefers a response that raced with the connection ending.
func (c *Conn) afterDeath(method string, id int64, ch chan wireMessage, result any) error {
	select {
	case m := <-ch:
		return decodeResult(method, m, result)
	default:
	}
	c.forget(id)
	return fmt.Errorf("acp: %s: %w", method, c.Err())
}

func decodeResult(method string, m wireMessage, result any) error {
	if m.Error != nil {
		return fmt.Errorf("acp: %s: %w", method, m.Error)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(m.Result, result); err != nil {
		return fmt.Errorf("acp: %s: decode result: %w", method, err)
	}
	return nil
}

func (c *Conn) notify(ctx context.Context, method string, params any) error {
	b, err := encodeNotification(method, params)
	if err != nil {
		return err
	}
	return c.enqueue(ctx, b)
}

func (c *Conn) notification(m wireMessage) {
	if c.isDead() {
		return
	}
	switch m.Method {
	case MethodSessionUpdate:
		var n SessionNotification
		if err := json.Unmarshal(m.Params, &n); err != nil {
			c.log.Warn("acp dropped malformed session update", "kind", knownKind(m.Params))
			return
		}
		c.h.SessionUpdate(n)
	case MethodCancelRequest:
		var p CancelRequestNotification
		if err := json.Unmarshal(m.Params, &p); err != nil || p.RequestID == nil {
			return
		}
		c.mu.Lock()
		cancel := c.incoming[idKey(p.RequestID)]
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	// Other notifications, including extensions, are ignored.
}

func (c *Conn) isDead() bool {
	select {
	case <-c.dead:
		return true
	default:
		return false
	}
}

func (c *Conn) startRequest(m wireMessage) {
	if c.isDead() {
		return
	}
	var run func(ctx context.Context) (any, error)
	switch m.Method {
	case MethodRequestPermission:
		var req RequestPermissionRequest
		if err := json.Unmarshal(m.Params, &req); err != nil {
			c.reply(m.ID, nil, &RPCError{Code: CodeInvalidParams, Message: "invalid params"})
			return
		}
		run = func(ctx context.Context) (any, error) { return c.h.RequestPermission(ctx, req) }
	case MethodElicitationCreate:
		var req CreateElicitationRequest
		if err := json.Unmarshal(m.Params, &req); err != nil {
			c.reply(m.ID, nil, &RPCError{Code: CodeInvalidParams, Message: "invalid params"})
			return
		}
		if req.Mode != ElicitationForm {
			c.reply(m.ID, nil, &RPCError{Code: CodeInvalidParams, Message: "unsupported elicitation mode"})
			return
		}
		run = func(ctx context.Context) (any, error) { return c.h.CreateElicitation(ctx, req) }
	default:
		// fs/*, terminal/* and anything else were not advertised.
		c.reply(m.ID, nil, &RPCError{Code: CodeMethodNotFound, Message: "method not found"})
		return
	}
	key := idKey(m.ID)
	ctx, cancel := context.WithCancel(c.base)
	c.mu.Lock()
	c.incoming[key] = cancel
	c.mu.Unlock()
	c.requests.Add(1)
	go func() {
		defer c.requests.Done()
		defer func() {
			c.mu.Lock()
			delete(c.incoming, key)
			c.mu.Unlock()
			cancel()
		}()
		result, err := run(ctx)
		var rpcErr *RPCError
		switch {
		case err == nil:
		case errors.As(err, &rpcErr):
		case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			rpcErr = &RPCError{Code: CodeRequestCancelled, Message: "request cancelled"}
		default:
			rpcErr = &RPCError{Code: CodeInternalError, Message: "internal error"}
		}
		c.reply(m.ID, result, rpcErr)
	}()
}

// reply answers an agent request; the answer is dropped if the connection
// has ended.
func (c *Conn) reply(id json.RawMessage, result any, rpcErr *RPCError) {
	b, err := encodeResponse(id, result, rpcErr)
	if err != nil {
		b, _ = encodeResponse(id, nil, &RPCError{Code: CodeInternalError, Message: "internal error"})
	}
	// Only a dead connection refuses the write, and then nobody is waiting.
	_ = c.enqueue(context.Background(), b)
}

// noHandler serves connections opened without a Handler.
type noHandler struct{}

var _ Handler = noHandler{}

func (noHandler) SessionUpdate(SessionNotification) {}

func (noHandler) RequestPermission(context.Context, RequestPermissionRequest) (RequestPermissionResponse, error) {
	return RequestPermissionResponse{}, &RPCError{Code: CodeMethodNotFound, Message: "method not found"}
}

func (noHandler) CreateElicitation(context.Context, CreateElicitationRequest) (CreateElicitationResponse, error) {
	return CreateElicitationResponse{}, &RPCError{Code: CodeMethodNotFound, Message: "method not found"}
}

// knownKind names a session/update kind for logs. Kinds this package does
// not know are agent-chosen text and are reported as "unknown".
func knownKind(params json.RawMessage) string {
	var p struct {
		Update struct {
			Kind string `json:"sessionUpdate"`
		} `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil || !isKnownUpdateKind(p.Update.Kind) {
		return "unknown"
	}
	return p.Update.Kind
}
