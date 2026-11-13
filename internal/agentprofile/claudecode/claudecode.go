// Package claudecode is the hybrid adapter for Claude Code, whose ACP side
// is the separately installed claude-agent-acp connector.
package claudecode

import (
	"strings"

	ap "github.com/rainchan/webmux/internal/agentprofile"
)

// Name identifies Claude Code on the wire and in hybrid session records.
const Name = "claudecode"

// Claude Code config option IDs used by claude-agent-acp.
const (
	claudeModel  = "model"
	claudeEffort = "effort"
	claudeMode   = "mode"
)

// Agent is the Claude Code adapter.
var Agent ap.Agent = adapter{SessionSteering: ap.SessionSteering{IdlePrompt: true}}

type adapter struct {
	ap.SessionSteering
}

var profile = &ap.Profile{
	DefaultChatMode:   "bypassPermissions",
	Name:              Name,
	Title:             "Claude Code",
	Icon:              "claudecode",
	Executables:       []string{"claude"},
	ProcessExecutable: claudeProcess,
	VerifiedVersion:   "2.1.316",
	Connector: &ap.Connector{
		Executable:      "claude-agent-acp",
		Title:           "Claude Code ACP 适配器",
		InstallCommand:  "npm install -g @agentclientprotocol/claude-agent-acp",
		DocsURL:         "https://github.com/agentclientprotocol/claude-agent-acp",
		VerifiedVersion: "0.99.1",
	},
	Launcher:       "node",
	AgentPathEnv:   "CLAUDE_CODE_EXECUTABLE",
	HiddenCommands: []string{"login", "logout"},
	ResumeFlag:     "--resume",
	InitialPrompt:  func(prompt string) ([]string, bool) { return []string{"--", prompt}, true },
	Positional:     ap.PositionalPrompt,
	Options:        claudeOptions(),
	ChatSide:       claudeChatSide,
	Subcommands: ap.SetOf("auth", "agents", "gateway", "import", "install", "logs", "mcp",
		"doctor", "setup-token", "stop", "kill", "rm", "ultrareview", "update", "upgrade",
		"completion", "help"),
	SessionSubcommands: ap.SetOf("attach"),
}

// claudeProcess also accepts the native installer's versioned binary name
// (for example 2.1.316), which is what Linux /proc/<pid>/exe reports.
func claudeProcess(base string) bool {
	if base == "claude" {
		return true
	}
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func claudeOptions() map[string]ap.Option {
	o := map[string]ap.Option{}
	ap.Options(o, ap.ArgSession, ap.ArityOptional, "-c", "--continue", "--cloud")
	ap.Options(o, ap.ArgSession, ap.ArityOne, "-r", "--resume", "--session-id", "--from-pr", "--teleport")
	ap.Options(o, ap.ArgSession, ap.ArityNone, "--fork-session")
	ap.Options(o, ap.ArgStore, ap.ArityNone, "--no-session-persistence")
	ap.Options(o, ap.ArgStore, ap.ArityOne, "--environment")
	ap.Options(o, ap.ArgDirectory, ap.ArityOne, "-w", "--worktree")
	ap.Options(o, ap.ArgDirectory, ap.ArityOptional, "--tmux")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityOne, "-p", "--print", "--output-format",
		"--input-format", "--json-schema", "--max-budget-usd")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityNone, "--bg", "--background", "--desktop",
		"-h", "--help", "-v", "--version")
	ap.Options(o, ap.ArgDrop, ap.ArityOne, "--image")
	ap.Options(o, ap.ArgKeep, ap.ArityOne, "--model", "--effort", "--fallback-model",
		"--permission-mode", "--settings", "--setting-sources", "--system-prompt",
		"--system-prompt-file", "--append-system-prompt", "--append-system-prompt-file",
		"--agents", "--agent", "--plugin-dir")
	ap.Options(o, ap.ArgKeep, ap.ArityVariadic, "--add-dir", "--mcp-config",
		"--allowedTools", "--allowed-tools", "--disallowedTools", "--disallowed-tools",
		"--tools", "--betas")
	ap.Options(o, ap.ArgKeep, ap.ArityNone, "--dangerously-skip-permissions",
		"--strict-mcp-config", "--bare")
	return o
}

func (adapter) Profile() *ap.Profile { return profile }

// Identify reads Claude Code's OSC 7501 root record. Its session ID comes
// from the process registration record rather than terminal output.
func (adapter) Identify(s ap.Signals) (ap.Identity, bool) {
	if s.ProgramApp != "claude-code" {
		return ap.Identity{}, false
	}
	return ap.Identity{Agent: Name, Cwd: s.RootCwd}, true
}

func (adapter) InterruptKeys() [][]byte { return [][]byte{[]byte("\x1b")} }

func (adapter) ResumesSessions() bool { return true }

// ReapplyOnLoad covers both load and resume: neither restores model/effort,
// while the connector restores mode from the transcript.
func (adapter) ReapplyOnLoad() []string { return []string{claudeModel, claudeEffort} }

func (adapter) ForceSwitchNote() string {
	return "cancel 只中断当前回合，后台 Bash 或子任务可能继续运行；强制切换会终止连接器并等待受管后代退出"
}

var permissionModes = ap.SetOf("acceptEdits", "plan", "auto", "bypassPermissions")

// claudeChatSide maps launch arguments to connector configuration. Options
// without a chat-side equivalent stay visible as unsupported rather than
// silently appearing to apply.
func claudeChatSide(kept []ap.KeptOption) ([]ap.Seed, []string) {
	var seeds []ap.Seed
	var unsupported []string
	modeIndex := -1
	dangerIndex := -1
	for i, o := range kept {
		switch o.Name {
		case "--model":
			seeds = append(seeds, ap.Seed{ConfigID: claudeModel, Value: o.Value, Arg: o.Token})
		case "--effort":
			seeds = append(seeds, ap.Seed{ConfigID: claudeEffort, Value: o.Value, Arg: o.Token})
		case "--permission-mode":
			modeIndex = i
		case "--dangerously-skip-permissions":
			dangerIndex = i
		default:
			unsupported = append(unsupported, o.Token)
		}
	}
	if dangerIndex >= 0 {
		seeds = appendMode(seeds, kept[dangerIndex], "bypassPermissions")
	} else if modeIndex >= 0 {
		o := kept[modeIndex]
		mode := o.Value
		if mode == "manual" {
			mode = "default"
		}
		if permissionModes[mode] || mode == "default" {
			seeds = appendMode(seeds, o, mode)
		} else {
			unsupported = append(unsupported, o.Token)
		}
	}
	return seeds, unsupported
}

func appendMode(seeds []ap.Seed, option ap.KeptOption, mode string) []ap.Seed {
	return append(seeds, ap.Seed{ConfigID: claudeMode, Value: mode, Arg: option.Token})
}
