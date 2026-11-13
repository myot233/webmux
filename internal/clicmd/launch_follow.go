package clicmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

// createFailure reports a failed POST /api/sessions by its step, with what
// had been created; nothing is rolled back.
func (e Env) createFailure(err error) int {
	var apiErr *localclient.APIError
	var failure contract.CreateSessionFailure
	if !errors.As(err, &apiErr) || json.Unmarshal(apiErr.Body, &failure) != nil || failure.Step == "" {
		return e.sessionFail(err)
	}
	reason := ""
	if failure.Reason != "" {
		reason = " (" + failure.Reason + ")"
	}
	_, _ = fmt.Fprintf(e.Stderr, "webmux run: the %s step failed: %s%s\n", failure.Step, failure.Error, reason)
	if failure.Connector != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux run: install %s: %s\n", failure.Connector.Title, failure.Connector.InstallCommand)
	}
	if failure.Worktree != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux run: worktree %s was created and is kept\n", failure.Worktree.Path)
	}
	if failure.SessionID != "" {
		_, _ = fmt.Fprintf(e.Stderr, "webmux run: session %s was started\n", failure.SessionID)
	}
	return contract.ExitError
}

// replyRun follows the first prompt's turn of a session created on its
// chat view and prints the reply.
func (e Env) replyRun(ctx context.Context, client *localclient.Client, result contract.CreateSessionResult, o runOptions) int {
	id := result.Session.ID
	where := ""
	if result.Worktree != nil {
		where = " · " + result.Worktree.Path
	}
	_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s%s\n", id, where)
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
	stream, err := sessionobserve.OpenChat(ctx, client, id, 64<<20)
	if err != nil {
		return e.Fail(err)
	}
	defer stream.Close()
	// The first prompt's turn is the first turn of the session.
	turn, err := stream.WaitTurn(ctx, sessionobserve.TurnSelector{FirstTurn: true}, sessionobserve.TurnPolicy{
		StopAtRequest: !o.autoWaitRequests, CollectEvents: true,
	})
	if err != nil {
		return e.waitFailure(err)
	}
	return e.reportTurn(cliTurn(turn), o.asJSON)
}

func mustLocal(ctx context.Context, e Env, dir string) string {
	local, err := e.localPath(ctx, dir)
	if err != nil {
		return dir
	}
	return local
}

// locationError reports a repository or worktree argument that names
// nothing as a usage error and anything else as a failure.
func (e Env) locationError(err error) int {
	if errors.Is(err, errRepoArgument) {
		return e.Usagef("%v", err)
	}
	return e.Fail(err)
}

func (e Env) readStdin() (string, error) {
	if e.Stdin == nil {
		return "", errors.New("standard input is not available")
	}
	data, err := io.ReadAll(e.Stdin)
	return strings.TrimRight(string(data), "\n"), err
}

// printDryRun writes the request run would send. Environment values are
// replaced: inherited ones by "<inherited>", explicit ones by "<explicit>".
// Attachments are the local files that would be uploaded first.
func (e Env) printDryRun(request contract.CreateSessionRequest, o runOptions) int {
	if request.Env != nil {
		masked := make(map[string]string, len(request.Env))
		for name := range request.Env {
			masked[name] = "<inherited>"
			if _, ok := o.env[name]; ok {
				masked[name] = "<explicit>"
			}
		}
		request.Env = masked
	}
	for _, file := range o.attachments {
		request.Attachments = append(request.Attachments, contract.ChatAttachment{Path: file})
	}
	out := map[string]any{"request": request}
	if request.NewWorktree != nil {
		out["pin"] = !o.noPin
	}
	writeJSON(e.Stdout, out)
	return contract.ExitOK
}

// worktreeDir is where a new worktree goes: dir resolved like other local
// paths (relative or ~), or the quick start default when dir is empty.
func (e Env) worktreeDir(ctx context.Context, repo contract.Repository, branch, dir string) string {
	if dir == "" {
		return defaultWorktreePath(repo, branch)
	}
	if local, err := e.localPath(ctx, dir); err == nil {
		return local
	}
	return dir
}

