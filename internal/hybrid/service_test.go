package hybrid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
)

const testTimeout = 20 * time.Second

var fakeAgent struct {
	once sync.Once
	dir  string
	err  error
}

// macOS places TMPDIR behind the /var -> /private/var symlink, while the
// fake agent reports and stores canonical working directories.
func TestMain(m *testing.M) {
	if dir, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		_ = os.Setenv("TMPDIR", dir)
	}
	code := m.Run()
	if fakeAgent.dir != "" {
		_ = os.RemoveAll(fakeAgent.dir)
	}
	os.Exit(code)
}

// agentPath builds the fake agent once, as executables named after both
// profiles so agentprofile recognises them.
func agentPath(t *testing.T, profile string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the fake agent")
	}
	fakeAgent.once.Do(func() {
		fakeAgent.dir, fakeAgent.err = os.MkdirTemp("", "hybrid-fakeagent-")
		if fakeAgent.err != nil {
			return
		}
		// A separate file per profile, not a hard link: macOS reports a
		// hard-linked executable under either name, breaking the root check.
		for _, name := range []string{agentprofile.OMP, agentprofile.OpenCode} {
			out, err := exec.Command("go", "build", "-o", filepath.Join(fakeAgent.dir, name), "../acp/testdata/fakeagent").CombinedOutput()
			if err != nil {
				fakeAgent.err = fmt.Errorf("go build: %w\n%s", err, out)
				return
			}
		}
	})
	if fakeAgent.err != nil {
		t.Fatal(fakeAgent.err)
	}
	return filepath.Join(fakeAgent.dir, profile)
}

type harness struct {
	t       *testing.T
	state   string
	store   string
	logs    *syncBuffer
	manager *session.Manager
	svc     *Service
	// prefs, when set before open, supplies chatIdleSleepMinutes.
	prefs *settings.Service
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newHarness(t *testing.T, scrollback int) *harness {
	t.Helper()
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}}
	h.open(scrollback)
	return h
}

func (h *harness) open(scrollback int) {
	h.manager = session.NewManager(session.Options{DefaultCwd: h.t.TempDir(), ScrollbackBytes: scrollback})
	h.svc = New(Options{StateDir: h.state, Manager: h.manager, ScrollbackBytes: scrollback, Version: "test", Logger: slog.New(slog.NewTextHandler(h.logs, nil)), Settings: h.prefs, Uploads: func(_, path string) (string, error) { return trust(path) }})
	svc, manager := h.svc, h.manager
	h.t.Cleanup(func() {
		svc.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		manager.Shutdown(ctx)
	})
}

// input describes a fake agent session in a fresh directory reached
// through a symlink; env adds or, with an empty value, removes variables.
func (h *harness) input(profile string, side Side, env ...string) CreateInput {
	h.t.Helper()
	real := h.t.TempDir()
	link := filepath.Join(h.t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		h.t.Fatal(err)
	}
	vars := map[string]string{
		"FAKEAGENT_STORE": h.store, "FAKEAGENT_PROFILE": profile,
		"FAKEAGENT_TURN_DELAY": "5ms", "API_KEY": "secret-value",
	}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if v == "" {
			delete(vars, k)
		} else {
			vars[k] = v
		}
	}
	return CreateInput{Cwd: link, Name: "agent", ConfigID: "cfgtest01", Cmd: []string{agentPath(h.t, profile)}, Env: vars, StartSide: side}
}

func (h *harness) create(in CreateInput) *session.Session {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	s, err := h.svc.Create(ctx, in)
	if err != nil {
		h.t.Fatalf("create: %v", err)
	}
	return s
}

func (h *harness) record(id string) Record {
	h.t.Helper()
	rec, ok := h.svc.records.get(id)
	if !ok {
		h.t.Fatalf("no record %s", id)
	}
	return rec
}

func (h *harness) file() string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.state, "hybrid-sessions.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

