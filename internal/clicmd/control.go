package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// exitsCommon are the statuses every server command shares.
func exitsCommon(extra ...ExitItem) []ExitItem {
	out := []ExitItem{{Code: 0, Text: "Done."}}
	out = append(out, extra...)
	return append(out,
		ExitItem{Code: 2, Text: "Usage error."},
		ExitItem{Code: 3, Text: "WebMux is not running."},
	)
}

var apiCommand = &Command{
	Name: "api", Group: GroupSessions, Args: "PATH",
	Summary: "Send one request to the local API and print the response body unchanged.",
	Description: "The escape hatch for endpoints without a dedicated command: any /api/ or /local/v1/ path, over the local socket, " +
		"with no token. The method is GET, or POST when -d is given. The body must be JSON; multipart uploads are not supported. " +
		"Restrictions on callers inside a session (stopping the server, reading the token, tunnel and Tailscale) still apply. " +
		"WebSocket paths are refused: use webmux events, logs, screen or chat watch.",
	Arguments: []Item{{Name: "PATH", Text: "An /api/ or /local/v1/ path, with an optional query string."}},
	Output:    "The response body as the server sent it; a non-2xx error message goes to standard error.",
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "The server answered with a non-2xx status."}),
	Examples: []Example{
		{Command: "webmux api /api/launch-configs", Text: "List the saved launch configs as JSON."},
		{Command: `webmux api -X PATCH /api/sessions/abcd2345 -d '{"name":"review"}'`, Text: "Rename a session through the raw API."},
		{Command: "webmux api -X DELETE /api/launch-configs/k3m9q2x7", Text: "Delete a launch config by ID."},
	},
	SeeAlso: []string{"webmux help", "docs/protocol.md in the repository"},
	Bind: func(fs *flag.FlagSet) Action {
		method := fs.String("X", "", "HTTP `METHOD` (default GET, or POST with -d)")
		data := fs.String("d", "", "JSON request body: literal `DATA`, @FILE, or - for standard input")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one path is required")
			}
			path := args[0]
			switch {
			case strings.HasPrefix(path, "/ws/"):
				return e.Usagef("%s is a WebSocket; use webmux events, logs, screen or chat watch", path)
			case !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/local/v1/"):
				return e.Usagef("the path must start with /api/ or /local/v1/")
			}
			var body []byte
			if *data != "" {
				var err error
				if body, err = e.readData(*data); err != nil {
					return e.Usagef("-d: %v", err)
				}
				if !json.Valid(body) {
					return e.Usagef("-d is not valid JSON")
				}
			}
			m := strings.ToUpper(*method)
			if m == "" {
				m = http.MethodGet
				if body != nil {
					m = http.MethodPost
				}
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			status, response, err := client.Raw(ctx, m, path, body)
			if err != nil {
				return e.Fail(err)
			}
			_, _ = e.Stdout.Write(response)
			if len(response) > 0 && !bytes.HasSuffix(response, []byte("\n")) {
				_, _ = io.WriteString(e.Stdout, "\n")
			}
			if status < 200 || status > 299 {
				var failure contract.ErrorResponse
				_ = json.Unmarshal(response, &failure)
				_, _ = fmt.Fprintf(e.Stderr, "webmux api: HTTP %d %s\n", status, failure.Error)
				return contract.ExitError
			}
			return contract.ExitOK
		}
	},
}

// readData reads -d: a literal, @FILE or - for standard input.
func (e Env) readData(value string) ([]byte, error) {
	switch {
	case value == "-":
		if e.Stdin == nil {
			return nil, errors.New("standard input is not available")
		}
		return io.ReadAll(e.Stdin)
	case strings.HasPrefix(value, "@"):
		return os.ReadFile(value[1:])
	}
	return []byte(value), nil
}

