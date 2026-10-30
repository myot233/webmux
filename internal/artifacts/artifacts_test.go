package artifacts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

func newManager(t *testing.T) *session.Manager {
	t.Helper()
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m
}

func create(t *testing.T, m *session.Manager) string {
	t.Helper()
	s, err := m.Create(session.CreateInput{Cmd: []string{"cat"}})
	if err != nil {
		t.Fatal(err)
	}
	return s.Info().ID
}

// remove ends and removes session id, as DELETE ?remove=1 does.
func remove(t *testing.T, m *session.Manager, id string) {
	t.Helper()
	if err := m.Close(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for m.Remove(id) != nil {
		if time.Now().After(deadline) {
			t.Fatal("session never exited")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func TestEnsureCreatesPrivateFoldersPerInstance(t *testing.T) {
	tmp := t.TempDir()
	m := newManager(t)
	id := create(t, m)
	a := New(Options{TempDir: tmp, StateDir: t.TempDir(), Manager: m})
	b := New(Options{TempDir: tmp, StateDir: t.TempDir(), Manager: m})

	dir, err := a.Ensure(id)
	if err != nil {
		t.Fatal(err)
	}
	other, err := b.Ensure(id)
	if err != nil || other == dir {
		t.Fatalf("instances share %q (%v)", other, err)
	}
	canonical, _ := filepath.EvalSymlinks(tmp)
	if filepath.Dir(filepath.Dir(dir)) != canonical || filepath.Base(dir) != id {
		t.Fatalf("folder %q", dir)
	}
	for _, p := range []string{filepath.Dir(dir), dir} {
		if info, err := os.Lstat(p); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", p, info.Mode(), err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := a.Ensure(id)
	if err != nil || again != dir || !exists(filepath.Join(dir, "report.md")) {
		t.Fatalf("second ensure %q %v", again, err)
	}
	uploads, err := a.EnsureUploads(id)
	if err != nil || uploads != filepath.Join(dir, UploadsDir) {
		t.Fatalf("uploads %q %v", uploads, err)
	}
	if existing, err := a.Existing(id); err != nil || existing != dir {
		t.Fatalf("existing %q %v", existing, err)
	}

	for _, missing := range []string{"abcdefgh", "../../etc", ""} {
		if _, err := a.Ensure(missing); !errors.Is(err, ErrNotFound) {
			t.Errorf("Ensure(%q): %v", missing, err)
		}
	}
	other2 := create(t, m)
	if _, err := a.Existing(other2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("never created: %v", err)
	}
}

// Something already at the folder's path that is not a private directory of
// this user is refused and left alone.
func TestEnsureRefusesForeignEntries(t *testing.T) {
	tmp := t.TempDir()
	m := newManager(t)
	id := create(t, m)
	s := New(Options{TempDir: tmp, StateDir: t.TempDir(), Manager: m})
	root := filepath.Dir(must(s.Path(id)))

	target := t.TempDir()
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ensure(id); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("symlinked root: %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 || !exists(root) {
		t.Fatalf("followed or removed the link: %v", entries)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ensure(id); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("shared root: %v", err)
	}
	if info, _ := os.Lstat(root); info.Mode().Perm() != 0o755 {
		t.Fatalf("root permissions changed to %v", info.Mode())
	}
}

func TestRemovedSessionsLoseTheirFolders(t *testing.T) {
	m := newManager(t)
	s := New(Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: m})
	gone, kept := create(t, m), create(t, m)
	goneDir, _ := s.Ensure(gone)
	keptDir, _ := s.Ensure(kept)
	s.Start(func(id string) bool { return id == kept })
	t.Cleanup(s.Close)

	remove(t, m, gone)
	remove(t, m, kept)
	eventually(t, "removed session's folder", func() bool { return !exists(goneDir) })
	// The retained folder survives its session leaving the manager.
	time.Sleep(50 * time.Millisecond)
	if !exists(keptDir) {
		t.Fatal("retained folder deleted")
	}
	s.Remove(kept)
	if exists(keptDir) {
		t.Fatal("explicit removal kept the folder")
	}
}

func TestSweepRemovesOnlyUnclaimedSessionFolders(t *testing.T) {
	m := newManager(t)
	s := New(Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: m})
	live := create(t, m)
	liveDir, _ := s.Ensure(live)
	root := filepath.Dir(liveDir)
	outside := t.TempDir()
	for _, name := range []string{"orphan22", "retain22", "notanid!"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkxxxx")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "victim"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	s.Sweep(func(id string) bool { return id == "retain22" })
	want := map[string]bool{live: true, "orphan22": false, "retain22": true, "notanid!": true, "linkxxxx": true}
	for name, kept := range want {
		if exists(filepath.Join(root, name)) != kept {
			t.Errorf("%s kept=%v, want %v", name, !kept, kept)
		}
	}
	if !exists(filepath.Join(outside, "victim")) {
		t.Fatal("sweep followed a symlink")
	}
}

func TestUploadPathAcceptsOnlyTheSessionsUploads(t *testing.T) {
	m := newManager(t)
	s := New(Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: m})
	a, b := create(t, m), create(t, m)
	aUploads, _ := s.EnsureUploads(a)
	bUploads, _ := s.EnsureUploads(b)
	write := func(p string) string {
		t.Helper()
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	own := write(filepath.Join(aUploads, "shot.png"))
	if got, err := s.UploadPath(a, own); err != nil || got != own {
		t.Fatalf("own upload %q %v", got, err)
	}
	if got, err := s.UploadPath(a, filepath.Join(aUploads, "..", UploadsDir, "shot.png")); err != nil || got != own {
		t.Fatalf("unclean path %q %v", got, err)
	}
	if err := os.Mkdir(filepath.Join(aUploads, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(aUploads, "link.png")
	if err := os.Symlink(own, link); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{
		"other session": write(filepath.Join(bUploads, "theirs.png")),
		"folder root":   write(filepath.Join(filepath.Dir(aUploads), "report.md")),
		"nested":        write(filepath.Join(aUploads, "nested", "deep.txt")),
		"outside":       write(filepath.Join(t.TempDir(), "x.txt")),
		"symlink":       link,
		"directory":     filepath.Join(aUploads, "nested"),
		"relative":      "uploads/shot.png",
		"missing":       filepath.Join(aUploads, "missing.png"),
	} {
		if _, err := s.UploadPath(a, p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}