// closeSession closes id as a user would and waits for its shell to go.
func (h *harness) closeSession(id string) {
	h.t.Helper()
	if err := h.manager.Close(id); err != nil {
		h.t.Fatal(err)
	}
	waitFor(h.t, "shell removed", func() bool { _, err := h.manager.Get(id); return err != nil })
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func resolved(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestCreateStartsEitherSide(t *testing.T) {
	for _, profile := range []string{agentprofile.OMP, agentprofile.OpenCode} {
		for _, side := range []Side{SideTUI, SideACP} {
			t.Run(profile+"/"+string(side), func(t *testing.T) {
				h := newHarness(t, 0)
				in := h.input(profile, side)
				s := h.create(in)
				info := s.Info()
				if info.Hybrid == nil || info.Hybrid.Side != string(side) || info.Hybrid.State != string(StateActive) || info.Hybrid.Agent != profile {
					t.Fatalf("hybrid info %+v", info.Hybrid)
				}
				if got := slices.Contains(info.Cmd, "--standalone"); got != (profile == agentprofile.OpenCode) {
					t.Fatalf("shown command %q", info.Cmd)
				}
				rec := h.record(info.ID)
				if rec.Side != side || rec.State != StateActive || rec.Agent != profile || rec.ConfigID != "cfgtest01" ||
					rec.Cwd != resolved(t, in.Cwd) || !slices.Equal(rec.Cmd, in.Cmd) || rec.PID <= 0 || rec.Process == nil {
					t.Fatalf("record %+v", rec)
				}
				switch side {
				case SideTUI:
					// The TUI may already have reported idle through OSC 7501, and
					// omp names its session at once; OpenCode only after a turn.
					if info.Status == session.StatusExited || s.Identity().PID != rec.PID ||
						(profile == agentprofile.OpenCode && info.Hybrid.AgentSessionID != nil) {
						t.Fatalf("terminal side: status %s pid %d record pid %d", info.Status, s.Identity().PID, rec.PID)
					}
				case SideACP:
					if info.Status != session.StatusIdle || s.Identity().PID != 0 || info.Generation != "" {
						t.Fatalf("chat side runs a terminal: %+v", info)
					}
					if info.Hybrid.AgentSessionID == nil || rec.AgentSessionID != *info.Hybrid.AgentSessionID {
						t.Fatalf("agent session %v, record %q", info.Hybrid.AgentSessionID, rec.AgentSessionID)
					}
					if _, ok := rec.Config["model"]; !ok {
						t.Fatalf("record config %v", rec.Config)
					}
					sub, err := h.svc.Subscribe(info.ID, nil)
					if err != nil {
						t.Fatal(err)
					}
					st := sub.Snapshot.State
					sub.Close()
					if st.AgentVersion == "" || st.Steering != (profile == agentprofile.OMP) || len(st.Config) != 2 || st.AgentSessionID == nil {
						t.Fatalf("chat state %+v", st)
					}
				}
				if strings.Contains(h.file(), "secret-value") {
					t.Fatal("record file holds an environment value")
				}
				h.closeSession(info.ID)
				rec = h.record(info.ID)
				if rec.State != StateClosed || rec.PID != 0 || rec.Process != nil {
					t.Fatalf("closed record %+v", rec)
				}
				file := h.file()
				if !strings.Contains(file, `"closed"`) || strings.Contains(file, "secret-value") {
					t.Fatalf("record file after close: %s", file)
				}
				if _, err := h.svc.Subscribe(info.ID, nil); !errors.Is(err, ErrNotFound) {
					t.Fatalf("subscribe after close: %v", err)
				}
			})
		}
	}
}

func TestCreateACPFailureRemovesSessionAndReturnsStderr(t *testing.T) {
	h := newHarness(t, 0)
	// Without a store the fake agent prints its error to stderr and exits.
	in := h.input(agentprofile.OMP, SideACP, "FAKEAGENT_STORE=")
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	_, err := h.svc.Create(ctx, in)
	if !errors.Is(err, ErrAgent) || !strings.Contains(err.Error(), "fakeagent:") {
		t.Fatalf("create: %v", err)
	}
	if list := h.manager.List(); len(list) != 0 {
		t.Fatalf("failed session listed: %+v", list)
	}
	if all := h.svc.records.all(); len(all) != 0 {
		t.Fatalf("failed session recorded: %+v", all)
	}
	if strings.Contains(h.logs.String(), "fakeagent:") {
		t.Fatalf("agent stderr logged: %s", h.logs.String())
	}
}

func TestCreateRejectsAgentErrors(t *testing.T) {
	h := newHarness(t, 0)
	for _, row := range []struct {
		env  string
		want string
	}{
		{"FAKEAGENT_FAIL=session/new=-32603", "-32603"},
		{"FAKEAGENT_PROTOCOL_VERSION=2", "协议版本不受支持"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		_, err := h.svc.Create(ctx, h.input(agentprofile.OpenCode, SideACP, row.env))
		cancel()
		if !errors.Is(err, ErrAgent) || !strings.Contains(err.Error(), row.want) {
			t.Fatalf("%s: %v", row.env, err)
		}
	}
	if len(h.manager.List()) != 0 || len(h.svc.records.all()) != 0 {
		t.Fatal("failed starts left sessions or records")
	}
}

func TestCreateValidatesInput(t *testing.T) {
	h := newHarness(t, 0)
	in := h.input(agentprofile.OMP, "chat")
	if _, err := h.svc.Create(context.Background(), in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad start side: %v", err)
	}
	in = h.input(agentprofile.OMP, SideTUI)
	in.Cmd = []string{"bash"}
	if _, err := h.svc.Create(context.Background(), in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported command: %v", err)
	}
	if len(h.manager.List()) != 0 {
		t.Fatal("invalid input created a session")
	}
}

func TestCorruptStoreDisablesHybridOnly(t *testing.T) {
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}}
	path := filepath.Join(h.state, "hybrid-sessions.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.open(0)
	if err := h.svc.Unavailable(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unavailable: %v", err)
	}
	_, err := h.svc.Create(context.Background(), h.input(agentprofile.OMP, SideTUI))
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), path) {
		t.Fatalf("create on a corrupt store: %v", err)
	}
	if len(h.manager.List()) != 0 {
		t.Fatal("hybrid session started without a record")
	}
	plain, err := h.manager.Create(session.CreateInput{Cwd: t.TempDir(), Cmd: []string{"sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatalf("ordinary session: %v", err)
	}
	if plain.Info().Hybrid != nil {
		t.Fatal("ordinary session has hybrid info")
	}
	if b, _ := os.ReadFile(path); string(b) != "{broken" {
		t.Fatalf("corrupt store rewritten: %q", b)
	}
}

func TestRecordLimit(t *testing.T) {
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}}
	r := openRecords(h.state)
	for i := range MaxRecords {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		if err := r.add(rec); err != nil {
			t.Fatal(err)
		}
	}
	r.close()
	h.open(0)
	_, err := h.svc.Create(context.Background(), h.input(agentprofile.OMP, SideTUI))
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("create past the limit: %v", err)
	}
	if len(h.manager.List()) != 0 {
		t.Fatal("session started past the record limit")
	}
}