// refreshBase fetches the upstream of base (default: the repository's
// default branch) and returns the commit a new branch starts from; a
// failed fetch is a warning and the local branch is used.
func (e Env) refreshBase(ctx context.Context, client *localclient.Client, repo contract.Repository, base string) (string, error) {
	repoPath := "/api/repositories/" + url.PathEscape(repo.ID)
	ref := base
	if ref == "" {
		var branches contract.Branches
		if err := client.Do(ctx, http.MethodGet, repoPath+"/branches", nil, &branches); err != nil {
			return "", err
		}
		if branches.DefaultBranch != nil {
			ref = *branches.DefaultBranch
		}
	}
	var refreshed contract.RefreshBranchBaseResponse
	if err := client.Do(ctx, http.MethodPost, repoPath+"/branches/refresh", contract.RefreshBranchBaseRequest{Branch: ref}, &refreshed); err != nil {
		return "", err
	}
	if refreshed.FetchWarning != "" {
		_, _ = fmt.Fprintln(e.Stderr, "webmux: fetching the base failed; using the local branch:", refreshed.FetchWarning)
	}
	return refreshed.Base, nil
}

// createWorktree creates a worktree like the Web UI's dialog and pins it
// to the home page unless pin is false; a failed pin is only a warning.
func (e Env) createWorktree(ctx context.Context, client *localclient.Client, repo contract.Repository, branch, base, dir string, existing, refresh, pin bool) (contract.Worktree, error) {
	dir = e.worktreeDir(ctx, repo, branch, dir)
	repoPath := "/api/repositories/" + url.PathEscape(repo.ID)
	if refresh && !existing {
		refreshed, err := e.refreshBase(ctx, client, repo, base)
		if err != nil {
			return contract.Worktree{}, err
		}
		base = refreshed
	}
	mode := "new"
	if existing {
		mode = "existing"
	}
	if base == "" && !existing {
		var branches contract.Branches
		if err := client.Do(ctx, http.MethodGet, repoPath+"/branches", nil, &branches); err != nil {
			return contract.Worktree{}, err
		}
		if branches.DefaultBase != nil {
			base = *branches.DefaultBase
		}
	}
	var tree contract.Worktree
	request := contract.CreateWorktreeRequest{Path: dir, Mode: mode, Branch: branch, Base: base}
	if err := client.Do(ctx, http.MethodPost, repoPath+"/worktrees", request, &tree); err != nil {
		return contract.Worktree{}, fmt.Errorf("create worktree: %w", err)
	}
	if pin {
		refreshed := repo
		refreshed.Worktrees = append(slices.Clone(repo.Worktrees), tree)
		if err := pinWorktrees(ctx, client, refreshed, []string{tree.ID}, true); err != nil {
			_, _ = fmt.Fprintln(e.Stderr, "webmux: the worktree was created but could not be shown on the home page:", err)
		}
	}
	return tree, nil
}

// detached prints a created or resumed session and waits for --wait.
// extra adds fields to the --json object.
func (e Env) detached(ctx context.Context, client *localclient.Client, created contract.SessionInfo, o runOptions, extra map[string]any) int {
	session := created
	if o.wait != "" {
		states, _ := parseStates(o.wait)
		waitCtx := ctx
		if o.timeout > 0 {
			var cancel context.CancelFunc
			waitCtx, cancel = context.WithTimeout(ctx, o.timeout)
			defer cancel()
		}
		watch, err := watchSessions(waitCtx, client)
		if err != nil {
			return e.Fail(err)
		}
		matched, err := watch.waitFor(waitCtx, waitOptions{ids: []string{created.ID}, states: states})
		watch.subscription.Close()
		if err != nil {
			return e.waitFailure(err)
		}
		session = matched[0]
	}
	if o.asJSON {
		out := map[string]any{"session": session}
		maps.Copy(out, extra)
		writeJSON(e.Stdout, out)
	} else {
		_, _ = fmt.Fprintln(e.Stdout, created.ID)
	}
	return contract.ExitOK
}

