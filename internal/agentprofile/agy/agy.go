// Package agy is the hybrid adapter for Antigravity CLI (agy). Its option and
// subcommand tables follow the measured baseline in
// openspec/changes/add-antigravity-acp-hybrid/baseline.md.
package agy

import (
	"slices"
	"strings"

	ap "github.com/rainchan/webmux/internal/agentprofile"
)

// Name identifies Antigravity CLI on the wire and in hybrid session records.
const Name = "agy"

// BackgroundStatusPrompt flushes agy's background completion, which its ACP
// server otherwise releases only with a later prompt.
const BackgroundStatusPrompt = "The background processes from the previous turn have exited. Please check the latest background task status and report the outcome to the user. Do not start any new tasks."

// TUISettingsJSON is the inline --settings payload injected into every agy TUI
// launch. It uses the title command hook to emit the conversation_id as an
// OSC 7501 child record to /dev/tty without writing anything to stdout.
const TUISettingsJSON = `{"title":{"type":"command","command":"id=$(sed -n 's/.*\"conversation_id\":\"\\([0-9a-f-]*\\)\".*/\\1/p'); [ -n \"$id\" ] && printf '\\033]7501;state=working:id=%s\\007' \"$id\" >/dev/tty 2>/dev/null || true"}}`

// Agent is the Antigravity CLI adapter.
var Agent ap.Agent = adapter{}

type adapter struct{}

var profile = &ap.Profile{
	Name:                   Name,
	Title:                  "Antigravity CLI",
	Icon:                   "antigravity",
	VerifiedVersion:        "1.4.12",
	BackgroundStatusPrompt: BackgroundStatusPrompt,
	ResumeFlag:             "--conversation",
	TUIInject:              []string{"--settings", TUISettingsJSON},
	InitialPrompt:          initialPrompt,
	Positional:             ap.PositionalPrompt,
	Options:                options(),
	ChatSide:               chatSide,
	Subcommands: ap.SetOf("acp", "agent", "agents", "changelog", "help",
		"install", "mcp", "mic-serve", "models", "plugin", "plugins",
		"remote-control", "update"),
}

func options() map[string]ap.Option {
	o := map[string]ap.Option{}
	ap.PassthroughOptions(o, ap.ArityNone, "--dangerously-skip-permissions", "--sandbox")
	ap.PassthroughOptions(o, ap.ArityOne, "--agent", "--add-dir", "--log-file")
	ap.Options(o, ap.ArgKeep, ap.ArityOne, "--model", "--effort", "--mode")
	ap.Options(o, ap.ArgDrop, ap.ArityNone, "-c", "--continue", "--remote-control")
	ap.Options(o, ap.ArgDrop, ap.ArityOne, "--conversation", "-i", "--prompt-interactive", "--settings")
	ap.Options(o, ap.ArgStore, ap.ArityOne, "--project", "--new-project")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityNone, "--disable-slash-commands", "-h", "--help", "-v", "--version")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityOne, "-p", "--print", "--prompt",
		"--output-format", "--input-format", "--json-schema", "--print-timeout")
	return o
}

func (adapter) Profile() *ap.Profile { return profile }

// Identify reads Antigravity CLI's OSC 7501 root record (app=antigravity) and
// the UUID v4 child record id emitted by the injected title hook.
func (adapter) Identify(s ap.Signals) (ap.Identity, bool) {
	if s.ProgramApp != "antigravity" {
		return ap.Identity{}, false
	}
	return ap.Identity{
		Agent:          Name,
		AgentSessionID: uuidOrEmpty(s.ProgramChildID),
		Cwd:            s.RootCwd,
	}, true
}

func (adapter) InterruptKeys() [][]byte { return [][]byte{[]byte("\x1b")} }

func (adapter) ResumesSessions() bool { return true }

// ReapplyOnLoad: agy acp resets model and thought_level to defaults when a new
// process loads or resumes a session.
func (adapter) ReapplyOnLoad() []string { return []string{"model", "thought_level"} }

// initialPrompt uses --prompt-interactive=<prompt> unless prompt starts with
// '-', which agy's parser rejects when it matches a flag name.
func initialPrompt(prompt string) ([]string, bool) {
	if strings.HasPrefix(prompt, "-") {
		return nil, false
	}
	return []string{"--prompt-interactive=" + prompt}, true
}

func chatSide(kept []ap.KeptOption) ([]ap.Seed, []string) {
	var seeds []ap.Seed
	var unsupported []string
	for _, o := range kept {
		switch o.Name {
		case "--model":
			seeds = append(seeds, ap.Seed{ConfigID: "model", Value: o.Value, Arg: o.Token})
		case "--effort":
			seeds = append(seeds, ap.Seed{ConfigID: "thought_level", Value: o.Value, Arg: o.Token})
		case "--mode":
			if !slices.Contains(unsupported, "--mode") {
				unsupported = append(unsupported, "--mode")
			}
		}
	}
	return seeds, unsupported
}

func uuidOrEmpty(s string) string {
	if len(s) != 36 {
		return ""
	}
	for i := range len(s) {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return ""
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return ""
			}
		}
	}
	return s
}
