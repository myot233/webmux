// Command fakeagent is a scriptable stand-in for omp and OpenCode in tests
// and E2E. Build it with
//
//	go build -o <dir>/fakeagent ./internal/acp/testdata/fakeagent
//
// Usage:
//
//	fakeagent [flags]       TUI mode
//	fakeagent [flags] acp   ACP agent on stdio
//
// Both modes share one session store (--store or FAKEAGENT_STORE), so a
// session written in one mode can be resumed or loaded in the other.
//
// TUI mode prints a transcript and reads keystrokes (raw mode when stdin is
// a terminal). Each session it opens is announced as a Warp cli-agent
// session_start event (agent, plugin_version, session_id, cwd) and OSC 7501
// root (state, app) and child (id) records; turns report working/idle.
// Enter submits a line; "/new" starts a new session, "/exit" quits. The
// interrupt key stops a turn: Esc for the omp profile, Esc Esc for
// opencode. --resume ID (alias -r) reopens an existing session; -s ID
// (alias --session) reopens or creates it. Without either, the omp profile
// creates a session at start and opencode at the first message. SIGHUP,
// SIGINT and SIGTERM release the lease and exit 0.
//
// ACP mode implements initialize, session/new|load|resume|list|close|
// prompt|cancel|set_config_option, $/cancel_request and, for the omp
// profile, _session/steering. A turn sends a thought, a tool call (with a
// permission request when enabled), optionally a form elicitation, a
// two-chunk agent message and a usage update. Prompt text containing
// /plan, /notice, /compact, /title, /unknown or /fs adds a plan, notice,
// compaction, session_info_update, an unknown update kind, or an
// fs/read_text_file request to the client.
//
// Flags (environment variable in brackets):
//
//	--store DIR            session store directory, required [FAKEAGENT_STORE]
//	--profile omp|opencode default omp [FAKEAGENT_PROFILE]
//	--turn-delay DUR       pause between streamed pieces, default 20ms [FAKEAGENT_TURN_DELAY]
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
//
// Agent flags such as --approval-mode, --yolo, --auto-approve, --auto,
// --config, --model, --thinking, --standalone, --login and -c are accepted;
// --yolo, --auto-approve, --auto and --approval-mode yolo suppress
// permission requests. Other positional arguments are ignored. Like omp,
// the omp profile's session/load answers the "mode" option at its default.
//
// The store's events.log records "<unixnano> <pid> <mode> <event> <id>" for
// open, close, fork and violation (a session opened while another live
// process held it, which also forks it like omp), and "call <method>" for
// each ACP request.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type config struct {
	store           string
	profile         string
	resume          string
	session         string
	turnDelay       time.Duration
	delays          map[string]time.Duration
	fails           map[string]int
	crash           string
	permission      bool
	elicit          bool
	ignoreInterrupt bool
	noResume        bool
	promptImage     bool
	embeddedContext bool
	protocolVersion int
	noise           string
}

func main() {
	cfg, acpMode, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		os.Exit(2)
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

// profileFromName lets the binary stand in for a real agent by name: copied
// or linked as "opencode" it mimics OpenCode, otherwise omp.
func profileFromName(argv0 string) string {
	if filepath.Base(argv0) == "opencode" {
		return "opencode"
	}
	return "omp"
}

func parseArgs(args []string) (config, bool, error) {
	cfg := config{
		store:           os.Getenv("FAKEAGENT_STORE"),
		profile:         envOr("FAKEAGENT_PROFILE", profileFromName(os.Args[0])),
		crash:           os.Getenv("FAKEAGENT_CRASH"),
		permission:      os.Getenv("FAKEAGENT_PERMISSION") == "1",
		elicit:          os.Getenv("FAKEAGENT_ELICIT") == "1",
		ignoreInterrupt: os.Getenv("FAKEAGENT_IGNORE_INTERRUPT") == "1",
		noResume:        os.Getenv("FAKEAGENT_NO_RESUME") == "1",
		promptImage:     os.Getenv("FAKEAGENT_PROMPT_IMAGE") == "1",
		embeddedContext: os.Getenv("FAKEAGENT_EMBEDDED_CONTEXT") == "1",
		noise:           os.Getenv("FAKEAGENT_STDOUT_NOISE"),
		turnDelay:       20 * time.Millisecond,
		protocolVersion: 1,
	}
	var err error
	if v := os.Getenv("FAKEAGENT_TURN_DELAY"); v != "" {
		if cfg.turnDelay, err = time.ParseDuration(v); err != nil {
			return cfg, false, fmt.Errorf("FAKEAGENT_TURN_DELAY: %w", err)
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
	fs.StringVar(&cfg.profile, "profile", cfg.profile, "")
	fs.StringVar(&cfg.resume, "resume", "", "")
	fs.StringVar(&cfg.resume, "r", "", "")
	fs.StringVar(&cfg.session, "s", "", "")
	fs.StringVar(&cfg.session, "session", "", "")
	fs.DurationVar(&cfg.turnDelay, "turn-delay", cfg.turnDelay, "")
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
	// Real agent flags the hybrid profiles keep or pass through.
	fs.StringVar(&approval, "approval-mode", "", "")
	fs.BoolVar(&yolo, "yolo", false, "")
	fs.BoolVar(&autoApprove, "auto-approve", false, "")
	fs.BoolVar(&auto, "auto", false, "")
	fs.Bool("standalone", false, "")
	fs.Bool("login", false, "")
	fs.Bool("c", false, "")
	fs.Bool("continue", false, "")
	fs.String("config", "", "")
	fs.String("model", "", "")
	fs.String("thinking", "", "")

	acpMode := false
	for {
		if err := fs.Parse(args); err != nil {
			return cfg, false, err
		}
		if fs.NArg() == 0 {
			break
		}
		if fs.Arg(0) == "acp" {
			acpMode = true
		}
		args = fs.Args()[1:]
	}
	if cfg.store == "" {
		return cfg, false, fmt.Errorf("--store or FAKEAGENT_STORE is required")
	}
	if cfg.profile != "omp" && cfg.profile != "opencode" {
		return cfg, false, fmt.Errorf("unknown profile %q", cfg.profile)
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
