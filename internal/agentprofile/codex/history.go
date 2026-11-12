package codex

import (
	"context"
	"errors"

	ap "github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/nativerpc"
)

// Only the audited protocol versions are enabled. Older or unverified versions
// keep working through inline; no guessed ACP extension is ever sent.
func (adapter) SupportsHistoryVersion(version string) bool {
	for _, verified := range []string{"0.166.0", "0.167.0"} {
		if order, valid := ap.CompareVersions(version, verified); valid && order == 0 {
			return true
		}
	}
	return false
}

type historyContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type historyItem struct {
	Type    string           `json:"type"`
	Role    string           `json:"role"`
	Content []historyContent `json:"content"`
}

func (a adapter) InjectHistory(ctx context.Context, thread ap.HistoryThread, history ap.HistoricalContext) (bool, error) {
	if !a.SupportsHistoryVersion(thread.Version) {
		return false, nil
	}
	if thread.ID == "" || thread.RPC == nil || thread.BeforeMutation == nil || len(history.Items) == 0 {
		return false, nativerpc.ErrProtocol
	}
	items := make([]historyItem, len(history.Items))
	for n, item := range history.Items {
		kind := "input_text"
		if item.Role == "assistant" {
			kind = "output_text"
		} else if item.Role != "user" {
			return false, nativerpc.ErrProtocol
		}
		items[n] = historyItem{Type: "message", Role: item.Role, Content: []historyContent{{Type: kind, Text: item.Text}}}
	}
	var resumed struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := thread.RPC.Call(ctx, "thread/resume", map[string]any{"threadId": thread.ID, "includeTurns": false}, &resumed); err != nil {
		return false, err
	}
	if resumed.Thread.ID != thread.ID {
		return false, nativerpc.ErrProtocol
	}
	if err := thread.BeforeMutation(); err != nil {
		return false, err
	}
	err := thread.RPC.Call(ctx, "thread/inject_items", struct {
		ThreadID string        `json:"threadId"`
		Items    []historyItem `json:"items"`
	}{thread.ID, items}, &struct{}{})
	var rpc *nativerpc.Error
	if errors.As(err, &rpc) && rpc.Code == -32601 {
		// The measured JSON-RPC dispatcher rejects unknown method names before
		// invoking a mutation handler. No other error is an inline fallback.
		return false, nil
	}
	return err == nil, err
}