// waitFailure maps an ended wait: a timeout is 124.
func (e Env) waitFailure(err error) int {
	if errors.Is(err, errTimeout) || errors.Is(err, context.DeadlineExceeded) {
		_, _ = fmt.Fprintln(e.Stderr, "webmux: timed out waiting")
		return contract.ExitTimeout
	}
	return e.Fail(err)
}

// followRun streams a created session until it exits. SIGINT and SIGTERM
// stop following (130, 143) and leave the program running, or end it first
// with --kill-on-interrupt.
func (e Env) followRun(ctx context.Context, client *localclient.Client, id string, o runOptions) int {
	_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s\n", id)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The goroutine hands over the signal that ended following. The
	// caller's context may be cancelled by the same signal first, so after a
	// cancellation the signal is awaited briefly.
	gotSignal := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case s := <-signals:
			cancel()
			gotSignal <- s
		case <-done:
		}
	}()
	out := e.Stdout
	var strip *stripper
	if o.strip {
		strip = newStripper(e.Stdout)
		out = strip
	}
	result, err := followTerminal(ctx, client, id, out, e.scriptLabel())
	if strip != nil {
		_ = strip.Flush()
	}
	var got os.Signal
	if ctx.Err() != nil {
		select {
		case got = <-gotSignal:
		case <-time.After(200 * time.Millisecond):
		}
	}
	if err != nil && ctx.Err() != nil {
		code := 130
		if got == syscall.SIGTERM {
			code = 143
		}
		if !o.killOnInterrupt {
			_, _ = fmt.Fprintf(e.Stderr, "\nwebmux: stopped following; session %s keeps running (webmux attach %s)\n", id, id)
			return code
		}
		return e.killAndWait(client, id, code)
	}
	if err != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: following session %s was interrupted: %v; it may still be running (webmux attach %s)\n", id, err, id)
		return contract.ExitError
	}
	if result.Moved {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s moved to its chat view and keeps running; follow it with webmux chat watch %s\n", id, id)
		return contract.ExitError
	}
	if result.Exited {
		return result.ExitCode
	}
	return contract.ExitOK
}

// killAndWait ends session id and returns its exit status, or fallback when
// it cannot be read within ten seconds.
func (e Env) killAndWait(client *localclient.Client, id string, fallback int) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	watch, err := watchSessions(ctx, client)
	if err != nil {
		return fallback
	}
	defer watch.subscription.Close()
	if err := client.Do(ctx, http.MethodDelete, "/api/sessions/"+url.PathEscape(id), nil, nil); err != nil {
		_, _ = fmt.Fprintln(e.Stderr, "webmux: could not end the session:", err)
		return fallback
	}
	matched, err := watch.waitFor(ctx, waitOptions{ids: []string{id}, states: []string{"exited"}})
	if err != nil || matched[0].ExitCode == nil {
		return fallback
	}
	return *matched[0].ExitCode
}

// scriptLabel names this client in the control indicator: "script", plus the
// session it runs in.
func (e Env) scriptLabel() string {
	if e.Getenv != nil {
		if id := e.Getenv(contract.SessionIDEnv); id != "" {
			return "script · " + id
		}
	}
	return "script"
}

var errUnknownLaunchConfig = errors.New("no saved launch config named")

// launchConfigByName finds a launch config summary by its exact name.
func launchConfigByName(ctx context.Context, client *localclient.Client, name string) (contract.LaunchConfigSummary, error) {
	var list []contract.LaunchConfigSummary
	if err := client.Do(ctx, http.MethodGet, "/api/launch-configs", nil, &list); err != nil {
		return contract.LaunchConfigSummary{}, err
	}
	names := make([]string, 0, len(list))
	for _, c := range list {
		if c.Name == name {
			return c, nil
		}
		names = append(names, fmt.Sprintf("%q", c.Name))
	}
	if len(names) == 0 {
		return contract.LaunchConfigSummary{}, fmt.Errorf("%w %q (no saved launch configs)", errUnknownLaunchConfig, name)
	}
	return contract.LaunchConfigSummary{}, fmt.Errorf("%w %q (saved: %s)", errUnknownLaunchConfig, name, strings.Join(names, ", "))
}

