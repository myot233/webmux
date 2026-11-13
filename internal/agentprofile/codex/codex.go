// Package codex is the hybrid adapter for Codex CLI, whose ACP side is the
// separately installed codex-acp connector. Its tables follow the
// measurements recorded in the add-codex-hybrid-profile change.
package codex

import (
	"strings"
	"time"

	ap "github.com/rainchan/webmux/internal/agentprofile"
)

// Name identifies Codex on the wire and in hybrid session records.
const Name = "codex"

// Codex config option IDs and mode values of the codex-acp connector.
const (
	codexModel     = "model"
	codexEffort    = "reasoning_effort"
	codexMode      = "mode"
	codexEffortKey = "model_reasoning_effort"
)

// Agent is the Codex adapter.
var Agent ap.Agent = adapter{SessionSteering: ap.SessionSteering{IdlePrompt: true}}

type adapter struct {
	ap.SessionSteering
}

// locks: Codex names its session in no terminal signal; its processes
// hold a writer lock per open session instead.
var locks = ap.LockLayout{HomeEnv: "CODEX_HOME", HomeDefault: ".codex", Dir: "thread-writer-locks", Suffix: ".lock"}

// profile: codex has no ACP mode of its own: the codex-acp connector
// translates ACP to the Codex app-server protocol and starts
// `codex app-server` itself (from CODEX_PATH, so both sides run the same
// codex).
var profile = &ap.Profile{
	DefaultChatMode: "agent-full-access",
	Name:            Name,
	Title:           "Codex CLI",
	Icon:            "codex",
	VerifiedVersion: "0.165.0",
	Connector: &ap.Connector{
		Executable:      "codex-acp",
		Title:           "Codex ACP 适配器",
		InstallCommand:  "npm install -g @agentclientprotocol/codex-acp",
		DocsURL:         "https://github.com/agentclientprotocol/codex-acp",
		VerifiedVersion: "2.3.0",
	},
	// npm installs run codex through a node script that starts the native
	// executable.
	Launcher: "node",
	// The chat side never runs a browser login.
	ACPEnv:         map[string]string{"NO_BROWSER": "1"},
	AgentPathEnv:   "CODEX_PATH",
	HiddenCommands: []string{"logout"},
	// Codex reports working while it loads.
	StartupQuiet: 3 * time.Second,
	// resume is a subcommand: global options come before it.
	ResumeFlag: "resume",
	// The default TUI hands turns to a shared app-server daemon outside the
	// session's process group, where a turn outlives the TUI.
	TUIInject:     []string{"--no-daemon"},
	InitialPrompt: func(prompt string) ([]string, bool) { return []string{"--", prompt}, true },
	Positional:    ap.PositionalPrompt,
	Options:       codexOptions(),
	ChatSide:      codexChatSide,
	Subcommands: ap.SetOf("exec", "e", "review", "login", "logout", "mcp", "plugin",
		"app-server", "remote-control", "app", "completion", "update", "doctor",
		"sandbox", "debug", "apply", "a", "cloud", "exec-server", "features",
		"agents", "queue", "archive", "delete", "unarchive", "help"),
	SubcommandPrefixes: []string{"migrate-"},
	SessionSubcommands: ap.SetOf("resume", "fork"),
}

func codexOptions() map[string]ap.Option {
	o := map[string]ap.Option{}
	ap.Options(o, ap.ArgDrop, ap.ArityOne, "-i", "--image")
	ap.Options(o, ap.ArgStore, ap.ArityOne, "--remote", "--remote-auth-token-env")
	ap.Options(o, ap.ArgDirectory, ap.ArityOne, "-C", "--cd")
	ap.Options(o, ap.ArgDirectory, ap.ArityNone, "--worktree")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityNone, "-h", "--help", "-V", "--version")
	ap.Options(o, ap.ArgKeep, ap.ArityOne, "-m", "--model", "-p", "--profile", "-s",
		"--sandbox", "-a", "--ask-for-approval", "-c", "--config", "--enable",
		"--disable", "--add-dir", "--local-provider")
	ap.Options(o, ap.ArgKeep, ap.ArityNone, "--approve-for-me",
		"--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust",
		"--search", "--oss", "--no-alt-screen", "--strict-config", "--no-daemon")
	return o
}

func (adapter) Profile() *ap.Profile { return profile }

// Identify reads Codex's OSC 7501 root record; the session ID comes from
// LockedSessions.
func (adapter) Identify(s ap.Signals) (ap.Identity, bool) {
	if s.ProgramApp != Name {
		return ap.Identity{}, false
	}
	return ap.Identity{Agent: Name, Cwd: s.RootCwd}, true
}

func (adapter) InterruptKeys() [][]byte { return [][]byte{[]byte("\x1b")} }

func (adapter) ResumesSessions() bool { return true }

// ReapplyOnLoad: session/load returns the connector's defaults, whatever
// the session last used.
func (adapter) ReapplyOnLoad() []string { return []string{codexModel, codexEffort, codexMode} }

func (adapter) LockedSessions(paths []string, getenv func(string) string, userHome string) []string {
	return locks.Sessions(paths, getenv, userHome)
}

// codexModes maps approval and sandbox options to the connector's mode, in
// priority order: the first present one wins.
var codexModes = []struct {
	name, value, mode string
}{
	{"--dangerously-bypass-approvals-and-sandbox", "", "agent-full-access"},
	{"--approve-for-me", "", "agent"},
	{"-s", "read-only", "read-only"},
	{"-s", "workspace-write", "workspace-write"},
}

// codexChatOnly are kept options the connector has no equivalent for.
var codexChatOnly = ap.SetOf("-p", "--profile", "--enable", "--disable", "--search",
	"--add-dir", "--oss", "--local-provider")

// codexChatSide derives the model, reasoning effort and mode from argv,
// and lists the options that do not reach the chat side.
func codexChatSide(kept []ap.KeptOption) ([]ap.Seed, []string) {
	var seeds []ap.Seed
	var unsupported []string
	mode := -1
	for _, o := range kept {
		name := o.Canonical(map[string]string{"--model": "-m", "--sandbox": "-s", "--ask-for-approval": "-a", "--config": "-c"})
		switch {
		case name == "-m":
			seeds = append(seeds, ap.Seed{ConfigID: codexModel, Value: o.Value, Arg: o.Token})
		case name == "-c":
			key, value, ok := strings.Cut(o.Value, "=")
			if ok && strings.TrimSpace(key) == codexEffortKey {
				seeds = append(seeds, ap.Seed{ConfigID: codexEffort, Value: unquote(strings.TrimSpace(value)), Arg: o.Token})
				continue
			}
			unsupported = append(unsupported, o.Token)
		case name == "-a":
			unsupported = append(unsupported, o.Token)
		case name == "-s" || name == "--approve-for-me" || name == "--dangerously-bypass-approvals-and-sandbox":
			i := modeIndex(name, o.Value)
			if i < 0 {
				unsupported = append(unsupported, o.Token)
				continue
			}
			if mode < 0 || i < mode {
				mode = i
			}
		case codexChatOnly[name]:
			unsupported = append(unsupported, o.Token)
		}
	}
	if mode >= 0 {
		m := codexModes[mode]
		arg := m.name
		if m.value != "" {
			arg += " " + m.value
		}
		seeds = append(seeds, ap.Seed{ConfigID: codexMode, Value: m.mode, Arg: arg})
	}
	return seeds, unsupported
}

func modeIndex(name, value string) int {
	for i, m := range codexModes {
		if m.name == name && m.value == value {
			return i
		}
	}
	return -1
}

// unquote strips one pair of matching TOML string quotes.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