// limitID is the i-th distinct record ID.
func limitID(i int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	return fmt.Sprintf("lim%c%c%c%c%c", alphabet[i/32/32%32], alphabet[i/32%32], alphabet[i%32], 'a', 'a')
}

func TestRecordedIDsAreReserved(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideTUI))
	id := s.Info().ID
	h.closeSession(id)
	if !h.svc.records.reserved(id) {
		t.Fatal("closed session ID not reserved")
	}
}

func TestTerminalExitInterrupts(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideTUI))
	id := s.Info().ID
	pid := s.Identity().PID
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "interrupted record", func() bool { return h.record(id).State == StateInterrupted })
	waitFor(t, "interrupted overlay", func() bool {
		info := s.Info()
		return info.Hybrid != nil && info.Hybrid.State == string(StateInterrupted) && info.Status == session.StatusExited
	})
	if rec := h.record(id); rec.InterruptedAt == nil || rec.PID != 0 || rec.Activity != "idle" {
		t.Fatalf("interrupted record %+v", rec)
	}
	if _, err := h.manager.Get(id); err != nil {
		t.Fatal("interrupted shell removed")
	}
	h.closeSession(id)
	if rec := h.record(id); rec.State != StateInterrupted {
		t.Fatalf("closing an interrupted session changed its record: %+v", rec)
	}
}

func TestRestartMarksActiveInterrupted(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	h.svc.Close()
	if rec, _ := openRecordsForTest(t, h.state).get(id); rec.State != StateActive {
		t.Fatalf("shutdown changed the record: %+v", rec)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.manager.Shutdown(ctx)
	h.open(0)
	rec := h.record(id)
	if rec.State != StateInterrupted || rec.Side != SideACP || rec.InterruptedAt == nil {
		t.Fatalf("record after restart %+v", rec)
	}
}

// TestCloseDuringStart: a service closing while ACP starts are in flight
// fails each Create instead of leaving it waiting, and every agent process
// a start opened is stopped.
func TestCloseDuringStart(t *testing.T) {
	h := newHarness(t, 0)
	const sessions = 4
	errs := make(chan error, sessions)
	for range sessions {
		in := h.input(agentprofile.OMP, SideACP, "FAKEAGENT_DELAY=initialize=500ms")
		go func() {
			_, err := h.svc.Create(context.Background(), in)
			errs <- err
		}()
	}
	waitFor(t, "agents starting", func() bool { return countCalls(t, h.store, acp.MethodInitialize) == sessions })
	pids := callerPIDs(t, h.store, acp.MethodInitialize)
	h.svc.Close()
	for range sessions {
		select {
		case err := <-errs:
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("create while closing: %v", err)
			}
		case <-time.After(testTimeout):
			t.Fatal("create still waiting after the service closed")
		}
	}
	for _, pid := range pids {
		waitFor(t, "agent "+strconv.Itoa(pid)+" stopped", func() bool { return syscall.Kill(pid, 0) != nil })
	}
}

// callerPIDs lists the fake agent processes that received method.
func callerPIDs(t *testing.T, store, method string) []int {
	t.Helper()
	text, err := readEventsLog(store)
	if err != nil {
		t.Fatal(err)
	}
	var pids []int
	for _, line := range strings.Split(text, "\n") {
		if f := strings.Fields(line); len(f) >= 5 && f[3] == "call" && f[4] == method {
			pid, err := strconv.Atoi(f[1])
			if err != nil {
				t.Fatal(err)
			}
			pids = append(pids, pid)
		}
	}
	return pids
}

// openRecordsForTest reads the store without locking it.
func openRecordsForTest(t *testing.T, dir string) *records {
	t.Helper()
	r := openRecords(dir)
	r.close()
	return r
}
