package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/rainchan/webmux/internal/acp"
)

// codexLockPath is the thread's writer lock, which Codex keeps open and
// locked while a client writes the thread.
func codexLockPath(id string) (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".codex")
	}
	return filepath.Join(home, "thread-writer-locks", id+".lock"), nil
}

// errThreadActive: another live Codex client writes the thread.
var errThreadActive = errors.New("this Codex session is in use by another Codex client")

// holdCodexLock opens and locks the thread's writer lock; the caller keeps
// the file open for as long as it writes the thread.
func holdCodexLock(id string) (*os.File, error) {
	p, err := codexLockPath(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("thread %s: %w", id, errThreadActive)
		}
		return nil, err
	}
	return f, nil
}

// codexLockHeld reports whether another process holds the thread's writer
// lock. The lock dies with its holder, so only a live writer counts.
func codexLockHeld(id string) (bool, error) {
	p, err := codexLockPath(id)
	if err != nil {
		return false, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	switch err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); {
	case err == nil:
		return false, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	case errors.Is(err, syscall.EWOULDBLOCK):
		return true, nil
	default:
		return false, err
	}
}

// threadActiveWriter is codex-acp's refusal to load or resume a thread
// another Codex client writes.
func threadActiveWriter(id string) error {
	data, err := json.Marshal(map[string]string{"reason": "thread_active_writer", "threadId": id})
	if err != nil {
		panic(err)
	}
	return &acp.RPCError{Code: acp.CodeInvalidRequest, Message: "Invalid request: This Codex session is in use by another Codex client", Data: data}
}

// selectOption is one select config option of a profile.
type selectOption struct {
	id, name, category, def string
	values                  []acp.ConfigSelectOption
}

