package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyUploadPublishesOnceAndSurvivesOriginalRemoval(t *testing.T) {
	manager := newManager(t)
	id := create(t, manager)
	service := New(Options{StateDir: t.TempDir(), TempDir: t.TempDir(), Manager: manager})
	source := filepath.Join(t.TempDir(), "image.png")
	body := []byte("immutable upload")
	if err := os.WriteFile(source, body, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(body)
	digest := hex.EncodeToString(hash[:])
	target, err := service.CopyUpload(id, source, "handoff-copy", digest, int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UploadPath(id, target); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private target: %v %v", info, err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	retry, err := service.CopyUpload(id, source, "handoff-copy", digest, int64(len(body)))
	if err != nil || retry != target {
		t.Fatalf("duplicate copy: %s %v", retry, err)
	}
	if err := os.WriteFile(target, []byte("foreign content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CopyUpload(id, source, "handoff-copy", digest, int64(len(body))); !errors.Is(err, ErrInvalid) {
		t.Fatal("overwrote mismatched destination", err)
	}
}

func TestCopyUploadNeverPublishesPartialOrSymlinkTargets(t *testing.T) {
	manager := newManager(t)
	id := create(t, manager)
	service := New(Options{StateDir: t.TempDir(), TempDir: t.TempDir(), Manager: manager})
	source := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(source, []byte("wrong"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("right"))
	digest := hex.EncodeToString(hash[:])
	if _, err := service.CopyUpload(id, source, "partial", digest, 5); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	dir, err := service.EnsureUploads(id)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial copy was published: %v %v", entries, err)
	}
	if err := os.WriteFile(source, []byte("right"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CopyUpload(id, source, "link", digest, 5); !errors.Is(err, ErrInvalid) {
		t.Fatal("followed target symlink", err)
	}
}
