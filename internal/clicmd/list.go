package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/rainchan/webmux/internal/contract"
)

// listStates are the values of ls --state.
var listStates = []string{"active", "closed", "interrupted", "archived", "all"}

var listCommand = &Command{
	Name: "ls", Group: GroupSessions,
	Summary: "List sessions, optionally filtered by state, label, parent, status or repository.",
	Description: "Without --state only active sessions are listed. A session with a chat view becomes closed or interrupted " +
		"when it ends and stays listed under those states (--archived lists both) until it is forgotten; webmux resume " +
		"brings it back. Filters combine: a session is listed only when it matches all of them. --label KEY=VALUE matches " +
		"a label value, --label KEY only requires the label to exist.",
	Output: "A table of ID, name, status (with exit code, or the state of an archived session, and the view of a session " +
		"with a chat view), command and launch directory. With --json, the server's SessionInfo array, filtered when " +
		"filters are given.",
	Exit: []ExitItem{
		{Code: 0, Text: "Listed."},
		{Code: 1, Text: "--repo names no registered repository, or the server refused."},
		{Code: 2, Text: "Usage error."},
		{Code: 3, Text: "WebMux is not running."},
	},
	Examples: []Example{
		{Command: "webmux ls", Text: "Show every active session."},
		{Command: "webmux ls --parent self --json", Text: "List the sessions this session created, as JSON."},
		{Command: "webmux ls --label run=42 --status exited", Text: "Show finished sessions of one scripted run."},
		{Command: "webmux ls --archived --repo .", Text: "Find an agent session of this repository to resume."},
	},
	SeeAlso: []string{"webmux get", "webmux label", "webmux resume"},
	Notes: map[string]Note{
		"label":    {Repeatable: true},
		"status":   {Repeatable: true},
		"state":    {Repeatable: true},
		"archived": {Conflicts: []string{"state"}},
	},
	Bind: func(flags *flag.FlagSet) Action {
		asJSON := flags.Bool("json", false, "print the server's session list as JSON")
		var labels, statuses, states listFlag
		flags.Var(&labels, "label", "only sessions with label `KEY=VALUE`, or with label KEY")
		parent := flags.String("parent", "", "only sessions created by `SESSION` (or self)")
		flags.Var(&statuses, "status", "only sessions in `STATUS`: running, waiting, idle or exited")
		flags.Var(&states, "state", "list sessions in `STATE`: active, closed, interrupted, archived or all (default: active)")
		archived := flags.Bool("archived", false, "list the closed and interrupted sessions (--state archived)")
		repo := flags.String("repo", "", "only sessions of the registered repository `REPO` (ID, path, or . for the current one)")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) > 0 {
				return e.Usagef("unexpected argument %q", args[0])
			}
			for _, status := range statuses {
				if !slices.Contains([]string{"running", "waiting", "idle", "exited"}, status) {
					return e.Usagef("unknown status %q", status)
				}
			}
			for _, state := range states {
				if !slices.Contains(listStates, state) {
					return e.Usagef("unknown state %q (%s)", state, strings.Join(listStates, ", "))
				}
			}
			if *archived {
				if len(states) > 0 {
					return e.Usagef("--archived and --state cannot be combined")
				}
				states = listFlag{"archived"}
			}
			parentID := ""
			if *parent != "" {
				id, code, ok := e.SessionID(*parent)
				if !ok {
					return code
				}
				parentID = id
			}
			return e.list(ctx, listFilter{labels: labels, parent: parentID, statuses: statuses, states: states, repo: *repo}, *asJSON)
		}
	},
}

// listFlag collects a repeatable string flag.
type listFlag []string

func (f *listFlag) String() string     { return strings.Join(*f, ",") }
func (f *listFlag) Set(v string) error { *f = append(*f, v); return nil }

type listFilter struct {
	labels   []string
	parent   string
	statuses []string
	// states go to the server; repo is resolved into repository.
	states     []string
	repo       string
	repository *contract.Repository
}

// local reports whether the filter drops sessions on the client.
func (f listFilter) local() bool {
	return len(f.labels) > 0 || f.parent != "" || len(f.statuses) > 0 || f.repository != nil
}

