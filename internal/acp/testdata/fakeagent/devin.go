package main

import (
	"encoding/json"

	"github.com/rainchan/webmux/internal/acp"
)

// devinCodeSessionLocked is Devin CLI's lock error, answered when
// session/load targets a session another live process holds.
const devinCodeSessionLocked = -32015

// devinSessionLocked is the -32015 answer whose data names the lock holder.
func devinSessionLocked(pid int) error {
	data, err := json.Marshal(map[string]any{
		"cognition.ai/errorKind":     "session_locked",
		"cognition.ai/retryable":     true,
		"cognition.ai/lockHolderPid": pid,
	})
	if err != nil {
		panic(err)
	}
	return &acp.RPCError{Code: devinCodeSessionLocked, Message: "session is locked by another process", Data: data}
}

// devinForceClose reports whether meta carries Devin's
// _meta."cognition.ai/force" session/close extension, which releases a lock
// another process holds.
func devinForceClose(meta json.RawMessage) bool {
	if len(meta) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(meta, &m) != nil {
		return false
	}
	var force bool
	return json.Unmarshal(m["cognition.ai/force"], &force) == nil && force
}

// sessionModes is the legacy modes block Devin answers session/new and
// session/load with, mirroring the stored mode option.
func sessionModes(profile string, ss *session) *acp.SessionModeState {
	if profile != "devin" {
		return nil
	}
	state := &acp.SessionModeState{CurrentModeID: ss.Config["mode"]}
	for _, o := range profileOptions(profile) {
		if o.id != "mode" {
			continue
		}
		for _, v := range o.values {
			state.AvailableModes = append(state.AvailableModes, acp.SessionMode{ID: v.Value, Name: v.Name})
		}
	}
	return state
}
