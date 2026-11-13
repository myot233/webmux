package mcphttp

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/jsonutil"
)

func object(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{'
}

func validParams(req contract.MCPRequest) bool {
	if raw := bytes.TrimSpace(req.Params); len(raw) > 0 && raw[0] != '{' && raw[0] != '[' {
		return false
	}
	switch req.Method {
	case "initialize":
		var params contract.MCPHTTPInitializeParams
		if !object(req.Params) || json.Unmarshal(req.Params, &params) != nil || params.ProtocolVersion == "" ||
			!object(params.Capabilities) || !object(params.ClientInfo) {
			return false
		}
		var client contract.MCPHTTPClientInfo
		return json.Unmarshal(params.ClientInfo, &client) == nil && client.Name != "" && client.Version != ""
	case "notifications/initialized", "ping":
		return len(req.Params) == 0 || object(req.Params)
	case "notifications/cancelled":
		_, ok := cancellation(req.Params)
		return ok
	case "tools/list":
		if len(req.Params) == 0 {
			return true
		}
		var params map[string]json.RawMessage
		if !object(req.Params) || json.Unmarshal(req.Params, &params) != nil {
			return false
		}
		if cursor, exists := params["cursor"]; exists {
			var value string
			return len(cursor) > 0 && cursor[0] == '"' && json.Unmarshal(cursor, &value) == nil && value == ""
		}
		return true
	case "tools/call":
		return object(req.Params)
	default:
		return true
	}
}

func cancellation(raw json.RawMessage) (string, bool) {
	var params contract.MCPCancelParams
	if !object(raw) || jsonutil.Decode(raw, &params) != nil {
		return "", false
	}
	// An omitted optional string is valid; an explicit null is not a string.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", false
	}
	if reason, exists := fields["reason"]; exists && (len(reason) == 0 || reason[0] != '"') {
		return "", false
	}
	if params.Reason != nil && (len(*params.Reason) > 256 || !utf8.ValidString(*params.Reason)) {
		return "", false
	}
	return rpcID(params.RequestID)
}
