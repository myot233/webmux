package contract

import "encoding/json"

// MCPRequest is a JSON-RPC request or notification.
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCPResponse carries exactly one result or protocol error.
type MCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

type MCPError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type MCPInitializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type MCPCapabilities struct {
	Tools struct{} `json:"tools"`
}
type MCPInitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	ServerInfo      MCPServerInfo   `json:"serverInfo"`
	Capabilities    MCPCapabilities `json:"capabilities"`
	Instructions    string          `json:"instructions"`
}
type MCPToolsResult struct {
	Tools []json.RawMessage `json:"tools"`
}
type MCPCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}
type MCPTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type MCPToolResult struct {
	IsError           bool             `json:"isError"`
	StructuredContent json.RawMessage  `json:"structuredContent,omitempty"`
	Content           []MCPTextContent `json:"content"`
}
