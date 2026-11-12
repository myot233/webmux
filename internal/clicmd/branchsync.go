package clicmd

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// maxDeleteBranches mirrors the server's limit on one branch delete.
const maxDeleteBranches = 50

var branchArgument = Item{Name: "BRANCH", Text: "A local branch of REPO."}

func branchesURL(repo contract.Repository) string {
	return "/api/repositories/" + url.PathEscape(repo.ID) + "/branches"
}

var repoFastForwardCommand = &Command{
	Name: "repo ff", Group: GroupWorkspace, Args: "REPO BRANCH",
	Summary: "Fast-forward a local branch to its upstream after previewing the commits it gains.",
	Description: "Uses the remote-tracking branch as last fetched, so run webmux repo fetch first for the newest; it never " +
		"fetches, merges or rebases. The preview goes to standard error: the commits, the worktree checking the branch " +
		"out and its running sessions, and what blocks it (no upstream, ahead of or diverged from it, uncommitted changes, " +
		"a merge or rebase in progress). With blockers nothing changes. A branch no worktree checks out only has its ref " +
		"moved. A checked-out branch is updated in its worktree; the sessions there keep running and see the new files, " +
		"so with running sessions it asks you to type yes on a terminal, and needs --yes without one. Untracked files the " +
		"upstream would overwrite make it fail, unchanged. Once Git starts, interrupting the command does not stop it.",
	Arguments: []Item{repoArgument, branchArgument},
	Output:    "\"BRANCH<TAB>COMMIT\", the commit the branch moved to; with --json, the FastForwardResult object (the refreshed Repository).",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "Also: blockers, the preview went stale, or Git refused (files would be overwritten, a hook failed)."},
		ExitItem{Code: 2, Text: "Also: running sessions, no terminal to confirm and no --yes, or REPO names nothing."},
	),
	Examples: []Example{
		{Command: "webmux repo fetch . && webmux repo ff . main", Text: "Bring main up to date with its upstream."},
		{Command: "webmux repo ff . feat/a --yes", Text: "Fast-forward in a script, even with sessions running in its worktree."},
	},
	SeeAlso: []string{"webmux repo fetch", "webmux repo branches", "webmux repo push"},
	Bind: func(fs *flag.FlagSet) Action {
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		asJSON := fs.Bool("json", false, "print the FastForwardResult object")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 {
				return e.Usagef("a repository and a branch are required")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			base := branchesURL(repo)
			var preview contract.FastForwardPreview
			if err := client.Do(ctx, http.MethodPost, base+"/fast-forward-preview", contract.FastForwardPreviewRequest{Branch: args[1]}, &preview); err != nil {
				return e.Fail(err)
			}
			e.printFastForwardPreview(preview)
			if len(preview.Blockers) > 0 {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: not fast-forwarded")
				return contract.ExitError
			}
			if len(preview.Sessions) > 0 && !*yes {
				if code, ok := e.confirmYes("Type yes to fast-forward while these sessions run: "); !ok {
					return code
				}
			}
			request := contract.FastForwardRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeSessions: len(preview.Sessions) > 0}
			var result contract.FastForwardResult
			if err := client.Do(ctx, http.MethodPost, base+"/fast-forward", request, &result); err != nil {
				return e.Fail(err)
			}
			if *asJSON {
				writeJSON(e.Stdout, result)
			} else {
				_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\n", preview.Branch, preview.To)
			}
			return contract.ExitOK
		}
	},
}

func (e Env) printFastForwardPreview(p contract.FastForwardPreview) {
	_, _ = fmt.Fprintf(e.Stderr, "webmux: fast-forwarding %s from %s to %s (%s), %d commits\n", p.Branch, shortHash(p.From), shortHash(p.To), p.Upstream, p.Commits.Total)
	e.printCommits(p.Commits)
	if p.Worktree != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   checked out at %s\n", p.Worktree.Path)
	}
	for _, s := range p.Sessions {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   session %s (%s) %s\n", s.ID, s.Status, s.Title)
	}
	e.printIssues(p.Blockers, p.Warnings)
}

