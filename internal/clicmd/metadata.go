package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// Commands is the registry of the session commands both clients share.
func Commands() Registry {
	return Registry{runCommand, attachCommand, listCommand, openCommand, getCommand, labelCommand, dirCommand, guideCommand, mcpCommand,
		chatGroup, chatWatchCommand, chatLogCommand, chatStatusCommand, chatAnswerCommand, chatSetCommand, chatPermissionCommand,
		chatQueueGroup, chatQueueListCommand, chatQueueRemoveCommand, chatQueueEditCommand, chatQueueSendNowCommand, chatQueueClearCommand, chatQueueResumeCommand,
		launchConfigGroup, launchConfigListCommand, launchConfigShowCommand, launchConfigAddCommand, launchConfigEditCommand, launchConfigRemoveCommand,
		launchConfigPresetsCommand, launchConfigRestoreCommand, launchConfigOptionsCommand,
		worktreeGroup, worktreeListCommand, worktreeShowCommand, worktreeCreateCommand, worktreeSwitchCommand, worktreeRemoveCommand, worktreePruneCommand, worktreePinCommand, worktreeUnpinCommand, worktreeHomeCommand,
		repoGroup, repoListCommand, repoAddCommand, repoRemoveCommand, repoShowCommand, repoRefreshCommand, repoFetchCommand, repoBranchesCommand, repoFastForwardCommand, repoPushCommand, repoBranchRemoveCommand, repoRemotesCommand, repoCommitsCommand, repoBaseCommand, repoSettingsCommand,
		forwardGroup, forwardListCommand, forwardAddCommand, forwardRemoveCommand,
		apiCommand, killCommand, rmCommand, renameCommand, sendCommand, interruptCommand, resizeCommand, viewCommand, resumeCommand,
		waitCommand, logsCommand, screenCommand, eventsCommand, notifyCommand, completionCommand, completeCommand}
}

var getCommand = &Command{
	Name: "get", Group: GroupSessions, Args: "[SESSION]",
	Summary: "Show a session's metadata: repository, branch, launch config, views, parent, children and labels.",
	Description: "Inside a WebMux session SESSION defaults to self. The details are gathered by the server in one request " +
		"(GET /api/sessions/{id}/details); they never include the token or launch environment values. " +
		"The artifacts folder is reported only when it exists; get never creates it.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Output: "Aligned key-value lines. With --json, the SessionDetails object. With --field, only that value: " +
		"strings unquoted, objects and arrays as JSON.",
	Exit: []ExitItem{
		{Code: 0, Text: "Printed."},
		{Code: 1, Text: "No such session, the server is too old, or --field names a null or missing value."},
		{Code: 2, Text: "Usage error, or no SESSION outside a session."},
		{Code: 3, Text: "WebMux is not running."},
	},
	Examples: []Example{
		{Command: "webmux get", Text: "Inside a session, show where it runs and what started it."},
		{Command: "branch=$(webmux get --field worktree.branch)", Text: "Read the current session's branch without jq."},
		{Command: "webmux get abcd2345 --json", Text: "Print the full details of another session as JSON."},
	},
	SeeAlso: []string{"webmux ls", "webmux label", "webmux guide sessions"},
	Notes:   map[string]Note{"field": {Conflicts: []string{"json"}}, "json": {Conflicts: []string{"field"}}},
	Bind: func(flags *flag.FlagSet) Action {
		asJSON := flags.Bool("json", false, "print the SessionDetails object as JSON")
		field := flags.String("field", "", "print only the value at dotted `PATH`, such as worktree.branch")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) > 1 {
				return e.Usagef("at most one session ID is allowed")
			}
			if *asJSON && *field != "" {
				return e.Usagef("--json and --field cannot be combined")
			}
			arg := "self"
			if len(args) == 1 {
				arg = args[0]
			} else if e.Getenv == nil || e.Getenv(contract.SessionIDEnv) == "" {
				return e.Usagef("a session ID is required outside a WebMux session")
			}
			id, code, ok := e.SessionID(arg)
			if !ok {
				return code
			}
			return e.get(ctx, id, *asJSON, *field)
		}
	},
}

