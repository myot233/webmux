package workspace

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Service) branches(ctx context.Context, r *repositoryState, repository Repository) (Branches, error) {
	result := Branches{Branches: []Branch{}, Bases: []Base{}, DefaultParent: filepath.Join(filepath.Dir(r.entry.AnchorPath), filepath.Base(r.entry.AnchorPath)+".worktrees")}
	data, err := s.git(ctx, r.entry.AnchorPath, "for-each-ref", "--sort=-committerdate",
		"--format=%(refname)%00%(objectname)%00%(subject)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(committerdate:iso-strict)%00",
		"refs/heads/", "refs/remotes/")
	if err != nil {
		return result, err
	}
	merged, err := s.mergedIntoDefault(ctx, r.entry.AnchorPath, repository)
	if err != nil {
		return result, err
	}
	const refFields = 6
	fields := bytes.Split(data, []byte{0})
	for i := 0; i+refFields-1 < len(fields); i += refFields {
		ref := strings.TrimPrefix(string(fields[i]), "\n")
		head := string(fields[i+1])
		subject := string(fields[i+2])
		committedAt := string(fields[i+5])
		if ref == "" {
			continue
		}
		kind := "remote"
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			kind = "local"
			branch := Branch{Name: name, Head: head, WorktreePaths: []string{}, Subject: subject, CommittedAt: committedAt}
			if upstream := string(fields[i+3]); upstream != "" {
				branch.Upstream = &upstream
				branch.Ahead, branch.Behind, branch.UpstreamGone = parseTrack(string(fields[i+4]))
			}
			for _, tree := range repository.Worktrees {
				if tree.Branch != nil && *tree.Branch == name {
					branch.WorktreePaths = append(branch.WorktreePaths, tree.Path)
				}
			}
			branch.MergedIntoDefault = merged[ref]
			result.Branches = append(result.Branches, branch)
		}
		result.Bases = append(result.Bases, Base{Name: ref, Head: head, Subject: subject, Kind: kind, CommittedAt: committedAt})
	}
	for _, tree := range repository.Worktrees {
		if tree.Kind == "main" && tree.Head != nil {
			result.DefaultBase = tree.Head
			result.DefaultBranch = tree.Branch
			subject, err := s.git(ctx, r.entry.AnchorPath, "log", "-1", "--no-show-signature", "--format=%s", *tree.Head, "--")
			if err != nil {
				return result, err
			}
			result.DefaultBaseSubject = strings.TrimSuffix(string(subject), "\n")
		}
	}
	return result, nil
}

func (s *Service) Branches(ctx context.Context, id string) (Branches, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(id)
	if err != nil {
		return Branches{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return Branches{}, err
	}
	defer release(r)
	repository, err := s.current(ctx, r)
	if err != nil {
		return Branches{}, err
	}
	return s.branches(ctx, r, repository)
}

// recentCommitLimit bounds the start-commit history offered to the browser.
const recentCommitLimit = 50

// maxCommitStarts bounds the tips merged into one history: the local branch
// tip and its freshly fetched upstream tip.
const maxCommitStarts = 2

// RecentCommits lists up to recentCommitLimit commits reachable from any of
// the commit object ids in `from` (a branch tip and optionally its upstream
// tip), newest first, so a fetched upstream commit appears beside local
// history. A requested tip outside the capped window is appended so it stays
// visible. Only full hexadecimal object ids are accepted so a value can never
// act as an option or ref expression.
func (s *Service) RecentCommits(ctx context.Context, id string, from []string) ([]Commit, error) {
	if len(from) == 0 || len(from) > maxCommitStarts {
		return nil, problemf(ErrInvalid, "from 需要 1 到 %d 个完整的提交 ID", maxCommitStarts)
	}
	for _, oid := range from {
		if !isObjectID(oid) {
			return nil, problemf(ErrInvalid, "from 必须是完整的提交 ID")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(id)
	if err != nil {
		return nil, err
	}
	if err := acquire(ctx, r); err != nil {
		return nil, err
	}
	defer release(r)
	if _, err := s.current(ctx, r); err != nil {
		return nil, err
	}
	for _, oid := range from {
		if _, err := s.git(ctx, r.entry.AnchorPath, "rev-parse", "--verify", "--end-of-options", oid+"^{commit}"); err != nil {
			if errorKind(err) != ErrInternal {
				return nil, err
			}
			return nil, problemf(ErrNotFound, "提交不存在")
		}
	}
	args := append([]string{"log", "-n", strconv.Itoa(recentCommitLimit), "--no-show-signature",
		"--format=%H%x00%s%x00%an%x00%cI%x00"}, from...)
	data, err := s.git(ctx, r.entry.AnchorPath, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	commits := parseCommitLog(data)
	// The cap spans the merged walk, so an older requested tip can fall
	// outside it. Resolve any missing tip on its own instead of leaving the
	// browser without a row for the fetched upstream commit.
	seen := make(map[string]bool, len(commits))
	for _, commit := range commits {
		seen[commit.Hash] = true
	}
	var missing []string
	for _, oid := range from {
		if !seen[oid] {
			seen[oid] = true
			missing = append(missing, oid)
		}
	}
	if len(missing) > 0 {
		extra, err := s.describeCommits(ctx, r.entry.AnchorPath, missing)
		if err != nil {
			return nil, err
		}
		commits = append(commits, extra...)
		sort.SliceStable(commits, func(i, j int) bool {
			return commitTime(commits[i].CommittedAt).After(commitTime(commits[j].CommittedAt))
		})
	}
	return commits, nil
}

func parseCommitLog(data []byte) []Commit {
	commits := []Commit{}
	fields := bytes.Split(data, []byte{0})
	for i := 0; i+3 < len(fields); i += 4 {
		commits = append(commits, Commit{
			Hash:        strings.TrimPrefix(string(fields[i]), "\n"),
			Subject:     string(fields[i+1]),
			Author:      string(fields[i+2]),
			CommittedAt: string(fields[i+3]),
		})
	}
	return commits
}

func commitTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func isObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// unsupportedContents is deliberately conservative: removal never traverses or
// cleans another repository, including ignored nested repositories/submodules.
func (s *Service) unsupportedContents(ctx context.Context, tree Worktree) error {
	data, err := s.git(ctx, tree.Path, "ls-files", "--stage", "-z")
	if err != nil {
		return err
	}
	for _, entry := range bytes.Split(data, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("160000 ")) {
			return errors.New("工作树含子模块，请在终端处理")
		}
	}
	return filepath.WalkDir(tree.Path, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return walkErr
		}
		if path == tree.Path {
			return nil
		}
		if entry.Name() == ".git" {
			if filepath.Dir(path) != tree.Path {
				return errors.New("工作树含嵌套 Git 仓库，请在终端处理")
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		if entry.IsDir() {
			// Bare repositories have no .git entry.
			if stat, err := os.Stat(filepath.Join(path, "HEAD")); err == nil && !stat.IsDir() {
				if objects, err := os.Stat(filepath.Join(path, "objects")); err == nil && objects.IsDir() {
					return errors.New("工作树含嵌套 bare 仓库，请在终端处理")
				}
			}
		}
		return nil
	})
}