var (
	defaultOptions = []selectOption{
		{id: "mode", name: "Mode", category: "mode", def: "code", values: []acp.ConfigSelectOption{
			{Value: "code", Name: "Code"}, {Value: "plan", Name: "Plan"},
		}},
		{id: "model", name: "Model", category: "model", def: "fast", values: []acp.ConfigSelectOption{
			{Value: "fast", Name: "Fast", Group: "builtin", GroupName: "Built-in"},
			{Value: "smart", Name: "Smart", Group: "builtin", GroupName: "Built-in"},
		}},
	}
	// codexOptions are codex-acp's options; it opens every loaded or
	// resumed thread at these defaults.
	codexOptions = []selectOption{
		{id: "mode", name: "Approval Preset", category: "mode", def: "agent", values: []acp.ConfigSelectOption{
			{Value: "read-only", Name: "Read Only"},
			{Value: "workspace-write", Name: "Workspace Write"},
			{Value: "agent", Name: "Agent"},
			{Value: "agent-full-access", Name: "Agent (full access)"},
		}},
		{id: "model", name: "Model", category: "model", def: "gpt-a", values: []acp.ConfigSelectOption{
			{Value: "gpt-a", Name: "GPT A"}, {Value: "gpt-b", Name: "GPT B"},
		}},
		{id: "reasoning_effort", name: "Reasoning Effort", category: "thought_level", def: "medium", values: []acp.ConfigSelectOption{
			{Value: "low", Name: "Low"}, {Value: "medium", Name: "Medium"}, {Value: "high", Name: "High"},
		}},
	}
	// claudeOptions are claude-agent-acp's options. Model and effort return
	// to these defaults after a cold load/resume, while mode is restored.
	claudeOptions = []selectOption{
		{id: "mode", name: "Permission Mode", category: "mode", def: "default", values: []acp.ConfigSelectOption{
			{Value: "default", Name: "Manual"},
			{Value: "acceptEdits", Name: "Accept Edits"},
			{Value: "plan", Name: "Plan"},
			{Value: "auto", Name: "Auto"},
			{Value: "bypassPermissions", Name: "Bypass Permissions"},
		}},
		{id: "model", name: "Model", category: "model", def: "opus", values: []acp.ConfigSelectOption{
			{Value: "opus", Name: "Opus"}, {Value: "sonnet", Name: "Sonnet"}, {Value: "haiku", Name: "Haiku"},
		}},
		{id: "effort", name: "Effort", category: "thought_level", def: "medium", values: []acp.ConfigSelectOption{
			{Value: "low", Name: "Low"}, {Value: "medium", Name: "Medium"}, {Value: "high", Name: "High"},
		}},
	}
	// agyOptions are Antigravity CLI's options. Both model and thought_level
	// reset to defaults on cross-process session/load and session/resume.
	agyOptions = []selectOption{
		{id: "model", name: "Model", category: "model", def: "gemini-4-argon", values: []acp.ConfigSelectOption{
			{Value: "gemini-4-argon", Name: "Gemini 4 Argon"},
			{Value: "gemini-3.8-flash", Name: "Gemini 3.8 Flash"},
		}},
		{id: "thought_level", name: "Thinking Level", category: "thought_level", def: "medium", values: []acp.ConfigSelectOption{
			{Value: "low", Name: "low"},
			{Value: "medium", Name: "medium"},
			{Value: "high", Name: "high"},
		}},
	}
	// devinOptions are Devin CLI's options: mode, model, thought_level and
	// speed. All of them persist across a cross-process session/load.
	devinOptions = []selectOption{
		{id: "mode", name: "Session Mode", category: "mode", def: "accept-edits", values: []acp.ConfigSelectOption{
			{Value: "accept-edits", Name: "Code", Description: "Write and edit code"},
			{Value: "smart", Name: "Smart", Description: "Auto-approve actions the model judges safe"},
			{Value: "ask", Name: "Ask", Description: "Answer questions without code changes"},
			{Value: "plan", Name: "Plan", Description: "Plan changes before implementing"},
			{Value: "bypass", Name: "Bypass Permissions", Description: "Auto-approve all tool calls unless your organization's policy requires approval"},
		}},
		{id: "model", name: "Model", category: "model", def: "swe-2-high", values: []acp.ConfigSelectOption{
			{Value: "adaptive", Name: "Adaptive"},
			{Value: "swe-2-high", Name: "SWE-2 High"},
			{Value: "swe-2-flash-medium", Name: "SWE-2 Flash"},
			{Value: "claude-fable-5-2-medium", Name: "Claude Fable"},
		}},
		{id: "thought_level", name: "Thinking Level", category: "thought_level", def: "medium", values: []acp.ConfigSelectOption{
			{Value: "low", Name: "low"},
			{Value: "medium", Name: "medium"},
			{Value: "high", Name: "high"},
		}},
		{id: "speed", name: "Speed", category: "speed", def: "standard", values: []acp.ConfigSelectOption{
			{Value: "standard", Name: "Standard"}, {Value: "fast", Name: "Fast"},
		}},
	}
	// kimiOptions are Kimi Code CLI's options. Only mode resets on a cold
	// load/resume; model and thinking retain their stored values.
	kimiOptions = []selectOption{
		{id: "model", name: "Model", category: "model", def: "kimi-code-plan-cn/k3.1-pro", values: []acp.ConfigSelectOption{
			{Value: "kimi-code-plan-cn/k3.1-pro", Name: "K3.1 Pro"},
			{Value: "kimi-code-plan-cn/kimi-for-coding", Name: "Kimi for Coding"},
		}},
		{id: "thinking", name: "Thinking", category: "thought_level", def: "high", values: []acp.ConfigSelectOption{
			{Value: "low", Name: "Low"}, {Value: "high", Name: "High"}, {Value: "max", Name: "Max"},
		}},
		{id: "mode", name: "Mode", category: "mode", def: "default", values: []acp.ConfigSelectOption{
			{Value: "default", Name: "Default"}, {Value: "plan", Name: "Plan"},
			{Value: "auto", Name: "Auto"}, {Value: "yolo", Name: "YOLO"},
		}},
	}
)

func profileOptions(profile string) []selectOption {
	switch profile {
	case "codex":
		return codexOptions
	case "claudecode":
		return claudeOptions
	case "agy":
		return agyOptions
	case "devin":
		return devinOptions
	case "kimi":
		return kimiOptions
	default:
		return defaultOptions
	}
}

func defaultConfig(profile string) map[string]string {
	out := map[string]string{}
	for _, o := range profileOptions(profile) {
		out[o.id] = o.def
	}
	return out
}

func configOptions(profile string, ss *session) []acp.ConfigOption {
	var out []acp.ConfigOption
	for _, o := range profileOptions(profile) {
		out = append(out, acp.ConfigOption{ID: o.id, Name: o.name, Category: o.category, Type: acp.ConfigSelect,
			Current: acp.ConfigValue{ID: ss.Config[o.id]}, Options: o.values})
	}
	return out
}

// validConfig reports whether value is one of the option's values.
func validConfig(profile, id, value string) bool {
	for _, o := range profileOptions(profile) {
		if o.id != id {
			continue
		}
		for _, v := range o.values {
			if v.Value == value {
				return true
			}
		}
	}
	return false
}
