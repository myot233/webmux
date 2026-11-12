package workspace

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// maxOverwrittenPaths bounds the paths listed when Git refuses to overwrite
// files.
const maxOverwrittenPaths = 50

// maxGitLine bounds the Git output line quoted in an error.
const maxGitLine = 300

// switchArgs is the git switch argv of plan; names are validated, and
// commits are full OIDs. It never forces, merges or ignores other
// worktrees.
func switchArgs(plan switchPlan) []string {
	switch plan.target.Kind {
	case SwitchLocal:
		return []string{"switch", "--no-guess", "--end-of-options", plan.target.Branch}
	case SwitchRemote:
		return []string{"switch", "--no-guess", "--track", "--create", *plan.info.Branch, "--end-of-options", "refs/remotes/" + plan.target.Ref}
	case SwitchNew:
		return []string{"switch", "--no-guess", "--no-track", "--create", *plan.info.Branch, "--end-of-options", plan.info.Commit}
	default:
		return []string{"switch", "--detach", "--end-of-options", plan.info.Commit}
	}
}

// switchFailure explains a failed git switch. before is the worktree Git
// started from and after the state re-read since (unless refreshErr).
func switchFailure(err error, stderr string, before, after Worktree, refreshErr error) error {
	switch errorKind(err) {
	case ErrTimeout:
		return problemf(ErrTimeout, "切换超过 %v 未完成，Git 已被终止，文件可能只更新了一部分；请在终端检查实际状态", switchTimeout)
	case ErrInternal:
	default:
		return err
	}
	if paths, more := overwrittenPaths(stderr); len(paths) != 0 {
		message := "以下文件会被切换覆盖，未切换: " + strings.Join(paths, ", ")
		if more != 0 {
			message += fmt.Sprintf(" 等另外 %d 个", more)
		}
		return problemf(ErrConflict, "%s", message)
	}
	// Redact before shortening, so a cut URL cannot keep half a secret.
	line := firstGitLine(redactLine(stderr))
	if refreshErr == nil && (!sameText(before.Branch, after.Branch) || !sameText(before.Head, after.Head) || before.Detached != after.Detached) {
		return problemf(ErrConflict, "检出已完成，但 Git 钩子或过滤器报告失败，请在终端检查: %s", line)
	}
	return problemf(ErrConflict, "Git 未完成切换，请刷新后在终端检查: %s", line)
}

// overwrittenLine is Git's one-line form of a refused overwrite.
var overwrittenLine = regexp.MustCompile(`'([^']+)' would be overwritten by (?:checkout|merge)`)

// overwrittenPaths lists the paths of Git's "would be overwritten by
// checkout" or "by merge" refusals, at most maxOverwrittenPaths, and counts
// the rest.
func overwrittenPaths(output string) ([]string, int) {
	var paths []string
	seen := map[string]bool{}
	more := 0
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		if len(paths) < maxOverwrittenPaths {
			paths = append(paths, path)
		} else {
			more++
		}
	}
	listing := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasSuffix(line, "would be overwritten by checkout:") || strings.HasSuffix(line, "would be overwritten by merge:") {
			listing = true
			continue
		}
		if path, ok := strings.CutPrefix(line, "\t"); listing && ok {
			add(path)
			continue
		}
		listing = false
		if match := overwrittenLine.FindStringSubmatch(line); match != nil {
			add(match[1])
		}
	}
	return paths, more
}

// gitProgress are the lines git switch reports on success, which do not
// explain a failure.
var gitProgress = []string{"Switched to ", "Already on ", "Your branch ", "HEAD is now at ", "Previous HEAD position ", "branch '"}

// firstGitLine is the first line of Git's output that is not progress,
// shortened to maxGitLine bytes.
func firstGitLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || slices.ContainsFunc(gitProgress, func(prefix string) bool { return strings.HasPrefix(line, prefix) }) {
			continue
		}
		if len(line) > maxGitLine {
			line = strings.ToValidUTF8(line[:maxGitLine], "") + "…"
		}
		return line
	}
	return ""
}

// urlInText finds URLs in Git output.
var urlInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s'"<>]+`)

// redactLine hides the credentials of every URL in line.
func redactLine(line string) string {
	return urlInText.ReplaceAllStringFunc(line, redactURL)
}