// sessionAction declares a command that sends one request about a session.
func sessionAction(name, summary, description, method, query string, examples []Example) *Command {
	return &Command{
		Name: name, Group: GroupSessions, Args: "SESSION", Summary: summary, Description: description,
		Arguments: []Item{sessionArgument}, Env: []Item{sessionIDEnv},
		Exit:     exitsCommon(ExitItem{Code: 1, Text: "No such session, or the server refused (for example while it changes views)."}),
		Examples: examples, SeeAlso: []string{"webmux ls", "webmux get"},
		Bind: func(fs *flag.FlagSet) Action {
			return func(ctx context.Context, e Env, args []string) int {
				if len(args) != 1 {
					return e.Usagef("exactly one session ID is required")
				}
				id, code, ok := e.SessionID(args[0])
				if !ok {
					return code
				}
				client, code := e.connect()
				if client == nil {
					return code
				}
				defer client.Close()
				if err := client.Do(ctx, method, "/api/sessions/"+url.PathEscape(id)+query, nil, nil); err != nil {
					return e.sessionFail(err)
				}
				return contract.ExitOK
			}
		},
	}
}

var killCommand = sessionAction("kill", "End a session's process; the session stays listed as exited.",
	"The process group gets SIGHUP, then SIGKILL if it does not exit. Use rm to also remove the session.",
	http.MethodDelete, "", []Example{
		{Command: "webmux kill abcd2345", Text: "Stop a runaway command."},
		{Command: "webmux ls --parent self --status running --json | jq -r '.[].id' | xargs -n1 webmux kill", Text: "End every running session this session created."},
	})

const (
	removeQuery = "?remove=1"
	forgetQuery = "?forget=1"
)

var rmCommand = &Command{
	Name: "rm", Group: GroupSessions, Args: "SESSION",
	Summary: "Remove a session; a running one is ended first. --forget also deletes its archive.",
	Description: "The session disappears from every client. A session with a chat view keeps its archive, so it can be " +
		"resumed (webmux resume) and is listed by webmux ls --archived; its artifacts folder stays with it. --forget deletes " +
		"the archive and the artifacts folder too; the agent keeps its own session history. An archived session can only " +
		"be forgotten. On a terminal --forget asks first; without one it needs --yes.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "No such session, it is archived and --forget was not given, or the server refused (for example while it changes views)."},
		ExitItem{Code: 2, Text: "Also: --forget with no terminal to ask and no --yes."},
	),
	Examples: []Example{
		{Command: "webmux rm abcd2345", Text: "Remove a session."},
		{Command: "webmux ls --label run=42 --json | jq -r '.[].id' | xargs -n1 webmux rm", Text: "Clean up every session of one scripted run."},
		{Command: "webmux ls --archived --json | jq -r '.[].id' | xargs -n1 webmux rm --forget --yes", Text: "Delete every archived session."},
	},
	SeeAlso: []string{"webmux kill", "webmux resume", "webmux ls"},
	Notes:   map[string]Note{"yes": {Requires: []string{"forget"}}},
	Bind: func(fs *flag.FlagSet) Action {
		forget := fs.Bool("forget", false, "also delete the archive and the artifacts folder")
		yes := fs.Bool("yes", false, "with --forget, do not ask for confirmation")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session ID is required")
			}
			if *yes && !*forget {
				return e.Usagef("--yes needs --forget")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			var info contract.SessionInfo
			if err := client.Do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(id), nil, &info); err != nil {
				return e.sessionFail(err)
			}
			archived := info.State != "" && info.State != contract.SessionStateActive
			if !*forget {
				if archived {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s is %s; delete its archive with webmux rm %s --forget\n", id, info.State, id)
					return contract.ExitError
				}
				return e.removeSession(ctx, client, id, removeQuery)
			}
			if !*yes {
				question := fmt.Sprintf("Forget %s session %s (%s)? Its archive and artifacts folder are deleted.", sessionState(info), id, oneLine(sessionName(info), 40))
				if code, ok := e.confirm(question); !ok {
					return code
				}
			}
			return e.removeSession(ctx, client, id, forgetQuery)
		}
	},
}

// sessionState is the lifecycle state of s, or its status while active.
func sessionState(s contract.SessionInfo) string {
	if s.State != "" && s.State != contract.SessionStateActive {
		return string(s.State)
	}
	return string(s.Status)
}

// sessionName is what identifies s to a person: its title, else its agent
// or command.
func sessionName(s contract.SessionInfo) string {
	switch {
	case s.Title != "":
		return s.Title
	case s.Agent != nil && s.Agent.Title != "":
		return s.Agent.Title
	case s.Agent != nil:
		return s.Agent.Name
	}
	return strings.Join(s.Cmd, " ")
}

