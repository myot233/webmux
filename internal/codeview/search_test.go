package codeview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSearchUsesConstantGitCalls(t *testing.T) {
	s, id, root := setup(t)
	gitTest(t, root, "init", "-q")
	for i := range 1000 {
		dir := fmt.Sprintf("dir-%04d", i)
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
		write(t, root, dir+"/file.txt", "content")
	}
	c, err := s.Context(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	script := fmt.Sprintf("#!/bin/sh\nprintf 'call\\n' >> %q\nexec %q \"$@\"\n", log, realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, query := range []string{"absent", "dir-0999/file.txt"} {
		if err := os.WriteFile(log, nil, 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		start := time.Now()
		result, err := s.Search(ctx, id, c.ID, query, false)
		cancel()
		if err != nil || !result.Complete || result.Limited {
			t.Fatalf("search %q: %+v, %v", query, result, err)
		}
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		count := strings.Count(string(calls), "\n")
		if count > 8 {
			t.Fatalf("search launched %d Git commands for 1000 directories", count)
		}
		t.Logf("%q: %d Git commands, %v", query, count, time.Since(start))
		if query == "absent" && len(result.Children) != 0 {
			t.Fatal("unexpected match")
		}
		if query != "absent" && (len(result.Children) != 1 || result.Children[0].Path != query || result.Children[0].Status != "??") {
			t.Fatalf("missing untracked result/status: %+v", result.Children)
		}
	}
}

func TestSearchIgnoreRulesAndBoundaries(t *testing.T) {
	s, id, root := setup(t)
	gitTest(t, root, "init", "-q")
	for _, dir := range []string{"ignored", "nested", "repo", "bare"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, root, ".gitignore", "ignored/\n*.tmp\n!keep.tmp\ntracked.txt\n")
	write(t, root, "nested/.gitignore", "hidden.txt\n")
	for _, path := range []string{"ignored/hidden.txt", "ignored/tracked.txt", "nested/hidden.txt", "nested/visible.txt", "discard.tmp", "keep.tmp", "tracked.txt"} {
		write(t, root, path, "content")
	}
	gitTest(t, root, "add", "-f", "tracked.txt", "ignored/tracked.txt")
	gitTest(t, root, "commit", "-qm", "tracked exceptions")
	write(t, root, "tracked.txt", "modified")
	gitTest(t, root, "init", "-q", "repo")
	write(t, root, "repo/secret.txt", "nested repository")
	gitTest(t, root, "init", "--bare", "-q", "bare")
	outside := t.TempDir()
	write(t, outside, "secret.txt", "external")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	c, err := s.Context(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, include := range []bool{false, true} {
		result, err := s.Search(context.Background(), id, c.ID, "", include)
		if err != nil || !result.Complete {
			t.Fatalf("search: %+v %v", result, err)
		}
		nodes := map[string]Node{}
		for _, n := range result.Children {
			path, err := nodePath(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			nodes[path] = n
		}
		for _, path := range []string{"nested/visible.txt", "keep.tmp", "tracked.txt", "ignored/tracked.txt"} {
			if n, ok := nodes[path]; !ok || n.Ignored {
				t.Errorf("visible path %q: %+v, present=%v", path, n, ok)
			}
		}
		for _, path := range []string{"ignored/hidden.txt", "nested/hidden.txt", "discard.tmp"} {
			n, ok := nodes[path]
			if ok != include || (ok && !n.Ignored) {
				t.Errorf("ignored path %q: %+v, present=%v", path, n, ok)
			}
		}
		for path := range nodes {
			if strings.HasPrefix(path, "repo/") || strings.HasPrefix(path, "bare/") || strings.HasPrefix(path, "link/") || strings.Contains(path, ".git/") {
				t.Errorf("crossed boundary: %q", path)
			}
		}
		if nodes["tracked.txt"].Status != " M" {
			t.Fatalf("tracked status: %+v", nodes["tracked.txt"])
		}
	}
}

func TestSearchRawDirectoryNames(t *testing.T) {
	s, id, root := setup(t)
	if err := os.Mkdir(filepath.Join(root, "bad\xff"), 0700); errors.Is(err, syscall.EILSEQ) {
		t.Skip("filesystem requires UTF-8 names")
	} else if err != nil {
		t.Fatal(err)
	}
	write(t, root, "bad\xff/file.txt", "content")
	c, err := s.Context(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Search(context.Background(), id, c.ID, "file.txt", false)
	if err != nil || len(result.Children) != 1 {
		t.Fatalf("search: %+v %v", result, err)
	}
	path, err := nodePath(result.Children[0].ID)
	if err != nil || path != "bad\xff/file.txt" || result.Children[0].Referenceable {
		t.Fatalf("raw path lost: %q %v", path, err)
	}
}

func TestSearchReportsIgnoreIndexLimit(t *testing.T) {
	s, id, root := setup(t)
	gitTest(t, root, "init", "-q")
	write(t, root, "visible.txt", "content")
	c, err := s.Context(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	records := filepath.Join(bin, "records")
	// A synthetic Git stream exercises both budgets without creating 100,000
	// files or depending on the host's maximum pathname length.
	script := fmt.Sprintf("#!/bin/sh\nfor arg do\n  if [ \"$arg\" = ls-files ]; then exec cat %q; fi\ndone\nexec %q \"$@\"\n", records, realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, size := range []int{20, 600} {
		var stream strings.Builder
		for i := 0; i <= min(100000, MaxBytes/size+1); i++ {
			fmt.Fprintf(&stream, "%0*d\x00", size, i)
		}
		if err := os.WriteFile(records, []byte(stream.String()), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := s.Search(context.Background(), id, c.ID, "", false)
		if err != nil || !result.Limited || result.Complete || len(result.Children) != 0 {
			t.Fatalf("ignore budget %d: %+v %v", size, result, err)
		}
	}
}