var chatGroup = &Command{
	Name: "chat", Group: GroupChat,
	Summary: "Act on what only the chat view has: requests, agent settings, the permission mode and the queue.",
}

var launchConfigGroup = &Command{
	Name: "launch-config", Group: GroupLaunch,
	Summary: "List, show, add, edit and remove the saved Web UI launch configs.",
}

// uploadFile uploads a local file to path and returns its server path.
func (e Env) uploadFile(ctx context.Context, client *localclient.Client, path, file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	var result contract.UploadResult
	if err := client.Upload(ctx, path, filepath.Base(file), f, &result); err != nil {
		return "", err
	}
	return result.Path, nil
}

var launchConfigOptionsCommand = &Command{
	Name: "launch-config options", Group: GroupLaunch, Args: "NAME",
	Summary: "List the options (model, mode, ...) the agent of a launch config with the chat view offers.",
	Description: "The agent reports its options from a throwaway session in a repository's main worktree " +
		"(POST /api/launch-configs/{id}/options). Use the IDs and values with webmux run --view chat --set ID=VALUE.",
	Arguments: []Item{{Name: "NAME", Text: "The exact name of a launch config with the chat view."}},
	Output:    "One option per line: ID, name, type, current value and choices. With --json, the ChatConfig array.",
	Exit: []ExitItem{
		{Code: 0, Text: "Listed."},
		{Code: 1, Text: "The launch config has no chat view (no_chat_view), or the agent could not be asked."},
		{Code: 2, Text: "Usage error, unknown launch config or repository."},
		{Code: 3, Text: "WebMux is not running."},
	},
	Examples: []Example{
		{Command: `webmux launch-config options "Claude Code"`, Text: "Show the agent's options and their values."},
		{Command: `webmux launch-config options Codex --json | jq '.[] | select(.id=="model") | .options[].value'`, Text: "List the models to pass to run --set model=..."},
	},
	SeeAlso: []string{"webmux run", "webmux chat set"},
	Bind: func(fs *flag.FlagSet) Action {
		repo := fs.String("repo", "", "repository whose main worktree the agent runs in: an ID, a path or . (default: the current directory's)")
		asJSON := fs.Bool("json", false, "print the ChatConfig array as JSON")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one launch config name is required")
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			summary, err := launchConfigByName(ctx, client, args[0])
			if err != nil {
				if errors.Is(err, errUnknownLaunchConfig) {
					return e.Usagef("%v", err)
				}
				return e.Fail(err)
			}
			if !summary.Chat {
				_, _ = fmt.Fprintf(e.Stderr, "webmux: launch config %q has no chat view (no_chat_view)\n", summary.Name)
				return contract.ExitError
			}
			repos, err := repositories(ctx, client)
			if err != nil {
				return e.Fail(err)
			}
			r, err := e.resolveRepository(ctx, repos, *repo)
			if err != nil {
				return e.locationError(err)
			}
			var options []contract.ChatConfig
			req := contract.LaunchOptionsRequest{RepositoryID: r.ID}
			if err := client.Do(ctx, http.MethodPost, "/api/launch-configs/"+url.PathEscape(summary.ID)+"/options", req, &options); err != nil {
				return e.sessionFail(err)
			}
			if *asJSON {
				writeJSON(e.Stdout, options)
				return contract.ExitOK
			}
			for _, option := range options {
				value := option.Value
				if option.Type == "boolean" && option.Enabled != nil {
					value = fmt.Sprint(*option.Enabled)
				}
				choices := make([]string, 0, len(option.Options))
				for _, c := range option.Options {
					choices = append(choices, c.Value)
				}
				line := fmt.Sprintf("%s\t%s\t%s\t%s", option.ID, option.Name, option.Type, value)
				if len(choices) > 0 {
					line += "\t" + strings.Join(choices, ",")
				}
				_, _ = fmt.Fprintln(e.Stdout, line)
			}
			return contract.ExitOK
		}
	},
}

// newRequestID is a clientRequestId for one logical request; retries of
// it reuse the value.
func newRequestID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return "cli-" + hex.EncodeToString(raw[:])
}
