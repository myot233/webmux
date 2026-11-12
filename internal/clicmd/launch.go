package clicmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/attach"
	"github.com/rainchan/webmux/internal/contract"
)

// labelFlag collects repeated --label KEY=VALUE.
type labelFlag map[string]string

func (f labelFlag) String() string { return "" }
func (f labelFlag) Set(value string) error {
	key, v, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return fmt.Errorf("--label expects KEY=VALUE, got %q", value)
	}
	f[key] = v
	return nil
}

// runOptions are the flags of webmux run.
type runOptions struct {
	launchConfig    string
	launchConfigSet bool
	name, cwd       string
	env             envFlag
	labels          labelFlag
	// Location.
	worktree, branch, base, worktreePath, repo      string
	newWorktree, existingBranch, noPin, refreshBase bool
	// View and first message.
	view, prompt, permission string
	init                     bool
	set                      labelFlag
	attachments              listFlag
	// How to follow the session.
	detach, attachMode, follow, strip, killOnInterrupt bool
	asJSON, dryRun, reply, autoWaitRequests            bool
	wait                                               string
	timeout                                            time.Duration
	// Initial terminal size; both or neither.
	cols, rows int
}

// runCommand creates a session from a saved launch config or an explicit
// command.
//
// A custom command inherits the caller's environment (Env.Environ) with
// --env on top, so it runs as it would in the calling shell. A launch config
// is named as in the Web UI (names are unique) and resolved to its ID through
// the summary list, which carries environment names only. The server then
// expands the ID, so the command and environment stay identical to the saved
// copy and secret values never reach the client.
var runCommand = &Command{
	Name: "run", Group: GroupLaunch, Args: "[-- COMMAND [ARG]...]", Strict: true, DashArgs: true,
	StrayHint: func(fs *flag.FlagSet) string {
		if f := fs.Lookup("new-worktree"); f != nil && f.Value.String() == "true" {
			return "--new-worktree takes no value; name the branch with --branch"
		}
		return "the command goes after --"
	},
	Summary: "Create a session in a directory or worktree, on its terminal or chat view; attach, detach, follow or wait.",
	Description: "Without --launch-config, the arguments after -- are the session's argv exactly as given; nothing is expanded by a shell. " +
		"A custom command inherits the caller's environment, minus terminal identity and WEBMUX_* variables, with --env on top. " +
		"With --launch-config, the server starts the saved Web UI launch config of that exact name, " +
		"including its environment, which never reaches the client.\n\n" +
		"Location matches the Web UI's new session dialog: the current directory (associated with the registered worktree it lies in), " +
		"an existing worktree (--worktree), or a new one (--new-worktree, on a branch named by --branch or generated), which is " +
		"pinned to the home page like the Web UI does. A launch config with the chat view can start on it (--view chat) with " +
		"agent settings, a permission mode and attachments; --prompt is the first message on either view. Everything is " +
		"created by one request; a failure names the step that failed and what was already created, and nothing is rolled back.\n\n" +
		"--cols and --rows (both) give the terminal its initial size; the first browser or attached terminal that controls the session " +
		"then resizes it to its own window (webmux resize changes it later).\n\n" +
		"Without a terminal (an agent's tool call, a pipe) run detaches instead of failing; --attach insists on attaching. " +
		"--follow streams the output to standard output without a terminal and exits with the program's exit status; " +
		"Ctrl+C only stops following unless --kill-on-interrupt. --reply follows the first prompt's turn like webmux send --reply.",
	Arguments: []Item{{Name: "COMMAND [ARG]...", Text: "The program and its arguments, after --."}},
	Env: []Item{
		{Name: "WEBMUX_SOCKET", Text: "Local API socket used when neither --socket nor --config is given."},
	},
	Output: "Detached: the session ID, or with --json an object {session, configWarnings} plus worktree when one was " +
		"created. Attached or --follow: the session's output; notices and config warnings go to standard error. " +
		"--reply: the agent's final reply. --dry-run prints the request with environment values hidden.",
	Exit: []ExitItem{
		{Code: 0, Text: "Created (detached), detached after attaching, --reply and the turn ended normally, or --follow and the program exited 0."},
		{Code: 1, Text: "A step failed (the worktree, the session, its settings or the first prompt), an upload failed, " +
			"following was lost, or with --reply the turn did not end with end_turn or waits for an answer."},
		{Code: 2, Text: "Usage error, unknown launch config or repository, a launch config without the chat view for --view chat, or --attach without a terminal."},
		{Code: 3, Text: "WebMux is not running."},
		{Code: 124, Text: "--wait or --reply timed out."},
		{Code: 130, Text: "--follow stopped by SIGINT; the session keeps running (143 for SIGTERM). Otherwise --follow exits with the program's own status, 128+N when signal N ended it."},
	},
	Examples: []Example{
		{Command: "webmux run -- claude", Text: "Start Claude Code in the current directory and attach; Ctrl+] detaches."},
		{Command: "webmux run --detach --cols 200 --rows 50 -- make test", Text: "Start a session whose terminal is 200 columns by 50 rows from its first output."},
		{Command: "webmux run --detach --name tests --cwd ~/src/app -- make test", Text: "Start a named session in another directory and print its ID."},
		{Command: "webmux run --follow --strip -- go test ./...", Text: "Run tests in a persistent session, stream plain text and exit with their status."},
		{Command: `webmux run --new-worktree --branch fix/lint --launch-config Codex --view chat --set model=gpt-5 --prompt "Fix all lint and summarize" --reply`, Text: "Start an agent on a new worktree's chat view and print its reply."},
		{Command: "webmux run --dry-run --label run=7 -- make build", Text: "Show the request run would send, without creating anything."},
	},
	SeeAlso: []string{"webmux attach", "webmux send", "webmux launch-config options", "webmux guide launch"},
	Notes: map[string]Note{
		"launch-config":      {Conflicts: []string{"env"}},
		"config-id":          {Conflicts: []string{"env"}},
		"env":                {Repeatable: true, Conflicts: []string{"launch-config"}},
		"label":              {Repeatable: true},
		"worktree":           {Conflicts: []string{"new-worktree"}},
		"new-worktree":       {Conflicts: []string{"worktree", "cwd"}},
		"branch":             {Requires: []string{"new-worktree"}},
		"base":               {Requires: []string{"new-worktree"}},
		"existing-branch":    {Requires: []string{"new-worktree", "branch"}},
		"worktree-path":      {Requires: []string{"new-worktree"}},
		"repo":               {Requires: []string{"new-worktree"}},
		"no-pin":             {Requires: []string{"new-worktree"}},
		"refresh-base":       {Requires: []string{"new-worktree"}},
		"view":               {Requires: []string{"launch-config"}},
		"init":               {Requires: []string{"launch-config"}},
		"set":                {Repeatable: true, Requires: []string{"view"}},
		"permission":         {Requires: []string{"view"}},
		"attach-file":        {Repeatable: true, Requires: []string{"view", "prompt"}},
		"reply":              {Requires: []string{"view", "prompt"}, Conflicts: []string{"wait", "follow"}},
		"auto-wait-requests": {Requires: []string{"reply"}},
		"follow":             {Conflicts: []string{"detach", "attach", "reply"}},
		"detach":             {Conflicts: []string{"attach", "follow"}},
		"attach":             {Conflicts: []string{"detach", "follow"}},
		"wait":               {Conflicts: []string{"attach", "follow", "reply"}},
		"cols":               {Requires: []string{"rows"}},
		"rows":               {Requires: []string{"cols"}},
	},
	Bind: func(fs *flag.FlagSet) Action {
		o := runOptions{env: envFlag{}, labels: labelFlag{}, set: labelFlag{}}
		setLaunchConfig := func(value string) error {
			if o.launchConfigSet && o.launchConfig != value {
				return errors.New("--launch-config and --config-id must name the same saved config")
			}
			o.launchConfig, o.launchConfigSet = value, true
			return nil
		}
		fs.Func("launch-config", "use a saved Web UI launch config by `NAME` (instead of -- COMMAND)", setLaunchConfig)
		fs.Func("config-id", "alias for --launch-config", setLaunchConfig)
		fs.StringVar(&o.name, "name", "", "session `NAME` (default: the first --prompt on the terminal view, else the terminal title)")
		fs.StringVar(&o.cwd, "cwd", "", "working `DIR` on the server (default: the current directory)")
		fs.Var(o.env, "env", "set `NAME=VALUE` in the session, overriding the inherited environment")
		fs.Var(o.labels, "label", "set label `KEY=VALUE` on the session")
		fs.StringVar(&o.worktree, "worktree", "", "start in a registered worktree, as `REPO:WORKTREE` or its path")
		fs.BoolVar(&o.newWorktree, "new-worktree", false, "create a worktree and start at its root")
		fs.StringVar(&o.branch, "branch", "", "the new worktree's branch `NAME` (default: generated, wt/<date>-<time>)")
		fs.StringVar(&o.base, "base", "", "start the new branch at `REF` (default: the repository's default base)")
		fs.BoolVar(&o.existingBranch, "existing-branch", false, "check out the existing --branch instead of creating it")
		fs.StringVar(&o.worktreePath, "worktree-path", "", "put the new worktree at `DIR` (default: <repository>.worktrees/<branch leaf>)")
		fs.StringVar(&o.repo, "repo", "", "repository of the new worktree: an ID, a path or . (default: the current directory's)")
		fs.BoolVar(&o.noPin, "no-pin", false, "do not show the new worktree on the home page")
		fs.BoolVar(&o.refreshBase, "refresh-base", false, "fetch the base branch's upstream first and start from it")
		fs.StringVar(&o.view, "view", "", "start on `VIEW`: terminal (the default) or chat, which needs a launch config with the chat view and implies --detach")
		fs.StringVar(&o.prompt, "prompt", "", "first message `TEXT` (- reads standard input): the first chat prompt, or typed with Enter on the terminal view")
		fs.BoolVar(&o.init, "init", false, "put the repository's initialization instruction before the first message")
		fs.Var(o.set, "set", "chat view: set agent option `ID=VALUE` (see webmux launch-config options)")
		fs.StringVar(&o.permission, "permission", "", "chat view: permission `MODE` ask or auto")
		fs.Var(&o.attachments, "attach-file", "chat view: upload the file at `PATH` and send it with the first prompt")
		fs.BoolVar(&o.detach, "detach", false, "create the session and print its ID without attaching")
		fs.BoolVar(&o.attachMode, "attach", false, "attach even when that needs a terminal this process lacks (exit 2)")
		fs.BoolVar(&o.follow, "follow", false, "stream the output to standard output and exit with the program's status")
		fs.BoolVar(&o.strip, "strip", false, "with --follow, remove terminal control sequences")
		fs.BoolVar(&o.killOnInterrupt, "kill-on-interrupt", false, "with --follow, end the program on SIGINT or SIGTERM")
		fs.BoolVar(&o.asJSON, "json", false, "print {session, ...} as JSON when detached, or the turn with --reply")
		fs.BoolVar(&o.dryRun, "dry-run", false, "print the request without creating anything; environment values are hidden")
		fs.StringVar(&o.wait, "wait", "", "detach, then wait until the session is first in `STATE` (see webmux wait)")
		fs.BoolVar(&o.reply, "reply", false, "chat view: follow the first prompt's turn and print the agent's final reply")
		fs.BoolVar(&o.autoWaitRequests, "auto-wait-requests", false, "with --reply, keep waiting while a permission request or question is open")
		fs.IntVar(&o.cols, "cols", 0, "initial terminal width in `N` columns (1-1000, with --rows; default 80)")
		fs.IntVar(&o.rows, "rows", 0, "initial terminal height in `N` rows (1-1000, with --cols; default 24)")
		fs.DurationVar(&o.timeout, "timeout", 0, "give up --wait or --reply after this `DURATION` (exit 124); 0 waits forever")
		return func(ctx context.Context, e Env, argv []string) int { return e.run(ctx, o, argv) }
	},
}