// removeSession removes or forgets session id with query; a session whose
// process still runs is ended first.
func (e Env) removeSession(ctx context.Context, client *localclient.Client, id, query string) int {
	err := client.Do(ctx, http.MethodDelete, "/api/sessions/"+url.PathEscape(id)+query, nil, nil)
	if isStatus(err, http.StatusConflict) {
		if _, isConflict := sessionConflict(err); !isConflict {
			err = endAndRemove(ctx, client, id, query)
		}
	}
	if err != nil {
		return e.sessionFail(err)
	}
	return contract.ExitOK
}

// rmEndTimeout bounds how long rm waits for a running session to exit.
const rmEndTimeout = 15 * time.Second

func isStatus(err error, status int) bool {
	var apiErr *localclient.APIError
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// endAndRemove ends a running session, waits for it to exit, and removes
// (or forgets) it with query.
func endAndRemove(ctx context.Context, client *localclient.Client, id, query string) error {
	ctx, cancel := context.WithTimeout(ctx, rmEndTimeout)
	defer cancel()
	watch, err := watchSessions(ctx, client)
	if err != nil {
		return err
	}
	defer watch.close()
	path := "/api/sessions/" + url.PathEscape(id)
	if err := client.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return err
	}
	if _, err := watch.waitFor(ctx, waitOptions{ids: []string{id}, states: []string{"exited"}}); err != nil {
		if !errors.Is(err, errSessionRemoved) {
			return fmt.Errorf("the session did not exit: %w", err)
		}
		if query == removeQuery {
			return nil
		}
	}
	return client.Do(ctx, http.MethodDelete, path+query, nil, nil)
}

var renameCommand = &Command{
	Name: "rename", Group: GroupSessions, Args: "SESSION [NAME]",
	Summary:   "Name a session; an empty NAME hands the title back to the terminal or agent.",
	Arguments: []Item{sessionArgument, {Name: "NAME", Text: "At most 512 bytes, no control characters."}},
	Env:       []Item{sessionIDEnv},
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "No such session or an invalid name."}),
	Examples: []Example{
		{Command: `webmux rename self "lint fixes"`, Text: "Name the current session."},
		{Command: "webmux rename abcd2345", Text: "Clear the name so the terminal title shows again."},
	},
	SeeAlso: []string{"webmux get", "webmux label"},
	Bind: func(fs *flag.FlagSet) Action {
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) < 1 || len(args) > 2 {
				return e.Usagef("a session ID and at most one name are required (quote a name with spaces)")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			if err := client.Do(ctx, http.MethodPatch, "/api/sessions/"+url.PathEscape(id), contract.RenameSessionRequest{Name: name}, nil); err != nil {
				return e.Fail(err)
			}
			return contract.ExitOK
		}
	},
}

var resizeCommand = &Command{
	Name: "resize", Group: GroupSessions, Args: "SESSION COLSxROWS",
	Summary: "Set the size of a session's terminal, in columns and rows.",
	Description: "Resizes the PTY, so the program redraws for the new size. It works while no browser or attached terminal controls the " +
		"session, or while a script does (webmux send makes this client the controller as \"script\"). A session that a browser or " +
		"attached terminal controls refuses with exit 1, because that client sends its own window size back and the two would fight; " +
		"--take takes control for the resize, then releases it. The first browser or attached terminal to take control afterwards " +
		"resizes the session to its own window. The terminal's replay and webmux screen use the controller's size.",
	Arguments: []Item{sessionArgument, {Name: "COLSxROWS", Text: "Columns and rows, each 1-1000, such as 120x40."}},
	Env:       []Item{sessionIDEnv},
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "No such session, it has exited or is on its chat view (no terminal), or a browser or attached terminal controls it and --take was not given."}),
	Examples: []Example{
		{Command: "webmux resize abcd2345 120x40", Text: "Make a script-driven session's terminal 120 columns by 40 rows."},
		{Command: "webmux resize abcd2345 200x50 --take", Text: "Resize a session even though a browser controls it."},
	},
	SeeAlso: []string{"webmux run", "webmux screen", "webmux send"},
	Bind: func(fs *flag.FlagSet) Action {
		take := fs.Bool("take", false, "take control from a browser or attached terminal for the resize, then release it")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 {
				return e.Usagef("a session ID and COLSxROWS are required")
			}
			cols, rows, ok := parseGrid(args[1])
			if !ok {
				return e.Usagef("%q is not COLSxROWS with each 1-%d, such as 120x40", args[1], contract.MaxTerminalDimension)
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			request := contract.SessionResizeRequest{Cols: cols, Rows: rows, Take: *take}
			if err := client.Do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(id)+"/resize", request, nil); err != nil {
				return e.sessionFail(err)
			}
			return contract.ExitOK
		}
	},
}

