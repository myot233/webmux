// Command fakeagent is a scriptable stand-in for omp, OpenCode, Codex CLI,
// Claude Code, Kimi Code CLI and ACP connectors in tests and E2E.
// Build it with
//
//	go build -o <dir>/fakeagent ./internal/acp/testdata/fakeagent
//
// Usage:
//
//	fakeagent [flags]       TUI mode
//	fakeagent [flags] acp   ACP agent on stdio
//
// The profile comes from FAKEAGENT_PROFILE, else from the executable name:
// "opencode" is OpenCode, "codex" and "codex-acp" are Codex, "claude" and
// "claude-agent-acp" are Claude Code, anything else omp. Named either
// connector it runs in ACP mode without arguments; "codex --version" (or -V)
// and "claude --version" print their respective versions and exit.
//
// All modes share one session store (--store or FAKEAGENT_STORE), so a
// session written in one mode can be resumed or loaded in the other.
//
// TUI mode prints a transcript and reads keystrokes (raw mode when stdin is
// a terminal). For omp and opencode each session it opens is announced as a
// Warp cli-agent session_start event (agent, plugin_version, session_id,
// cwd) and OSC 7501 root (state, app) and child (id) records; turns report
// working/idle. Enter submits a line; "/new" starts a new session, "/exit"
// quits. The interrupt key stops a turn: Esc for omp and Codex, Esc Esc for
// opencode. --resume ID (alias -r) reopens an existing session; -s ID
// (alias --session) reopens or creates it. Without either, omp and Codex
// create a session at start and opencode at the first message. Like the real
// TUIs, a session started in the terminal is only saved, and so listed, once
// it has a message. SIGHUP, SIGINT and SIGTERM release the lease and exit 0.
//
// The Codex TUI reports only OSC 7501 root records with app=codex: working
// while it starts (--startup-delay), then idle, and working/idle around
// turns; no Warp events and no child id. Session IDs are UUIDv7 strings. It
// holds $CODEX_HOME/thread-writer-locks/<id>.lock (CODEX_HOME defaults to
// ~/.codex) open and flock-ed for every thread it opened, for the life of
// the process: "/new" keeps the previous thread's lock. "resume ID" (after
// any global flags) opens a fresh blank thread's lock and then ID's. A
// further positional argument is a prompt submitted at start.
//
// The Claude TUI queries OSC 7501, waits for the terminal's answer, then
// reports idle/app=claude-code without a startup working state. Turns report
// working/done, one Esc interrupts to idle, and exit clears the root record.
// It writes <CLAUDE_CONFIG_DIR>/sessions/<pid>.json before the first prompt,
// updates it for /new, retains the requested ID for --resume and deletes it on
// exit. Multiple Claude TUIs may register the same ID for occupancy tests.
//
// The Kimi TUI reports only app=kimi-code root records and no session ID. Its
// IDs have a session_ prefix, and its ACP side retains model and thinking on
// load/resume while resetting only mode. Image and embedded-resource prompts
// are advertised, steering is not. Its trust environment is logged to
// events.log.
//
// ACP mode implements initialize, session/new|load|resume|list|close|
// prompt|cancel|set_config_option, $/cancel_request and, for omp, Codex and
// Claude, _session/steering. A turn sends a thought, a tool call (with a
// permission request when enabled), optionally a form elicitation, a
// two-chunk agent message and a usage update. Prompt text containing
// /plan, /notice, /compact, /title, /untitle, /unknown or /fs adds a plan,
// notice, compaction, session_info_update setting or clearing the title,
// an unknown update kind, or an fs/read_text_file request to the client.
//
// The Codex ACP side mimics codex-acp: agentInfo
// @agentclientprotocol/codex-acp "Codex", options mode (read-only,
// workspace-write, agent, agent-full-access), model (gpt-a, gpt-b) and
// reasoning_effort (low, medium, high), all back at their defaults after
// session/load and session/resume, and commands review and logout. Steering
// with _meta.steering.idleBehavior "promptRequired" and no running turn
// answers {outcome: promptRequired, reason: noRunningTurn}; without it an
// idle steer starts a turn ({outcome: startedNewTurn}); during a turn it
// answers {outcome: injected}. Loading or resuming a thread whose lock
// another live process holds fails with -32600 and data.reason
// "thread_active_writer". At start it logs "env CODEX_PATH=<v>
// NO_BROWSER=<v>" (each only when set, "-" when neither is) to events.log.
//
// The Claude ACP side mimics claude-agent-acp 0.99.1: ACP v1 with list, load,
// resume, image and embedded-resource prompts, steering and model/effort/mode
// options. Cold load/resume resets model and effort but restores mode. With
// FAKEAGENT_MODE_FALLBACK=1, requesting auto publishes a notice and actually
// selects acceptEdits. Authentication commands are absent. At start it logs
// "env CLAUDE_CODE_EXECUTABLE=<v>" ("-" when unset) to events.log.
//
// Flags (environment variable in brackets):
//
//	--store DIR            session store directory, required [FAKEAGENT_STORE]
//	--profile omp|opencode default omp [FAKEAGENT_PROFILE, which also takes codex];
//	                       under the Codex profile --profile is Codex's own flag
//	--turn-delay DUR       pause between streamed pieces, default 20ms [FAKEAGENT_TURN_DELAY]
//	--startup-delay DUR    Codex TUI: how long it reports working at start, default 0 [FAKEAGENT_STARTUP_DELAY]
//	--delay M=DUR,...      delay ACP method M before answering [FAKEAGENT_DELAY]
//	--fail M=CODE,...      answer ACP method M with JSON-RPC error CODE [FAKEAGENT_FAIL]
//	--crash POINT          exit 3 at POINT: an ACP method name (before its
//	                       answer), "prompt-mid" (after the first chunk) or
//	                       "tui-turn" (during a TUI turn) [FAKEAGENT_CRASH]
//	--permission           ask permission for the turn's tool call [FAKEAGENT_PERMISSION=1]
//	--elicit               ask a form elicitation each turn [FAKEAGENT_ELICIT=1]
//	--ignore-interrupt     TUI mode: the interrupt key does not stop a turn [FAKEAGENT_IGNORE_INTERRUPT=1]
//	--no-resume            ACP mode: do not declare session/resume [FAKEAGENT_NO_RESUME=1]
//	--prompt-image         initialize advertises promptCapabilities.image [FAKEAGENT_PROMPT_IMAGE=1]
//	--embedded-context     initialize advertises promptCapabilities.embeddedContext [FAKEAGENT_EMBEDDED_CONTEXT=1]
//	--protocol-version N   initialize answer, default 1 [FAKEAGENT_PROTOCOL_VERSION]
//	--stdout-noise TEXT    ACP mode: print TEXT as a plain stdout line first [FAKEAGENT_STDOUT_NOISE]
//	(environment only)     Codex --version output, default 0.166.0 [FAKEAGENT_CODEX_VERSION]
//	(environment only)     Claude --version output, default 2.1.316 [FAKEAGENT_CLAUDE_VERSION]
//	(environment only)     codex-acp agentInfo.version, default 2.5.0 [FAKEAGENT_CONNECTOR_VERSION]
//	                       (claude-agent-acp default 0.99.1)
//	(environment only)     fall back from auto to acceptEdits [FAKEAGENT_MODE_FALLBACK=1]
//
// For omp and opencode, agent flags such as --approval-mode, --yolo,
// --auto-approve, --auto, --config, --model, --thinking, --standalone,
// --login and -c are accepted; --yolo, --auto-approve, --auto and
// --approval-mode yolo suppress permission requests. Other positional
// arguments are ignored. Like omp, the omp profile's session/load answers
// the "mode" option at its default. The Codex profile accepts --no-daemon,
// -m/--model, -s/--sandbox, -a/--ask-for-approval, -c/--config,
// -p/--profile, --search, --dangerously-bypass-approvals-and-sandbox,
// --approve-for-me, --enable, --disable and --add-dir, without effect.
//
// The store's events.log records "<unixnano> <pid> <mode> <event> <id>" for
// open, close, fork and violation (a session opened while another live
// process held it, which also forks it like omp), thread_active_writer (a
// Codex load or resume refused) and "call <method>" for each ACP request.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type config struct {
	store            string
	profile          string
	resume           string
	session          string
	settings         string
	prompt           string // Codex TUI: submitted at start
	version          bool   // Codex/Claude: print the version and exit
	codexVersion     string
	claudeVersion    string
	connectorVersion string
	turnDelay        time.Duration
	startupDelay     time.Duration
	delays           map[string]time.Duration
	fails            map[string]int
	crash            string
	permission       bool
	elicit           bool
	ignoreInterrupt  bool
	noResume         bool
	promptImage      bool
	embeddedContext  bool
	modeFallback     bool
	protocolVersion  int
	noise            string
}

