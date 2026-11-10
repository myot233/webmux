package session

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/contract"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/pathutil"
	"github.com/rainchan/webmux/internal/procpeek"
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
	// ExcludePorts are WebMux's own listening ports, never reported in
	// Info.ListeningPorts.
	ExcludePorts []int
	// Executable is the running server's executable, advertised to every
	// session as WEBMUX_BIN; empty advertises nothing.
	Executable string
	// PreLaunch, when set, runs once for every session about to start a
	// command, with the resolved launch directory, the argv and the session's
	// explicit environment. It prepares the command's surroundings (for
	// example an agent's folder-trust record) and returns the environment for
	// the launch. It must not block for long or fail the launch.
	PreLaunch func(cwd string, argv []string, env map[string]string) map[string]string
	// SubmitDelay computes how long a command's terminal UI needs between
	// submitted text and Enter. Nil means every command submits without delay.
	SubmitDelay func(argv []string) time.Duration
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
	// updatesDone closes when the classification loop has returned.
	updatesDone chan struct{}
	stopOnce    sync.Once
	stopped     bool
	initErr     error
	// createMu serializes process startup without holding mu across fork/exec.
	createMu sync.Mutex
	// reserved reports IDs random generation must skip (SetIDReserver).
	reserved func(id string) bool
	// socket is the local API socket advertised as WEBMUX_SOCKET (SetSocket).
	socket string
}

// Error kinds; the workspace maps them to request errors.
var (
	ErrNotFound     = errors.New("session not found")
	ErrRunning      = errors.New("session is still running")
	ErrInvalid      = errors.New("invalid session request")
	ErrShuttingDown = errors.New("server is shutting down")
)

// Terminal grid limits: a new PTY is DefaultCols by DefaultRows, and every
// resize and requested initial size is 1..MaxDimension on both axes.
const (
	DefaultCols  = 80
	DefaultRows  = 24
	MaxDimension = contract.MaxTerminalDimension
)

// ValidateSize accepts a terminal grid of 1..MaxDimension on both axes.
func ValidateSize(cols, rows int) error {
	if cols < 1 || rows < 1 || cols > MaxDimension || rows > MaxDimension {
		return fmt.Errorf("terminal size must be 1..%d columns by 1..%d rows", MaxDimension, MaxDimension)
	}
	return nil
}

// ValidateInitialSize validates the optional initial size of a create: both
// zero means the default, otherwise both must be set and in range.
func ValidateInitialSize(cols, rows int) error {
	if cols == 0 && rows == 0 {
		return nil
	}
	if cols == 0 || rows == 0 {
		return errors.New("cols and rows must be set together")
	}
	return ValidateSize(cols, rows)
}

// requestedSize is ValidateInitialSize as the PTY grid; zero means default.
func requestedSize(cols, rows int) (uint16, uint16, error) {
	if err := ValidateInitialSize(cols, rows); err != nil {
		return 0, 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return uint16(cols), uint16(rows), nil
}

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
		sessions:    make(map[string]*Session),
		subs:        make(map[*ListSubscriber]struct{}),
		options:     o,
		done:        make(chan struct{}),
		updatesDone: make(chan struct{}),
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
	// Cols and Rows are the PTY's initial size, both zero for the default;
	// setting only one is invalid.
	Cols, Rows int
}