// Run implements `webmux run`.
func Run(ctx context.Context, e Env, args []string) int { return e.Run(ctx, runCommand, args) }

// validate checks the flag combinations that need no server.
func (o *runOptions) validate(argv []string) error {
	chat := o.view == string(contract.ViewChat)
	switch {
	case (o.cols != 0 || o.rows != 0) && (o.cols < 1 || o.rows < 1 || o.cols > contract.MaxTerminalDimension || o.rows > contract.MaxTerminalDimension):
		return fmt.Errorf("--cols and --rows must be given together, each 1-%d", contract.MaxTerminalDimension)
	case o.launchConfigSet && o.launchConfig == "":
		return errors.New("--launch-config requires a non-empty saved config name")
	case o.launchConfigSet && len(argv) > 0:
		return errors.New("--launch-config cannot be combined with a command after --")
	case o.launchConfigSet && len(o.env) > 0:
		return errors.New("--launch-config cannot be combined with --env")
	case !o.launchConfigSet && (len(argv) == 0 || argv[0] == ""):
		return errors.New("a command is required after --")
	case o.worktree != "" && o.newWorktree:
		return errors.New("--worktree and --new-worktree cannot be combined")
	case !o.newWorktree && (o.branch != "" || o.base != "" || o.existingBranch || o.worktreePath != "" || o.repo != "" || o.noPin || o.refreshBase):
		return errors.New("--branch, --base, --existing-branch, --worktree-path, --repo, --no-pin and --refresh-base need --new-worktree")
	case o.existingBranch && o.branch == "":
		return errors.New("--existing-branch needs --branch")
	case o.newWorktree && o.cwd != "":
		return errors.New("--cwd cannot be combined with --new-worktree: the session starts at the new worktree's root (pick the repository with --repo)")
	case o.view != "" && o.view != string(contract.ViewTerminal) && !chat:
		return fmt.Errorf("--view is terminal or chat, not %q", o.view)
	case (chat || o.init) && !o.launchConfigSet:
		return errors.New("--view chat and --init need --launch-config")
	case !chat && (len(o.set) > 0 || o.permission != "" || len(o.attachments) > 0 || o.reply):
		return errors.New("--set, --permission, --attach-file and --reply need --view chat")
	case o.permission != "" && o.permission != string(contract.ChatPermissionAsk) && o.permission != string(contract.ChatPermissionAuto):
		return fmt.Errorf("--permission is ask or auto, not %q", o.permission)
	case len(o.attachments) > 0 && o.prompt == "":
		return errors.New("--attach-file needs --prompt: attachments go with the first prompt")
	case o.reply && o.prompt == "":
		return errors.New("--reply needs --prompt: it prints the reply to the first prompt")
	case chat && o.attachMode:
		return errors.New("--view chat cannot be attached; it starts detached")
	case countTrue(o.detach, o.attachMode, o.follow) > 1:
		return errors.New("--detach, --attach and --follow cannot be combined")
	case o.follow && (chat || o.reply):
		return errors.New("--follow cannot be combined with --view chat or --reply")
	case (o.strip || o.killOnInterrupt) && !o.follow:
		return errors.New("--strip and --kill-on-interrupt need --follow")
	case o.wait != "" && o.reply:
		return errors.New("--wait and --reply cannot be combined")
	case o.wait != "" && (o.attachMode || o.follow):
		return errors.New("--wait works only when the session is detached")
	case o.autoWaitRequests && !o.reply:
		return errors.New("--auto-wait-requests needs --reply")
	case o.timeout != 0 && o.wait == "" && !o.reply:
		return errors.New("--timeout needs --wait or --reply")
	}
	if o.wait != "" {
		if _, err := parseStates(o.wait); err != nil {
			return err
		}
	}
	return nil
}

