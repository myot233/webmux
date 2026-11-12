package clicmd

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// Home page display states of a worktree, as the Web UI decides them.
const (
	homePinned   = "pinned"
	homeSessions = "sessions"
	homeHidden   = "hidden"
)

// home is what the home page display of every repository depends on: the
// homeWorktrees setting, active sessions, and archived (closed or
// interrupted) sessions, each placed as the web home page places them.
type home struct {
	pins     map[string][]string
	sessions map[string][]contract.SessionInfo // by worktree ID
	archived map[string][]contract.SessionInfo // by worktree ID
}

func loadHome(ctx context.Context, client *localclient.Client) (home, error) {
	h := home{sessions: map[string][]contract.SessionInfo{}, archived: map[string][]contract.SessionInfo{}}
	var snapshot contract.SettingsSnapshot
	if err := client.Do(ctx, http.MethodGet, "/api/settings", nil, &snapshot); err != nil {
		return h, err
	}
	h.pins = snapshot.Values.HomeWorktrees
	var sessions []contract.SessionInfo
	if err := client.Do(ctx, http.MethodGet, "/api/sessions", nil, &sessions); err != nil {
		return h, err
	}
	for _, s := range sessions {
		if s.Workspace != nil {
			h.sessions[s.Workspace.WorktreeID] = append(h.sessions[s.Workspace.WorktreeID], s)
		}
	}
	var archived []contract.SessionInfo
	if err := client.Do(ctx, http.MethodGet, "/api/sessions?state=archived", nil, &archived); err != nil || len(archived) == 0 {
		return h, nil
	}
	var repositories []contract.Repository
	if err := client.Do(ctx, http.MethodGet, "/api/repositories", nil, &repositories); err != nil {
		return h, err
	}
	for _, s := range archived {
		if id, ok := archivedWorktree(s, repositories); ok {
			h.archived[id] = append(h.archived[id], s)
		}
	}
	return h, nil
}

// archivedWorktree is the web's archived session placement: the recorded
// worktree wins; a session recorded before associations were kept joins
// the registered worktree with the longest path containing its cwd.
func archivedWorktree(s contract.SessionInfo, repositories []contract.Repository) (string, bool) {
	if s.Workspace != nil {
		return s.Workspace.WorktreeID, true
	}
	_, tree, ok := findWorktree(repositories, s.Cwd)
	return tree.ID, ok
}

// pinned is the web's pinnedIds: the curated list, or the main worktree.
func (h home) pinned(repo contract.Repository) ([]string, bool) {
	list, curated := h.pins[repo.ID]
	if !curated {
		return mainWorktrees(repo), false
	}
	return list, true
}

// status is the web's home layout for one worktree: pinned, shown for its
// sessions or archived records, or hidden.
func (h home) status(repo contract.Repository, tree contract.Worktree) string {
	list, _ := h.pinned(repo)
	switch {
	case slices.Contains(list, tree.ID):
		return homePinned
	case len(h.sessions[tree.ID]) > 0 || len(h.archived[tree.ID]) > 0:
		return homeSessions
	}
	return homeHidden
}

// worktreeView is a Worktree with its home page state and session count.
type worktreeView struct {
	contract.Worktree
	Home     string `json:"home"`
	Sessions int    `json:"sessions"`
}

type repositoryView struct {
	contract.Repository
	Worktrees []worktreeView `json:"worktrees"`
}

func (h home) view(repo contract.Repository) repositoryView {
	v := repositoryView{Repository: repo, Worktrees: make([]worktreeView, 0, len(repo.Worktrees))}
	for _, t := range repo.Worktrees {
		v.Worktrees = append(v.Worktrees, worktreeView{Worktree: t, Home: h.status(repo, t), Sessions: len(h.sessions[t.ID])})
	}
	return v
}

