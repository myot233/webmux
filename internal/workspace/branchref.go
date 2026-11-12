package workspace

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

// maxListedCommits bounds the commits a branch operation preview lists.
const maxListedCommits = 20

// Blocker and warning codes of branch operation previews, besides the
// shared ones of a switch.
const (
	issueMissing    = "missing"
	issueUpstream   = "upstream"
	issueDiverged   = "diverged"
	issueAhead      = "ahead"
	issueBehind     = "behind"
	issueRemote     = "remote"
	issueGone       = "upstreamGone"
	issuePushRemote = "pushRemote"
)

// CommitList is the first maxListedCommits commits of a range, newest
// first, and how many the range holds.
type CommitList struct {
	Total int
	Items []Commit
}

// localBranch is a local branch and its configured upstream, read in one
// for-each-ref.
type localBranch struct {
	name   string
	commit string
	// upstream is the upstream's full refname and upstreamShort its short
	// name; both "" without a configured upstream.
	upstream      string
	upstreamShort string
	// upstreamCommit is the upstream tip, "" without one or when gone.
	upstreamCommit string
	// remote and merge are branch.<name>.remote and .merge.
	remote string
	merge  string
	// pushRemote is where a plain git push would go: branch.<name>.pushRemote,
	// remote.pushDefault, then remote.
	pushRemote string
}

// readBranch reads local branch name; a malformed name is ErrInvalid and a
// missing branch ErrNotFound.
func (s *Service) readBranch(ctx context.Context, anchor, name string) (localBranch, error) {
	if err := s.validBranch(ctx, anchor, name); err != nil {
		return localBranch{}, err
	}
	ref := "refs/heads/" + name
	data, err := s.git(ctx, anchor, "for-each-ref",
		"--format=%(refname)%00%(objectname)%00%(upstream)%00%(upstream:short)%00%(upstream:remotename)%00%(upstream:remoteref)%00%(push:remotename)%00", ref)
	if err != nil {
		return localBranch{}, err
	}
	// The pattern also matches refs below ref, so only the exact name counts.
	const branchFields = 7
	fields := bytes.Split(data, []byte{0})
	for i := 0; i+branchFields-1 < len(fields); i += branchFields {
		if strings.TrimPrefix(string(fields[i]), "\n") != ref {
			continue
		}
		b := localBranch{
			name: name, commit: string(fields[i+1]), upstream: string(fields[i+2]), upstreamShort: string(fields[i+3]),
			remote: string(fields[i+4]), merge: string(fields[i+5]), pushRemote: string(fields[i+6]),
		}
		if b.upstream != "" {
			if b.upstreamCommit, err = s.resolveCommit(ctx, anchor, b.upstream); err != nil {
				return localBranch{}, err
			}
		}
		return b, nil
	}
	return localBranch{}, problemf(ErrNotFound, "分支不存在: %s", name)
}

// aheadBehind counts the commits of from missing in to and of to missing
// in from; both are full OIDs.
func (s *Service) aheadBehind(ctx context.Context, anchor, from, to string) (int, int, error) {
	data, err := s.git(ctx, anchor, "rev-list", "--left-right", "--count", from+"..."+to, "--")
	if err != nil {
		return 0, 0, err
	}
	left, right, _ := strings.Cut(strings.TrimSpace(string(data)), "\t")
	ahead, aheadErr := strconv.Atoi(left)
	behind, behindErr := strconv.Atoi(right)
	if aheadErr != nil || behindErr != nil {
		return 0, 0, problemf(ErrInternal, "无法解析 Git 的提交计数: %q", data)
	}
	return ahead, behind, nil
}

// listCommits lists the commits rev-list selects with revisions, full OIDs
// and options this package builds: at most maxListedCommits and the total.
func (s *Service) listCommits(ctx context.Context, anchor string, revisions ...string) (CommitList, error) {
	count, err := s.git(ctx, anchor, append(append([]string{"rev-list", "--count"}, revisions...), "--")...)
	if err != nil {
		return CommitList{}, err
	}
	total, err := strconv.Atoi(strings.TrimSpace(string(count)))
	if err != nil {
		return CommitList{}, problemf(ErrInternal, "无法解析 Git 的提交计数: %q", count)
	}
	list := CommitList{Total: total, Items: []Commit{}}
	if total == 0 {
		return list, nil
	}
	args := append([]string{"log", "-n", strconv.Itoa(maxListedCommits), "--no-show-signature", "--format=%H%x00%s%x00%an%x00%cI%x00"}, revisions...)
	data, err := s.git(ctx, anchor, append(args, "--")...)
	if err != nil {
		return CommitList{}, err
	}
	list.Items = parseCommitLog(data)
	return list, nil
}

// mergedIntoDefault lists the local branches, by full refname, that the
// main worktree's HEAD contains, with one for-each-ref; none for a bare or
// unborn main worktree.
func (s *Service) mergedIntoDefault(ctx context.Context, anchor string, repository Repository) (map[string]bool, error) {
	merged := map[string]bool{}
	for _, tree := range repository.Worktrees {
		if tree.Kind != "main" || tree.Head == nil || !isObjectID(*tree.Head) {
			continue
		}
		data, err := s.git(ctx, anchor, "for-each-ref", "--merged="+*tree.Head, "--format=%(refname)", "refs/heads/")
		if err != nil {
			return nil, err
		}
		for _, ref := range strings.Split(string(data), "\n") {
			if ref != "" {
				merged[ref] = true
			}
		}
	}
	return merged, nil
}
