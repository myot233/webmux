// Package nativerpc is a bounded stdio JSON-RPC client for a short-lived native
// provider helper. It has no ACP methods, process ownership or application logs.
package nativerpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"
)

var (
	ErrDisconnected = errors.New("native RPC disconnected")
	ErrProtocol     = errors.New("native RPC protocol error")
)

// Error deliberately omits provider error text, which may contain user input.
type Error struct{ Code int }

func (e *Error) Error() string { return fmt.Sprintf("native RPC error %d", e.Code) }

type frame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Conn struct {
	in        io.WriteCloser
	out       io.ReadCloser
	callMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	next      int64
	responses chan frame
	done      chan struct{}
}

func New(out io.ReadCloser, in io.WriteCloser) *Conn {
	c := &Conn{in: in, out: out, responses: make(chan frame, 1), done: make(chan struct{})}
	go c.read()
	return c
}

func (c *Conn) closePipes() {
	c.closeOnce.Do(func() { _ = c.in.Close(); _ = c.out.Close() })
}

// Close unblocks and joins the only reader. The caller still owns process exit.
func (c *Conn) Close() { c.closePipes(); <-c.done }

func (c *Conn) write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return json.NewEncoder(c.in).Encode(value)
}

func (c *Conn) read() {
	defer close(c.done)
	defer c.closePipes()
	scanner := bufio.NewScanner(c.out)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		body := scanner.Bytes()
		var f frame
		if !utf8.Valid(body) || json.Unmarshal(body, &f) != nil || (f.JSONRPC != "" && f.JSONRPC != "2.0") {
			return
		}
		if f.Method != "" {
			if len(f.ID) > 0 {
				// This helper never starts turns or handles interactive approvals.
				if c.write(map[string]any{"id": f.ID, "error": map[string]any{"code": -32601, "message": "method not supported"}}) != nil {
					return
				}
			}
			continue
		}
		select {
		case c.responses <- f:
		default:
			return // no unbounded response queue
		}
	}
}

func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	c.callMu.Lock()
	defer c.callMu.Unlock()
	stop := context.AfterFunc(ctx, c.closePipes)
	defer stop()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.next++
	id := c.next
	if c.write(map[string]any{"id": id, "method": method, "params": params}) != nil {
		return ErrDisconnected
	}
	var response frame
	select {
	case response = <-c.responses:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		// EOF may follow an acknowledgement immediately. Consume an already
		// parsed response before classifying the call as ambiguous.
		select {
		case response = <-c.responses:
		default:
			return ErrDisconnected
		}
	}
	var got int64
	if json.Unmarshal(response.ID, &got) != nil || got != id {
		return ErrProtocol
	}
	if response.Error != nil {
		if len(response.Result) != 0 {
			return ErrProtocol
		}
		return response.Error
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return ErrProtocol
	}
	if result != nil && json.Unmarshal(response.Result, result) != nil {
		return ErrProtocol
	}
	return nil
}

func (c *Conn) Notify(ctx context.Context, method string) error {
	stop := context.AfterFunc(ctx, c.closePipes)
	defer stop()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.write(map[string]any{"method": method}) != nil {
		return ErrDisconnected
	}
	return nil
}
