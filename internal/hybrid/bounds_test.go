package hybrid

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func bigRecord(id string, size int, state State, updated time.Time) Record {
	rec := testRecord(id)
	rec.Cmd = []string{"agent", strings.Repeat("x", size)}
	rec.State = state
	rec.UpdatedAt = updated
	return rec
}

// The file is bounded in bytes as well as records: long commands fill it
// long before MaxRecords, and the oldest archived records make room.
func TestRecordsByteLimitEvictsOldestArchived(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var seeded []Record
	for i := range 9 {
		seeded = append(seeded, bigRecord(limitID(i), 100_000, StateClosed, base.Add(time.Duration(i)*time.Minute)))
	}
	seedRecords(t, r, seeded)
	evicted, err := r.add(bigRecord("zzzzzzzz", 200_000, StateActive, base.Add(time.Hour)), nil)
	if err != nil {
		t.Fatalf("add past the byte limit: %v", err)
	}
	if len(evicted) != 1 || evicted[0].ID != limitID(0) {
		t.Fatalf("evicted %+v, want only %s", evicted, limitID(0))
	}
	if !r.reserved("zzzzzzzz") || r.reserved(limitID(0)) || len(r.all()) != 9 {
		t.Fatalf("records after eviction: %d", len(r.all()))
	}
	if err := r.unavailable(); err != nil {
		t.Fatalf("store became unavailable: %v", err)
	}
}

// With nothing archived to drop, a full file refuses the new record with
// ErrLimit and stays usable, instead of freezing hybrid sessions.
func TestRecordsByteLimitWithoutArchivedIsErrLimit(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	now := time.Now().UTC()
	var seeded []Record
	for i := range 9 {
		seeded = append(seeded, bigRecord(limitID(i), 100_000, StateActive, now))
	}
	seedRecords(t, r, seeded)
	if _, err := r.add(bigRecord("zzzzzzzz", 200_000, StateActive, now), nil); !errors.Is(err, ErrLimit) {
		t.Fatalf("add: %v, want ErrLimit", err)
	}
	if err := r.unavailable(); err != nil || len(r.all()) != 9 {
		t.Fatalf("store changed: %v, %d records", err, len(r.all()))
	}
}

// A coalesced write the store refuses is reported instead of silently
// leaving memory ahead of the disk.
func TestRecordsCoalescedWriteFailureIsLogged(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	var logs lockedBuffer
	r.setLogger(slog.New(slog.NewTextHandler(&logs, nil)))
	seedRecords(t, r, []Record{testRecord("abcdefgh")})
	if _, err := r.update("abcdefgh", false, func(rec *Record) {
		rec.Cmd = []string{strings.Repeat("x", 2<<20)}
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed write logged", func() bool { return strings.Contains(logs.String(), "write failed") })
}

func bigOptionReport(options, choices int) []ConfigOption {
	var out []ConfigOption
	for i := range options {
		o := ConfigOption{ID: fmt.Sprintf("opt%d", i), Name: strings.Repeat("n", 1000), Category: "model", Type: "select", Value: fmt.Sprintf("choice%d", choices-1)}
		for j := range choices {
			o.Options = append(o.Options, ConfigChoice{Value: fmt.Sprintf("choice%d", j), Name: strings.Repeat("m", 500), Description: strings.Repeat("d", 2000)})
		}
		out = append(out, o)
	}
	return out
}

func TestStoreOptionsIsBounded(t *testing.T) {
	stored := storeOptions(bigOptionReport(100, 400))
	if len(stored) != maxEntryOptions {
		t.Fatalf("%d options stored, want %d", len(stored), maxEntryOptions)
	}
	size := 0
	for _, o := range stored {
		if len([]rune(o.Name)) > maxNameRunes {
			t.Fatalf("name not clipped: %d", len(o.Name))
		}
		selected := false
		for _, c := range o.Choices {
			size += len(c.Value) + len(c.Name) + len(c.Description)
			selected = selected || c.Value == o.Value
		}
		if !selected {
			t.Fatalf("option %s lost its selected choice", o.ID)
		}
	}
	// Each option keeps its selected choice past its share, so allow that
	// slack; the report is nowhere near its 30+ MiB input.
	if size > maxEntryBytes+maxEntryOptions*(500+2000) {
		t.Fatalf("stored choice text %d bytes", size)
	}
	if got := loadOptions(storeOptions(bigOptionReport(1, 3))); len(got) != 1 || len(got[0].Options) != 3 {
		t.Fatalf("small report changed: %+v", got)
	}
}

func TestAgentOptionsFileStaysWithinItsLimit(t *testing.T) {
	dir := t.TempDir()
	o := openAgentOptions(dir)
	if err := o.unavailable(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := range 120 {
		o.remember(fmt.Sprintf("cfg%05d", i), bigOptionReport(maxEntryOptions, 60), now.Add(time.Duration(i)*time.Second))
	}
	o.close()
	info, err := os.Stat(filepath.Join(dir, "agent-options.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatalf("agent-options.json is %d bytes", info.Size())
	}
	reopened := openAgentOptions(dir)
	defer reopened.close()
	if err := reopened.unavailable(); err != nil {
		t.Fatalf("file not readable again: %v", err)
	}
	if got, _ := reopened.get("cfg00119"); len(got) == 0 {
		t.Fatal("the newest report was dropped")
	}
	if got, _ := reopened.get("cfg00000"); len(got) != 0 {
		t.Fatal("the oldest report outlived the size limit")
	}
}

func TestRecordsFinalWriteFailureIsLogged(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	var logs lockedBuffer
	r.setLogger(slog.New(slog.NewTextHandler(&logs, nil)))
	seedRecords(t, r, []Record{testRecord("abcdefgh")})
	if _, err := r.update("abcdefgh", false, func(rec *Record) { rec.Title = "pending" }); err != nil {
		t.Fatal(err)
	}
	r.commitStore = func(storedRecords) error { return errors.New("injected final write failure") }
	r.close()
	if !strings.Contains(logs.String(), "final write failed") {
		t.Fatalf("final write failure not logged: %s", logs.String())
	}
}
