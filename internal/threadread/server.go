package threadread

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
func Serve(ctx context.Context, input io.Reader, output io.Writer, r Reader) error {
	if !r.Valid() {
		return errors.New("invalid history scope")
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
				result = map[string]any{"protocolVersion": v, "serverInfo": map[string]string{"name": "webmux-thread-read", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}, "instructions": Instructions}
				initialized = true
			case "ping":
				result = map[string]any{}
			case "tools/list":
				if !ready {
					code, message = -32600, "Initialize first"
					break
				}
				result = map[string]any{"tools": []any{toolDefinition()}}
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
				if json.Unmarshal(req.Params, &p) != nil || p.Name != "thread_read" {
					code, message = -32602, "Unknown tool"
					break
				}
				callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				value, err := r.Read(callCtx, p.Arguments)
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

func toolDefinition() map[string]any {
	return map[string]any{"name": "thread_read", "description": "Read frozen handoff history, including omitted items. " + Instructions, "annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false}, "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"view"}, "properties": map[string]any{
		"view":            map[string]any{"type": "string", "enum": []string{"activity"}},
		"afterPosition":   map[string]any{"type": "string", "pattern": "^(0|[1-9][0-9]{0,18})$"},
		"itemId":          map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"},
		"textOffset":      map[string]any{"type": "integer", "minimum": 0},
		"limit":           map[string]any{"type": "integer", "default": 20, "minimum": 1, "maximum": 100},
		"maxBytesPerItem": map[string]any{"type": "integer", "default": 4000, "minimum": 4, "maximum": 65536},
	}}}
}
