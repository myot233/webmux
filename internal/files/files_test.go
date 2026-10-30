package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompleteOrdersVisibleBeforeHiddenAndFollowsLinks(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	for _, name := range []string{"beta", "alpha", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"linked": filepath.Join(root, "alpha"), "file-link": file, "broken": filepath.Join(root, "missing")} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	s := New(root)
	all := []string{filepath.Join(root, "alpha"), filepath.Join(root, "beta"), filepath.Join(root, "linked"), filepath.Join(root, ".hidden")}
	for _, row := range []struct {
		prefix string
		want   []string
	}{
		{"", all},
		{"~", all},
		{"~/", all},
		{root + "/", all},
		{root + "/b", []string{filepath.Join(root, "beta")}},
		{"~/.h", []string{filepath.Join(root, ".hidden")}},
		{root + "/zzz", []string{}},
	} {
		got, err := s.Complete(row.prefix)
		if err != nil || !reflect.DeepEqual(got, row.want) {
			t.Errorf("Complete(%q) = %v, %v; want %v", row.prefix, got, err, row.want)
		}
	}
	for _, prefix := range []string{root + "/missing/", file + "/"} {
		if _, err := s.Complete(prefix); !errors.Is(err, ErrInvalid) || err.Error() == "" {
			t.Errorf("Complete(%q) error %v, want ErrInvalid", prefix, err)
		}
	}
}

func TestMkdir(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if err := s.Mkdir(""); !errors.Is(err, ErrInvalid) || err.Error() != "path required" {
		t.Fatalf("empty path: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := s.Mkdir(nested); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(nested); err != nil || !info.IsDir() {
		t.Fatalf("not created: %v", err)
	}
	blocker := filepath.Join(root, "file")
	_ = os.WriteFile(blocker, nil, 0o600)
	if err := s.Mkdir(filepath.Join(blocker, "child")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("under a file: %v", err)
	}
}

func TestStoreUploadIsPrivateAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	upload, err := s.StoreUpload(dir, `..\dir/../note.txt`, strings.NewReader("hello"), 64)
	if err != nil {
		t.Fatal(err)
	}
	if upload.Path != filepath.Join(dir, "note.txt") {
		t.Fatalf("path %q", upload.Path)
	}
	if file, _ := os.Stat(upload.Path); file.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", file.Mode())
	}
	if data, _ := os.ReadFile(upload.Path); string(data) != "hello" {
		t.Fatalf("content %q", data)
	}
	upload.Discard()
	if _, err := os.Stat(upload.Path); !os.IsNotExist(err) {
		t.Fatalf("discard left %v", err)
	}

	for _, name := range []string{"", ".", "..", "bad\x00name", "bad\u0085name"} {
		if _, err := s.StoreUpload(dir, name, strings.NewReader("x"), 64); !errors.Is(err, ErrInvalid) {
			t.Errorf("filename %q: %v", name, err)
		}
	}
	if _, err := s.StoreUpload(dir, "big", strings.NewReader(strings.Repeat("x", 65)), 64); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized: %v", err)
	}
	cause := errors.New("connection reset")
	_, err = s.StoreUpload(dir, "broken", io.MultiReader(strings.NewReader("x"), failingReader{cause}), 64)
	if !errors.Is(err, ErrRead) || !errors.Is(err, cause) || err.Error() != "read uploaded file: connection reset" {
		t.Errorf("read failure: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("failed uploads left %d entries", len(entries))
	}
}

// A taken name gets the smallest free " (n)" before its extension; a failed
// upload removes only its own file.
func TestStoreUploadNeverReplacesFiles(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	for _, name := range []string{"shot.png", "shot (2).png", ".env", "README"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{"shot.png": "shot (3).png", ".env": ".env (2)", "README": "README (2)", "new.txt": "new.txt"} {
		upload, err := s.StoreUpload(dir, name, strings.NewReader("new"), 64)
		if err != nil || upload.Path != filepath.Join(dir, want) {
			t.Errorf("%s stored as %v, %v; want %s", name, upload, err, want)
		}
	}
	if _, err := s.StoreUpload(dir, "shot.png", strings.NewReader(strings.Repeat("x", 65)), 64); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized: %v", err)
	}
	for _, name := range []string{"shot.png", "shot (2).png", ".env", "README"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != "old" {
			t.Errorf("%s changed: %q %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "shot (4).png")); !os.IsNotExist(err) {
		t.Fatalf("failed upload left its file: %v", err)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

// Generate arbitrarily large sources without allocating their contents.
type generated struct{}

func (generated) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestBoundedCopyAllowsExactLimit(t *testing.T) {
	for _, size := range []int64{0, 63, 64, 65, 1000} {
		n, err := copyBounded(io.Discard, io.LimitReader(generated{}, size), 64)
		want := min(size, 65)
		if err != nil || n != want {
			t.Fatalf("size %d: copied %d, want %d (%v)", size, n, want, err)
		}
	}
}