var worktreeSwitchCommand = &Command{
	Name: "worktree switch", Group: GroupWorkspace, Args: "REPO WORKTREE [BRANCH]",
	Summary: "Switch a worktree to another branch or commit after previewing what it affects.",
	Description: "Give exactly one target: BRANCH, a local branch no other worktree has checked out; --track REMOTE/BRANCH, " +
		"which creates the local branch BRANCH tracking it; --create NAME, a new branch without an upstream starting at " +
		"--from (default: the worktree's current commit); or --detach REF, a commit. The preview is printed to standard " +
		"error first: what blocks the switch (uncommitted changes, a merge or rebase in progress, a branch checked out " +
		"elsewhere), warnings, and the running sessions, which keep running and see the switched files. With blockers " +
		"nothing changes. With running sessions it asks you to type yes on a terminal; without a terminal it needs --yes. " +
		"Changes are never stashed, carried or discarded; once Git starts, interrupting the command does not stop it.",
	Arguments: []Item{repoArgument, worktreeArgument, {Name: "BRANCH", Text: "The local branch to switch to."}},
	Output:    "The branch the worktree is on, or the commit for --detach; with --json, the Worktree.",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "Also: blockers, the preview went stale, or Git refused (files would be overwritten, a hook failed)."},
		ExitItem{Code: 2, Text: "Also: running sessions, no terminal to confirm and no --yes."},
	),
	Examples: []Example{
		{Command: "webmux worktree switch . main feat/a", Text: "Switch the main worktree to an existing branch."},
		{Command: "webmux worktree switch . main --track origin/fix/b", Text: "Check out a remote branch as fix/b tracking it."},
		{Command: "webmux worktree switch . exp/a --create exp/b --from main --yes", Text: "Start a new branch in a script, even with sessions running."},
		{Command: "webmux worktree switch . main --detach v1.2.0", Text: "Look at a tagged commit."},
	},
	SeeAlso: []string{"webmux worktree create", "webmux repo branches", "webmux worktree show"},
	Notes: map[string]Note{
		"track": {Conflicts: []string{"create", "detach"}}, "create": {Conflicts: []string{"track", "detach"}},
		"detach": {Conflicts: []string{"track", "create", "from"}}, "from": {Requires: []string{"create"}},
	},
	Bind: func(fs *flag.FlagSet) Action {
		track := fs.String("track", "", "create a local branch tracking the remote-tracking branch `REMOTE/BRANCH`")
		create := fs.String("create", "", "create the new branch `NAME`")
		from := fs.String("from", "", "start --create at `REF` (default: the worktree's current commit)")
		detach := fs.String("detach", "", "check out the commit `REF` without a branch")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		asJSON := fs.Bool("json", false, "print the Worktree")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 && len(args) != 3 {
				return e.Usagef("a repository, a worktree and a target are required")
			}
			targets := 0
			for _, given := range []bool{len(args) == 3, *track != "", *create != "", *detach != ""} {
				if given {
					targets++
				}
			}
			if targets != 1 {
				return e.Usagef("give exactly one of BRANCH, --track REMOTE/BRANCH, --create NAME or --detach REF")
			}
			if *from != "" && *create == "" {
				return e.Usagef("--from applies to --create only")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			tree, err := e.worktreeIn(ctx, repo, args[1])
			if err != nil {
				return e.locationError(err)
			}
			var target contract.SwitchTarget
			switch {
			case len(args) == 3:
				target = contract.SwitchTarget{Kind: "local", Branch: args[2]}
			case *track != "":
				target = contract.SwitchTarget{Kind: "remote", Ref: *track}
			case *create != "":
				start := *from
				if start == "" {
					if tree.Head == nil {
						return e.Usagef("the worktree has no commit: give --from REF")
					}
					start = *tree.Head
				}
				target = contract.SwitchTarget{Kind: "new", Name: *create, StartPoint: start}
			default:
				target = contract.SwitchTarget{Kind: "detach", Commit: *detach}
			}
			base := "/api/repositories/" + url.PathEscape(repo.ID) + "/worktrees/" + url.PathEscape(tree.ID)
			var preview contract.SwitchPreview
			if err := client.Do(ctx, http.MethodPost, base+"/switch-preview", contract.SwitchPreviewRequest{Target: target}, &preview); err != nil {
				return e.Fail(err)
			}
			e.printSwitchPreview(preview)
			if len(preview.Blockers) > 0 {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: not switched")
				return contract.ExitError
			}
			if len(preview.Sessions) > 0 && !*yes {
				if code, ok := e.confirmYes("Type yes to switch while these sessions run: "); !ok {
					return code
				}
			}
			request := contract.SwitchRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeSessions: len(preview.Sessions) > 0}
			var switched contract.Worktree
			if err := client.Do(ctx, http.MethodPost, base+"/switch", request, &switched); err != nil {
				return e.Fail(err)
			}
			switch {
			case *asJSON:
				writeJSON(e.Stdout, switched)
			case switched.Branch != nil:
				_, _ = fmt.Fprintln(e.Stdout, *switched.Branch)
			case switched.Head != nil:
				_, _ = fmt.Fprintln(e.Stdout, *switched.Head)
			}
			return contract.ExitOK
		}
	},
}

