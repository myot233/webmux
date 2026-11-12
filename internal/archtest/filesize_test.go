package archtest

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maxSourceLines is the strict upper bound (< 500 lines, i.e. <= 499) for a
// non-test Go source file.
const maxSourceLines = 499

// TestSourceFileSize keeps every non-test Go source file under 500 lines so
// responsibilities stay focused and navigable.
func TestSourceFileSize(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lines := countLines(raw)
			if lines > maxSourceLines {
				rel, _ := filepath.Rel(root, path)
				t.Errorf(
					"%s has %d lines (limit: < 500 lines).\n"+
						"Split this file by domain responsibility or architectural sub-concern into focused sibling files (or a dedicated package if appropriate):\n"+
						"  1. Group cohesive types, methods, or subcommands by the noun or lifecycle stage they own.\n"+
						"  2. Name new files in snake_case after that responsibility (docs/agent-rules/naming.md); never use catch-all names like util, utils, helper, helpers, misc, more, common, extra, or part2.\n"+
						"  3. Do not delete comments/docstrings or compress formatting onto single lines just to pass the line count.",
					filepath.ToSlash(rel), lines,
				)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte{'\n'})
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}