// Create starts a session. Env holds explicit variables added to the server's
// environment for this session only (see ValidateEnv). ConfigID records the
// saved launch config the session came from, empty for an explicit command, so
// clients can apply that config's display settings.
func (m *Manager) Create(in CreateInput) (*Session, error) {
	cwd, err := m.resolveCreate(&in)
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

// resolveCreate validates the shared create input, returns the launch
// directory and lets the pre-launch hook update the input's environment.
func (m *Manager) resolveCreate(in *CreateInput) (string, error) {
	if m.initErr != nil {
		return "", fmt.Errorf("process reaper: %w", m.initErr)
	}
	if err := ValidateEnv(in.Env); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if _, _, err := requestedSize(in.Cols, in.Rows); err != nil {
		return "", err
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
	if m.options.PreLaunch != nil && len(in.Cmd) > 0 {
		in.Env = m.options.PreLaunch(cwd, slices.Clone(in.Cmd), maps.Clone(in.Env))
	}
	return cwd, nil
}

func (m *Manager) startOptions(in CreateInput) startOptions {
	cols, rows, _ := requestedSize(in.Cols, in.Rows)
	var submitDelay time.Duration
	if m.options.SubmitDelay != nil {
		submitDelay = m.options.SubmitDelay(in.Cmd)
	}
	return startOptions{
		agentProtocol:   !m.options.DisableAgentProtocol,
		bellAttention:   !m.options.DisableBellAttention,
		programStatus:   !m.options.DisableProgramStatus,
		capacity:        m.options.ScrollbackBytes,
		env:             in.Env,
		configID:        in.ConfigID,
		terminalVersion: m.options.TerminalVersion,
		submitDelay:     submitDelay,
		serverEnv:       m.serverEnv(),
		cols:            cols,
		rows:            rows,
	}
}

// SetSocket records the local API socket that sessions created from now on
// find in WEBMUX_SOCKET. The server calls it once the socket listens, before
// it serves requests.
func (m *Manager) SetSocket(path string) {
	m.mu.Lock()
	m.socket = path
	m.mu.Unlock()
}

// serverEnv lists WEBMUX_SOCKET and WEBMUX_BIN for a new session, leaving out
// a value the server does not know.
func (m *Manager) serverEnv() []string {
	m.mu.Lock()
	socket := m.socket
	m.mu.Unlock()
	var env []string
	if socket != "" {
		env = append(env, contract.SocketEnv+"="+socket)
	}
	if m.options.Executable != "" {
		env = append(env, contract.BinEnv+"="+m.options.Executable)
	}
	return env
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
	defer close(m.updatesDone)
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	previous := make(map[string]Info)
	previewSent := make(map[string]time.Time)
	var snapshot procpeek.Snapshot
	var ports procpeek.PortScanner
	probeTick, portTick := false, false
	for {
		select {
		case <-m.done:
			return
		case now := <-timer.C:
			probeTick = !probeTick
			if probeTick {
				table := procpeek.Processes()
				reapAdopted(table)
				snapshot = procpeek.NewSnapshot(table)
			}
			m.mu.Lock()
			list := make([]*Session, 0, len(m.sessions))
			for _, s := range m.sessions {
				list = append(list, s)
			}
			m.mu.Unlock()
			// Classify every 500 ms using the once-per-second process table.
			// This leaves headroom for delivering the 3-second waiting signal.
			// Classification reads /proc, so it runs without the manager lock.
			// Listening ports change rarely: every other process table.
			if probeTick {
				portTick = !portTick
				if portTick {
					m.scanPorts(&ports, snapshot, list)
				}
			}
			infos := make(map[*Session]Info, len(list))
			for _, s := range list {
				s.evaluate(now, snapshot, m.options.IdleAfter, m.patterns)
				infos[s] = s.Info()
			}
			m.mu.Lock()
			for id, s := range m.sessions {
				info, evaluated := infos[s]
				if !evaluated {
					// Created after classification; the next tick covers it.
					continue
				}
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
	reapAdopted(procpeek.Processes())
	m.stopOnce.Do(func() { close(m.done) })
	<-m.updatesDone
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

// MaxNameBytes bounds a user-chosen session name, the same limit an OSC title
// is held to.
const MaxNameBytes = 512

// Rename gives session id the user-chosen name, which then outranks terminal
// and agent titles. An empty name hands the title back to them, showing the
// launch directory until one arrives. A name is trimmed and may not hold
// control characters or exceed MaxNameBytes.
func (m *Manager) Rename(id, name string) (Info, error) {
	name = strings.TrimSpace(name)
	if len(name) > MaxNameBytes || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return Info{}, fmt.Errorf("%w: 名称不能含控制字符或超过 %d 字节", ErrInvalid, MaxNameBytes)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return Info{}, ErrNotFound
	}
	s.mu.Lock()
	s.userTitle = name
	s.info.Title = filepath.Base(s.info.Cwd)
	s.info.PlaceholderTitle = name == ""
	if name != "" {
		s.info.Title = name
	}
	info := s.infoLocked()
	s.broadcastLocked(s.stateLocked())
	s.mu.Unlock()
	m.publishLocked(Event{Type: "updated", Session: &info})
	return info, nil
}
