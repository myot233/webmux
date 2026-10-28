// Package sessioncmd implements the session subcommands shared by the Linux
// CLI and the Windows console client: run, attach, ls and open. Each client
// supplies its local API client, console and working-directory rules.
package sessioncmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/rainchan/webmux/internal/attach"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// Console is a raw terminal that can be restored.
type Console interface {
	attach.Terminal
	Restore() error
}

// Env is what a client provides to the commands.
type Env struct {
	// Flags registers client-specific flags (for example --config) on every
	// subcommand before parsing; Connect runs after parsing.
	Flags   func(*flag.FlagSet)
	Connect func() (*localclient.Client, error)
	Stdout  io.Writer
	Stderr  io.Writer
	// OpenConsole enters raw mode; it returns attach.ErrNotTerminal when
	// standard input or output is not a terminal.
	OpenConsole func() (Console, error)
	// OpenBrowser opens a URL in the default browser.
	OpenBrowser func(url string) error
	// ResolveCwd maps --cwd (or "" for the current directory) to a path
	// on the server.
	ResolveCwd func(ctx context.Context, cwd string) (string, error)
	// Environ returns the caller's environment, which `run` forwards to a
	// custom-command session. Nil forwards nothing: the Windows client must
	// not copy Windows variables into WSL.
	Environ func() []string
	// NotRunningHint tells how to start the server.
	NotRunningHint string
	// Label names this client in the browser's control indicator.
	Label string
}

// Fail prints err and returns its exit code: 3 when the server is not
// running, 1 otherwise.
func (e Env) Fail(err error) int {
	if errors.Is(err, localclient.ErrNotRunning) {
		_, _ = fmt.Fprintln(e.Stderr, "webmux: WebMux is not running;", e.NotRunningHint)
		return contract.ExitNotRunning
	}
	_, _ = fmt.Fprintln(e.Stderr, "webmux:", err)
	return contract.ExitError
}

func (e Env) usagef(flags *flag.FlagSet, format string, args ...any) int {
	_, _ = fmt.Fprintf(e.Stderr, "webmux %s: %s\n", flags.Name(), fmt.Sprintf(format, args...))
	flags.Usage()
	return contract.ExitUsage
}

func (e Env) flags(name, synopsis string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(e.Stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintf(e.Stderr, "usage: webmux %s %s\n", name, synopsis)
		flags.PrintDefaults()
	}
	if e.Flags != nil {
		e.Flags(flags)
	}
	return flags
}

func (e Env) connect() (*localclient.Client, int) {
	client, err := e.Connect()
	if err != nil {
		return nil, e.Fail(err)
	}
	return client, contract.ExitOK
}

// envFlag collects repeated --env NAME=VALUE.
type envFlag map[string]string

func (f envFlag) String() string { return "" }
func (f envFlag) Set(value string) error {
	name, v, ok := strings.Cut(value, "=")
	if !ok || name == "" {
		return fmt.Errorf("--env expects NAME=VALUE, got %q", value)
	}
	f[name] = v
	return nil
}

