// Package qoder is the hybrid adapter for Qoder CLI. Its ACP mode is a
// built-in --acp flag rather than a subcommand. The international and the
// Chinese edition share the CLI; New builds either from a Variant.
package qoder

import (
	"slices"
	"strings"

	ap "github.com/rainchan/webmux/internal/agentprofile"
)

// Name identifies Qoder on the wire and in hybrid session records.
const Name = "qoder"

// Config option IDs used by the built-in ACP.
const (
	qoderModel  = "model"
	qoderEffort = "effort"
	qoderMode   = "mode"
)

// Variant is one edition of the Qoder CLI.
type Variant struct {
	// Name identifies the edition on the wire and in session records.
	Name  string
	Title string
	// Executables are the binary names the edition ships under; a versioned
	// name (for example qodercli-1.1.81, which is what Linux /proc/<pid>/exe
	// reports) matches too.
	Executables []string
	// ProgramApps are the terminal program ids, besides Name, in the OSC 7501
	// root record.
	ProgramApps []string
}

// Agent is the international Qoder adapter.
var Agent = New(Variant{
	Name:        Name,
	Title:       "Qoder",
	Executables: []string{"qoder", "qodercli"},
	ProgramApps: []string{"qoder-cli"},
})

// New builds the adapter for edition v.
func New(v Variant) ap.Agent {
	return &adapter{
		SessionSteering: ap.SessionSteering{IdlePrompt: true},
		variant:         v,
		profile: &ap.Profile{
			DefaultChatMode:   "bypass_permissions",
			ChatModeAliases:   [][]string{{"bypass_permissions", "bypassPermissions", "yolo"}, {"accept_edits", "acceptEdits"}},
			Name:              v.Name,
			Title:             v.Title,
			Icon:              "qoder",
			Executables:       v.Executables,
			ProcessExecutable: func(base string) bool { return runsProcess(v.Executables, base) },
			VerifiedVersion:   "1.1.81",
			// Built-in ACP via --acp flag, no connector needed.
			ACPArg:         "--acp",
			HiddenCommands: []string{"login", "logout"},
			ResumeFlag:     "--resume",
			Positional:     ap.PositionalPrompt,
			Options:        qoderOptions(),
			ChatSide:       qoderChatSide,
			Subcommands: ap.SetOf("mcp", "hooks", "hook", "rollback", "update", "config",
				"plugins", "plugin", "commit", "agents", "agent", "skills", "skill",
				"feedback", "security", "login", "status", "remote-control", "sites",
				"wiki"),
			SessionSubcommands: ap.SetOf("fork-session"),
		},
	}
}

type adapter struct {
	ap.SessionSteering
	variant Variant
	profile *ap.Profile
}

// runsProcess reports whether base is one of executables or a versioned
// build of one.
func runsProcess(executables []string, base string) bool {
	for _, exe := range executables {
		if base == exe || strings.HasPrefix(base, exe+"-") {
			return true
		}
	}
	return false
}

