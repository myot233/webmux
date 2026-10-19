package codeview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestConcurrencyGateBeforeGitValidation(t *testing.T) {
	s, id, root := setup(t)
	gitTest(t, root, "init", "-q")
	write(t, root, "a.txt", "content\n")
	c, err := s.Context(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.File(context.Background(), id, c.ID, nodeID("a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "calls")
	// Only trusted test paths are interpolated into this subprocess wrapper.
	script := fmt.Sprintf("#!/bin/sh\nprintf 'call\\n' >> %q\nexec %q \"$@\"\n", marker, realGit)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	operations := []struct {
		name string
		run  func(context.Context) error
	}{
		{"Context", func(ctx context.Context) error { _, err := s.Context(ctx, id); return err }},
		{"File", func(ctx context.Context) error { _, err := s.File(ctx, id, c.ID, nodeID("a.txt")); return err }},
		{"FileSource", func(ctx context.Context) error {
			_, err := s.FileSource(ctx, id, c.ID, nodeID("a.txt"), "worktree")
			return err
		}},
		{"SnapshotFile", func(ctx context.Context) error { _, err := s.SnapshotFile(ctx, id, f.Snapshot, ""); return err }},
		{"Tree", func(ctx context.Context) error { _, err := s.Tree(ctx, id, c.ID, "", false, ""); return err }},
		{"Search", func(ctx context.Context) error { _, err := s.Search(ctx, id, c.ID, "a", false); return err }},
		{"Changes", func(ctx context.Context) error { _, err := s.Changes(ctx, id, c.ID); return err }},
		{"Diff", func(ctx context.Context) error { _, err := s.Diff(ctx, id, c.ID, "missing"); return err }},
		{"Check", func(ctx context.Context) error { _, err := s.Check(ctx, id, CheckRequest{Context: c.ID}); return err }},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			s.slots <- struct{}{}
			s.slots <- struct{}{}
			defer func() { <-s.slots; <-s.slots }()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if err := op.run(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("queued operation returned %v, want deadline exceeded", err)
			}
			if calls, err := os.ReadFile(marker); err == nil {
				t.Errorf("Git started with both slots occupied: %q", calls)
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}
}

func TestConcurrencyGateBeforeFilesystemValidation(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached=%t", cached), func(t *testing.T) {
			s, id, root := setup(t)
			if cached {
				if _, err := s.Context(context.Background(), id); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
			s.slots <- struct{}{}
			s.slots <- struct{}{}
			defer func() { <-s.slots; <-s.slots }()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if _, err := s.Context(ctx, id); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("filesystem validation ran before acquisition: %v", err)
			}
		})
	}
}

func TestConcurrencyGateResumesAfterRelease(t *testing.T) {
	s, id, root := setup(t)
	write(t, root, "a.txt", "content\n")
	c, err := s.Context(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	defer func() { <-s.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		f, err := s.File(ctx, id, c.ID, nodeID("a.txt"))
		if err == nil && f.Document.Text != "content\n" {
			err = fmt.Errorf("file content: %q", f.Document.Text)
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		t.Fatalf("operation finished with both slots occupied: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	<-s.slots
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