const (
	connectorName       = "codex-acp"
	claudeConnectorName = "claude-agent-acp"
)

func main() {
	if filepath.Base(os.Args[0]) == "node" {
		runNodeLauncher()
		return
	}
	cfg, acpMode, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		os.Exit(2)
	}
	if cfg.version {
		if cfg.profile == "claudecode" {
			fmt.Println(cfg.claudeVersion + " (Claude Code)")
		} else {
			fmt.Println("codex-cli " + cfg.codexVersion)
		}
		return
	}
	mode := "tui"
	if acpMode {
		mode = "acp"
	}
	st, err := openStore(cfg.store, mode, cfg.profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		os.Exit(1)
	}
	if acpMode {
		err = runACP(cfg, st, os.Stdin, os.Stdout)
	} else {
		err = runTUI(cfg, st, os.Stdin, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		os.Exit(1)
	}
}

// runNodeLauncher mimics an npm native-binary launcher. The script named in
// os.Args[1] is only a marker; the isolated test points at its native fake
// with FAKEAGENT_NATIVE so process identity covers a node root and Claude child.
func runNodeLauncher() {
	native := os.Getenv("FAKEAGENT_NATIVE")
	if native == "" {
		fmt.Fprintln(os.Stderr, "fakeagent: FAKEAGENT_NATIVE is required for node")
		os.Exit(2)
	}
	cmd := exec.Command(native, os.Args[2:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "fakeagent: node launcher:", err)
		os.Exit(1)
	}
}

