package hybrid

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/runtime"
)

func testRecord(id string) Record {
	now := time.Now().UTC()
	return Record{
		ID: id, Name: "omp", Title: "session title", ConfigID: "cfgaaaaa", Agent: "omp", Cwd: "/p",
		Cmd: []string{"omp"}, Side: SideTUI, PreviousIDs: []string{}, Config: map[string]string{},
		State: StateActive, CreatedAt: now, UpdatedAt: now,
	}
}

func openTestRecords(t *testing.T, dir string) *records {
	t.Helper()
	r := openRecords(dir)
	t.Cleanup(r.close)
	if err := r.unavailable(); err != nil {
		t.Fatal(err)
	}
	return r
}

// seedRecords stores recs as one write, so limit tests do not commit (and
// fsync) the growing file once per record.
func seedRecords(t *testing.T, r *records, recs []Record) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		t.Fatal(err)
	}
	if err := r.commitLocked(recs); err != nil {
		t.Fatal(err)
	}
	r.list = recs
}

func readFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "hybrid-sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRecordsPersistAndReserveIDs(t *testing.T) {
	dir := t.TempDir()
	r := openRecords(dir)
	if r.reserved("abcdefgh") {
		t.Fatal("empty store reserved an ID")
	}
	rec := testRecord("abcdefgh")
	if _, err := r.add(rec, nil); err != nil {
		t.Fatal(err)
	}
	if !r.reserved("abcdefgh") || !strings.Contains(readFile(t, dir), `"abcdefgh"`) {
		t.Fatal("added record not reserved or not written immediately")
	}
	if _, err := r.add(rec, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate add: %v", err)
	}
	r.close()
	r = openTestRecords(t, dir)
	got, ok := r.get("abcdefgh")
	if all := r.all(); !ok || got.Agent != "omp" || got.Title != "session title" || got.State != StateActive || len(all) != 1 || all[0].ID != "abcdefgh" {
		t.Fatalf("reopened record %+v %v", got, ok)
	}
	if err := r.remove("abcdefgh"); err != nil || r.reserved("abcdefgh") {
		t.Fatalf("remove: %v", err)
	}
	if err := r.remove("abcdefgh"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestRecordsCorruptFileIsUnavailableAndKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hybrid-sessions.json")
	if err := os.WriteFile(path, []byte(`{"v":1,"records":[{"id":"bad"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := openRecords(dir)
	defer r.close()
	err := r.unavailable()
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), path) {
		t.Fatalf("unavailable = %v", err)
	}
	if _, err := r.add(testRecord("abcdefgh"), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("add on corrupt store: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != `{"v":1,"records":[{"id":"bad"}` {
		t.Fatalf("corrupt file changed: %s", b)
	}
}

func TestRecordsLimitDropsOldestArchived(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seeded := make([]Record, 0, MaxRecords)
	for i := range MaxRecords {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		rec.UpdatedAt = base.Add(time.Duration(i) * time.Minute)
		seeded = append(seeded, rec)
	}
	seedRecords(t, r, seeded)
	evicted, err := r.add(testRecord("zzzzzzzz"), nil)
	if err != nil {
		t.Fatalf("add to a full store: %v", err)
	}
	if len(evicted) != 1 || evicted[0].ID != limitID(0) {
		t.Fatalf("evicted %+v, want %s", evicted, limitID(0))
	}
	if got := len(r.all()); got != MaxRecords {
		t.Fatalf("records after adding: %d", got)
	}
	if r.reserved(limitID(0)) || !r.reserved("zzzzzzzz") {
		t.Fatal("oldest archived record kept or new record missing")
	}
}

func TestRecordsRestorePutsAnEvictedRecordBack(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seeded := make([]Record, 0, MaxRecords)
	for i := range MaxRecords {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		rec.UpdatedAt = base.Add(time.Duration(i) * time.Minute)
		seeded = append(seeded, rec)
	}
	seedRecords(t, r, seeded)
	evicted, err := r.add(testRecord("zzzzzzzz"), nil)
	if err != nil || len(evicted) != 1 {
		t.Fatalf("add: %+v, %v", evicted, err)
	}
	// A failed start removed its own record, so the store has room again.
	if err := r.remove("zzzzzzzz"); err != nil {
		t.Fatal(err)
	}
	if err := r.restore(evicted[0]); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !r.reserved(evicted[0].ID) || len(r.all()) != MaxRecords {
		t.Fatal("evicted record not restored")
	}
	if err := r.restore(evicted[0]); !errors.Is(err, ErrConflict) {
		t.Fatalf("restoring twice: %v", err)
	}
}

func TestRecordsLimitKeepsActiveRecords(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	seeded := make([]Record, 0, MaxRecords)
	for i := range MaxRecords {
		seeded = append(seeded, testRecord(limitID(i)))
	}
	seedRecords(t, r, seeded)
	if r.accepts(nil) {
		t.Fatal("a store of only active records accepts another")
	}
	if _, err := r.add(testRecord("zzzzzzzz"), nil); !errors.Is(err, ErrLimit) {
		t.Fatalf("add without an archived record: %v", err)
	}
	if got := len(r.all()); got != MaxRecords {
		t.Fatalf("records after a refused add: %d", got)
	}
}

func TestRecordsLimitSkipsResumingRecords(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seeded := make([]Record, 0, MaxRecords)
	for i := range MaxRecords {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		rec.UpdatedAt = base.Add(time.Duration(i) * time.Minute)
		seeded = append(seeded, rec)
	}
	seedRecords(t, r, seeded)
	// The oldest record is being resumed, so the next oldest goes instead.
	skip := func(id string) bool { return id == limitID(0) }
	if !r.accepts(skip) {
		t.Fatal("a store with another archived record accepts nothing")
	}
	evicted, err := r.add(testRecord("zzzzzzzz"), skip)
	if err != nil {
		t.Fatal(err)
	}
	if len(evicted) != 1 || evicted[0].ID != limitID(1) {
		t.Fatalf("evicted %+v, want %s", evicted, limitID(1))
	}
	// Nothing else may be dropped once the remaining records are skipped.
	if r.accepts(func(string) bool { return true }) {
		t.Fatal("a store of skipped records accepts another")
	}
}

func TestRecordsLimitPrefersLeastRecentlyUpdated(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seeded := make([]Record, 0, MaxRecords)
	for i := range MaxRecords {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		rec.UpdatedAt = base.Add(time.Duration(MaxRecords-i) * time.Minute)
		seeded = append(seeded, rec)
	}
	// An interrupted record updated long ago goes before otherwise older
	// records; active records never qualify even when they look oldest.
	seeded[3].State = StateInterrupted
	seeded[3].UpdatedAt = base
	seeded[7].State = StateActive
	seeded[7].UpdatedAt = base.Add(-time.Hour)
	seedRecords(t, r, seeded)
	evicted, err := r.add(testRecord("zzzzzzzz"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(evicted) != 1 || evicted[0].ID != limitID(3) {
		t.Fatalf("evicted %+v, want %s", evicted, limitID(3))
	}
}

func TestSetAgentSessionIDKeepsBoundedHistory(t *testing.T) {
	rec := testRecord("abcdefgh")
	for i := range 12 {
		rec.SetAgentSessionID(fmt.Sprintf("id%d", i))
	}
	rec.SetAgentSessionID("id5")
	if rec.AgentSessionID != "id5" || len(rec.PreviousIDs) != maxPreviousIDs || slices.Contains(rec.PreviousIDs, "id5") || rec.PreviousIDs[len(rec.PreviousIDs)-1] != "id11" {
		t.Fatalf("history %q current %q", rec.PreviousIDs, rec.AgentSessionID)
	}
}

func TestRecordsCoalesceRoutineUpdates(t *testing.T) {
	dir := t.TempDir()
	r := openRecords(dir)
	if _, err := r.add(testRecord("abcdefgh"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.update("abcdefgh", false, func(rec *Record) { rec.Config["model"] = "haiku" }); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, dir), "haiku") {
		t.Fatal("routine update written before the delay")
	}
	deadline := time.Now().Add(3 * persistDelay)
	for !strings.Contains(readFile(t, dir), "haiku") {
		if time.Now().After(deadline) {
			t.Fatal("routine update never written")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := r.update("abcdefgh", false, func(rec *Record) { rec.Config["mode"] = "plan" }); err != nil {
		t.Fatal(err)
	}
	r.close()
	if !strings.Contains(readFile(t, dir), "plan") {
		t.Fatal("close did not flush the pending update")
	}
	if _, err := r.update("abcdefgh", true, func(*Record) {}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("update after close: %v", err)
	}
}

func TestRecoverAfterRestart(t *testing.T) {
	dir := t.TempDir()
	orphan := exec.Command("sleep", "60")
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := orphan.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = orphan.Wait(); close(exited) }()
	t.Cleanup(func() { _ = syscall.Kill(-orphan.Process.Pid, syscall.SIGKILL); <-exited })
	identity, err := runtime.ProcessIdentity(orphan.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	bystander := exec.Command("sleep", "60")
	bystander.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := bystander.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-bystander.Process.Pid, syscall.SIGKILL); _ = bystander.Wait() })

	r := openRecords(dir)
	matching := testRecord("abcdefgh")
	matching.PID, matching.Process = orphan.Process.Pid, &identity
	stale := testRecord("abcdefgi")
	stale.PID, stale.Process = bystander.Process.Pid, &runtime.Identity{Start: "not-the-same"}
	closed := testRecord("abcdefgj")
	closed.State = StateClosed
	for _, rec := range []Record{matching, stale, closed} {
		if _, err := r.add(rec, nil); err != nil {
			t.Fatal(err)
		}
	}
	r.close()

	r = openTestRecords(t, dir)
	if _, err := r.recoverAfterRestart(time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("leftover process with matching identity survived")
	}
	if syscall.Kill(bystander.Process.Pid, 0) != nil {
		t.Fatal("process with a different start identity was killed")
	}
	for id, want := range map[string]State{"abcdefgh": StateInterrupted, "abcdefgi": StateInterrupted, "abcdefgj": StateClosed} {
		got, _ := r.get(id)
		if got.State != want || (want == StateInterrupted && (got.InterruptedAt == nil || got.PID != 0 || got.Process != nil)) {
			t.Fatalf("%s after restart: %+v", id, got)
		}
	}
	if !strings.Contains(readFile(t, dir), `"interrupted"`) {
		t.Fatal("recovery not written")
	}
}

// startOrphan starts a process group standing in for an agent left behind,
// returning its PID, start identity and a channel closed when it exits.
func startOrphan(t *testing.T) (int, runtime.Identity, <-chan struct{}) {
	t.Helper()
	orphan := exec.Command("sleep", "60")
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := orphan.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = orphan.Wait(); close(exited) }()
	t.Cleanup(func() { _ = syscall.Kill(-orphan.Process.Pid, syscall.SIGKILL); <-exited })
	identity, err := runtime.ProcessIdentity(orphan.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return orphan.Process.Pid, identity, exited
}

// TestRecoverAfterRestartEndsUnconfirmedLeftovers: interrupted and closed
// records keep the identity of a process that was not confirmed gone (a
// failed release or stop); a restart ends it before anything can resume
// the agent session, and leaves the records' state alone.
func TestRecoverAfterRestartEndsUnconfirmedLeftovers(t *testing.T) {
	dir := t.TempDir()
	interruptedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	r := openRecords(dir)
	exits := map[string]<-chan struct{}{}
	for id, state := range map[string]State{"abcdefgh": StateInterrupted, "abcdefgi": StateClosed} {
		pid, identity, exited := startOrphan(t)
		rec := testRecord(id)
		rec.State, rec.PID, rec.Process = state, pid, &identity
		if state == StateInterrupted {
			rec.Activity, rec.InterruptedAt = "switching", &interruptedAt
		}
		if _, err := r.add(rec, nil); err != nil {
			t.Fatal(err)
		}
		exits[id] = exited
	}
	r.close()

	r = openTestRecords(t, dir)
	if _, err := r.recoverAfterRestart(time.Now()); err != nil {
		t.Fatal(err)
	}
	for id, exited := range exits {
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: leftover process survived the restart", id)
		}
		if got, _ := r.get(id); got.PID != 0 || got.Process != nil {
			t.Fatalf("%s after restart: %+v", id, got)
		}
	}
	if got, _ := r.get("abcdefgh"); got.State != StateInterrupted || got.Activity != "switching" || got.InterruptedAt == nil || !got.InterruptedAt.Equal(interruptedAt) {
		t.Fatalf("interrupted record after restart: %+v", got)
	}
	if got, _ := r.get("abcdefgi"); got.State != StateClosed || got.InterruptedAt != nil {
		t.Fatalf("closed record after restart: %+v", got)
	}
}

func TestRecordsReadDoesNotWaitForCommit(t *testing.T) {
	r := openTestRecords(t, t.TempDir())
	seedRecords(t, r, []Record{testRecord("abcdefgh")})
	entered := make(chan struct{})
	release := make(chan struct{})
	r.commitStore = func(storedRecords) error {
		close(entered)
		<-release
		return nil
	}
	updated := make(chan error, 1)
	go func() {
		_, err := r.update("abcdefgh", true, func(rec *Record) { rec.Title = "committed" })
		updated <- err
	}()
	<-entered
	read := make(chan Record, 1)
	go func() {
		rec, _ := r.get("abcdefgh")
		read <- rec
	}()
	select {
	case rec := <-read:
		if rec.Title != "session title" {
			t.Fatalf("read during commit = %+v", rec)
		}
	case <-time.After(time.Second):
		close(release)
		<-updated
		t.Fatal("record read waited for filesystem commit")
	}
	close(release)
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
}

func TestRecordsImmediateFailureKeepsAcceptedUpdateDirty(t *testing.T) {
	dir := t.TempDir()
	r := openTestRecords(t, dir)
	seedRecords(t, r, []Record{testRecord("abcdefgh")})
	if _, err := r.update("abcdefgh", false, func(rec *Record) { rec.Title = "accepted" }); err != nil {
		t.Fatal(err)
	}
	realCommit := r.commitStore
	r.commitStore = func(storedRecords) error { return errors.New("injected write failure") }
	if _, err := r.update("abcdefgh", true, func(rec *Record) { rec.Title = "rejected" }); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("immediate update: %v", err)
	}
	r.mu.Lock()
	dirty := r.dirty
	r.mu.Unlock()
	if !dirty {
		t.Fatal("failed immediate update cleared the earlier dirty state")
	}
	r.commitStore = realCommit
	r.close()
	file := readFile(t, dir)
	if !strings.Contains(file, `"title": "accepted"`) || strings.Contains(file, "rejected") {
		t.Fatalf("final records file = %s", file)
	}
}
