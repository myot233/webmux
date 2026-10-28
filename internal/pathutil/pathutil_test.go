package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ in, want string }{
		{"~", home},
		{"~/work/repo", filepath.Join(home, "work", "repo")},
		{"/abs/../path", "/path"},
		{"relative", filepath.Join(cwd, "relative")},
		{"~user", filepath.Join(cwd, "~user")},
	} {
		if got, err := Expand(row.in); err != nil || got != row.want {
			t.Errorf("Expand(%q) = %q, %v; want %q", row.in, got, err, row.want)
		}
	}
}

func TestUploadPath(t *testing.T) {
	dir, err := os.MkdirTemp("", UploadDirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := UploadPath(path); err != nil {
		t.Fatalf("UploadPath: %v", err)
	} else if !filepath.IsAbs(got) {
		t.Fatalf("UploadPath returned %q", got)
	}
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "relative", outside, dir, filepath.Join(dir, "missing")} {
		if _, err := UploadPath(bad); err == nil {
			t.Errorf("UploadPath(%q) accepted", bad)
		}
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := UploadPath(link); err == nil {
		t.Errorf("UploadPath accepted a symlink")
	}
}

// macOS's default TMPDIR is /var/folders/..., behind /var -> /private/var: a
// path UploadPath returns must validate again unchanged.
func TestUploadPathSymlinkedTempDir(t *testing.T) {
	target := t.TempDir()
	tmp := filepath.Join(t.TempDir(), "tmp-link")
	if err := os.Symlink(target, tmp); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	dir, err := os.MkdirTemp("", UploadDirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(path, []byte("png"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := UploadPath(path)
	if err != nil {
		t.Fatalf("UploadPath(%q): %v", path, err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != canonical {
		t.Fatalf("UploadPath(%q) = %q, want %q", path, got, canonical)
	}
	again, err := UploadPath(got)
	if err != nil || again != got {
		t.Fatalf("UploadPath(%q) = %q, %v; want it unchanged", got, again, err)
	}
}
