// Package mcpprotocol implements transport-independent MCP sessions.
package mcpprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

const defaultVersion = "2025-11-25"

type Tool struct {
	Definition map[string]any
	Call       func(context.Context, json.RawMessage) (any, error)
}

type Config struct {
	Name, Instructions string
	Tools              []Tool
	CallTimeout        time.Duration
	Versions           []string
}

// Registry owns a validated, immutable snapshot of tool definitions and handlers.
type Registry struct {
	name, instructions string
	timeout            time.Duration
	versions           []string
	definitions        []json.RawMessage
	calls              map[string]func(context.Context, json.RawMessage) (any, error)
}

func New(config Config) (*Registry, error) {
	if config.Name == "" || config.CallTimeout <= 0 {
		return nil, errors.New("invalid MCP server")
	}
	r := &Registry{name: config.Name, instructions: config.Instructions, timeout: config.CallTimeout, versions: append([]string(nil), config.Versions...), definitions: make([]json.RawMessage, 0, len(config.Tools)), calls: make(map[string]func(context.Context, json.RawMessage) (any, error), len(config.Tools))}
	if len(r.versions) == 0 {
		r.versions = []string{"2024-11-05", "2025-03-26", "2025-06-18", defaultVersion}
	}
	for _, tool := range config.Tools {
		name, ok := tool.Definition["name"].(string)
		if !ok || name == "" || tool.Call == nil || r.calls[name] != nil {
			return nil, errors.New("invalid MCP tool")
		}
		definition, err := json.Marshal(tool.Definition)
		if err != nil {
			return nil, err
		}
		r.definitions = append(r.definitions, definition)
		r.calls[name] = tool.Call
	}
	return r, nil
}

type Session struct {
	registry           *Registry
	mu                 sync.Mutex
	initialized, ready bool
	version            string
}

func (r *Registry) NewSession() *Session   { return &Session{registry: r} }
func (s *Session) Ready() bool             { s.mu.Lock(); defer s.mu.Unlock(); return s.ready }
func (s *Session) ProtocolVersion() string { s.mu.Lock(); defer s.mu.Unlock(); return s.version }

func protocolError(id json.RawMessage, code int, message string) *contract.MCPResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &contract.MCPResponse{JSONRPC: "2.0", ID: id, Error: &contract.MCPError{Code: code, Message: message}}
}

func validate(req contract.MCPRequest) *contract.MCPResponse {
	if req.JSONRPC != "2.0" || req.Method == "" {
		return protocolError(nil, -32600, "Invalid request")
	}
	if len(req.ID) != 0 {
		var id any
		if json.Unmarshal(req.ID, &id) != nil {
			return protocolError(nil, -32600, "Invalid request")
		}
		switch id.(type) {
		case string, float64:
		default:
			return protocolError(nil, -32600, "Invalid request")
		}
	}
	return nil
}

// Decode checks JSON and the JSON-RPC envelope without changing session state.
func Decode(raw []byte) (contract.MCPRequest, *contract.MCPResponse) {
	var req contract.MCPRequest
	if json.Unmarshal(raw, &req) != nil {
		return req, protocolError(nil, -32700, "Parse error")
	}
	return req, validate(req)
}

// MCPResult encodes the complete tools/call result, including its JSON text copy.
func MCPResult(value any, callErr error) (json.RawMessage, error) {
	result := contract.MCPToolResult{Content: make([]contract.MCPTextContent, 1)}
	result.Content[0].Type = "text"
	if callErr != nil {
		result.IsError = true
		result.Content[0].Text = callErr.Error()
	} else {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		result.StructuredContent = data
		result.Content[0].Text = string(data)
	}
	return json.Marshal(result)
}

// Dispatch updates handshake state under a short lock; handlers run outside it.
// A nil response denotes an accepted notification.
func (s *Session) Dispatch(ctx context.Context, req contract.MCPRequest) (*contract.MCPResponse, error) {
	if invalid := validate(req); invalid != nil {
		return invalid, nil
	}
	s.mu.Lock()
	if len(req.ID) == 0 {
		if req.Method == "notifications/initialized" && s.initialized {
			s.ready = true
		}
		s.mu.Unlock()
		return nil, nil
	}
	var result any
	var call func(context.Context, json.RawMessage) (any, error)
	var arguments json.RawMessage
	var failure *contract.MCPResponse
	switch req.Method {
	case "initialize":
		if s.initialized {
			failure = protocolError(req.ID, -32600, "Already initialized")
			break
		}
		var params contract.MCPInitializeParams
		if json.Unmarshal(req.Params, &params) != nil || params.ProtocolVersion == "" {
			failure = protocolError(req.ID, -32602, "Invalid initialize parameters")
			break
		}
		version := defaultVersion
		for _, supported := range s.registry.versions {
			if params.ProtocolVersion == supported {
				version = supported
				break
			}
		}
		s.version, s.initialized = version, true
		result = contract.MCPInitializeResult{ProtocolVersion: version, ServerInfo: contract.MCPServerInfo{Name: s.registry.name, Version: "1"}, Instructions: s.registry.instructions}
	case "ping":
		result = struct{}{}
	case "tools/list":
		if !s.ready {
			failure = protocolError(req.ID, -32600, "Initialize first")
			break
		}
		result = contract.MCPToolsResult{Tools: s.registry.definitions}
	case "tools/call":
		if !s.ready {
			failure = protocolError(req.ID, -32600, "Initialize first")
			break
		}
		var params contract.MCPCallParams
		if json.Unmarshal(req.Params, &params) != nil || s.registry.calls[params.Name] == nil {
			failure = protocolError(req.ID, -32602, "Unknown tool")
			break
		}
		call, arguments = s.registry.calls[params.Name], params.Arguments
	default:
		failure = protocolError(req.ID, -32601, "Method not found")
	}
	s.mu.Unlock()
	if failure != nil {
		return failure, nil
	}
	var data json.RawMessage
	var err error
	if call != nil {
		callCtx, cancel := context.WithTimeout(ctx, s.registry.timeout)
		value, callErr := call(callCtx, arguments)
		cancel()
		data, err = MCPResult(value, callErr)
	} else {
		data, err = json.Marshal(result)
	}
	if err != nil {
		return nil, err
	}
	return &contract.MCPResponse{JSONRPC: "2.0", ID: req.ID, Result: data}, nil
}
