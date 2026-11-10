// Package agentprofile is the agent-neutral core of hybrid sessions: the
// Agent adapter interface with its optional capabilities, the Profile data
// an adapter describes its command line with, argv classification, and the
// Registry that recognises commands and terminal signals. It starts no
// processes and holds no state, and names no agent: each agent is an
// adapter in its own subpackage (opencode, omp, codex), listed by
// agentprofile/agents.
package agentprofile

import (
	"encoding/json"
	"slices"
	"time"
)

// Agent is one CLI agent that hybrid sessions can move between a terminal
// UI and an ACP connection. Behaviour that only some agents have is an
// optional interface (Resumer, ConfigReapplier, Steerer, LockTracker,
// ForceSwitchNoter); callers type-assert for it.
type Agent interface {
	// Profile is the agent's static description; it is read-only.
	Profile() *Profile
	// Identify interprets terminal signals; ok is false when they do not
	// come from this agent.
	Identify(Signals) (Identity, bool)
	// InterruptKeys are the key presses that stop a running turn in the
	// terminal UI, written one at a time with a gap between them.
	InterruptKeys() [][]byte
}

// Resumer is an agent whose session/resume (no history replay) may be used
// when reconnecting after a crash, if the agent declares it; otherwise
// crash reconnects use session/load.
type Resumer interface {
	ResumesSessions() bool
}

// ConfigReapplier is an agent whose session/load does not restore some
// config options, so their recorded values are set again after a load.
type ConfigReapplier interface {
	ReapplyOnLoad() []string
}

// ConfigErrorHandler reports whether a configuration error means the wanted
// value is already in effect and therefore needs no further write.
type ConfigErrorHandler interface {
	ConfigErrorHandled(err error, configID, value string) bool
}

// ConfigErrorHandled reports whether err is an idempotent configuration error.
func ConfigErrorHandled(a Agent, err error, configID, value string) bool {
	h, ok := a.(ConfigErrorHandler)
	return ok && h.ConfigErrorHandled(err, configID, value)
}

// SubmitDelayer is an agent whose TUI needs time between a text submission
// and its Enter key so the Enter is not consumed as part of a paste burst.
type SubmitDelayer interface {
	SubmitDelay() time.Duration
}

// SubmitDelay reports how long to wait between a text submission and Enter.
func SubmitDelay(a Agent) time.Duration {
	d, ok := a.(SubmitDelayer)
	if !ok {
		return 0
	}
	return d.SubmitDelay()
}

// Steerer is an agent with a private extension that adds text to the
// running turn.
type Steerer interface {
	// SteeringMethod is the extension's JSON-RPC method.
	SteeringMethod() string
	// SteeringAvailable reads the initialize result's _meta.
	SteeringAvailable(initMeta json.RawMessage) bool
	// SteeringMeta is the _meta sent with each steering request, nil for
	// none.
	SteeringMeta() json.RawMessage
}

// LockTracker is an agent whose session IDs come from the writer locks its
// processes hold open rather than from its terminal signals.
type LockTracker interface {
	// LockedSessions returns the agent session IDs whose writer locks are
	// among paths, the open files of the terminal UI's processes, in paths
	// order. getenv reads the session's environment; userHome is the
	// service user's home directory.
	LockedSessions(paths []string, getenv func(string) string, userHome string) []string
}

// ProcessFacts are the agent-neutral, in-memory facts needed to validate a
// process registration record. Start and Domain are opaque, platform-specific
// values; adapters only compare them with their records.
type ProcessFacts struct {
	PID        int
	Executable string
	Cwd        string
	Start      string
	Domain     string
}

// ProcessRegistry is an agent whose session IDs and external writers can be
// discovered from per-process registration records. The lookup callback
// supplies facts for another PID without exposing process data to clients.
type ProcessRegistry interface {
	RegisteredSession(process ProcessFacts, getenv func(string) string, userHome string) (string, bool)
	ExternallyHeld(sessionID string, current ProcessFacts, lookup func(int) (ProcessFacts, bool), getenv func(string) string, userHome string) bool
}

// ForceSwitchNoter is an agent that says what forcing a switch while its
// terminal UI runs a turn loses; others get DefaultForceSwitchNote.
type ForceSwitchNoter interface {
	ForceSwitchNote() string
}

// DefaultForceSwitchNote is what forcing a switch loses for an agent
// without a ForceSwitchNoter.
const DefaultForceSwitchNote = "已输出的部分可能丢失；回合不会在后台继续"

// ForceSwitchNote is a's note on forcing a switch.
func ForceSwitchNote(a Agent) string {
	if n, ok := a.(ForceSwitchNoter); ok {
		return n.ForceSwitchNote()
	}
	return DefaultForceSwitchNote
}

// ResumesSessions reports whether a permits session/resume.
func ResumesSessions(a Agent) bool {
	r, ok := a.(Resumer)
	return ok && r.ResumesSessions()
}

// ReapplyOnLoad lists the config options a's session/load does not keep.
func ReapplyOnLoad(a Agent) []string {
	if r, ok := a.(ConfigReapplier); ok {
		return r.ReapplyOnLoad()
	}
	return nil
}

// Connector is a separately installed program that provides an agent's ACP
// side when the agent has no ACP mode of its own. WebMux never installs it;
// it checks that it can be found before every ACP start.
type Connector struct {
	// Executable is looked up through PATH like the agent itself.
	Executable     string
	Title          string
	InstallCommand string
	DocsURL        string
	// VerifiedVersion is the newest release measured; the ACP side reports
	// the connector's version, so an older one only produces a notice.
	VerifiedVersion string
}