func (f listFilter) match(s contract.SessionInfo) bool {
	if f.parent != "" && s.ParentID != f.parent {
		return false
	}
	if len(f.statuses) > 0 && !slices.Contains(f.statuses, string(s.Status)) {
		return false
	}
	if f.repository != nil && !inRepository(s, *f.repository) {
		return false
	}
	for _, label := range f.labels {
		key, value, hasValue := strings.Cut(label, "=")
		got, ok := s.Labels[key]
		if !ok || (hasValue && got != value) {
			return false
		}
	}
	return true
}

// inRepository reports whether s belongs to repo: by its workspace, or for
// a session without one, by a launch directory inside one of its worktrees.
func inRepository(s contract.SessionInfo, repo contract.Repository) bool {
	if s.Workspace != nil {
		return s.Workspace.RepositoryID == repo.ID
	}
	return slices.ContainsFunc(repo.Worktrees, func(t contract.Worktree) bool { return within(t.Path, s.Cwd) })
}

// List implements `webmux ls`.
func List(ctx context.Context, e Env, args []string) int { return e.Run(ctx, listCommand, args) }

func (e Env) list(ctx context.Context, filter listFilter, asJSON bool) int {
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	if filter.repo != "" {
		repos, err := repositories(ctx, client)
		if err != nil {
			return e.Fail(err)
		}
		r, err := e.resolveRepository(ctx, repos, filter.repo)
		if err != nil {
			return e.Fail(err)
		}
		filter.repository = &r
	}
	path := "/api/sessions"
	if len(filter.states) > 0 {
		path += "?" + url.Values{"state": filter.states}.Encode()
	}
	var raw json.RawMessage
	if err := client.Do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return e.Fail(err)
	}
	if asJSON && !filter.local() {
		_, _ = fmt.Fprintf(e.Stdout, "%s\n", raw)
		return contract.ExitOK
	}
	var list []contract.SessionInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return e.Fail(err)
	}
	if filter.local() {
		// Filtering re-encodes the array, so --json keeps every field the
		// server sent only through the raw objects.
		var objects []json.RawMessage
		if err := json.Unmarshal(raw, &objects); err != nil {
			return e.Fail(err)
		}
		keptRaw := make([]json.RawMessage, 0, len(list))
		kept := make([]contract.SessionInfo, 0, len(list))
		for i, s := range list {
			if filter.match(s) {
				kept = append(kept, s)
				keptRaw = append(keptRaw, objects[i])
			}
		}
		if asJSON {
			data, err := json.Marshal(keptRaw)
			if err != nil {
				return e.Fail(err)
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s\n", data)
			return contract.ExitOK
		}
		list = kept
	}
	// Active sessions by ID; archived ones keep the server's order, most
	// recently updated first.
	active := func(s contract.SessionInfo) bool { return s.State == "" || s.State == contract.SessionStateActive }
	sort.SliceStable(list, func(i, j int) bool {
		if active(list[i]) != active(list[j]) {
			return active(list[i])
		}
		return active(list[i]) && list[i].ID < list[j].ID
	})
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCOMMAND\tCWD")
	for _, s := range list {
		status := string(s.Status)
		switch {
		case !active(s):
			status = string(s.State)
		case s.ExitCode != nil:
			status = fmt.Sprintf("%s (%d)", s.Status, *s.ExitCode)
		}
		if hasChatView(s) {
			status += " · " + viewLabel(s)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Title, status, strings.Join(s.Cmd, " "), s.Cwd)
	}
	_ = w.Flush()
	return contract.ExitOK
}

// viewLabel names the view a session with a chat view is on, for ls.
func viewLabel(s contract.SessionInfo) string {
	switch {
	case s.Transition != nil:
		return "切换中"
	case s.View == contract.ViewChat:
		return "聊天"
	default:
		return "终端"
	}
}

func keyNames() string {
	return strings.Join(slices.Sorted(maps.Keys(contract.InputKeys)), ", ")
}

// lastLines returns the last n lines of data.
func lastLines(data []byte, n int) []byte {
	trimmed := bytes.TrimRight(data, "\n")
	cut := len(trimmed)
	for i := 0; i < n; i++ {
		at := bytes.LastIndexByte(trimmed[:cut], '\n')
		if at < 0 {
			return data
		}
		cut = at
	}
	return data[cut+1:]
}
