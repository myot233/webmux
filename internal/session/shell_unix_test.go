//go:build linux || darwin

package session

import (
	"context"
	"errors"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
)

type fakeDriver struct {
	closes chan struct{}
	exits  chan int
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{closes: make(chan struct{}, 4), exits: make(chan int, 4)}
}
func (d *fakeDriver) Close() error       { d.closes <- struct{}{}; return nil }
func (d *fakeDriver) RunExited(code int) { d.exits <- code }

func shellManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m
}

func createShell(t *testing.T, m *Manager, d Driver, argv ...string) *Session {
	t.Helper()
	s, err := m.CreateShell(ShellInput{CreateInput: CreateInput{Cmd: []string{"agent"}}, Argv: argv}, d)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// nextMessage waits for the subscriber's next queued message.
func nextMessage(t *testing.T, sub *Subscriber) Message {
	t.Helper()
	select {
	case m := <-sub.Out:
		return m
	case <-time.After(10 * time.Second):
		t.Fatal("no terminal message")
		return Message{}
	}
}

func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestEndRunMovesSubscribersWithoutExit(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	driver := newFakeDriver()
	s := createShell(t, m, driver, "/bin/sh", "-c", "printf READY; exec sleep 30")
	waitForOutput(t, s, "READY")
	s.SetOverlay(&Overlay{Hybrid: &HybridInfo{Side: "acp", Phase: "switching"}})
	sub := attached(s.AttachClient(AttachOptions{Client: "browser"}))
	s.mu.Lock()
	pid := s.run.cmd.Process.Pid
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.EndRun(ctx); err != nil {
		t.Fatal(err)
	}
	if !processGone(pid) {
		t.Fatal("EndRun returned while the run's leader still exists")
	}
	for {
		message := nextMessage(t, sub)
		if message.Exit != nil {
			t.Fatal("a run ended for a switch published an exit frame")
		}
		if message.Moved != nil {
			if *message.Moved != "acp" {
				t.Fatalf("moved to %q", *message.Moved)
			}
			break
		}
	}
	if len(sub.Out) != 0 {
		t.Fatalf("messages after moved: %v", queuedKinds(sub))
	}
	info := s.Info()
	if info.ExitCode != nil || info.Status != StatusIdle || info.Clients != 0 || info.Hybrid == nil || info.Hybrid.Side != "acp" {
		t.Fatalf("shell after EndRun: %+v", info)
	}
	if _, err := s.AttachClient(AttachOptions{}); !errors.Is(err, ErrNoRun) {
		t.Fatalf("attach without a run: %v", err)
	}
	if err := s.Write([]byte("x")); !errors.Is(err, ErrNoRun) {
		t.Fatalf("write without a run: %v", err)
	}
	if err := s.EndRun(ctx); !errors.Is(err, ErrNoRun) {
		t.Fatalf("second EndRun: %v", err)
	}
	if _, err := m.Get(info.ID); err != nil {
		t.Fatal("shell left the manager")
	}
	select {
	case code := <-driver.exits:
		t.Fatalf("EndRun reported a natural exit %d", code)
	case <-s.Done():
		t.Fatal("shell ended with its run")
	default:
	}
}

// A replay request that arrives after MoveClients must not discard Moved: the
// run goes on, but the connection has to learn that it was taken off.
func TestReplayAfterMoveClientsKeepsMoved(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s := createShell(t, m, newFakeDriver(), "/bin/sh", "-c", "printf READY; exec sleep 30")
	waitForOutput(t, s, "READY")
	s.SetOverlay(&Overlay{Hybrid: &HybridInfo{Side: "acp", Phase: "switching"}})
	sub := attached(s.AttachClient(AttachOptions{Client: "browser"}))
	s.MoveClients()
	s.Replay(sub)
	for {
		message := nextMessage(t, sub)
		if message.Moved != nil {
			if *message.Moved != "acp" {
				t.Fatalf("moved to %q", *message.Moved)
			}
			break
		}
	}
	if len(sub.Out) != 0 {
		t.Fatalf("messages after moved: %v", queuedKinds(sub))
	}
}

func TestStartRunAfterEndRunResetsOldGeneration(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s := createShell(t, m, newFakeDriver(), "/bin/sh", "-c", "printf FIRST; exec sleep 30")
	waitForOutput(t, s, "FIRST")
	if err := s.StartRun([]string{"/bin/sleep", "30"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("StartRun over a live run: %v", err)
	}
	old := s.Info().Generation
	s.mu.Lock()
	_, end := s.run.ring.Range()
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.EndRun(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.StartRun([]string{"/bin/sh", "-c", "printf SECOND; exec sleep 30"}); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, s, "SECOND")
	info := s.Info()
	if info.Generation == old || info.Status == StatusExited || info.ExitCode != nil {
		t.Fatalf("new run: %+v", info)
	}
	sub := attached(s.AttachClient(AttachOptions{Client: "browser", Resume: &ResumePoint{Generation: old, Offset: end}}))
	defer s.Detach(sub)
	if control := nextMessage(t, sub); control.Control == nil {
		t.Fatalf("control first: %+v", control)
	}
	position := nextMessage(t, sub).Position
	if position == nil || position.Mode != "reset" || position.Generation != info.Generation {
		t.Fatalf("resume of the old generation: %+v", position)
	}
	if snapshot := nextMessage(t, sub).Frame; len(snapshot) == 0 || snapshot[0] != 1 || string(snapshot[1:]) != "SECOND" {
		t.Fatalf("snapshot %q", snapshot)
	}
}

func TestManagerCloseDelegatesToDriver(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	driver := newFakeDriver()
	s := createShell(t, m, driver, "/bin/sleep", "30")
	if err := m.Close(s.Info().ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-driver.closes:
	case <-time.After(time.Second):
		t.Fatal("Manager.Close did not reach the driver")
	}
	time.Sleep(100 * time.Millisecond)
	if info := s.Info(); info.Status == StatusExited {
		t.Fatalf("Manager.Close hung up a driven shell itself: %+v", info)
	}
}

func TestShutdownEndsShellsWithoutDriver(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	driver := newFakeDriver()
	s := createShell(t, m, driver, "/bin/sleep", "30")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Shutdown(ctx)
	select {
	case <-s.Done():
	default:
		t.Fatal("shutdown did not end the shell")
	}
	if info := s.Info(); info.Status != StatusExited {
		t.Fatalf("run survived shutdown: %+v", info)
	}
	select {
	case <-driver.closes:
		t.Fatal("shutdown closed through the driver")
	case <-driver.exits:
		t.Fatal("shutdown reported a natural run exit")
	default:
	}
	if err := s.StartRun([]string{"/bin/sleep", "30"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("StartRun after shutdown: %v", err)
	}
}

func TestDriverShellNaturalExitPublishesExit(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	driver := newFakeDriver()
	s := createShell(t, m, driver, "/bin/sh", "-c", "read -r line; exit 3")
	sub := attached(s.AttachClient(AttachOptions{Client: "browser"}))
	defer s.Detach(sub)
	if err := s.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	for {
		if message := nextMessage(t, sub); message.Exit != nil {
			if *message.Exit != 3 {
				t.Fatalf("exit %d", *message.Exit)
			}
			break
		} else if message.Moved != nil {
			t.Fatal("natural exit sent moved")
		}
	}
	select {
	case code := <-driver.exits:
		if code != 3 {
			t.Fatalf("RunExited(%d)", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("driver was not told of the natural exit")
	}
	id := s.Info().ID
	if _, err := m.Get(id); err != nil {
		t.Fatal("exited shell left the manager")
	}
	if info := s.Info(); info.Status != StatusExited || info.ExitCode == nil || *info.ExitCode != 3 {
		t.Fatalf("exited shell: %+v", info)
	}
	// The last output stays replayable, followed by the exit.
	late := attached(s.AttachClient(AttachOptions{Client: "late"}))
	if kinds := queuedKinds(late); len(kinds) == 0 || kinds[len(kinds)-1] != 3 {
		t.Fatalf("late replay %q", kinds)
	}
	select {
	case <-s.Done():
		t.Fatal("shell ended with its run")
	default:
	}
	if err := m.Remove(id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("removed shell is not done")
	}
}

func TestReservedIDsAreNeverAssigned(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	var mu sync.Mutex
	var reserved []string
	m.SetIDReserver(func(id string) bool {
		mu.Lock()
		defer mu.Unlock()
		if len(reserved) < 20 {
			reserved = append(reserved, id)
			return true
		}
		return false
	})
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	shell := createShell(t, m, nil)
	mu.Lock()
	defer mu.Unlock()
	if len(reserved) != 20 {
		t.Fatalf("reserver consulted %d times", len(reserved))
	}
	for _, id := range []string{s.Info().ID, shell.Info().ID} {
		if slices.Contains(reserved, id) {
			t.Fatalf("reserved ID %s assigned", id)
		}
	}
	// Recovery reuses a reserved ID explicitly; a taken one is refused.
	recovered, err := m.CreateShell(ShellInput{ID: reserved[0]}, nil)
	if err != nil || recovered.Info().ID != reserved[0] {
		t.Fatalf("explicit reserved ID: %v", err)
	}
	if _, err = m.CreateShell(ShellInput{ID: reserved[0]}, nil); !errors.Is(err, ErrExists) {
		t.Fatalf("taken ID: %v", err)
	}
	if _, err = m.CreateShell(ShellInput{ID: "NOT/AN/ID"}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed ID: %v", err)
	}
}

func TestOverlayReportsChatSideAndIsPublished(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s := createShell(t, m, newFakeDriver())
	list := m.Subscribe()
	defer m.Unsubscribe(list)
	agentSession := "ses_1"
	since := time.Now()
	overlay := &Overlay{
		Hybrid:    &HybridInfo{Side: "acp", State: "active", Agent: "opencode", AgentSessionID: &agentSession},
		Status:    StatusWaiting,
		Attention: &Attention{Kind: "permission", Source: SourceAgent, Since: since},
		Preview:   "Allow edit?",
	}
	s.SetOverlay(overlay)
	agentSession = "changed by caller"
	info := s.Info()
	if info.Status != StatusWaiting || info.Preview != "Allow edit?" || info.Attention == nil || info.Attention.Kind != "permission" ||
		info.Hybrid == nil || info.Hybrid.AgentSessionID == nil || *info.Hybrid.AgentSessionID != "ses_1" {
		t.Fatalf("chat-side info: %+v", info)
	}
	deadline := time.After(5 * time.Second)
	for published := false; !published; {
		select {
		case event := <-list.Out:
			published = event.Type == "updated" && event.Session.ID == info.ID && event.Session.Hybrid != nil && event.Session.Status == StatusWaiting
		case <-deadline:
			t.Fatal("overlay was not published to list subscribers")
		}
	}
	// While a run exists, only Hybrid comes from the overlay.
	if err := s.StartRun([]string{"/bin/sh", "-c", "printf TUI; exec sleep 30"}); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, s, "TUI")
	info = s.Info()
	if info.Hybrid == nil || info.Status == StatusWaiting || info.Attention != nil || info.Preview == "Allow edit?" {
		t.Fatalf("terminal-side info: %+v", info)
	}
	s.SetOverlay(nil)
	if s.Info().Hybrid != nil {
		t.Fatal("cleared overlay still reported")
	}
}

func TestOrdinarySessionHasNoHybridInfo(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Info().Hybrid != nil {
		t.Fatal("ordinary session reports hybrid info")
	}
	if err = s.StartRun([]string{"/bin/sleep", "1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("StartRun on an ordinary session: %v", err)
	}
	if _, err = s.StartSidecar([]string{"/bin/sleep", "1"}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("StartSidecar on an ordinary session: %v", err)
	}
}
