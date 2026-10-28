package session

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/pathutil"
)

// Absorb list event bursts; slow subscribers disconnect rather than blocking sessions.
const sessionListQueueCapacity = 256

type Options struct {
	ScrollbackBytes int
	DefaultCwd      string
	// TerminalVersion is advertised as TERM_PROGRAM_VERSION inside newly
	// created sessions. It is deliberately kept separate from the protocol
	// version so launch presets can still override the terminal identity.
	TerminalVersion string
	IdleAfter       time.Duration
	PromptPatterns  []string
	// Zero values keep every signal source enabled.
	DisableAgentProtocol bool
	DisableBellAttention bool
	// DisableProgramStatus ignores OSC 7501 and leaves its query unanswered.
	DisableProgramStatus bool
}
type Event struct {
	Type     string
	Sessions []Info
	Session  *Info
	ID       string
}
type ListSubscriber struct {
	Out    chan Event
	Slow   chan struct{}
	failed bool
}
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	subs     map[*ListSubscriber]struct{}
	options  Options
	patterns []*regexp.Regexp
	done     chan struct{}
	stopOnce sync.Once
	stopped  bool
	initErr  error
	// createMu serializes process startup without holding mu across fork/exec.
	createMu sync.Mutex
	// reserved reports IDs random generation must skip (SetIDReserver).
	reserved func(id string) bool
}

// Error kinds; the workspace maps them to request errors.
var (
	ErrNotFound     = errors.New("session not found")
	ErrRunning      = errors.New("session is still running")
	ErrInvalid      = errors.New("invalid session request")
	ErrShuttingDown = errors.New("server is shutting down")
)

func NewManager(o Options) *Manager {
	if o.ScrollbackBytes == 0 {
		o.ScrollbackBytes = 2 * 1024 * 1024
	}
	if o.IdleAfter == 0 {
		o.IdleAfter = 30 * time.Second
	}
	if o.TerminalVersion == "" {
		o.TerminalVersion = "dev"
	}
	if o.PromptPatterns == nil {
		o.PromptPatterns = []string{`[$#%>❯›?:]\s*$`, `(?i)\(y/n\)|\[y/n\]|yes.*no|press enter`}
	}
	m := &Manager{
		sessions: make(map[string]*Session),
		subs:     make(map[*ListSubscriber]struct{}),
		options:  o,
		done:     make(chan struct{}),
	}
	m.initErr = enableReaper()
	for _, pattern := range o.PromptPatterns {
		m.patterns = append(m.patterns, regexp.MustCompile(pattern))
	}
	go m.updates()
	return m
}

// CreateInput is the resolved start request. Cmd and Env may come from the
// saved launch config named by ConfigID; the workspace layer validates that
// client input does not combine the two forms.
type CreateInput struct {
	Cwd      string
	Cmd      []string
	Name     string
	Env      map[string]string
	ConfigID string
}

// Create starts a session. Env holds explicit variables added to the server's
// environment for this session only (see ValidateEnv). ConfigID records the
// saved launch config the session came from, empty for an explicit command, so
// clients can apply that config's display settings.
func (m *Manager) Create(in CreateInput) (*Session, error) {
	cwd, err := m.resolveCreate(in)
	if err != nil {
		return nil, err
	}
	if len(in.Cmd) == 0 || in.Cmd[0] == "" {
		return nil, fmt.Errorf("%w: command requires an executable", ErrInvalid)
	}
	// Serialize creates, but run fork/exec and its bounded cleanup outside mu.
	m.createMu.Lock()
	defer m.createMu.Unlock()
	m.mu.Lock()
	stopped := m.stopped
	m.mu.Unlock()
	if stopped {
		return nil, ErrShuttingDown
	}
	id, err := m.newID()
	if err != nil {
		return nil, err
	}
	s, err := start(id, cwd, in.Name, in.Cmd, m.startOptions(in))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err = m.register(s); err != nil {
		return nil, err
	}
	return s, nil
}

// resolveCreate validates the shared create input and returns the launch
// directory.
func (m *Manager) resolveCreate(in CreateInput) (string, error) {
	if m.initErr != nil {
		return "", fmt.Errorf("process reaper: %w", m.initErr)
	}
	if err := ValidateEnv(in.Env); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	cwd := in.Cwd
	if cwd == "" {
		cwd = m.options.DefaultCwd
	}
	cwd, err := pathutil.Expand(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	stat, err := os.Stat(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: directory: %w", ErrInvalid, err)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("%w: path is not a directory", ErrInvalid)
	}
	return cwd, nil
}

func (m *Manager) startOptions(in CreateInput) startOptions {
	return startOptions{
		agentProtocol:   !m.options.DisableAgentProtocol,
		bellAttention:   !m.options.DisableBellAttention,
		programStatus:   !m.options.DisableProgramStatus,
		capacity:        m.options.ScrollbackBytes,
		env:             in.Env,
		configID:        in.ConfigID,
		terminalVersion: m.options.TerminalVersion,
	}
}

// newID draws a random ID that is neither in use nor reserved. Callers hold
// createMu, so no other create can take it before registration.
func (m *Manager) newID() (string, error) {
	m.mu.Lock()
	reserved := m.reserved
	m.mu.Unlock()
	for range 64 {
		var raw [5]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", err
		}
		id := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
		if reserved != nil && reserved(id) {
			continue
		}
		if _, err := m.Get(id); err != nil {
			return id, nil
		}
	}
	return "", errors.New("no free session ID")
}