// profileFromName lets the binary stand in for every built-in agent and the
// third-party ACP connectors by executable name; unknown names mimic omp.
func profileFromName(argv0 string) string {
	switch filepath.Base(argv0) {
	case "opencode":
		return "opencode"
	case "codex", connectorName:
		return "codex"
	case "claude", claudeConnectorName:
		return "claudecode"
	case "agy":
		return "agy"
	case "qoder", "qodercli":
		return "qoder"
	case "qodercn", "qoderclicn", "qoder-cn":
		return "qodercn"
	case "kimi":
		return "kimi"
	}
	return "omp"
}

func parseArgs(args []string) (config, bool, error) {
	cfg := config{
		store:            os.Getenv("FAKEAGENT_STORE"),
		profile:          envOr("FAKEAGENT_PROFILE", profileFromName(os.Args[0])),
		crash:            os.Getenv("FAKEAGENT_CRASH"),
		permission:       os.Getenv("FAKEAGENT_PERMISSION") == "1",
		elicit:           os.Getenv("FAKEAGENT_ELICIT") == "1",
		ignoreInterrupt:  os.Getenv("FAKEAGENT_IGNORE_INTERRUPT") == "1",
		noResume:         os.Getenv("FAKEAGENT_NO_RESUME") == "1",
		promptImage:      os.Getenv("FAKEAGENT_PROMPT_IMAGE") == "1",
		embeddedContext:  os.Getenv("FAKEAGENT_EMBEDDED_CONTEXT") == "1",
		modeFallback:     os.Getenv("FAKEAGENT_MODE_FALLBACK") == "1",
		noise:            os.Getenv("FAKEAGENT_STDOUT_NOISE"),
		codexVersion:     envOr("FAKEAGENT_CODEX_VERSION", "0.166.0"),
		claudeVersion:    envOr("FAKEAGENT_CLAUDE_VERSION", "2.1.316"),
		connectorVersion: envOr("FAKEAGENT_CONNECTOR_VERSION", "2.5.0"),
		turnDelay:        20 * time.Millisecond,
		protocolVersion:  1,
	}
	var err error
	for _, d := range []struct {
		env string
		to  *time.Duration
	}{{"FAKEAGENT_TURN_DELAY", &cfg.turnDelay}, {"FAKEAGENT_STARTUP_DELAY", &cfg.startupDelay}} {
		if v := os.Getenv(d.env); v != "" {
			if *d.to, err = time.ParseDuration(v); err != nil {
				return cfg, false, fmt.Errorf("%s: %w", d.env, err)
			}
		}
	}
	if v := os.Getenv("FAKEAGENT_PROTOCOL_VERSION"); v != "" {
		if cfg.protocolVersion, err = strconv.Atoi(v); err != nil {
			return cfg, false, fmt.Errorf("FAKEAGENT_PROTOCOL_VERSION: %w", err)
		}
	}
	delays, fails := os.Getenv("FAKEAGENT_DELAY"), os.Getenv("FAKEAGENT_FAIL")
	var approval string
	var yolo, autoApprove, auto bool

	fs := flag.NewFlagSet("fakeagent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.store, "store", cfg.store, "")
	fs.DurationVar(&cfg.turnDelay, "turn-delay", cfg.turnDelay, "")
	fs.DurationVar(&cfg.startupDelay, "startup-delay", cfg.startupDelay, "")
	fs.StringVar(&delays, "delay", delays, "")
	fs.StringVar(&fails, "fail", fails, "")
	fs.StringVar(&cfg.crash, "crash", cfg.crash, "")
	fs.BoolVar(&cfg.permission, "permission", cfg.permission, "")
	fs.BoolVar(&cfg.elicit, "elicit", cfg.elicit, "")
	fs.BoolVar(&cfg.ignoreInterrupt, "ignore-interrupt", cfg.ignoreInterrupt, "")
	fs.BoolVar(&cfg.noResume, "no-resume", cfg.noResume, "")
	fs.BoolVar(&cfg.promptImage, "prompt-image", cfg.promptImage, "")
	fs.BoolVar(&cfg.embeddedContext, "embedded-context", cfg.embeddedContext, "")
	fs.IntVar(&cfg.protocolVersion, "protocol-version", cfg.protocolVersion, "")
	fs.StringVar(&cfg.noise, "stdout-noise", cfg.noise, "")
	codex := cfg.profile == "codex"
	claude := cfg.profile == "claudecode"
	if codex {
		// Codex CLI flags the hybrid profile keeps or passes through.
		for _, name := range []string{"no-daemon", "search", "dangerously-bypass-approvals-and-sandbox", "approve-for-me"} {
			fs.Bool(name, false, "")
		}
		for _, name := range []string{"m", "model", "s", "sandbox", "a", "ask-for-approval", "c", "config", "p", "profile", "enable", "disable", "add-dir"} {
			fs.String(name, "", "")
		}
		fs.BoolVar(&cfg.version, "version", false, "")
		fs.BoolVar(&cfg.version, "V", false, "")
	} else if claude {
		fs.StringVar(&cfg.resume, "resume", "", "")
		fs.StringVar(&cfg.resume, "r", "", "")
		fs.BoolVar(&cfg.version, "version", false, "")
		fs.BoolVar(&cfg.version, "v", false, "")
		for _, name := range []string{
			"model", "effort", "fallback-model", "permission-mode", "settings",
			"setting-sources", "system-prompt", "system-prompt-file",
			"append-system-prompt", "append-system-prompt-file", "agents", "agent",
			"plugin-dir", "add-dir", "mcp-config", "allowedTools", "allowed-tools",
			"disallowedTools", "disallowed-tools", "tools", "betas",
		} {
			fs.String(name, "", "")
		}
		for _, name := range []string{"dangerously-skip-permissions", "strict-mcp-config", "bare"} {
			fs.Bool(name, false, "")
		}
	} else {
		fs.StringVar(&cfg.profile, "profile", cfg.profile, "")
		fs.StringVar(&cfg.resume, "resume", "", "")
		fs.StringVar(&cfg.resume, "r", "", "")
		fs.StringVar(&cfg.resume, "conversation", "", "")
		fs.StringVar(&cfg.session, "s", "", "")
		fs.StringVar(&cfg.session, "S", "", "")
		fs.StringVar(&cfg.session, "session", "", "")
		fs.StringVar(&cfg.settings, "settings", "", "")
		// Real agent flags the hybrid profiles keep or pass through.
		fs.StringVar(&approval, "approval-mode", "", "")
		fs.BoolVar(&yolo, "yolo", false, "")
		fs.BoolVar(&yolo, "dangerously-skip-permissions", false, "")
		fs.BoolVar(&autoApprove, "auto-approve", false, "")
		fs.BoolVar(&auto, "auto", false, "")
		fs.Bool("plan", false, "")
		fs.Bool("standalone", false, "")
		fs.Bool("login", false, "")
		fs.Bool("c", false, "")
		fs.Bool("continue", false, "")
		fs.Bool("sandbox", false, "")
		fs.Bool("remote-control", false, "")
		fs.String("config", "", "")
		fs.String("m", "", "")
		fs.String("model", "", "")
		fs.String("thinking", "", "")
		fs.String("effort", "", "")
		fs.String("mode", "", "")
		fs.String("agent", "", "")
		fs.String("agent-file", "", "")
		fs.String("skills-dir", "", "")
		fs.String("add-dir", "", "")
		fs.String("log-file", "", "")
		fs.StringVar(&cfg.prompt, "prompt", "", "")
		fs.StringVar(&cfg.prompt, "i", "", "")
		fs.StringVar(&cfg.prompt, "prompt-interactive", "", "")
	}

	// Third-party connectors take no arguments: their names select ACP mode.
	acpMode := filepath.Base(os.Args[0]) == connectorName || filepath.Base(os.Args[0]) == claudeConnectorName
	var acpFlag bool
	if !codex && !claude {
		fs.BoolVar(&acpFlag, "acp", false, "")
	}
	positional := 0
	resumeNext := false
	optionsDone := false
	for {
		if !optionsDone {
			for i, a := range args {
				if a == "--" {
					optionsDone = true
					if err := fs.Parse(args[:i]); err != nil {
						return cfg, false, err
					}
					args = append(fs.Args(), args[i+1:]...)
					break
				}
				if !strings.HasPrefix(a, "-") || a == "-" {
					break
				}
			}
			if !optionsDone {
				if err := fs.Parse(args); err != nil {
					return cfg, false, err
				}
				args = fs.Args()
			}
		}
		if len(args) == 0 {
			break
		}
		arg := args[0]
		positional++
		switch {
		case !optionsDone && arg == "acp":
			acpMode = true
		case codex && resumeNext:
			cfg.resume, resumeNext = arg, false
		case codex && !optionsDone && positional == 1 && arg == "resume":
			resumeNext = true
		case (codex || claude || cfg.profile == "omp") && cfg.prompt == "":
			cfg.prompt = arg
		}
		args = args[1:]
	}
	if acpFlag {
		acpMode = true
	}
	if cfg.version {
		return cfg, acpMode, nil
	}
	if resumeNext {
		return cfg, false, fmt.Errorf("resume needs a session id")
	}
	if cfg.store == "" {
		return cfg, false, fmt.Errorf("--store or FAKEAGENT_STORE is required")
	}
	if cfg.profile != "omp" && cfg.profile != "opencode" && cfg.profile != "codex" && cfg.profile != "claudecode" && cfg.profile != "agy" && cfg.profile != "qoder" && cfg.profile != "qodercn" && cfg.profile != "kimi" {
		return cfg, false, fmt.Errorf("unknown profile %q", cfg.profile)
	}
	if cfg.profile == "claudecode" && os.Getenv("FAKEAGENT_CONNECTOR_VERSION") == "" {
		cfg.connectorVersion = "0.99.1"
	}
	if yolo || autoApprove || auto || approval == "yolo" {
		cfg.permission = false
	}
	if cfg.delays, err = parseMap(delays, time.ParseDuration); err != nil {
		return cfg, false, fmt.Errorf("delay: %w", err)
	}
	if cfg.fails, err = parseMap(fails, strconv.Atoi); err != nil {
		return cfg, false, fmt.Errorf("fail: %w", err)
	}
	return cfg, acpMode, nil
}

func parseMap[T any](spec string, parse func(string) (T, error)) (map[string]T, error) {
	out := map[string]T{}
	for _, item := range strings.Split(spec, ",") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("%q is not METHOD=VALUE", item)
		}
		parsed, err := parse(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[strings.TrimSpace(k)] = parsed
	}
	return out, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func crashAt(cfg config, point string) {
	if cfg.crash == point {
		os.Exit(3)
	}
}
