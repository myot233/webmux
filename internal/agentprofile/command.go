package agentprofile

import (
	"fmt"
	"slices"
	"strings"
)

// Form is how a launch config's argv runs the agent.
type Form int

const (
	// FormDirect executes the agent: [executable, args...].
	FormDirect Form = iota
	// FormStartup runs the agent through WebMux's interactive startup-file
	// wrapper so ~/.bashrc or ~/.zshrc applies:
	// bash|zsh -ic 'exec -- "$@"' webmux executable args...
	FormStartup
)

const (
	startupScript = `exec -- "$@"`
	// startupACPScript moves the agent's stdout to fd 3, the protocol pipe,
	// so anything the startup file prints stays on stdout/stderr and out of
	// ACP.
	startupACPScript = `exec -- "$@" 1>&3 3>&-`
	startupName      = "webmux"
)

// Reason codes explain why a command cannot use hybrid mode. Clients map
// them to text; they are stable wire values.
type Reason string

const (
	// ReasonUnknownAgent: no built-in profile matches the command.
	ReasonUnknownAgent Reason = "unknown_agent"
	// ReasonUnsupportedForm: a profile's executable appears, but not in a
	// direct or startup-file-wrapped form.
	ReasonUnsupportedForm Reason = "unsupported_form"
	// ReasonSessionStore: an option makes the TUI and ACP use different
	// session stores.
	ReasonSessionStore Reason = "session_store"
	// ReasonDirectoryArg: the command names its own working directory.
	ReasonDirectoryArg Reason = "directory_arg"
	// ReasonNonInteractive: an option or subcommand starts no interactive UI.
	ReasonNonInteractive Reason = "non_interactive"
	// ReasonSessionArg: a subcommand opens an existing session.
	ReasonSessionArg Reason = "session_arg"
)

// Message is the agent-neutral explanation of r; Registry.Explain adds
// the supported agents.
func (r Reason) Message() string {
	switch r {
	case ReasonUnsupportedForm:
		return "命令写法不受支持，请直接执行 agent 或使用“加载 shell 启动文件”"
	case ReasonSessionStore:
		return "该参数使聊天视图无法找到会话"
	case ReasonDirectoryArg:
		return "请在新建会话时选择目录，不要在命令中写目录"
	case ReasonNonInteractive:
		return "该参数不启动交互界面"
	case ReasonSessionArg:
		return "该参数打开已有会话，混合配置请不要指定会话"
	default:
		return "未识别到支持的 Agent"
	}
}

// UnsupportedError reports why argv cannot run as a hybrid session. Arg is
// the offending argument, empty when the whole command is unrecognised.
type UnsupportedError struct {
	Reason Reason
	Arg    string
}

func (e *UnsupportedError) Error() string {
	if e.Arg == "" {
		return fmt.Sprintf("hybrid mode unavailable: %s", e.Reason)
	}
	return fmt.Sprintf("hybrid mode unavailable: %s (%s)", e.Reason, e.Arg)
}

// Support is the hybrid capability of a command, as reported to clients.
// Connector is the profile's ACP connector, whether or not it is installed.
type Support struct {
	Supported bool
	Agent     string
	Reason    Reason
	Connector *Connector
	// Agents are the display names of every supported agent.
	Agents []string
}

// Seed is a chat-side config value derived from the launch arguments: the
// option ConfigID set to Value, because of argument Arg as written.
type Seed struct {
	ConfigID string
	Value    string
	Arg      string
}

// KeptOption is one kept option: its name, value (empty without one) and
// the argument as written, an option and its value joined by a space.
type KeptOption struct {
	Name, Value, Token string
}

// Canonical maps long option names to the short ones in aliases.
func (o KeptOption) Canonical(aliases map[string]string) string {
	if short, ok := aliases[o.Name]; ok {
		return short
	}
	return o.Name
}

// Command is a parsed launch command for a registered agent.
type Command struct {
	Agent   Agent
	Profile *Profile
	Form    Form
	// Shell is the startup-file shell of a FormStartup command.
	Shell string
	// Executable is argv's agent executable as written (possibly a path).
	Executable string
	args       []string
	kept       []string
	acpKept    []string
	keptOpts   []KeptOption
	dropped    []string
	hasPrompt  bool
}

func newCommand(agent Agent, form Form, shell, exe string, args []string) (Command, error) {
	cmd := Command{Agent: agent, Profile: agent.Profile(), Form: form, Shell: shell, Executable: exe, args: slices.Clone(args)}
	if err := cmd.classify(); err != nil {
		return Command{}, err
	}
	return cmd, nil
}