func (e Env) get(ctx context.Context, id string, asJSON bool, field string) int {
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	var raw json.RawMessage
	if err := client.Do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(id)+"/details", nil, &raw); err != nil {
		return e.failNew(ctx, client, err)
	}
	if asJSON {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, raw, "", "  "); err != nil {
			return e.Fail(err)
		}
		_, _ = fmt.Fprintf(e.Stdout, "%s\n", pretty.Bytes())
		return contract.ExitOK
	}
	if field != "" {
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return e.Fail(err)
		}
		value, ok := lookup(doc, field)
		if !ok || value == nil {
			_, _ = fmt.Fprintln(e.Stdout)
			return contract.ExitError
		}
		if s, isString := value.(string); isString {
			_, _ = fmt.Fprintln(e.Stdout, s)
			return contract.ExitOK
		}
		data, err := json.Marshal(value)
		if err != nil {
			return e.Fail(err)
		}
		_, _ = fmt.Fprintf(e.Stdout, "%s\n", data)
		return contract.ExitOK
	}
	var d contract.SessionDetails
	if err := json.Unmarshal(raw, &d); err != nil {
		return e.Fail(err)
	}
	printDetails(e, d)
	return contract.ExitOK
}

// lookup follows a dotted path through decoded JSON; array elements are
// addressed by index.
func lookup(doc any, path string) (any, bool) {
	current := doc
	for part := range strings.SplitSeq(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			next, ok := node[part]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			current = node[i]
		default:
			return nil, false
		}
	}
	return current, true
}

func printDetails(e Env, d contract.SessionDetails) {
	s := d.Session
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	row := func(key, value string) {
		if value != "" {
			_, _ = fmt.Fprintf(w, "%s:\t%s\n", key, value)
		}
	}
	status := string(s.Status)
	if s.ExitCode != nil {
		status = fmt.Sprintf("%s (%d)", s.Status, *s.ExitCode)
	}
	if s.Attention != nil {
		status += " · attention: " + string(s.Attention.Kind)
	}
	row("ID", s.ID)
	row("Name", s.Title)
	row("Status", status)
	row("Command", strings.Join(s.Cmd, " "))
	row("Directory", s.Cwd)
	if d.Repository != nil {
		row("Repository", d.Repository.Name+" ("+d.Repository.Path+")")
	}
	if d.Worktree != nil {
		branch := "(detached)"
		if d.Worktree.Branch != nil {
			branch = *d.Worktree.Branch
		}
		row("Worktree", branch+" · "+d.Worktree.Path)
	}
	switch {
	case d.LaunchConfig != nil:
		row("Launch config", d.LaunchConfig.Name)
	case d.LaunchConfigMissing:
		row("Launch config", "(deleted)")
	}
	if hasChatView(s) {
		view := string(s.View)
		if s.Transition != nil {
			view += " → " + string(s.Transition.To) + " (" + string(s.Transition.Phase) + ")"
		}
		if s.Agent != nil {
			view += " · " + s.Agent.Name
			if s.Agent.SessionID != nil {
				view += " · agent session " + *s.Agent.SessionID
			}
		}
		row("View", view)
	}
	if s.State != "" && s.State != contract.SessionStateActive {
		row("State", string(s.State))
	}
	if d.ArtifactsDir != nil {
		row("Artifacts", *d.ArtifactsDir)
	}
	row("Parent", d.ParentID)
	if len(d.Children) > 0 {
		row("Children", strings.Join(d.Children, " "))
	}
	if len(d.Labels) > 0 {
		keys := slices.Sorted(maps.Keys(d.Labels))
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k+"="+d.Labels[k])
		}
		row("Labels", strings.Join(pairs, " "))
	}
	if len(s.ListeningPorts) > 0 {
		ports := make([]string, 0, len(s.ListeningPorts))
		for _, p := range s.ListeningPorts {
			ports = append(ports, strconv.Itoa(p.Port))
		}
		row("Ports", strings.Join(ports, " "))
	}
	_ = w.Flush()
}