func countTrue(values ...bool) int {
	n := 0
	for _, v := range values {
		if v {
			n++
		}
	}
	return n
}

func (e Env) run(ctx context.Context, o runOptions, argv []string) int {
	if err := o.validate(argv); err != nil {
		return e.Usagef("%v", err)
	}
	// Waiting needs the session detached, and the chat view has no
	// terminal to attach to.
	if o.wait != "" || o.reply || o.view == string(contract.ViewChat) {
		o.detach = true
	}
	if o.prompt == "-" {
		text, err := e.readStdin()
		if err != nil {
			return e.Fail(err)
		}
		o.prompt = text
	}
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	dir, err := e.ResolveCwd(ctx, o.cwd)
	if err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "webmux run:", err)
		return contract.ExitUsage
	}
	request := contract.CreateSessionRequest{
		Cols: o.cols, Rows: o.rows,
		Cwd: dir, Name: o.name, Prompt: o.prompt, UseInitInstruction: o.init,
		View: contract.SessionView(o.view), PermissionMode: contract.ChatPermissionMode(o.permission),
	}
	if len(o.labels) > 0 {
		request.Labels = o.labels
	}
	if len(o.set) > 0 {
		request.AgentConfig = o.set
	}
	if o.launchConfigSet {
		summary, err := launchConfigByName(ctx, client, o.launchConfig)
		if err != nil {
			if errors.Is(err, errUnknownLaunchConfig) {
				_, _ = fmt.Fprintln(e.Stderr, "webmux run:", err)
				return contract.ExitUsage
			}
			return e.Fail(err)
		}
		if request.View == contract.ViewChat && !summary.Chat {
			return e.Usagef("--view chat needs a launch config with the chat view; %q has none", summary.Name)
		}
		request.ConfigID = &summary.ID
	} else {
		request.Cmd = argv
		request.Env = e.sessionEnv(o.env)
	}
	// Location: an existing worktree, the worktree the directory lies in,
	// or a new one the server creates first.
	var repos []contract.Repository
	var newRepo contract.Repository
	repos, err = repositories(ctx, client)
	if err != nil && (o.worktree != "" || o.newWorktree) {
		return e.Fail(err)
	}
	switch {
	case o.worktree != "":
		repo, tree, err := e.resolveWorktree(ctx, repos, o.worktree)
		if err != nil {
			return e.locationError(err)
		}
		if o.cwd == "" {
			request.Cwd = tree.Path
		} else if !within(tree.Path, mustLocal(ctx, e, o.cwd)) {
			return e.Usagef("--cwd %s is not inside worktree %s", o.cwd, tree.Path)
		}
		request.Workspace = &contract.WorkspaceReference{RepositoryID: repo.ID, WorktreeID: tree.ID}
	case o.newWorktree:
		newRepo, err = e.resolveRepository(ctx, repos, o.repo)
		if err != nil {
			return e.locationError(err)
		}
		request.Cwd = ""
		request.NewWorktree = &contract.NewWorktree{RepositoryID: newRepo.ID, Mode: "new", Branch: o.branch, Base: o.base}
		if o.existingBranch {
			request.NewWorktree.Mode = "existing"
		}
		if o.worktreePath != "" {
			request.NewWorktree.Path = e.worktreeDir(ctx, newRepo, o.branch, o.worktreePath)
		}
	default:
		// The server associates the session with this worktree anyway;
		// naming it lets --init find the repository's instruction.
		if local, err := e.localPath(ctx, o.cwd); err == nil {
			if repo, tree, ok := findWorktree(repos, local); ok {
				request.Workspace = &contract.WorkspaceReference{RepositoryID: repo.ID, WorktreeID: tree.ID}
			}
		}
	}
	if request.UseInitInstruction && request.Workspace == nil && request.NewWorktree == nil {
		return e.Usagef("--init needs a registered worktree: run inside one, or use --worktree or --new-worktree")
	}
	if o.dryRun {
		return e.printDryRun(request, o)
	}
	// An attached run needs a terminal: check before creating anything.
	var console Console
	if !o.detach && !o.follow {
		c, err := e.OpenConsole()
		switch {
		case errors.Is(err, attach.ErrNotTerminal) && !o.attachMode:
			_, _ = fmt.Fprintln(e.Stderr, "webmux run: no terminal; detaching (use --attach to insist)")
			o.detach = true
		case errors.Is(err, attach.ErrNotTerminal):
			_, _ = fmt.Fprintln(e.Stderr, "webmux:", err)
			return contract.ExitUsage
		case err != nil:
			return e.Fail(err)
		default:
			console = c
		}
	}
	restore := func() {
		if console != nil {
			_ = console.Restore()
		}
	}
	if o.refreshBase && !o.existingBranch {
		base, err := e.refreshBase(ctx, client, newRepo, o.base)
		if err != nil {
			restore()
			return e.Fail(err)
		}
		request.NewWorktree.Base = base
	}
	for _, file := range o.attachments {
		path, err := e.uploadFile(ctx, client, "/api/uploads", file)
		if err != nil {
			restore()
			return e.Fail(fmt.Errorf("upload %s: %w; nothing was created", file, err))
		}
		request.Attachments = append(request.Attachments, contract.ChatAttachment{Path: path})
	}
	request.ClientRequestID = newRequestID()
	var result contract.CreateSessionResult
	if err := client.Do(ctx, http.MethodPost, "/api/sessions", request, &result); err != nil {
		restore()
		return e.createFailure(err)
	}
	// An attached terminal is raw already: lines end with CR LF.
	eol := "\n"
	if console != nil {
		eol = "\r\n"
	}
	for _, warning := range result.ConfigWarnings {
		_, _ = fmt.Fprintf(e.Stderr, "webmux run: %s%s", warning, eol)
	}
	if result.Worktree != nil && !o.noPin {
		repo := newRepo
		repo.Worktrees = append(slices.Clone(newRepo.Worktrees), *result.Worktree)
		if err := pinWorktrees(ctx, client, repo, []string{result.Worktree.ID}, true); err != nil {
			_, _ = fmt.Fprintf(e.Stderr, "webmux: the worktree was created but could not be shown on the home page: %v%s", err, eol)
		}
	}
	created := result.Session
	switch {
	case o.follow:
		return e.followRun(ctx, client, created.ID, o)
	case o.reply:
		return e.replyRun(ctx, client, result, o)
	case o.detach:
		extra := map[string]any{"configWarnings": nonNil(result.ConfigWarnings)}
		if result.Worktree != nil {
			extra["worktree"] = result.Worktree
		}
		return e.detached(ctx, client, created, o, extra)
	}
	// Announce the session ID before the session's output starts; stdout
	// stays the session's. The terminal is already raw, so the line ends with
	// CR LF.
	_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s · Ctrl+] to detach\r\n", created.ID)
	return e.attach(ctx, client, created.ID, false, console)
}
