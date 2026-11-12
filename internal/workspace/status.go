package workspace

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// worktreeStatus is what one `git status --porcelain=v2 -z --branch` reports.
// Dirty, untracked and ignored keep the removal checks' meaning: dirty is any
// tracked change (ordinary, renamed or unmerged entry).
type worktreeStatus struct {
	oid      string // "(initial)" before the first commit
	head     string // "(detached)" without a branch
	upstream *string
	ahead    *int
	behind   *int
	// upstreamGone: an upstream is configured but has no branch.ab line,
	// because its ref is missing.
	upstreamGone bool
	changes      WorktreeChanges
	dirty        bool
	untracked    bool
	ignored      bool
}

// parseStatus reads porcelain v2 records. A renamed or copied entry ("2")
// is followed by an extra NUL-terminated original path, which is skipped.
func parseStatus(data []byte) (worktreeStatus, error) {
	var status worktreeStatus
	if len(data) == 0 {
		return status, nil
	}
	if data[len(data)-1] != 0 {
		return status, errors.New("状态输出不是 NUL 格式（来自 Git）")
	}
	abSeen := false
	records := bytes.Split(data[:len(data)-1], []byte{0})
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 3 || record[1] != ' ' {
			return status, errors.New("状态记录无效（来自 Git）")
		}
		switch record[0] {
		case '#':
			key, value, _ := strings.Cut(string(record[2:]), " ")
			switch key {
			case "branch.oid":
				status.oid = value
			case "branch.head":
				status.head = value
			case "branch.upstream":
				status.upstream = strptr(value)
			case "branch.ab":
				abSeen = true
				status.ahead, status.behind = parseAheadBehind(value)
			}
		case '1', '2', 'u':
			if len(record) < 5 || record[4] != ' ' {
				return status, errors.New("状态记录无效（来自 Git）")
			}
			status.dirty = true
			if record[0] == 'u' {
				status.changes.Conflicted++
				continue
			}
			if record[2] != '.' {
				status.changes.Staged++
			}
			if record[3] != '.' {
				status.changes.Unstaged++
			}
			if record[0] == '2' {
				i++
				if i >= len(records) {
					return status, errors.New("重命名状态记录不完整（来自 Git）")
				}
			}
		case '?':
			status.untracked = true
			status.changes.Untracked++
		case '!':
			status.ignored = true
		default:
			return status, errors.New("状态记录无效（来自 Git）")
		}
	}
	status.upstreamGone = status.upstream != nil && !abSeen
	return status, nil
}

// parseAheadBehind reads "+A -B"; "+? -?" (--no-ahead-behind) is not
// computed and yields nil counts.
func parseAheadBehind(value string) (*int, *int) {
	plus, minus, ok := strings.Cut(value, " ")
	if !ok || !strings.HasPrefix(plus, "+") || !strings.HasPrefix(minus, "-") {
		return nil, nil
	}
	ahead, aheadErr := strconv.Atoi(plus[1:])
	behind, behindErr := strconv.Atoi(minus[1:])
	if aheadErr != nil || behindErr != nil {
		return nil, nil
	}
	return &ahead, &behind
}

// parseTrack reads for-each-ref's %(upstream:track,nobracket) for a branch
// with an upstream; LC_ALL=C keeps its words fixed. Empty means in sync and
// "gone" means the upstream ref is missing.
func parseTrack(track string) (ahead, behind *int, gone bool) {
	if track == "gone" {
		return nil, nil, true
	}
	a, b := 0, 0
	if track != "" {
		for _, part := range strings.Split(track, ", ") {
			word, count, ok := strings.Cut(part, " ")
			n, err := strconv.Atoi(count)
			if !ok || err != nil {
				return nil, nil, false
			}
			switch word {
			case "ahead":
				a = n
			case "behind":
				b = n
			default:
				return nil, nil, false
			}
		}
	}
	return &a, &b, false
}

// operationMarkers are the files Git keeps in a worktree's own git directory
// while an operation is in progress, in the order they are reported.
var operationMarkers = []struct{ file, operation string }{
	{"MERGE_HEAD", "merge"},
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
	{"BISECT_LOG", "bisect"},
}

// maxGitFile bounds reading a worktree's .git file, which holds one path.
const maxGitFile = 64 << 10

// worktreeGitDir finds a worktree's git directory without running Git: .git
// itself, or the "gitdir: " target of a .git file, relative to the worktree.
// Like Git, the whole file minus trailing line ends is the path, so paths
// with newlines survive.
func worktreeGitDir(path string) (string, error) {
	dotGit := filepath.Join(path, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return dotGit, nil
	}
	file, err := os.Open(dotGit)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxGitFile+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxGitFile {
		return "", errors.New(".git 文件过大")
	}
	target, ok := strings.CutPrefix(strings.TrimRight(string(data), "\r\n"), "gitdir: ")
	if !ok || target == "" {
		return "", errors.New(".git 文件缺少 gitdir")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(path, target)
	}
	return target, nil
}

// detectOperation reports the Git operation in progress in a worktree, or
// nil for none or when its git directory cannot be read.
func detectOperation(path string) *string {
	dir, err := worktreeGitDir(path)
	if err != nil {
		return nil
	}
	for _, marker := range operationMarkers {
		if _, err := os.Lstat(filepath.Join(dir, marker.file)); err == nil {
			return strptr(marker.operation)
		}
	}
	return nil
}

// lastFetched is the newest FETCH_HEAD modification time of the repository
// and its linked worktrees, which keep their own under worktrees/<name>.
func lastFetched(commonDir string) *time.Time {
	var newest time.Time
	consider := func(path string) {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	consider(filepath.Join(commonDir, "FETCH_HEAD"))
	entries, _ := os.ReadDir(filepath.Join(commonDir, "worktrees"))
	for _, entry := range entries {
		if entry.IsDir() {
			consider(filepath.Join(commonDir, "worktrees", entry.Name(), "FETCH_HEAD"))
		}
	}
	if newest.IsZero() {
		return nil
	}
	newest = newest.UTC()
	return &newest
}