// register publishes a started session, or ends it when shutdown began.
// Callers hold createMu.
func (m *Manager) register(s *Session) error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		s.Close()
		// Shutdown waits for createMu, so finish cleanup before it snapshots.
		s.awaitProcesses(context.Background())
		if s.shell {
			s.retire(true)
		}
		return ErrShuttingDown
	}
	m.sessions[s.info.ID] = s
	info := s.Info()
	m.publishLocked(Event{Type: "created", Session: &info})
	m.mu.Unlock()
	slog.Info("session created", "id", s.info.ID)
	return nil
}

// Done closes when the manager shuts down. Consumers use it to release resources.
func (m *Manager) Done() <-chan struct{} { return m.done }

func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []Info {
	list := make([]Info, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s.Info())
	}
	sort.Slice(list, func(i, j int) bool { return list[i].LastActiveAt.After(list[j].LastActiveAt) })
	return list
}

// Close handles a user's close request. A driven shell is handed to its
// driver, which decides how it ends or refuses with its own error.
func (m *Manager) Close(id string) error {
	s, err := m.Get(id)
	if err != nil {
		return err
	}
	if s.driver != nil {
		return s.driver.Close()
	}
	s.Close()
	return nil
}

// Remove drops an exited session, or a shell without live processes; the
// shell ends for good first, so it cannot start a process after removal.
func (m *Manager) Remove(id string) error {
	s, err := m.Get(id)
	if err != nil {
		return err
	}
	// retire may wait for a fork in progress: never under mu.
	if s.shell && !s.retire(false) {
		return ErrRunning
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[id] != s {
		return ErrNotFound
	}
	if !s.shell && s.Info().Status != StatusExited {
		return ErrRunning
	}
	delete(m.sessions, id)
	m.publishLocked(Event{Type: "removed", ID: id})
	return nil
}
func (m *Manager) Subscribe() *ListSubscriber {
	m.mu.Lock()
	defer m.mu.Unlock()
	sub := &ListSubscriber{Out: make(chan Event, sessionListQueueCapacity), Slow: make(chan struct{})}
	sub.Out <- Event{Type: "snapshot", Sessions: m.listLocked()}
	m.subs[sub] = struct{}{}
	return sub
}
func (m *Manager) Unsubscribe(sub *ListSubscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subs, sub)
}
func (m *Manager) publishLocked(e Event) {
	for sub := range m.subs {
		if sub.failed {
			continue
		}
		select {
		case sub.Out <- e:
		default:
			sub.failed = true
			close(sub.Slow)
		}
	}
}
func (m *Manager) updates() {
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	previous := make(map[string]Info)
	previewSent := make(map[string]time.Time)
	var snapshot processSnapshot
	probeTick := false
	for {
		select {
		case <-m.done:
			return
		case now := <-timer.C:
			probeTick = !probeTick
			if probeTick {
				table := processes()
				reapAdopted(table)
				snapshot = newProcessSnapshot(table)
			}
			m.mu.Lock()
			for id, s := range m.sessions {
				// Classify every 500 ms using the once-per-second process table.
				// This leaves headroom for delivering the 3-second waiting signal.
				s.evaluate(now, snapshot, m.options.IdleAfter, m.patterns)
				info := s.Info()
				old, known := previous[id]
				if known && now.Sub(previewSent[id]) < 2*time.Second {
					info.Preview = old.Preview
				}
				if !known || !sameDisplay(old, info) {
					m.publishLocked(Event{Type: "updated", Session: &info})
					s.publishState()
					if !known || old.Preview != info.Preview {
						previewSent[id] = now
					}
					previous[id] = info
				}
			}
			for id := range previous {
				if _, exists := m.sessions[id]; !exists {
					delete(previous, id)
					delete(previewSent, id)
				}
			}
			m.mu.Unlock()
		}
	}
}
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	// Wait for an in-flight create to either register or clean up its process
	// before taking the list Shutdown will close.
	m.createMu.Lock()
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	m.createMu.Unlock()
	// Shells are closed without their drivers: their records must survive
	// for recovery after restart.
	for _, s := range list {
		s.Close()
	}
	for _, s := range list {
		s.awaitProcesses(ctx)
		if s.shell {
			s.retire(true)
		}
	}
	reapAdopted(processes())
	m.stopOnce.Do(func() { close(m.done) })
}

// SetWorkspace commits a separately resolved association only for the same
// session and immutable launch directory. No filesystem or Git work runs here.
func (m *Manager) SetWorkspace(id, cwd string, workspace *Workspace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	s.mu.Lock()
	if s.info.Cwd != cwd || sameWorkspace(s.info.Workspace, workspace) {
		s.mu.Unlock()
		return
	}
	if workspace == nil {
		s.info.Workspace = nil
	} else {
		value := *workspace
		s.info.Workspace = &value
	}
	info := s.infoLocked()
	s.broadcastLocked(s.stateLocked())
	s.mu.Unlock()
	m.publishLocked(Event{Type: "updated", Session: &info})
}