var repoPushCommand = &Command{
	Name: "repo push", Group: GroupWorkspace, Args: "REPO BRANCH",
	Summary: "Push a local branch to its upstream, or publish one without an upstream, after a preview.",
	Description: "A branch with an upstream is pushed there when it is ahead and not behind. A branch without one, or whose " +
		"upstream branch was deleted, is published: to --remote, by default remote.pushDefault, origin or the only remote, " +
		"as --to (default: its own name), which then becomes its upstream. The preview goes to standard error: the target, " +
		"its push URL with credentials redacted, and the commits to push. It always asks you to type yes on a terminal and " +
		"needs --yes without one. The previewed commit is pushed, never a later one: a branch that moved meanwhile needs a " +
		"new preview. It never forces, deletes remote branches or pushes tags, and pre-push hooks run. Git does not prompt: " +
		"a remote needing a password, a token or a host key confirmation fails as auth; once Git starts, interrupting the " +
		"command does not stop it.",
	Arguments: []Item{repoArgument, branchArgument},
	Output: "\"STATUS<TAB>REMOTE/TARGET<TAB>MESSAGE\", status ok, auth, network, timeout, rejected or error; with --json, the " +
		"PushResult object (status, message and the refreshed Repository).",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "Also: blockers, the preview went stale, or the push failed (rejected, auth, network, timeout)."},
		ExitItem{Code: 2, Text: "Also: no terminal to confirm and no --yes, --remote or --to for a branch with an upstream, no remote decided, or REPO names nothing."},
	),
	Examples: []Example{
		{Command: "webmux repo push . feat/a", Text: "Push the commits feat/a is ahead of its upstream."},
		{Command: "webmux repo push . feat/c --yes", Text: "Publish an agent's new branch to origin in a script."},
		{Command: "webmux repo push . feat/c --remote fork --to review/c --yes", Text: "Publish under another name on another remote."},
	},
	SeeAlso: []string{"webmux repo fetch", "webmux repo remotes", "webmux repo ff"},
	Bind: func(fs *flag.FlagSet) Action {
		remote := fs.String("remote", "", "publish to `REMOTE` (only for a branch without an upstream)")
		to := fs.String("to", "", "publish as the remote branch `NAME` (default: the branch's name)")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		asJSON := fs.Bool("json", false, "print the PushResult object")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 {
				return e.Usagef("a repository and a branch are required")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			if *remote != "" || *to != "" {
				if code, ok := e.publishable(ctx, client, repo, args[1]); !ok {
					return code
				}
			}
			base := branchesURL(repo)
			var preview contract.PushPreview
			if err := client.Do(ctx, http.MethodPost, base+"/push-preview", contract.PushPreviewRequest{Branch: args[1], Remote: *remote, Target: *to}, &preview); err != nil {
				return e.Fail(err)
			}
			e.printPushPreview(preview)
			for _, b := range preview.Blockers {
				if b.Code == "remote" {
					return e.Usagef("choose the remote with --remote")
				}
			}
			if len(preview.Blockers) > 0 {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: not pushed")
				return contract.ExitError
			}
			if !*yes {
				if code, ok := e.confirmYes("Type yes to push: "); !ok {
					return code
				}
			}
			var result contract.PushResult
			if err := client.Do(ctx, http.MethodPost, base+"/push", contract.PushRequest{ConfirmationID: preview.ConfirmationID}, &result); err != nil {
				return e.Fail(err)
			}
			code = contract.ExitOK
			if result.Status != "ok" {
				code = contract.ExitError
			}
			if *asJSON {
				writeJSON(e.Stdout, result)
				return code
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s\t%s/%s\t%s\n", result.Status, preview.Remote, preview.Target, result.Message)
			return code
		}
	},
}

// publishable refuses --remote and --to for a branch with an existing
// upstream, which is always pushed there.
func (e Env) publishable(ctx context.Context, client *localclient.Client, repo contract.Repository, branch string) (int, bool) {
	branches, err := branchesOf(ctx, client, repo)
	if err != nil {
		return e.Fail(err), false
	}
	for _, b := range branches.Branches {
		if b.Name == branch && b.Upstream != nil && !b.UpstreamGone {
			return e.Usagef("%s tracks %s and is pushed there: --remote and --to only publish a branch without an upstream", branch, *b.Upstream), false
		}
	}
	return contract.ExitOK, true
}