func splitForm(argv []string) (Form, string, string, []string, bool) {
	if len(argv) >= 6 && argv[0] == "/usr/bin/env" && argv[1] == ShellBash && argv[2] == "-ic" && argv[3] == startupScript && argv[4] == startupName {
		return FormStartup, ShellBash, argv[5], argv[6:], true
	}
	if len(argv) >= 5 && argv[1] == "-ic" && argv[2] == startupScript && argv[3] == startupName && SupportedStartupShell(argv[0]) {
		return FormStartup, argv[0], argv[4], argv[5:], true
	}
	if len(argv) >= 1 && argv[0] != "" && argv[0] != ShellBash && argv[0] != ShellZsh {
		return FormDirect, "", argv[0], argv[1:], true
	}
	return FormDirect, "", "", nil, false
}

func (c *Command) classify() error {
	p := c.Profile
	positionalSeen := false
	optionsDone := false
	for i := 0; i < len(c.args); i++ {
		token := c.args[i]
		if !optionsDone && token == "--" {
			optionsDone = true
			c.hasPrompt = true
			continue
		}
		if optionsDone || !strings.HasPrefix(token, "-") || token == "-" {
			if !positionalSeen && !optionsDone {
				if p.SessionSubcommands[token] {
					return &UnsupportedError{Reason: ReasonSessionArg, Arg: token}
				}
				if p.Subcommands[token] || hasAnyPrefix(token, p.SubcommandPrefixes) {
					return &UnsupportedError{Reason: ReasonNonInteractive, Arg: token}
				}
			}
			if p.Positional == PositionalDirectory || (p.Positional == PositionalDirectoryPrompt && !optionsDone) {
				return &UnsupportedError{Reason: ReasonDirectoryArg, Arg: token}
			}
			positionalSeen = true
			c.hasPrompt = true
			c.dropped = append(c.dropped, token)
			continue
		}
		name, inlineValue, inline := strings.Cut(token, "=")
		opt, known := p.Options[name]
		taken := []string{token}
		if known && !inline {
			next := i + 1
			switch {
			case opt.Arity == ArityOne && next < len(c.args):
				taken = append(taken, c.args[next])
				i = next
			case opt.Arity == ArityOptional && next < len(c.args) && !strings.HasPrefix(c.args[next], "-"):
				taken = append(taken, c.args[next])
				i = next
			case opt.Arity == ArityVariadic:
				for next < len(c.args) && !strings.HasPrefix(c.args[next], "-") {
					taken = append(taken, c.args[next])
					i = next
					next++
				}
			}
		}
		if !known {
			c.dropped = append(c.dropped, token)
			continue
		}
		switch opt.Class {
		case ArgKeep:
			c.kept = append(c.kept, taken...)
			if p.PassToACP || opt.PassToACP {
				c.acpKept = append(c.acpKept, taken...)
			}
			kept := KeptOption{Name: name, Value: inlineValue, Token: strings.Join(taken, " ")}
			if len(taken) > 1 {
				kept.Value = taken[1]
			}
			c.keptOpts = append(c.keptOpts, kept)
		case ArgDrop:
			if name == "--prompt" || name == "-i" || name == "--prompt-interactive" {
				c.hasPrompt = true
			}
			c.dropped = append(c.dropped, strings.Join(taken, " "))
		case ArgStore:
			return &UnsupportedError{Reason: ReasonSessionStore, Arg: name}
		case ArgDirectory:
			return &UnsupportedError{Reason: ReasonDirectoryArg, Arg: name}
		case ArgNonInteractive:
			return &UnsupportedError{Reason: ReasonNonInteractive, Arg: name}
		case ArgSession:
			return &UnsupportedError{Reason: ReasonSessionArg, Arg: name}
		}
	}
	return nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func (c Command) wrap(script string, args ...string) []string {
	return c.wrapExec(script, c.Executable, args...)
}

// wrapExec runs executable in the command's form.
func (c Command) wrapExec(script, executable string, args ...string) []string {
	if c.Form == FormStartup {
		prefix := []string{c.Shell, "-ic"}
		if c.Shell == ShellBash {
			prefix = []string{"/usr/bin/env", ShellBash, "-ic"}
		}
		return append(append(prefix, script, startupName, executable), args...)
	}
	return append([]string{executable}, args...)
}

// Lookup runs `command -v` for each name in the command's form, so a
// startup-file form sees the PATH ~/.bashrc or ~/.zshrc sets up: its
// output ends with a line of the marker followed by the first name's path,
// or nothing when that is not found, then one such line per further name.
// Startup-file output comes before the marker lines. FormDirect returns
// nil: the caller looks names up in the session's PATH itself.
func (c Command) Lookup(names ...string) []string {
	if c.Form != FormStartup {
		return nil
	}
	script := `for n in "$@"; do printf '\n` + LookupMarker + `%s' "$(command -v -- "$n" 2>/dev/null)"; done; printf '\n'`
	return append([]string{c.Shell, "-ic", script, startupName}, names...)
}

// LookupMarker starts each answer line of a Lookup command.
const LookupMarker = "\x01webmux-path:"

// injected returns the profile's TUI options that args does not already hold.
func (c Command) injected(args []string) []string {
	if len(c.Profile.TUIInject) == 0 || hasSubslice(args, c.Profile.TUIInject) {
		return nil
	}
	return slices.Clone(c.Profile.TUIInject)
}

func hasSubslice(args, sub []string) bool {
	if len(sub) > len(args) {
		return false
	}
	for i := 0; i <= len(args)-len(sub); i++ {
		if slices.Equal(args[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

// withoutOverriddenInject removes user-supplied instances of a two-token
// TUIInject option (such as --settings <json>) from args so a later option in
// the initial TUI launch cannot override the injected setting.
func (c Command) withoutOverriddenInject(args []string) []string {
	if len(c.Profile.TUIInject) != 2 {
		return args
	}
	flag, want := c.Profile.TUIInject[0], c.Profile.TUIInject[1]
	var out []string
	optionsDone := false
	for i := 0; i < len(args); i++ {
		token := args[i]
		if !optionsDone && token == "--" {
			optionsDone = true
			out = append(out, token)
			continue
		}
		if !optionsDone {
			if token == flag {
				if i+1 < len(args) {
					if args[i+1] == want {
						out = append(out, token, args[i+1])
					}
					i++
				}
				continue
			}
			if strings.HasPrefix(token, flag+"=") {
				continue
			}
		}
		out = append(out, token)
	}
	return out
}

// TUIArgv is the first terminal launch: the original command plus the
// profile's required TUI options, in the original form.
func (c Command) TUIArgv() []string {
	cleaned := c.withoutOverriddenInject(c.args)
	return c.wrap(startupScript, append(c.injected(cleaned), cleaned...)...)
}

// InitialTUIArgv is the first terminal launch with prompt passed through the
// agent's command-line interface when the profile supports it and the launch
// command does not already carry a prompt. When ok is false, the caller should
// start TUIArgv() and type the prompt into the terminal once it is quiet.
func (c Command) InitialTUIArgv(prompt string) (argv []string, ok bool) {
	if prompt == "" || c.hasPrompt || c.Profile.InitialPrompt == nil {
		return nil, false
	}
	extra, ok := c.Profile.InitialPrompt(prompt)
	if !ok || len(extra) == 0 {
		return nil, false
	}
	cleaned := c.withoutOverriddenInject(c.args)
	args := append(c.injected(cleaned), cleaned...)
	args = append(args, extra...)
	return c.wrap(startupScript, args...), true
}

// ResumeArgv opens agentSessionID in the terminal UI, keeping only the
// options the profile preserves. Dropped lists the omitted arguments as
// written, an option and its value joined by a space.
func (c Command) ResumeArgv(agentSessionID string) (argv, dropped []string) {
	args := append(c.injected(c.kept), c.kept...)
	args = append(args, c.Profile.ResumeFlag, agentSessionID)
	return c.wrap(startupScript, args...), slices.Clone(c.dropped)
}

// ACPArgv starts the agent's ACP server in the original form: the agent's
// acp subcommand (or ACPArg if set), or the profile's connector without
// arguments. A startup-file form writes ACP to fd 3, which the caller must
// connect to the protocol pipe (ProtocolOnFD3). Unsupported lists kept
// options that change behaviour but do not reach the chat side, for the
// user to be told.
func (c Command) ACPArgv() (argv, unsupported []string) {
	_, unsupported = c.ChatConfig()
	if connector := c.Profile.Connector; connector != nil {
		return c.wrapExec(startupACPScript, connector.Executable), unsupported
	}
	acpArg := c.Profile.ACPArg
	if acpArg == "" {
		acpArg = "acp"
	}
	args := append([]string{acpArg}, c.acpKept...)
	return c.wrap(startupACPScript, args...), unsupported
}

// ChatConfig derives chat-side config values from the kept options, and
// lists the kept options the chat side cannot apply.
func (c Command) ChatConfig() (seeds []Seed, unsupported []string) {
	if c.Profile.ChatSide == nil {
		return nil, nil
	}
	return c.Profile.ChatSide(c.keptOpts)
}

// ProtocolOnFD3 reports that ACPArgv's process writes the protocol to fd 3.
func (c Command) ProtocolOnFD3() bool { return c.Form == FormStartup }

// NativeProtocolArgv uses the same startup-file form and the exact executable
// resolved for the connector, without reapplying launch options to a saved
// native thread. Arguments remain argv entries, never shell-expanded text.
func (c Command) NativeProtocolArgv(executable string, args ...string) []string {
	return c.wrapExec(startupACPScript, executable, args...)
}

// Dropped lists the arguments that resume and ACP commands omit.
func (c Command) Dropped() []string { return slices.Clone(c.dropped) }
