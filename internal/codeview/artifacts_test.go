package codeview

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/session"
)

// The artifacts folder is a second, read-only root without Git: browsing,
// reading and snapshots work against its context, diffs do not, symlinks
// are not followed, and the workspace root is unaffected.
func TestArtifactsRoot(t *testing.T) {
	ctx := context.Background()
	work := t.TempDir()
	m := session.NewManager(session.Options{DefaultCwd: work})
	item, err := m.Create(session.CreateInput{Cwd: work, Cmd: []string{"/bin/sh"}})
	if err != nil {
		t.Fatal(err)
	}
	folders := artifacts.New(artifacts.Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: m})
	s := New(m, folders)
	t.Cleanup(func() {
		s.Close()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(c)
	})
	id := item.Info().ID

	if _, err := s.Context(ctx, id, "home"); errorKind(err) != ErrInvalid {
		t.Fatalf("unknown root: %v", err)
	}
	if _, err := s.Context(ctx, id, RootArtifacts); errorKind(err) != ErrNotFound {
		t.Fatalf("folder not created yet: %v", err)
	}
	dir, err := folders.Ensure(id)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "report.md", "# done\n")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	write(t, filepath.Dir(outside), "secret.txt", "secret\n")
	if err := os.Symlink(outside, filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	c, err := s.Context(ctx, id, RootArtifacts)
	if err != nil || c.Kind != RootArtifacts || c.Root != dir || !c.Browse || c.Diff || c.Ignore {
		t.Fatalf("artifacts context %+v %v", c, err)
	}
	w, err := s.Context(ctx, id, "")
	if err != nil || w.ID == c.ID || w.Root == dir {
		t.Fatalf("workspace context %+v %v", w, err)
	}

	listing, err := s.Tree(ctx, id, c.ID, "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, n := range listing.Children {
		names[n.Name] = true
	}
	if !names["report.md"] {
		t.Fatalf("listing %+v", listing.Children)
	}
	f, err := s.File(ctx, id, c.ID, nodeID("report.md"))
	if err != nil || f.Document.Text != "# done\n" {
		t.Fatalf("file %+v %v", f.Document, err)
	}
	if again, err := s.SnapshotFile(ctx, id, f.Snapshot, ""); err != nil || again.Document.Text != "# done\n" {
		t.Fatalf("snapshot %+v %v", again.Document, err)
	}
	if escaped, err := s.File(ctx, id, c.ID, nodeID("escape.txt")); err == nil && escaped.Document.Text == "secret\n" {
		t.Fatal("followed a symlink out of the folder")
	}
	if _, err := s.Changes(ctx, id, c.ID); err == nil {
		t.Fatal("artifacts root offered Git changes")
	}
	if _, err := s.Tree(ctx, id, w.ID, "", false, ""); err != nil {
		t.Fatalf("workspace tree after artifacts: %v", err)
	}

	// A replaced folder gets a new context; the old one conflicts. The old
	// folder is moved aside, not deleted, so the filesystem cannot reuse its
	// inode for the new one.
	if err := os.Rename(dir, dir+".old"); err != nil {
		t.Fatal(err)
	}
	if _, err := folders.Ensure(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tree(ctx, id, c.ID, "", false, ""); errorKind(err) != ErrConflict {
		t.Fatalf("stale context: %v", err)
	}
	if next, err := s.Context(ctx, id, RootArtifacts); err != nil || next.ID == c.ID {
		t.Fatalf("rebound context %+v %v", next, err)
	}
}