var labelCommand = &Command{
	Name: "label", Group: GroupSessions, Args: "SESSION KEY=VALUE|KEY-...",
	Summary: "Set or remove labels on a session; KEY- removes KEY.",
	Description: "Labels are key-value pairs for scripts to mark and find sessions, kept in memory with the session. " +
		"Keys match [a-z0-9][a-z0-9._-]{0,62}; values hold at most 256 characters and no control characters; " +
		"a session holds at most 16 labels. All changes apply together or not at all.",
	Arguments: []Item{
		sessionArgument,
		{Name: "KEY=VALUE", Text: "Set label KEY to VALUE."},
		{Name: "KEY-", Text: "Remove label KEY."},
	},
	Env:    []Item{sessionIDEnv},
	Output: "The resulting labels, one KEY=VALUE per line. With --json, the updated SessionInfo.",
	Exit: []ExitItem{
		{Code: 0, Text: "Updated."},
		{Code: 1, Text: "No such session, an invalid label, or the server is too old."},
		{Code: 2, Text: "Usage error."},
		{Code: 3, Text: "WebMux is not running."},
	},
	Examples: []Example{
		{Command: "webmux label self task=lint run=42", Text: "Mark the current session."},
		{Command: "webmux label abcd2345 task-", Text: "Remove the task label from a session."},
	},
	SeeAlso: []string{"webmux ls", "webmux get", "webmux run"},
	Bind: func(flags *flag.FlagSet) Action {
		asJSON := flags.Bool("json", false, "print the updated SessionInfo as JSON")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) < 2 {
				return e.Usagef("a session ID and at least one KEY=VALUE or KEY- are required")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			update := contract.LabelsUpdate{Set: map[string]string{}}
			for _, arg := range args[1:] {
				if key, value, ok := strings.Cut(arg, "="); ok {
					update.Set[key] = value
					continue
				}
				if key, ok := strings.CutSuffix(arg, "-"); ok && key != "" {
					update.Remove = append(update.Remove, key)
					continue
				}
				return e.Usagef("%q is neither KEY=VALUE nor KEY-", arg)
			}
			return e.label(ctx, id, update, *asJSON)
		}
	},
}

func (e Env) label(ctx context.Context, id string, update contract.LabelsUpdate, asJSON bool) int {
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	var raw json.RawMessage
	if err := client.Do(ctx, http.MethodPatch, "/api/sessions/"+url.PathEscape(id)+"/labels", update, &raw); err != nil {
		return e.failNew(ctx, client, err)
	}
	if asJSON {
		_, _ = fmt.Fprintf(e.Stdout, "%s\n", raw)
		return contract.ExitOK
	}
	var info contract.SessionInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return e.Fail(err)
	}
	for _, key := range slices.Sorted(maps.Keys(info.Labels)) {
		_, _ = fmt.Fprintf(e.Stdout, "%s=%s\n", key, info.Labels[key])
	}
	return contract.ExitOK
}

// failNew reports err from an endpoint newer servers have: a route the
// server does not know answers 404 without an error message, which means
// the server predates this CLI.
func (e Env) failNew(ctx context.Context, client *localclient.Client, err error) int {
	var apiErr *localclient.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound || apiErr.Message != "" {
		return e.Fail(err)
	}
	server := "unknown"
	if status, statusErr := client.Status(ctx); statusErr == nil && status.Version != "" {
		server = status.Version
	}
	mine := e.Version
	if mine == "" {
		mine = "unknown"
	}
	_, _ = fmt.Fprintf(e.Stderr, "webmux: the server is too old for this command (server %s, CLI %s); use \"$WEBMUX_BIN\", the server's own executable\n", server, mine)
	return contract.ExitError
}