// Run implements `run [--launch-config NAME] [--name] [--cwd] [--env K=V]... [--detach] -- argv...`.
//
// A custom command inherits the caller's environment (Env.Environ) with
// --env on top, so it runs as it would in the calling shell. A launch config
// is named as in the Web UI (names are unique) and resolved to its ID through
// the summary list, which carries environment names only. The server then
// expands the ID, so the command and environment stay identical to the saved
// copy and secret values never reach the client.
func Run(ctx context.Context, e Env, args []string) int {
	flags := e.flags("run", "[--launch-config NAME] [--name NAME] [--cwd DIR] [--env NAME=VALUE]... [--detach] -- COMMAND [ARG]...")
	var launchConfig string
	var launchConfigSet bool
	setLaunchConfig := func(value string) error {
		if launchConfigSet && launchConfig != value {
			return errors.New("--launch-config and --config-id must name the same saved config")
		}
		launchConfig = value
		launchConfigSet = true
		return nil
	}
	flags.Func("launch-config", "use a saved Web UI launch config by name (instead of -- COMMAND)", setLaunchConfig)
	flags.Func("config-id", "alias for --launch-config", setLaunchConfig)
	name := flags.String("name", "", "session name")
	cwd := flags.String("cwd", "", "working directory on the server (default: the current directory)")
	detach := flags.Bool("detach", false, "create the session and print its ID and link without attaching")
	env := envFlag{}
	flags.Var(env, "env", "set NAME=VALUE in the session (repeatable), overriding the inherited environment")
	if err := flags.Parse(args); err != nil {
		return contract.ExitUsage
	}
	argv := flags.Args()
	if launchConfigSet && launchConfig == "" {
		return e.usagef(flags, "--launch-config requires a non-empty saved config name")
	}
	if launchConfigSet && len(argv) > 0 {
		return e.usagef(flags, "--launch-config cannot be combined with a command after --")
	}
	if launchConfigSet && len(env) > 0 {
		return e.usagef(flags, "--launch-config cannot be combined with --env")
	}
	if !launchConfigSet && (len(argv) == 0 || argv[0] == "") {
		return e.usagef(flags, "a command is required after --")
	}
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	dir, err := e.ResolveCwd(ctx, *cwd)
	if err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "webmux run:", err)
		return contract.ExitUsage
	}
	request := contract.CreateSessionRequest{Cwd: dir, Name: *name}
	if launchConfigSet {
		id, err := launchConfigID(ctx, client, launchConfig)
		if err != nil {
			if errors.Is(err, errUnknownLaunchConfig) {
				_, _ = fmt.Fprintln(e.Stderr, "webmux run:", err)
				return contract.ExitUsage
			}
			return e.Fail(err)
		}
		request.ConfigID = &id
	} else {
		request.Cmd = argv
		request.Env = e.sessionEnv(env)
	}
	// An attached run needs a terminal: check before creating anything.
	var console Console
	if !*detach {
		if console, code = e.openConsole(); console == nil {
			return code
		}
	}
	var created contract.SessionInfo
	if err := client.Do(ctx, http.MethodPost, "/api/sessions", request, &created); err != nil {
		if console != nil {
			_ = console.Restore()
		}
		return e.Fail(err)
	}
	if *detach {
		link, err := link(ctx, client, created.ID)
		if err != nil {
			return e.Fail(err)
		}
		_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\n", created.ID, link)
		return contract.ExitOK
	}
	// Tell the user where to find the session (for example on a phone)
	// before the session's output starts; stdout stays the session's. The
	// terminal is already raw, so the line ends with CR LF.
	if link, err := link(ctx, client, created.ID); err == nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s · %s · Ctrl+] to detach\r\n", created.ID, link)
	} else {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s · Ctrl+] to detach\r\n", created.ID)
	}
	return e.attach(ctx, client, created.ID, false, console)
}

var errUnknownLaunchConfig = errors.New("no saved launch config named")

// launchConfigID maps a launch config name to its ID.
func launchConfigID(ctx context.Context, client *localclient.Client, name string) (string, error) {
	var list []contract.LaunchConfigSummary
	if err := client.Do(ctx, http.MethodGet, "/api/launch-configs", nil, &list); err != nil {
		return "", err
	}
	names := make([]string, 0, len(list))
	for _, c := range list {
		if c.Name == name {
			return c.ID, nil
		}
		names = append(names, strconv.Quote(c.Name))
	}
	if len(names) == 0 {
		return "", fmt.Errorf("%w %q (no saved launch configs)", errUnknownLaunchConfig, name)
	}
	return "", fmt.Errorf("%w %q (saved: %s)", errUnknownLaunchConfig, name, strings.Join(names, ", "))
}

// Variables that describe the calling process rather than the session: the
// session's working directory is --cwd, not the caller's.
var callerOnlyEnv = map[string]bool{"PWD": true, "OLDPWD": true, "_": true}

// sessionEnv is the caller's environment, minus variables the server owns,
// caller-only variables and names a create request rejects, with explicit
// --env values on top. Nil means nothing to send.
func (e Env) sessionEnv(explicit envFlag) map[string]string {
	var inherited []string
	if e.Environ != nil {
		inherited = e.Environ()
	}
	env := make(map[string]string, len(inherited)+len(explicit))
	for _, entry := range inherited {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || callerOnlyEnv[name] || contract.ServerOwnedEnv(name) || !contract.ValidEnvName(name) || strings.ContainsRune(value, 0) {
			continue
		}
		env[name] = value
	}
	maps.Copy(env, explicit)
	if len(env) == 0 {
		return nil
	}
	return env
}

// Attach implements `attach <id> [--observe]`.
func Attach(ctx context.Context, e Env, args []string) int {
	flags := e.flags("attach", "[--observe] SESSION")
	observe := flags.Bool("observe", false, "watch without taking control, sending input or resizing")
	if err := flags.Parse(args); err != nil {
		return contract.ExitUsage
	}
	if flags.NArg() != 1 {
		return e.usagef(flags, "exactly one session ID is required")
	}
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	console, code := e.openConsole()
	if console == nil {
		return code
	}
	return e.attach(ctx, client, flags.Arg(0), *observe, console)
}