func (e Env) printSwitchPreview(p contract.SwitchPreview) {
	to := "detached " + shortHash(p.Target.Commit)
	if p.Target.Branch != nil {
		to = *p.Target.Branch + " at " + shortHash(p.Target.Commit)
		if p.Target.CreatesBranch {
			to = "new branch " + to
		}
	}
	_, _ = fmt.Fprintf(e.Stderr, "webmux: switching %s (%s) to %s %s\n", p.Worktree.Path, worktreeName(p.Worktree), to, oneLine(p.Target.Subject, 60))
	if p.Target.Upstream != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   tracking %s\n", *p.Target.Upstream)
	}
	for _, s := range p.Sessions {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   session %s (%s) %s\n", s.ID, s.Status, s.Title)
	}
	e.printIssues(p.Blockers, p.Warnings)
}

// confirmYes asks for yes to be typed on a terminal.
func (e Env) confirmYes(prompt string) (int, bool) {
	if e.Interactive == nil || !e.Interactive() || e.Stdin == nil {
		return e.Usagef("not a terminal: repeat with --yes to confirm"), false
	}
	_, _ = fmt.Fprint(e.Stderr, prompt)
	answer, _ := bufio.NewReader(e.Stdin).ReadString('\n')
	if strings.TrimSpace(answer) != "yes" {
		_, _ = fmt.Fprintln(e.Stderr, "webmux: cancelled; nothing changed")
		return contract.ExitError, false
	}
	return contract.ExitOK, true
}

// pinCommand builds worktree pin and unpin.
func pinCommand(name string, pinned bool, summary string, examples []Example) *Command {
	return &Command{
		Name: "worktree " + name, Group: GroupWorkspace, Args: "REPO WORKTREE...",
		Summary: summary,
		Description: "Only this repository's entry of the homeWorktrees setting changes, and every open browser follows. A " +
			"conflicting write from another client is read again and the change replayed, at most three times. A worktree " +
			"with sessions or archived sessions stays on the home page whether or not it is added.",
		Arguments: []Item{repoArgument, worktreeArgument},
		Exit:      exitsCommon(ExitItem{Code: 1, Text: "Also: the setting kept conflicting."}, ExitItem{Code: 2, Text: "Also: REPO or WORKTREE names nothing."}),
		Examples:  examples,
		SeeAlso:   []string{"webmux worktree home", "webmux worktree ls"},
		Bind: func(fs *flag.FlagSet) Action {
			return func(ctx context.Context, e Env, args []string) int {
				if len(args) < 2 {
					return e.Usagef("a repository and at least one worktree are required")
				}
				client, repo, code := e.repoAndClient(ctx, args[0])
				if client == nil {
					return code
				}
				defer client.Close()
				var ids []string
				for _, arg := range args[1:] {
					tree, err := e.worktreeIn(ctx, repo, arg)
					if err != nil {
						return e.locationError(err)
					}
					ids = append(ids, tree.ID)
				}
				if err := pinWorktrees(ctx, client, repo, ids, pinned); err != nil {
					return e.Fail(err)
				}
				if !pinned {
					if h, err := loadHome(ctx, client); err == nil {
						for _, id := range ids {
							if len(h.sessions[id]) > 0 || len(h.archived[id]) > 0 {
								_, _ = fmt.Fprintf(e.Stderr, "webmux: %s stays on the home page while it has sessions or archived sessions\n", id)
							}
						}
					}
				}
				return contract.ExitOK
			}
		},
	}
}

