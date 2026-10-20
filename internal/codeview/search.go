package codeview

import (
	"context"
	"errors"
	"strings"

	"github.com/rainchan/webmux/internal/workspace"
)

type ignoredPaths map[string]bool

func (paths ignoredPaths) contains(path string) bool {
	for {
		if paths[path] {
			return true
		}
		i := strings.LastIndexByte(path, '/')
		if i < 0 {
			return false
		}
		path = path[:i]
	}
}

// Git collapses wholly ignored directories, so the filesystem walk can skip
// them before spending its traversal budget. Tracked files remain visible.
// A bounded index and one final status query replace per-directory Git calls.
func (b *binding) searchIgnored(ctx context.Context) (ignoredPaths, bool, error) {
	paths := ignoredPaths{}
	if !b.Ignore {
		return paths, false, nil
	}
	limitReached := errors.New("search ignore budget")
	bytes := 0
	err := workspace.ReadGitRecords(ctx, b.Root, func(path string) error {
		bytes += len(path)
		if len(paths) >= 100000 || bytes > MaxBytes {
			return limitReached
		}
		paths[strings.TrimSuffix(path, "/")] = true
		return nil
	}, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if errors.Is(err, limitReached) {
		return nil, true, nil
	}
	return paths, false, err
}
