// Package devin is the hybrid adapter for Devin CLI.
package devin

import (
	"encoding/json"
	"errors"

	"github.com/rainchan/webmux/internal/acp"
	ap "github.com/rainchan/webmux/internal/agentprofile"
)

// Name identifies Devin CLI on the wire and in hybrid session records.
const Name = "devin"

const (
	devinModel = "model"
	devinMode  = "mode"
)

// Agent is the Devin CLI adapter.
var Agent ap.Agent = adapter{}

type adapter struct{}

var profile = &ap.Profile{
	Name:              Name,
	Title:             "Devin CLI",
	Icon:              "devin",
	VerifiedVersion:   "3000.12.57",
	VersionExecutable: true,
	HiddenCommands:    []string{"login", "logout"},
	ResumeFlag:        "--resume",
	Positional:        ap.PositionalDirectoryPrompt,
	Options:           options(),
	ChatSide:          chatSide,
	Subcommands: ap.SetOf(
		"acp", "auth", "cloud", "desktop", "doctor", "forward", "help",
		"list", "ls", "mcp", "migrate", "models", "plugins", "rm", "rules",
		"sandbox", "setup", "skills", "ssh", "uninstall", "update", "version",
	),
}

func options() map[string]ap.Option {
	o := map[string]ap.Option{}
	ap.PassthroughOptions(o, ap.ArityOne, "--model")
	ap.Options(o, ap.ArgKeep, ap.ArityOne, "--permission-mode", "--verbosity")
	ap.Options(o, ap.ArgKeep, ap.ArityOptional, "--respect-workspace-trust", "--export")
	ap.Options(o, ap.ArgKeep, ap.ArityNone, "--sandbox")
	ap.Options(o, ap.ArgDrop, ap.ArityOne, "--prompt-file")
	ap.Options(o, ap.ArgSession, ap.ArityNone, "-c", "--continue")
	ap.Options(o, ap.ArgSession, ap.ArityOptional, "-r", "--resume")
	// --cloud drives cloud sessions and --config can replace the session
	// store; the chat side would not find the session.
	ap.Options(o, ap.ArgStore, ap.ArityNone, "--cloud")
	ap.Options(o, ap.ArgStore, ap.ArityOne, "--config")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityOptional, "-p", "--print")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityNone, "-h", "--help", "-V", "--version")
	return o
}

func (adapter) Profile() *ap.Profile { return profile }

// Identify reads Devin's OSC 7501 root record. The record carries no
// session ID; it comes from the generic session/list candidate fallback.
func (adapter) Identify(s ap.Signals) (ap.Identity, bool) {
	if s.ProgramApp != "devin" {
		return ap.Identity{}, false
	}
	return ap.Identity{Agent: Name, Cwd: s.RootCwd}, true
}

func (adapter) InterruptKeys() [][]byte { return [][]byte{[]byte("\x1b")} }

// Devin answers session/resume with "Method not found", so crash
// reconnects always use session/load: no Resumer.

// devinLockedError is the JSON-RPC code of a session another process holds.
const devinLockedError = -32015

// LockHeldBy parses Devin's session_locked error, whose data carries
// cognition.ai/errorKind and cognition.ai/lockHolderPid.
func (adapter) LockHeldBy(err error) (int, bool) {
	var rpc *acp.RPCError
	if !errors.As(err, &rpc) || rpc.Code != devinLockedError || len(rpc.Data) == 0 {
		return 0, false
	}
	var data struct {
		Kind string `json:"cognition.ai/errorKind"`
		PID  int    `json:"cognition.ai/lockHolderPid"`
	}
	if json.Unmarshal(rpc.Data, &data) != nil || data.Kind != "session_locked" || data.PID <= 0 {
		return 0, false
	}
	return data.PID, true
}

// ForceCloseMeta makes session/close release a lock another process holds,
// Devin's advertised cognition.ai/force close extension. Measured: the same
// _meta on session/load does not take the lock.
func (adapter) ForceCloseMeta() json.RawMessage {
	return json.RawMessage(`{"cognition.ai/force":true}`)
}

// chatSide maps launch arguments to ACP configuration. --model seeds the
// model option; the mappable --permission-mode values seed mode, and every
// option the chat side cannot apply is reported.
func chatSide(kept []ap.KeptOption) ([]ap.Seed, []string) {
	var seeds []ap.Seed
	var unsupported []string
	for _, o := range kept {
		switch o.Name {
		case "--model":
			seeds = append(seeds, ap.Seed{ConfigID: devinModel, Value: o.Value, Arg: o.Token})
		case "--permission-mode":
			if mode, ok := devinChatModes[o.Value]; ok {
				seeds = append(seeds, ap.Seed{ConfigID: devinMode, Value: mode, Arg: o.Token})
			} else {
				unsupported = append(unsupported, o.Token)
			}
		default:
			unsupported = append(unsupported, o.Token)
		}
	}
	return seeds, unsupported
}

// devinChatModes maps --permission-mode values to Devin's ACP mode ids;
// "auto" has no chat-side mode.
var devinChatModes = map[string]string{
	"accept-edits": "accept-edits",
	"smart":        "smart",
	"dangerous":    "bypass",
}