var (
	worktreePinCommand = pinCommand("pin", true, "Add worktrees to the home page.", []Example{
		{Command: "webmux worktree pin . exp/a", Text: "Show a worktree on every browser's home page."},
		{Command: "webmux worktree pin . exp/a exp/b", Text: "Add several at once."},
	})
	worktreeUnpinCommand = pinCommand("unpin", false, "Take worktrees off the home page.", []Example{
		{Command: "webmux worktree unpin . exp/a", Text: "Hide a worktree you are done with (it stays while it has sessions)."},
		{Command: "webmux worktree ls . --home --json | jq -r '.[].worktrees[] | select(.kind != \"main\") | .id' | xargs webmux worktree unpin .", Text: "Leave only the main worktree."},
	})
)

var worktreeHomeCommand = &Command{
	Name: "worktree home", Group: GroupWorkspace, Args: "REPO [WORKTREE...]",
	Summary: "Show or replace which worktrees of a repository are added to the home page.",
	Description: "Without options, the added worktrees are listed and marked as the default when nobody curated the repository " +
		"(only the main worktree). --set replaces the list with the given worktrees; --reset deletes the repository's " +
		"entry, so the home page shows its main worktree again, plus worktrees with sessions or archived records.",
	Arguments: []Item{repoArgument, {Name: "WORKTREE", Text: "With --set, the worktrees to add (ID, branch or path)."}},
	Output:    "Lines \"ID<TAB>BRANCH<TAB>PATH\"; with --json, {repositoryId, default, worktrees}.",
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "Also: the setting kept conflicting."}),
	Examples: []Example{
		{Command: "webmux worktree home .", Text: "See which worktrees the home page shows."},
		{Command: "webmux worktree home . --reset", Text: "Go back to showing only the main worktree."},
	},
	SeeAlso: []string{"webmux worktree pin", "webmux worktree ls"},
	Notes:   map[string]Note{"set": {Conflicts: []string{"reset"}}, "reset": {Conflicts: []string{"set"}}},
	Bind: func(fs *flag.FlagSet) Action {
		set := fs.Bool("set", false, "replace the list with the WORKTREE arguments")
		reset := fs.Bool("reset", false, "delete the repository's entry (show the main worktree)")
		asJSON := fs.Bool("json", false, "print {repositoryId, default, worktrees}")
		return func(ctx context.Context, e Env, args []string) int {
			switch {
			case len(args) < 1:
				return e.Usagef("a repository is required")
			case *set && *reset:
				return e.Usagef("--set and --reset cannot be combined")
			case len(args) > 1 && !*set:
				return e.Usagef("worktrees are taken only with --set")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			switch {
			case *set:
				ids := []string{}
				for _, arg := range args[1:] {
					tree, err := e.worktreeIn(ctx, repo, arg)
					if err != nil {
						return e.locationError(err)
					}
					ids = append(ids, tree.ID)
				}
				err := updateHome(ctx, client, repo, func([]string, bool) ([]string, bool) { return ids, true })
				if err != nil {
					return e.Fail(err)
				}
			case *reset:
				if err := updateHome(ctx, client, repo, func([]string, bool) ([]string, bool) { return nil, false }); err != nil {
					return e.Fail(err)
				}
			}
			h, err := loadHome(ctx, client)
			if err != nil {
				return e.Fail(err)
			}
			ids, curated := h.pinned(repo)
			trees := []contract.Worktree{}
			for _, t := range repo.Worktrees {
				if slices.Contains(ids, t.ID) {
					trees = append(trees, t)
				}
			}
			if *asJSON {
				writeJSON(e.Stdout, map[string]any{"repositoryId": repo.ID, "default": !curated, "worktrees": trees})
				return contract.ExitOK
			}
			if !curated {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: not curated; the home page shows the main worktree")
			}
			for _, t := range trees {
				_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\t%s\n", t.ID, worktreeName(t), t.Path)
			}
			return contract.ExitOK
		}
	},
}