func qoderOptions() map[string]ap.Option {
	o := map[string]ap.Option{}
	// Session args: prevent hybrid mode (select existing session).
	ap.Options(o, ap.ArgSession, ap.ArityNone, "--fork-session")
	ap.Options(o, ap.ArgSession, ap.ArityOne, "--session-id",
		"--from-pr", "--remote-session", "--remote-control", "--remote")
	// Drop args: session-resuming options dropped on resume/ACP.
	ap.Options(o, ap.ArgDrop, ap.ArityOptional, "-c", "--continue")
	ap.Options(o, ap.ArgDrop, ap.ArityOptional, "-r", "--resume")
	// Directory args: prevent hybrid mode (specify own working directory).
	ap.Options(o, ap.ArgDirectory, ap.ArityOne, "-w", "--cwd")
	ap.Options(o, ap.ArgDirectory, ap.ArityOptional, "--worktree")
	// Non-interactive args: prevent hybrid mode (no interactive UI).
	ap.Options(o, ap.ArgNonInteractive, ap.ArityNone, "-p", "--print", "-h", "--help",
		"-v", "--version", "--list-models", "--list-sessions",
		"--no-session-persistence")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityOne, "-o", "--output-format",
		"--input-format", "--delete-session")
	// Store args: prevent hybrid mode (different config store).
	ap.Options(o, ap.ArgStore, ap.ArityOne, "--config-dir")
	// Drop args: omitted from resume/ACP commands.
	ap.Options(o, ap.ArgDrop, ap.ArityOne, "--attachment", "-i", "--prompt-interactive",
		"-n", "--name")
	// Keep args: forwarded to resume and (if PassToACP) ACP commands.
	ap.Options(o, ap.ArgKeep, ap.ArityOne, "-m", "--model", "--reasoning-effort",
		"--thinking", "--thinking-budget", "--context-window",
		"--permission-mode", "--fallback-model",
		"--system-prompt", "--append-system-prompt",
		"--output-style", "--max-output-tokens",
		"--max-model-request-retries",
		"--agents", "--agent", "--plugin-dir",
		"--settings", "--setting-sources",
		"--mcp-config", "--allowed-mcp-server-names",
		"--allowed-tools", "--disallowed-tools", "--tools",
		"--add-dir")
	ap.Options(o, ap.ArgKeep, ap.ArityNone, "--dangerously-skip-permissions", "--yolo",
		"--strict-mcp-config", "--bare", "-d", "--debug")
	return o
}

func (a adapter) Profile() *ap.Profile { return a.profile }

// Identify reads the OSC 7501 root record. Session ID comes from
// session/list candidate fallback rather than terminal signals.
func (a adapter) Identify(s ap.Signals) (ap.Identity, bool) {
	if s.ProgramApp != a.variant.Name && !slices.Contains(a.variant.ProgramApps, s.ProgramApp) {
		return ap.Identity{}, false
	}
	return ap.Identity{Agent: a.variant.Name, Cwd: s.RootCwd}, true
}

func (adapter) InterruptKeys() [][]byte { return [][]byte{[]byte("\x1b")} }

func (adapter) ResumesSessions() bool { return true }

// ReapplyOnLoad: session/load does not restore model/effort.
func (adapter) ReapplyOnLoad() []string { return []string{qoderModel, qoderEffort} }

var permissionModes = ap.SetOf(
	"default", "accept_edits", "acceptEdits",
	"bypass_permissions", "bypassPermissions", "yolo",
	"dont_ask", "dontAsk", "auto", "plan",
)

// qoderChatSide maps launch arguments to ACP configuration.
func qoderChatSide(kept []ap.KeptOption) ([]ap.Seed, []string) {
	var seeds []ap.Seed
	var unsupported []string
	modeIndex := -1
	dangerIndex := -1
	for i, o := range kept {
		switch o.Name {
		case "-m", "--model":
			seeds = append(seeds, ap.Seed{ConfigID: qoderModel, Value: o.Value, Arg: o.Token})
		case "--reasoning-effort":
			seeds = append(seeds, ap.Seed{ConfigID: qoderEffort, Value: o.Value, Arg: o.Token})
		case "--permission-mode":
			modeIndex = i
		case "--dangerously-skip-permissions", "--yolo":
			dangerIndex = i
		default:
			unsupported = append(unsupported, o.Token)
		}
	}
	if dangerIndex >= 0 {
		seeds = appendMode(seeds, kept[dangerIndex], "bypass_permissions")
	} else if modeIndex >= 0 {
		o := kept[modeIndex]
		mode := o.Value
		if permissionModes[mode] {
			seeds = appendMode(seeds, o, normalizeMode(mode))
		} else {
			unsupported = append(unsupported, o.Token)
		}
	}
	return seeds, unsupported
}

// normalizeMode maps alias variants to the canonical snake_case form.
func normalizeMode(mode string) string {
	switch mode {
	case "acceptEdits":
		return "accept_edits"
	case "bypassPermissions", "yolo":
		return "bypass_permissions"
	case "dontAsk":
		return "dont_ask"
	default:
		return mode
	}
}

func appendMode(seeds []ap.Seed, option ap.KeptOption, mode string) []ap.Seed {
	return append(seeds, ap.Seed{ConfigID: qoderMode, Value: mode, Arg: option.Token})
}