// ArgClass says what happens to a recognised option when webmux builds a
// resume or ACP command from a launch config's argv.
type ArgClass int

const (
	// ArgKeep is written into resume commands and, for profiles that pass
	// options through, into the ACP command.
	ArgKeep ArgClass = iota
	// ArgDrop is omitted from resume and ACP commands and listed to the user.
	ArgDrop
	// ArgStore prevents hybrid mode: the TUI and ACP would see different
	// session stores.
	ArgStore
	// ArgDirectory prevents hybrid mode: the command names its own directory.
	ArgDirectory
	// ArgNonInteractive prevents hybrid mode: no interactive UI starts.
	ArgNonInteractive
	// ArgSession prevents hybrid mode: the option selects an agent session.
	ArgSession
)

// Arity is how many values an option consumes after the option itself.
type Arity int

const (
	ArityNone Arity = iota
	ArityOne
	// ArityOptional consumes the next token unless it looks like an option.
	ArityOptional
	// ArityVariadic consumes values until the next option.
	ArityVariadic
)

// Option is how an agent treats one command-line option.
type Option struct {
	Class     ArgClass
	Arity     Arity
	PassToACP bool
}

// Options builds an option table entry for each name.
func Options(table map[string]Option, class ArgClass, arity Arity, names ...string) {
	for _, name := range names {
		table[name] = Option{Class: class, Arity: arity}
	}
}

// PassthroughOptions builds an ArgKeep option table entry for each name that
// is also passed through to the agent's acp subcommand.
func PassthroughOptions(table map[string]Option, arity Arity, names ...string) {
	for _, name := range names {
		table[name] = Option{Class: ArgKeep, Arity: arity, PassToACP: true}
	}
}

// Positional says what a non-option argument means for the agent.
type Positional int

const (
	// PositionalPrompt arguments are an initial prompt and are dropped.
	PositionalPrompt Positional = iota
	// PositionalDirectory arguments name a working directory.
	PositionalDirectory
)

// Profile is an agent's static description: identity, versions, how its
// command line is classified and rewritten, and how its processes run.
type Profile struct {
	// Name is the stable agent identifier on the wire and in records.
	Name string
	// Title is the display name.
	Title string
	// Icon is the agent's brand glyph: a key of the launch-config icon
	// catalog, which the browser renders next to Title.
	Icon string
	// Executables are the base names the agent runs as; empty means Name.
	Executables []string
	// ProcessExecutable optionally recognizes a native process whose name on
	// disk differs from the launch command, such as a versioned installation.
	ProcessExecutable func(base string) bool
	// VerifiedVersion is the newest release measured; older versions only
	// produce a notice.
	VerifiedVersion string
	// Connector provides the ACP side; nil when the agent's own executable
	// has an acp subcommand.
	Connector *Connector
	// Launcher is an interpreter that may run as the PTY's root process when
	// its direct child is the agent's native executable (npm installs).
	Launcher string
	// ACPEnv are variables the ACP process gets unless the launch config
	// sets them.
	ACPEnv map[string]string
	// AgentPathEnv names the ACP variable that receives the resolved path of
	// the terminal UI's executable, so both sides run the same agent; that
	// executable's --version is the agent version.
	AgentPathEnv string
	// HiddenCommands are agent slash commands left out of the chat view's
	// command menu.
	HiddenCommands []string
	// StartupQuiet ignores the terminal UI's busy signal for this long after
	// it starts.
	StartupQuiet time.Duration

	// ResumeFlag opens a session in the terminal UI: an option or a
	// subcommand, followed by the session ID.
	ResumeFlag string
	// ACPArg is the argument that starts the built-in ACP server. Empty
	// means the default "acp" subcommand; set to "--acp" for agents whose
	// ACP mode is a flag rather than a subcommand.
	ACPArg string
	// TUIInject are options every terminal run needs.
	TUIInject []string
	// PassToACP writes the kept options into the ACP command too.
	PassToACP bool
	// ModelChoiceValues labels model select choices with their value, the
	// full "provider/model" id, for agents whose model display names repeat
	// across providers.
	ModelChoiceValues bool
	// ChatSide derives chat-side config values from the kept options and
	// lists the kept options the chat side cannot apply.
	ChatSide func(kept []KeptOption) ([]Seed, []string)
	// InitialPrompt returns the arguments to append to a fresh terminal launch
	// to submit prompt at start, and true when the agent supports doing so for
	// prompt. Nil or a false return falls back to typing the prompt into the
	// terminal once it is quiet.
	InitialPrompt func(prompt string) ([]string, bool)
	Options       map[string]Option
	Positional    Positional
	Subcommands   map[string]bool
	// SubcommandPrefixes also start no interactive UI.
	SubcommandPrefixes []string
	// SessionSubcommands open an existing session.
	SessionSubcommands map[string]bool
}

// RunsAs reports whether executable base name base runs this agent.
func (p *Profile) RunsAs(base string) bool {
	if len(p.Executables) == 0 {
		return base == p.Name
	}
	return slices.Contains(p.Executables, base)
}

// RunsProcess reports whether a running native process belongs to this agent.
func (p *Profile) RunsProcess(base string) bool {
	if p.ProcessExecutable != nil {
		return p.ProcessExecutable(base)
	}
	return p.RunsAs(base)
}

// SetOf builds a set of names.
func SetOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}
