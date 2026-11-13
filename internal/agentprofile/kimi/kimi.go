// Package kimi is the hybrid adapter for Kimi Code CLI.
package kimi

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	ap "github.com/rainchan/webmux/internal/agentprofile"
)

// Name identifies Kimi Code CLI on the wire and in hybrid session records.
const Name = "kimi"

const (
	kimiModel = "model"
	kimiMode  = "mode"
)

// Agent is the Kimi Code CLI adapter.
var Agent ap.Agent = adapter{}

type adapter struct{}

var profile = &ap.Profile{
	DefaultChatMode: "yolo",
	Name:            Name,
	Title:           "Kimi Code CLI",
	Icon:            "kimi",
	VerifiedVersion: "2.5.2",
	HiddenCommands:  []string{"login", "logout"},
	ResumeFlag:      "--session",
	Positional:      ap.PositionalPrompt,
	Options:         options(),
	ChatSide:        chatSide,
	Subcommands: ap.SetOf(
		"export", "fork", "provider", "session", "acp", "web", "server",
		"rc", "remote", "login", "doctor", "vis", "install-desktop",
		"install-app", "app", "migrate", "import-sessions", "upgrade",
		"update", "help",
	),
}

func options() map[string]ap.Option {
	o := map[string]ap.Option{}
	ap.Options(o, ap.ArgKeep, ap.ArityOne, "-m", "--model", "--skills-dir", "--add-dir")
	ap.Options(o, ap.ArgKeep, ap.ArityNone, "--auto", "--yolo", "--plan")
	ap.Options(o, ap.ArgDrop, ap.ArityNone, "-c", "--continue")
	ap.Options(o, ap.ArgDrop, ap.ArityOptional, "-S", "--session")
	ap.Options(o, ap.ArgStore, ap.ArityOne, "--environment")
	ap.Options(o, ap.ArgDirectory, ap.ArityOne, "--environment-cwd")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityOne, "-p", "--prompt", "--output-format")
	ap.Options(o, ap.ArgNonInteractive, ap.ArityNone, "-h", "--help", "-V", "--version")
	return o
}

func (adapter) Profile() *ap.Profile { return profile }

// Identify reads Kimi's OSC 7501 root record. The session ID comes from the
// generic session/list candidate fallback rather than terminal signals.
func (adapter) Identify(s ap.Signals) (ap.Identity, bool) {
	if s.ProgramApp != "kimi-code" {
		return ap.Identity{}, false
	}
	return ap.Identity{Agent: Name, Cwd: s.RootCwd}, true
}

func (adapter) InterruptKeys() [][]byte { return [][]byte{[]byte("\x1b")} }

// SubmitDelay keeps Enter out of Kimi's paste-burst input path.
func (adapter) SubmitDelay() time.Duration { return 200 * time.Millisecond }

func (adapter) ResumesSessions() bool { return true }

// ReapplyOnLoad: load and resume retain model and thinking but reset mode.
func (adapter) ReapplyOnLoad() []string { return []string{kimiMode} }

// ConfigErrorHandled treats Kimi's idempotent mode error as success. A cold
// load/resume can report default while the underlying session is already in
// the requested mode.
func (adapter) ConfigErrorHandled(err error, configID, value string) bool {
	if configID != kimiMode {
		return false
	}
	var rpc *acp.RPCError
	if !errors.As(err, &rpc) || rpc.Code != acp.CodeInternalError {
		return false
	}
	var data struct {
		Details string `json:"details"`
	}
	if json.Unmarshal(rpc.Data, &data) != nil {
		return false
	}
	if value == "yolo" {
		value = "YOLO"
	}
	return data.Details == "Already in "+value+" mode"
}

// chatSide maps launch arguments to ACP configuration. Plan wins over the
// automatic modes; every option that cannot reach the chat side is reported.
func chatSide(kept []ap.KeptOption) ([]ap.Seed, []string) {
	var seeds []ap.Seed
	var unsupported []string
	modeIndex := -1
	for i, o := range kept {
		switch o.Name {
		case "-m", "--model":
			seeds = append(seeds, ap.Seed{ConfigID: kimiModel, Value: o.Value, Arg: o.Token})
		case "--plan":
			if modeIndex >= 0 {
				unsupported = append(unsupported, kept[modeIndex].Token)
			}
			modeIndex = i
		case "--auto", "--yolo":
			if modeIndex >= 0 {
				unsupported = append(unsupported, o.Token)
			} else {
				modeIndex = i
			}
		default:
			unsupported = append(unsupported, o.Token)
		}
	}
	if modeIndex >= 0 {
		o := kept[modeIndex]
		seeds = append(seeds, ap.Seed{ConfigID: kimiMode, Value: o.Name[2:], Arg: o.Token})
	}
	return seeds, unsupported
}