func (e Env) printPushPreview(p contract.PushPreview) {
	verb := "pushing"
	if p.Publish {
		verb = "publishing"
	}
	_, _ = fmt.Fprintf(e.Stderr, "webmux: %s %s at %s to %s/%s (%s), %d commits\n", verb, p.Branch, shortHash(p.Commit), p.Remote, p.Target, p.PushURL, p.Commits.Total)
	e.printCommits(p.Commits)
	e.printIssues(p.Blockers, p.Warnings)
}

var repoBranchRemoveCommand = &Command{
	Name: "repo branch-rm", Group: GroupWorkspace, Args: "REPO (BRANCH... | --cleanup)",
	Summary: "Delete local branches after previewing which ones have commits no other ref keeps.",
	Description: "Give branches, or --cleanup for every branch the Web UI offers to clean up: not checked out by any " +
		"worktree, and merged into the default branch or with its upstream branch deleted (at most 50 at once). The " +
		"preview goes to standard error in two groups: contained branches, whose commits another branch, remote-tracking " +
		"branch, tag, worktree HEAD or WebMux protection ref still reaches, and unmerged ones, listing the commits only " +
		"they keep (a squash-merged branch is one). Unmerged branches are deleted only with --unmerged and are skipped " +
		"otherwise; checked-out branches never are. On a terminal the command asks first; without a terminal it needs " +
		"--yes. A branch that moved since the preview is skipped. Each branch's configuration goes with it, remote " +
		"branches stay, and a deleted branch's line names the command restoring it, which works until Git's garbage " +
		"collection removes unreachable commits.",
	Arguments: []Item{repoArgument, {Name: "BRANCH", Text: "A local branch of REPO to delete; repeatable."}},
	Output: "Lines \"STATUS<TAB>BRANCH<TAB>COMMIT<TAB>MESSAGE\", status deleted, skipped or failed; with --json, the " +
		"BranchDeleteResult object. With nothing to clean up, a note on standard error (and empty results with --json).",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "Also: a branch failed to be deleted, nothing could be deleted, or the confirmation was refused."},
		ExitItem{Code: 2, Text: "Also: no terminal to confirm and no --yes, branches and --cleanup together, or REPO names nothing."},
	),
	Examples: []Example{
		{Command: "webmux repo branch-rm . --cleanup", Text: "Delete the branches left behind by removed worktrees."},
		{Command: "webmux repo branch-rm . feat/sq --unmerged --yes", Text: "Delete a squash-merged branch in a script."},
	},
	SeeAlso: []string{"webmux repo branches", "webmux worktree rm", "webmux worktree prune"},
	Bind: func(fs *flag.FlagSet) Action {
		cleanup := fs.Bool("cleanup", false, "select every clean-up candidate")
		unmerged := fs.Bool("unmerged", false, "also delete branches with commits no other ref keeps")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		asJSON := fs.Bool("json", false, "print the BranchDeleteResult object")
		return func(ctx context.Context, e Env, args []string) int {
			switch {
			case len(args) == 0:
				return e.Usagef("a repository is required")
			case *cleanup && len(args) > 1:
				return e.Usagef("give branches or --cleanup, not both")
			case !*cleanup && len(args) == 1:
				return e.Usagef("give branches to delete, or --cleanup")
			case len(args)-1 > maxDeleteBranches:
				return e.Usagef("at most %d branches at once", maxDeleteBranches)
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			names := args[1:]
			if *cleanup {
				branches, err := branchesOf(ctx, client, repo)
				if err != nil {
					return e.Fail(err)
				}
				names = cleanupCandidates(branches)
				if len(names) == 0 {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: %s has no branches to clean up\n", repo.Name)
					if *asJSON {
						writeJSON(e.Stdout, contract.BranchDeleteResult{Results: []contract.BranchDeleteItemResult{}, Repository: repo})
					}
					return contract.ExitOK
				}
				if len(names) > maxDeleteBranches {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: %d candidates; previewing the first %d, run again for the rest\n", len(names), maxDeleteBranches)
					names = names[:maxDeleteBranches]
				}
			}
			base := branchesURL(repo)
			var preview contract.BranchDeletePreview
			if err := client.Do(ctx, http.MethodPost, base+"/delete-preview", contract.BranchDeletePreviewRequest{Branches: names}, &preview); err != nil {
				return e.Fail(err)
			}
			deleting, groups := e.printBranchDeletePreview(preview, *unmerged)
			if preview.ConfirmationID == "" {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: none of these branches can be deleted")
				return contract.ExitError
			}
			if !*yes {
				if code, ok := e.confirm(fmt.Sprintf("Delete %d local branches of %s? Remote branches stay.", deleting, repo.Name)); !ok {
					return code
				}
			}
			var result contract.BranchDeleteResult
			request := contract.BranchDeleteRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeUnmerged: *unmerged}
			if err := client.Do(ctx, http.MethodPost, base+"/delete", request, &result); err != nil {
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
				message := r.Message
				if r.Status == "skipped" && groups[r.Branch] == "unmerged" && !*unmerged {
					message += " (repeat with --unmerged to delete it)"
				}
				_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\t%s\t%s\n", r.Status, r.Branch, r.Commit, message)
			}
			return code
		}
	},
}

