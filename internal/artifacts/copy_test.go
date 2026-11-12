package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestCopyStagedUploadRequiresOwnerAndUsesNormalRetentionCleanup(t *testing.T) {
	manager := newManager(t)
	state, temp := t.TempDir(), t.TempDir()
	service := New(Options{StateDir: state, TempDir: temp, Manager: manager})
	const owner = "abcdefgh"
	keep := true
	service.Start(func(id string) bool { return id == owner && keep })
	// Stop the event watcher so this fixture can change retention without
	// sharing mutable predicate state with a goroutine.
	service.Close()
	stage, err := service.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stage, "proof.txt")
	data := []byte("source-owned draft bytes")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CopyStagedUpload("bbbbbbbb", source, int64(len(data))); !errors.Is(err, ErrNotFound) {
		t.Fatal("unretained owner accepted", err)
	}
	owned, err := service.CopyStagedUpload(owner, source, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UploadPath(owner, owned); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureUploads(owner); !errors.Is(err, ErrNotFound) {
		t.Fatal("ordinary upload creation gained access to offline owners", err)
	}
	again, err := service.CopyStagedUpload(owner, source, int64(len(data)))
	if err != nil || again != owned {
		t.Fatal("duplicate snapshot changed ownership", err)
	}
	service.Sweep(func(id string) bool { return id == owner })
	if body, err := os.ReadFile(owned); err != nil || string(body) != string(data) {
		t.Fatal("retention removed draft bytes", err)
	}
	service.clearStaging()
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("staging cleanup changed", err)
	}
	if _, err := service.CopyStagedUpload(owner, owned, int64(len(data))); !errors.Is(err, ErrInvalid) {
		t.Fatal("nonstaged input accepted", err)
	}
	keep = false
	service.Sweep(func(string) bool { return false })
	if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("forgotten owner leaked durable files", err)
	}
	stage, err = service.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "proof.txt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CopyStagedUpload(owner, source, int64(len(data))); !errors.Is(err, ErrNotFound) {
		t.Fatal("forgotten owner was recreated", err)
	}
}

func TestStagedSnapshotsPreserveOrdinaryUploadsAndRevisedContent(t *testing.T) {
	manager := newManager(t)
	id := create(t, manager)
	service := New(Options{StateDir: t.TempDir(), TempDir: t.TempDir(), Manager: manager})
	dir, err := service.EnsureUploads(id)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(ordinary, []byte("ordinary source upload"), 0600); err != nil {
		t.Fatal(err)
	}
	stage, err := service.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stage, "report.txt")
	first := []byte("first staged report")
	if err := os.WriteFile(source, first, 0600); err != nil {
		t.Fatal(err)
	}
	owned, err := service.CopyStagedUpload(id, source, int64(len(first)))
	if err != nil {
		t.Fatal("same-basename source upload blocked snapshot", err)
	}
	service.clearStaging()
	stage, err = service.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	source = filepath.Join(stage, "report.txt")
	second := []byte("revised staged report")
	if err := os.WriteFile(source, second, 0600); err != nil {
		t.Fatal(err)
	}
	revised, err := service.CopyStagedUpload(id, source, int64(len(second)))
	if err != nil || revised == owned {
		t.Fatal("revised content did not get independent ownership", revised, err)
	}
	for path, want := range map[string]string{ordinary: "ordinary source upload", owned: string(first), revised: string(second), source: string(second)} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("changed bytes at %s: %q %v", path, got, err)
		}
	}
}

func TestConcurrentStagedSnapshotsReuseOneCopyAndRejectTampering(t *testing.T) {
	manager := newManager(t)
	id := create(t, manager)
	service := New(Options{StateDir: t.TempDir(), TempDir: t.TempDir(), Manager: manager})
	stage, err := service.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	// Content identity must not overflow a legal maximum-length basename.
	source := filepath.Join(stage, strings.Repeat("r", 251)+".txt")
	data := []byte("same validated content")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	const attempts = 12
	paths := make([]string, attempts)
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Go(func() {
			paths[i], errs[i] = service.CopyStagedUpload(id, source, int64(len(data)))
		})
	}
	wg.Wait()
	for i := range attempts {
		if errs[i] != nil || paths[i] != paths[0] {
			t.Fatalf("concurrent repeat %d: %s %v", i, paths[i], errs[i])
		}
	}
	entries, err := os.ReadDir(filepath.Dir(paths[0]))
	if err != nil || len(entries) != 1 {
		t.Fatal("repeat left extra copies", entries, err)
	}
	if err := os.WriteFile(paths[0], []byte(strings.Repeat("x", len(data))), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CopyStagedUpload(id, source, int64(len(data))); !errors.Is(err, ErrInvalid) {
		t.Fatal("tampered existing snapshot was reused or overwritten", err)
	}
}
