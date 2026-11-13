package contract

import (
	"context"
	"encoding/json"
)

// mcpOwnerKey is deliberately private: HTTP headers and client metadata cannot
// manufacture the authenticated owner marker.
type mcpOwnerKey struct{}

// WithMCPOwner marks a request only after the TCP API has validated Host,
// the configured owner Bearer credential, and Origin. This profile supports
// only that owner, not scoped credentials or OAuth identities.
func WithMCPOwner(ctx context.Context) context.Context {
	return context.WithValue(ctx, mcpOwnerKey{}, true)
}

// MCPOwner reports whether the trusted API authenticated the instance owner.
func MCPOwner(ctx context.Context) bool {
	owner, _ := ctx.Value(mcpOwnerKey{}).(bool)
	return owner
}

// MCPCancelParams is the strictly checked HTTP cancellation notification.
// RequestID remains JSON so string and numeric identities stay distinct.
type MCPCancelParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Reason    *string         `json:"reason,omitempty"`
}

// MCPRequestCancelledData distinguishes observation cancellation from a task
// interruption or a business rollback.
type MCPRequestCancelledData struct {
	Kind string `json:"kind"`
}

// MCPHTTPInitializeParams permits initialize-era extension metadata while the
// transport checks the required object shapes before discarding clientInfo.
type MCPHTTPInitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ClientInfo      json.RawMessage `json:"clientInfo"`
}

type MCPHTTPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
