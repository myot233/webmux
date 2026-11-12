package statefile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func testDatabaseContract(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	d, err := OpenDatabase(ctx, dir, Conversation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.SQL.Exec("CREATE TABLE example(value TEXT); INSERT INTO example VALUES('durable')"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"conversations.sqlite", "conversations.sqlite.lock"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private file: %v %v", info, err)
		}
	}
	if other, err := OpenDatabase(ctx, dir, Conversation); err == nil {
		_ = other.Close()
		t.Fatal("second owner acquired lock")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDatabase(ctx, dir, Conversation)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	var value string
	if err := reopened.SQL.QueryRow("SELECT value FROM example").Scan(&value); err != nil || value != "durable" {
		t.Fatalf("reopen: %q %v", value, err)
	}
}

func TestDatabaseRejectsSymlinks(t *testing.T) {
	for _, suffix := range []string{"", ".lock", "-journal", "-wal", "-shm"} {
		t.Run(suffix, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, "conversations.sqlite"+suffix)); err != nil {
				t.Fatal(err)
			}
			if db, err := OpenDatabase(context.Background(), dir, Conversation); err == nil {
				_ = db.Close()
				t.Fatal("followed link")
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "untouched" {
				t.Fatalf("target mutated: %q %v", got, err)
			}
		})
	}
}