// openConsole enters raw mode; a missing terminal is a usage error.
func (e Env) openConsole() (Console, int) {
	console, err := e.OpenConsole()
	if errors.Is(err, attach.ErrNotTerminal) {
		_, _ = fmt.Fprintln(e.Stderr, "webmux:", err)
		return nil, contract.ExitUsage
	}
	if err != nil {
		return nil, e.Fail(err)
	}
	return console, contract.ExitOK
}

// attach runs until detach or exit and restores console before returning.
func (e Env) attach(ctx context.Context, client *localclient.Client, id string, observe bool, console Console) int {
	result, err := attach.Run(ctx, client, id, console, attach.Options{Observe: observe, Label: e.Label})
	_ = console.Restore()
	switch {
	case errors.Is(err, attach.ErrMoved):
		_, _ = fmt.Fprintln(e.Stderr, "webmux:", err)
		return contract.ExitError
	case err != nil:
		return e.Fail(err)
	case result.Detached:
		_, _ = fmt.Fprintf(e.Stderr, "\r\n[detached from %s; it keeps running]\n", id)
		return contract.ExitOK
	case result.Moved:
		// Passive detach: the session lives on in the chat view.
		_, _ = fmt.Fprintf(e.Stderr, "\r\n[已切换到聊天视图，可在 Web 端继续；切回终端后用 `webmux attach %s` 重新接入]\n", id)
		return contract.ExitOK
	}
	return result.ExitCode
}

// List implements `ls [--json]`.
func List(ctx context.Context, e Env, args []string) int {
	flags := e.flags("ls", "[--json]")
	asJSON := flags.Bool("json", false, "print the server's session list as JSON")
	if err := flags.Parse(args); err != nil {
		return contract.ExitUsage
	}
	if flags.NArg() > 0 {
		return e.usagef(flags, "unexpected argument %q", flags.Arg(0))
	}
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	var raw json.RawMessage
	if err := client.Do(ctx, http.MethodGet, "/api/sessions", nil, &raw); err != nil {
		return e.Fail(err)
	}
	if *asJSON {
		_, _ = fmt.Fprintf(e.Stdout, "%s\n", raw)
		return contract.ExitOK
	}
	var list []contract.SessionInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return e.Fail(err)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCOMMAND\tCWD")
	for _, s := range list {
		status := string(s.Status)
		if s.ExitCode != nil {
			status = fmt.Sprintf("%s (%d)", s.Status, *s.ExitCode)
		}
		if s.Hybrid != nil {
			status += " · " + hybridSide(s.Hybrid)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Title, status, strings.Join(s.Cmd, " "), s.Cwd)
	}
	_ = w.Flush()
	return contract.ExitOK
}

// hybridSide names where a hybrid session runs, for `ls`.
func hybridSide(h *contract.SessionHybrid) string {
	switch {
	case h.Side == contract.HybridSideSwitching || h.Phase != "":
		return "切换中"
	case h.Side == contract.HybridSideACP && h.Dormant:
		return "聊天·休眠"
	case h.Side == contract.HybridSideACP:
		return "聊天"
	default:
		return "终端"
	}
}

// Open implements `open <id> [--browser]`.
func Open(ctx context.Context, e Env, args []string) int {
	flags := e.flags("open", "[--browser] SESSION")
	browser := flags.Bool("browser", false, "also open the link in the default browser")
	if err := flags.Parse(args); err != nil {
		return contract.ExitUsage
	}
	if flags.NArg() != 1 {
		return e.usagef(flags, "exactly one session ID is required")
	}
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	id := flags.Arg(0)
	if err := client.Do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(id), nil, nil); err != nil {
		return e.Fail(err)
	}
	link, err := link(ctx, client, id)
	if err != nil {
		return e.Fail(err)
	}
	_, _ = fmt.Fprintln(e.Stdout, link)
	if *browser {
		if err := e.OpenBrowser(link); err != nil {
			return e.Fail(fmt.Errorf("open browser: %w", err))
		}
	}
	return contract.ExitOK
}

// link is the session's token-free local link.
func link(ctx context.Context, client *localclient.Client, id string) (string, error) {
	status, err := client.Status(ctx)
	if err != nil {
		return "", err
	}
	if status.URLs.Local == "" {
		return "", errors.New("the server reported no local link")
	}
	return strings.TrimSuffix(status.URLs.Local, "/") + "/s/" + url.PathEscape(id), nil
}