// cleanupCandidates are the branches the Web UI offers to clean up: not
// checked out, and merged into the default branch or with their upstream
// branch gone.
func cleanupCandidates(branches contract.Branches) []string {
	var names []string
	for _, b := range branches.Branches {
		if len(b.WorktreePaths) == 0 && (b.UpstreamGone || b.MergedIntoDefault) {
			names = append(names, b.Name)
		}
	}
	return names
}

// printBranchDeletePreview prints the preview by group to standard error
// and returns how many branches the confirmed request deletes and each
// branch's group.
func (e Env) printBranchDeletePreview(p contract.BranchDeletePreview, unmerged bool) (int, map[string]string) {
	deleting := 0
	groups := map[string]string{}
	for _, item := range p.Items {
		groups[item.Branch] = item.Group
		switch {
		case len(item.Blockers) > 0:
			for _, b := range item.Blockers {
				if b.Path != "" {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: blocked %s: %s (%s)\n", item.Branch, b.Message, b.Path)
				} else {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: blocked %s: %s\n", item.Branch, b.Message)
				}
			}
		case item.Group == "unmerged":
			note := "skipped without --unmerged"
			if unmerged {
				note, deleting = "deleted with --unmerged", deleting+1
			}
			_, _ = fmt.Fprintf(e.Stderr, "webmux: unmerged %s at %s: %d commits only it keeps, %s\n", item.Branch, shortHash(item.Commit), item.Unmerged.Total, note)
			e.printCommits(item.Unmerged)
		default:
			deleting++
			_, _ = fmt.Fprintf(e.Stderr, "webmux: contained %s at %s\n", item.Branch, shortHash(item.Commit))
		}
	}
	return deleting, groups
}

// printCommits lists a preview's commits to standard error.
func (e Env) printCommits(list contract.CommitList) {
	for _, c := range list.Items {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   %s %s\n", shortHash(c.Hash), oneLine(c.Subject, 60))
	}
	if more := list.Total - len(list.Items); more > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   and %d more\n", more)
	}
}

func (e Env) printIssues(blockers, warnings []contract.Issue) {
	for _, b := range blockers {
		if b.Path != "" {
			_, _ = fmt.Fprintf(e.Stderr, "webmux:   blocked: %s (%s)\n", b.Message, b.Path)
		} else {
			_, _ = fmt.Fprintf(e.Stderr, "webmux:   blocked: %s\n", b.Message)
		}
	}
	for _, w := range warnings {
		_, _ = fmt.Fprintf(e.Stderr, "webmux:   warning: %s\n", w.Message)
	}
}
