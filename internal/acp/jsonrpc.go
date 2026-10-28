package acp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// MaxLineBytes bounds one protocol line, excluding its newline. Tool output
// can be far larger than bufio.Scanner's 64 KiB default, so lines are read
// whole up to this cap; a longer line is a protocol error, never truncated.
const MaxLineBytes = 32 << 20

// JSON-RPC and ACP error codes (schema ErrorCode).
const (
	CodeParseError       = -32700
	CodeInvalidRequest   = -32600
	CodeMethodNotFound   = -32601
	CodeInvalidParams    = -32602
	CodeInternalError    = -32603
	CodeRequestCancelled = -32800
	CodeAuthRequired     = -32000
	CodeResourceNotFound = -32002
)

var (
	// ErrClosed reports a connection ended by Close.
	ErrClosed = errors.New("acp: connection closed")
	// ErrDisconnected reports a connection whose agent output ended: the
	// process exited, closed stdout, or could no longer be written to.
	ErrDisconnected = errors.New("acp: agent disconnected")
	// ErrProtocol reports output that is not line-delimited JSON-RPC. The
	// connection is closed; the offending bytes are neither kept nor logged.
	ErrProtocol = errors.New("acp: protocol error")
	// ErrUnsupportedVersion reports an initialize response with a protocol
	// version other than ProtocolVersion. The caller ends the agent.
	ErrUnsupportedVersion = errors.New("acp: unsupported protocol version")
)

// ProtocolError describes why agent output was rejected. It carries the
// line length only, never its content.
type ProtocolError struct {
	Reason string
	Bytes  int
}

// Protocol error reasons.
const (
	ReasonLineTooLong    = "line too long"
	ReasonInvalidJSON    = "invalid json"
	ReasonInvalidMessage = "invalid json-rpc message"
)

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("acp: protocol error: %s (%d bytes)", e.Reason, e.Bytes)
}

func (e *ProtocolError) Unwrap() error { return ErrProtocol }

// RPCError is a JSON-RPC error object. Error() reports the code only:
// agents put user code and file content in messages, and callers log errors.
// Message stays available for showing to the user who made the request.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return "acp: rpc error " + strconv.Itoa(e.Code) }

// wireMessage is one JSON-RPC 2.0 request, notification or response. An
// absent id stays nil; an explicit null id is the bytes "null".
type wireMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func (m *wireMessage) isRequest() bool      { return m.Method != "" && m.ID != nil }
func (m *wireMessage) isNotification() bool { return m.Method != "" && m.ID == nil }

// parseMessage validates one line as a JSON-RPC 2.0 message.
func parseMessage(line []byte) (wireMessage, error) {
	var m wireMessage
	if !json.Valid(line) {
		return m, &ProtocolError{Reason: ReasonInvalidJSON, Bytes: len(line)}
	}
	if err := json.Unmarshal(line, &m); err != nil {
		return m, &ProtocolError{Reason: ReasonInvalidMessage, Bytes: len(line)}
	}
	if m.JSONRPC != "2.0" {
		return m, &ProtocolError{Reason: ReasonInvalidMessage, Bytes: len(line)}
	}
	if m.Method != "" {
		if m.Result != nil || m.Error != nil {
			return m, &ProtocolError{Reason: ReasonInvalidMessage, Bytes: len(line)}
		}
		return m, nil
	}
	if m.ID == nil || (m.Result == nil) == (m.Error == nil) {
		return m, &ProtocolError{Reason: ReasonInvalidMessage, Bytes: len(line)}
	}
	return m, nil
}

// readLine returns the next line without its terminator, reusing buf. A
// final line without a newline is returned before io.EOF.
func readLine(br *bufio.Reader, buf []byte) ([]byte, error) {
	buf = buf[:0]
	for {
		frag, err := br.ReadSlice('\n')
		if len(buf)+len(frag) > MaxLineBytes+1 {
			return nil, &ProtocolError{Reason: ReasonLineTooLong, Bytes: len(buf) + len(frag)}
		}
		buf = append(buf, frag...)
		switch {
		case err == nil:
			buf = bytes.TrimSuffix(buf[:len(buf)-1], []byte{'\r'})
			if len(buf) > MaxLineBytes {
				return nil, &ProtocolError{Reason: ReasonLineTooLong, Bytes: len(buf)}
			}
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(buf) > 0:
			return buf, nil
		default:
			return nil, err
		}
	}
}

func encodeRequest(id int64, method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("acp: encode %s params: %w", method, err)
	}
	return frame(wireMessage{JSONRPC: "2.0", ID: json.RawMessage(strconv.FormatInt(id, 10)), Method: method, Params: p})
}

func encodeNotification(method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("acp: encode %s params: %w", method, err)
	}
	return frame(wireMessage{JSONRPC: "2.0", Method: method, Params: p})
}

func encodeResponse(id json.RawMessage, result any, rpcErr *RPCError) ([]byte, error) {
	m := wireMessage{JSONRPC: "2.0", ID: id, Error: rpcErr}
	if rpcErr == nil {
		r, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("acp: encode result: %w", err)
		}
		m.Result = r
	}
	return frame(m)
}

func frame(m wireMessage) ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("acp: encode message: %w", err)
	}
	return append(b, '\n'), nil
}

// idKey canonicalises a JSON-RPC id so a request and a later
// $/cancel_request naming it compare equal regardless of spacing.
func idKey(id json.RawMessage) string {
	var b bytes.Buffer
	if err := json.Compact(&b, id); err != nil {
		return string(id)
	}
	return b.String()
}