// parseGrid reads COLSxROWS, each 1-contract.MaxTerminalDimension.
func parseGrid(s string) (cols, rows int, ok bool) {
	c, r, found := strings.Cut(strings.ToLower(s), "x")
	if !found {
		return 0, 0, false
	}
	cols, errCols := strconv.Atoi(c)
	rows, errRows := strconv.Atoi(r)
	if errCols != nil || errRows != nil || cols < 1 || rows < 1 || cols > contract.MaxTerminalDimension || rows > contract.MaxTerminalDimension {
		return 0, 0, false
	}
	return cols, rows, true
}

var waitCommand = &Command{
	Name: "wait", Group: GroupSessions, Args: "SESSION...",
	Summary: "Wait until sessions reach a state: running, waiting, idle, exited or attention.",
	Description: "Subscribes to the session list (/ws/sessions) and returns when every session (or with --any, one) is in one of the " +
		"--until states: a status (running, waiting, idle, exited) or attention, a latched signal that the session needs you. " +
		"waiting means the session needs you, so a shell at its prompt is waiting; idle is long silence, not completion " +
		"(webmux guide sessions explains each). Without --changed the current state counts; with --changed only a change " +
		"after subscribing does. --stable requires the condition to hold that long, which filters short-lived transitions.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "Nothing; with --json the matching SessionInfo, or an array for several sessions.",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "A session was removed, or exited when exited was not awaited."},
		ExitItem{Code: 124, Text: "Timed out."},
	),
	Examples: []Example{
		{Command: "webmux wait abcd2345 --until attention,idle --changed --timeout 30m", Text: "Wait until an agent needs you or finishes its turn."},
		{Command: "webmux wait abcd2345 --until exited --json | jq .exitCode", Text: "Wait for a command and read its exit code."},
		{Command: "webmux wait a1 b2 c3 --until attention --changed --any", Text: "Return when the first of three agents needs you or finishes its turn."},
	},
	SeeAlso: []string{"webmux send --wait", "webmux events", "webmux guide sessions"},
	Bind: func(fs *flag.FlagSet) Action {
		until := fs.String("until", "", "comma-separated `STATES` to wait for (required)")
		changed := fs.Bool("changed", false, "ignore the state at subscription; wait for a change into the states")
		stable := fs.Duration("stable", 0, "require the condition to hold for this `DURATION`")
		anyOne := fs.Bool("any", false, "return when one session matches instead of all")
		timeout := fs.Duration("timeout", 0, "give up after this `DURATION` (exit 124); 0 waits forever")
		asJSON := fs.Bool("json", false, "print the matching SessionInfo")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) == 0 {
				return e.Usagef("at least one session ID is required")
			}
			if *until == "" {
				return e.Usagef("--until is required")
			}
			states, err := parseStates(*until)
			if err != nil {
				return e.Usagef("%v", err)
			}
			ids := make([]string, 0, len(args))
			for _, arg := range args {
				id, code, ok := e.SessionID(arg)
				if !ok {
					return code
				}
				ids = append(ids, id)
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			if *timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, *timeout)
				defer cancel()
			}
			watch, err := watchSessions(ctx, client)
			if err != nil {
				return e.Fail(err)
			}
			defer watch.close()
			matched, err := watch.waitFor(ctx, waitOptions{ids: ids, states: states, changed: *changed, stable: *stable, any: *anyOne})
			if err != nil {
				return e.waitFailure(err)
			}
			if !slices.Contains(states, "exited") {
				for _, s := range matched {
					if s.Status == contract.SessionStatusExited {
						_, _ = fmt.Fprintf(e.Stderr, "webmux wait: session %s exited\n", s.ID)
						return contract.ExitError
					}
				}
			}
			if *asJSON {
				if len(matched) == 1 && len(ids) == 1 {
					writeJSON(e.Stdout, matched[0])
				} else {
					writeJSON(e.Stdout, matched)
				}
			}
			return contract.ExitOK
		}
	},
}