// stale reports a registration safe to prune, as the Web UI selects them:
// an unlocked prunable linked worktree. The server checks again that its
// directory is missing.
func stale(t contract.Worktree) bool {
	return t.Kind == "linked" && t.Prunable && !t.Locked
}

var worktreePruneCommand = &Command{
	Name: "worktree prune", Group: GroupWorkspace, Args: "REPO",
	Summary: "Remove the registrations of linked worktrees whose directories are gone; their branches stay.",
	Description: "Selects REPO's unlocked prunable linked worktrees and previews removing them together; the preview goes " +
		"to standard error and the server checks again that each directory is missing. A locked one is left out: unlock " +
		"it in a terminal first. An entry the preview blocks (its directory came back, sessions run in it) is skipped. " +
		"On a terminal the command asks first; without a terminal it needs --yes. Archived sessions of a removed worktree " +
		"are kept. Unlike git worktree prune, only the previewed entries are removed.",
	Arguments: []Item{repoArgument},
	Output: "Lines \"STATUS<TAB>PATH<TAB>BRANCH<TAB>ERROR\", status removed, skipped or failed; with --json, the " +
		"BatchRemoveResult object. With nothing to prune, a note on standard error (and empty results with --json).",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "Also: a registration failed to be removed, or the confirmation was refused."},
		ExitItem{Code: 2, Text: "Also: no terminal to confirm and no --yes, or REPO names nothing."},
	),
	Examples: []Example{
		{Command: "webmux worktree prune .", Text: "Clean up after deleting worktree directories by hand."},
		{Command: "webmux worktree prune . --yes --json | jq -r '.results[] | select(.status != \"removed\") | .path'", Text: "Prune in a script and list what was not removed."},
	},
	SeeAlso: []string{"webmux worktree ls", "webmux worktree rm"},
	Bind: func(fs *flag.FlagSet) Action {
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		asJSON := fs.Bool("json", false, "print the BatchRemoveResult object")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one repository is required")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			ids := []string{}
			for _, t := range repo.Worktrees {
				if stale(t) {
					ids = append(ids, t.ID)
				}
			}
			if len(ids) == 0 {
				_, _ = fmt.Fprintf(e.Stderr, "webmux: %s has no stale worktree registrations to prune\n", repo.Name)
				if *asJSON {
					writeJSON(e.Stdout, contract.BatchRemoveResult{Results: []contract.BatchRemoveItemResult{}})
				}
				return contract.ExitOK
			}
			base := "/api/repositories/" + url.PathEscape(repo.ID) + "/worktrees"
			var preview contract.BatchRemovalPreview
			if err := client.Do(ctx, http.MethodPost, base+"/batch-removal-preview", contract.BatchRemovalPreviewRequest{WorktreeIDs: ids}, &preview); err != nil {
				return e.Fail(err)
			}
			for _, item := range preview.Items {
				e.printPruneItem(item)
			}
			if !*yes {
				if code, ok := e.confirm(fmt.Sprintf("Remove %d stale worktree registrations of %s? Branches stay.", len(preview.Items), repo.Name)); !ok {
					return code
				}
			}
			var result contract.BatchRemoveResult
			if err := client.Do(ctx, http.MethodPost, base+"/batch-remove", contract.BatchRemoveWorktreesRequest{ConfirmationID: preview.ConfirmationID}, &result); err != nil {
				return e.Fail(err)
			}
			code = contract.ExitOK
			for _, r := range result.Results {
				if r.Status == "failed" {
					code = contract.ExitError
				}
			}
			if *asJSON {
				writeJSON(e.Stdout, result)
				return code
			}
			for _, r := range result.Results {
				_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\t%s\t%s\n", r.Status, r.Path, r.Branch, r.Error)
			}
			return code
		}
	},
}

// printPruneItem prints one entry of a prune preview to standard error.
func (e Env) printPruneItem(item contract.RemovalPreviewItem) {
	_, _ = fmt.Fprintf(e.Stderr, "webmux: pruning %s (%s)\n", item.Worktree.Path, worktreeName(item.Worktree))
	for _, r := range item.Archived {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   archived session %s (%s) is kept\n", r.ID, sessionName(r))
	}
	for _, b := range item.Blockers {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   skipped: %s\n", b)
	}
}
