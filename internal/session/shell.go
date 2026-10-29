package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

var (
	// ErrNoRun: the shell has no PTY run, e.g. it is on the chat side.
	ErrNoRun = errors.New("session has no terminal run")
	// ErrExists: a requested session ID is already in use.
	ErrExists = errors.New("session ID already exists")
	// ErrClosed: the shell was closed and starts no more processes.
	ErrClosed = errors.New("session is closed")
)

// Driver owns a shell's lifecycle beyond its PTY runs (internal/hybrid).
// The session calls it without holding any session or manager lock.
type Driver interface {
	// Close handles a user's close request (Manager.Close); the driver
	// decides how to end the shell, or refuses with an error Manager.Close
	// returns (a hybrid session in the middle of a switch). Shutdown never
	// calls it.
	Close() error
	// RunExited reports that the current run exited by itself, after its
	// exit was published like an ordinary session's. Runs ended by EndRun
	// or Session.Close are not reported.
	RunExited(code int)
}

// HybridInfo is the driver's description of a hybrid shell.
type HybridInfo struct {
	Side           string
	Phase          string
	Dormant        bool
	State          string
	Agent          string
	AgentSessionID *string
}

// Overlay is what a driver reports for its shell. Hybrid is always shown;
// Status, Attention and Preview replace the PTY-derived values only while
// the shell has no run. A zero Status keeps the shell's own.
type Overlay struct {
	Hybrid    *HybridInfo
	Status    Status
	Attention *Attention
	Preview   string
}

func (h *HybridInfo) clone() *HybridInfo {
	if h == nil {
		return nil
	}
	c := *h
	if h.AgentSessionID != nil {
		id := *h.AgentSessionID
		c.AgentSessionID = &id
	}
	return &c
}

func (o *Overlay) clone() *Overlay {
	c := *o
	c.Hybrid = o.Hybrid.clone()
	if o.Attention != nil {
		attention := *o.Attention
		c.Attention = &attention
	}
	return &c
}

// applyTo writes the overlay into an Info copy that owns no shared pointers.
func (o *Overlay) applyTo(i *Info, noRun bool) {
	i.Hybrid = o.Hybrid.clone()
	if !noRun {
		return
	}
	if o.Status != "" {
		i.Status = o.Status
	}
	i.Attention = nil
	if o.Attention != nil {
		attention := *o.Attention
		i.Attention = &attention
	}
	i.Preview = o.Preview
}

func (o *Overlay) side() string {
	if o == nil || o.Hybrid == nil {
		return ""
	}
	return o.Hybrid.Side
}

// shellState is the part of Session only shells use. shell, driver and the
// identity fields are immutable after creation; the rest is owned by mu.
type shellState struct {
	shell  bool
	driver Driver
	// startMu serializes forking runs and sidecars against Close.
	startMu  sync.Mutex
	closed   bool
	retired  bool
	overlay  *Overlay
	sidecars map[*Sidecar]struct{}
}

// ShellInput is a CreateInput for a shell. Cmd is the launch command shown
// for the shell across its runs. ID reuses a known ID (recovery) and fails
// with ErrExists when taken; empty draws a random unreserved one. A non-empty
// Argv starts the first run immediately; empty starts without a run. Overlay
// is the driver's initial overlay, already part of the created event.
type ShellInput struct {
	CreateInput
	ID      string
	Argv    []string
	Overlay *Overlay
}

// CreateShell registers a shell driven by d. It is listed and published like
// any session.
func (m *Manager) CreateShell(in ShellInput, d Driver) (*Session, error) {
	cwd, err := m.resolveCreate(in.CreateInput)
	if err != nil {
		return nil, err
	}
	if len(in.Argv) > 0 && in.Argv[0] == "" {
		return nil, fmt.Errorf("%w: command requires an executable", ErrInvalid)
	}
	if in.ID != "" && !validID(in.ID) {
		return nil, fmt.Errorf("%w: malformed session ID", ErrInvalid)
	}
	m.createMu.Lock()
	defer m.createMu.Unlock()
	m.mu.Lock()
	stopped := m.stopped
	m.mu.Unlock()
	if stopped {
		return nil, ErrShuttingDown
	}
	id := in.ID
	if id == "" {
		if id, err = m.newID(); err != nil {
			return nil, err
		}
	} else if _, err = m.Get(id); err == nil {
		return nil, ErrExists
	}
	options := m.startOptions(in.CreateInput)
	s := newSession(id, cwd, in.Name, options)
	s.shell = true
	s.driver = d
	s.sidecars = make(map[*Sidecar]struct{})
	s.info.Cmd = append([]string(nil), in.Cmd...)
	s.info.Status = StatusIdle
	if in.Overlay != nil {
		s.overlay = in.Overlay.clone()
	}
	if len(in.Argv) > 0 {
		r, generation, err := launchRun(id, cwd, in.Argv, 80, 24, options)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		s.installRunLocked(r, generation)
		s.startRun(r)
	}
	if err = m.register(s); err != nil {
		return nil, err
	}
	return s, nil
}

