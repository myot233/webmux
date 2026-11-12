// Package mcpstdio provides bounded MCP stdio transport for local API clients.
package mcpstdio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const protocolVersion = "2025-11-25"

// The server intentionally implements the legacy initialize era. Modern clients
// receive MethodNotFound for server/discover and can negotiate legacy MCP.
// No request, response or historical text is written to application logs.
// Tool binds a public definition to a local API operation.
type Tool struct {
	Definition map[string]any
	Call       func(context.Context, json.RawMessage) (any, error)
}

// Server serves one initialized MCP connection. Calls are serialized and bounded
// by CallTimeout; tool errors are isolated from the transport.
type Server struct {
	Name, Instructions string
	Tools              []Tool
	CallTimeout        time.Duration
}

func (s Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if s.Name == "" || s.CallTimeout <= 0 {
		return errors.New("invalid MCP server")
	}
	definitions := make([]any, 0, len(s.Tools))
	calls := make(map[string]func(context.Context, json.RawMessage) (any, error), len(s.Tools))
	for _, tool := range s.Tools {
		name, ok := tool.Definition["name"].(string)
		if !ok || name == "" || tool.Call == nil || calls[name] != nil {
			return errors.New("invalid MCP tool")
		}
		definitions = append(definitions, tool.Definition)
		calls[name] = tool.Call
	}
	scan := bufio.NewScanner(input)
	scan.Buffer(make([]byte, 4096), 64<<10)
	enc := json.NewEncoder(output)
	initialized, ready := false, false
	for scan.Scan() {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		var result any
		code, message := 0, ""
		parseErr := json.Unmarshal(scan.Bytes(), &req)
		valid := parseErr == nil && req.JSONRPC == "2.0" && req.Method != ""
		if parseErr != nil {
			code, message = -32700, "Parse error"
		} else if !valid {
			code, message = -32600, "Invalid request"
		} else if len(req.ID) == 0 {
			if req.Method == "notifications/initialized" && initialized {
				ready = true
			}
			continue
		} else {
			var id any
			if json.Unmarshal(req.ID, &id) != nil || id == nil {
				code, message = -32600, "Invalid request"
				req.ID = json.RawMessage("null")
			} else {
				switch id.(type) {
				case string, float64:
				default:
					req.ID = json.RawMessage("null")
					code, message = -32600, "Invalid request"
				}
			}
		}
		if code == 0 {
			switch req.Method {
			case "initialize":
				if initialized {
					code, message = -32600, "Already initialized"
					break
				}
				var p struct {
					ProtocolVersion string `json:"protocolVersion"`
				}
				if json.Unmarshal(req.Params, &p) != nil || p.ProtocolVersion == "" {
					code, message = -32602, "Invalid initialize parameters"
					break
				}
				v := p.ProtocolVersion
				switch v {
				case "2024-11-05", "2025-03-26", "2025-06-18", protocolVersion:
				default:
					v = protocolVersion
				}
				result = map[string]any{"protocolVersion": v, "serverInfo": map[string]string{"name": s.Name, "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}, "instructions": s.Instructions}
				initialized = true
			case "ping":
				result = map[string]any{}
			case "tools/list":
				if !ready {
					code, message = -32600, "Initialize first"
					break
				}
				result = map[string]any{"tools": definitions}
			case "tools/call":
				if !ready {
					code, message = -32600, "Initialize first"
					break
				}
				var p struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
					Meta      json.RawMessage `json:"_meta,omitempty"`
				}
				if json.Unmarshal(req.Params, &p) != nil || calls[p.Name] == nil {
					code, message = -32602, "Unknown tool"
					break
				}
				callCtx, cancel := context.WithTimeout(ctx, s.CallTimeout)
				value, err := calls[p.Name](callCtx, p.Arguments)
				cancel()
				if err != nil {
					result = map[string]any{"isError": true, "content": []any{map[string]string{"type": "text", "text": err.Error()}}}
					break
				}
				data, err := json.Marshal(value)
				if err != nil {
					return err
				}
				result = map[string]any{"isError": false, "structuredContent": value, "content": []any{map[string]string{"type": "text", "text": string(data)}}}
			default:
				code, message = -32601, "Method not found"
			}
		}
		response := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage("null")}
		if valid && len(req.ID) > 0 {
			response["id"] = req.ID
		}
		if code != 0 {
			response["error"] = map[string]any{"code": code, "message": message}
		} else {
			response["result"] = result
		}
		if err := enc.Encode(response); err != nil {
			return err
		}
	}
	return scan.Err()
}
