package agentprofile

import "context"

// HistoryRPC is a dedicated native connection. It is never an ACP connection;
// the lifecycle owner admits it only after the previous writer has exited.
type HistoryRPC interface {
	Call(context.Context, string, any, any) error
}

type HistoryThread struct {
	ID, Version    string
	RPC            HistoryRPC
	BeforeMutation func() error
}

// HistoricalItem contains portable text and provenance only. It cannot carry
// reasoning, provider tool state, files, configuration or a new user request.
type HistoricalItem struct {
	ID, Role, Text string
}

type HistoricalContext struct{ Items []HistoricalItem }

// HistoryInjector is optional. False is reserved for known lack of support
// before history mutation; any ambiguous result is an error, never a fallback.
type HistoryInjector interface {
	SupportsHistoryVersion(string) bool
	InjectHistory(context.Context, HistoryThread, HistoricalContext) (accepted bool, err error)
}