// validID accepts the generated form: eight lowercase base32 characters.
func validID(id string) bool {
	if len(id) != 8 {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}

// StartRun starts a new PTY run of the shell under a new generation. The
// previous run must have ended; a resume point of its generation gets a
// reset. A run that exited by itself is replaced, together with the
// subscribers that already received its exit.
func (s *Session) StartRun(argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return fmt.Errorf("%w: command requires an executable", ErrInvalid)
	}
	if !s.shell {
		return fmt.Errorf("%w: not a shell", ErrInvalid)
	}
	s.startMu.Lock()
	defer s.startMu.Unlock()
	s.mu.Lock()
	closed, previous := s.closed, s.run
	cols, rows := s.info.Cols, s.info.Rows
	s.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if previous != nil && !previous.ended() {
		return ErrRunning
	}
	// Only this method installs runs and startMu is held: previous stays.
	r, generation, err := launchRun(s.info.ID, s.info.Cwd, argv, cols, rows, s.options)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	s.mu.Lock()
	if previous != nil {
		s.dropSubscribersLocked(nil)
	}
	s.resetRunStateLocked()
	if cols != 80 || rows != 24 {
		s.info.Preview = s.preview.Resize(cols, rows)
	}
	s.installRunLocked(r, generation)
	s.mu.Unlock()
	s.startRun(r)
	return nil
}

// EndRun ends the current run for a switch: it hangs up the process group,
// waits for process cleanup and PTY drain, then sends every terminal
// subscriber Moved instead of an exit and releases them and the controller.
// It returns once the run's processes are gone, so a following writer is
// the only one. A run that already exited is released the same way.
func (s *Session) EndRun(ctx context.Context) error {
	s.mu.Lock()
	r := s.run
	if r == nil {
		s.mu.Unlock()
		return ErrNoRun
	}
	if r.ended() {
		s.releaseRunLocked()
		s.mu.Unlock()
		return nil
	}
	r.moved = true
	s.mu.Unlock()
	r.close(s.info.ID)
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		forceSession(r.cmd.Process.Pid, s.info.ID)
		return ctx.Err()
	}
}

// releaseRunLocked detaches the current run from the shell: subscribers get
// Moved and the shell reports its overlay until the next run.
func (s *Session) releaseRunLocked() {
	moved := s.overlay.side()
	s.dropSubscribersLocked(&moved)
	s.run = nil
	s.resetRunStateLocked()
	s.info.Status = StatusIdle
}

// dropSubscribersLocked ends every terminal subscription, after sending
// moved when given.
func (s *Session) dropSubscribersLocked(moved *string) {
	if moved != nil {
		s.broadcastLocked(movedMessage(*moved))
	}
	clear(s.subs)
	s.control = nil
	s.controlSub = nil
}

// resetRunStateLocked forgets what the previous run's output established.
func (s *Session) resetRunStateLocked() {
	s.info.ExitCode = nil
	s.info.Attention = nil
	s.info.ReportedCwd = nil
	s.info.Preview = ""
	s.info.Title = filepath.Base(s.info.Cwd)
	if s.userTitle != "" {
		s.info.Title = s.userTitle
	}
	s.osc = OSCScanner{}
	s.preview = Preview{}
	s.program = nil
	s.identity = Identity{}
	s.structured = false
	s.replies = nil
}

// MoveClients ends every terminal connection with Moved while the run goes
// on: a switch that first interrupts the terminal UI takes its clients off
// at once, and they may attach again if the switch is cancelled.
func (s *Session) MoveClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	moved := s.overlay.side()
	s.dropSubscribersLocked(&moved)
}

// SetOverlay replaces the driver's overlay; nil clears it. List subscribers
// see the change through the manager's periodic update.
func (s *Session) SetOverlay(o *Overlay) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o == nil {
		s.overlay = nil
		return
	}
	s.overlay = o.clone()
}

// retire ends a shell for good, so it starts no more processes and Done
// closes. Unless forced, it refuses while a run or sidecar is live.
func (s *Session) retire(force bool) bool {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !force && (len(s.sidecars) > 0 || s.run != nil && !s.run.ended()) {
		return false
	}
	s.closed = true
	if !s.retired {
		s.retired = true
		close(s.done)
	}
	return true
}

// awaitProcesses waits for the current run and sidecars to end, forcing
// them once ctx expires.
func (s *Session) awaitProcesses(ctx context.Context) {
	s.mu.Lock()
	r := s.run
	sidecars := make([]*Sidecar, 0, len(s.sidecars))
	for sidecar := range s.sidecars {
		sidecars = append(sidecars, sidecar)
	}
	s.mu.Unlock()
	if r != nil {
		select {
		case <-r.done:
		case <-ctx.Done():
			forceSession(r.cmd.Process.Pid, s.info.ID)
			awaitForced(r.done)
		}
	}
	for _, sidecar := range sidecars {
		select {
		case <-sidecar.Done():
		case <-ctx.Done():
			forceSession(sidecar.pid, "")
			awaitForced(sidecar.Done())
		}
	}
}

// forcedExitWait bounds the join of a run or sidecar after its tree was
// killed: its goroutines finish promptly unless cleanup itself is stuck.
const forcedExitWait = 2 * time.Second

// awaitForced joins the goroutines of a force-killed process tree, waiting
// at most forcedExitWait so a stuck cleanup cannot hold shutdown.
func awaitForced(done <-chan struct{}) {
	timer := time.NewTimer(forcedExitWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

// SetIDReserver installs, once at assembly, the predicate for IDs that
// random generation must skip because a record still owns them.
func (m *Manager) SetIDReserver(reserved func(id string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reserved = reserved
}
