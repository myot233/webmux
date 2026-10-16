package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusWaiting Status = "waiting"
	StatusIdle    Status = "idle"
	StatusExited  Status = "exited"
)

type Workspace struct {
	RepositoryID string `json:"repositoryId"`
	WorktreeID   string `json:"worktreeId"`
	WorktreePath string `json:"worktreePath"`
}

type Info struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Cwd          string     `json:"cwd"`
	Preset       string     `json:"preset"`
	Cmd          []string   `json:"cmd"`
	Status       Status     `json:"status"`
	ExitCode     *int       `json:"exitCode"`
	Preview      string     `json:"preview"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastActiveAt time.Time  `json:"lastActiveAt"`
	Cols         uint16     `json:"cols"`
	Rows         uint16     `json:"rows"`
	Clients      int        `json:"clients"`
	Workspace    *Workspace `json:"workspace"`
	Attention    *Attention `json:"attention"`
}
type Subscriber struct {
	Out    chan []byte
	Slow   chan struct{}
	failed bool
}
type Session struct {
	mu        sync.Mutex
	inputGate chan struct{}
	info      Info
	userTitle string
	terminal  *os.File
	cmd       *exec.Cmd
	ring      *RingBuffer
	osc       OSCScanner
	signals   *SignalHandler
	// lastInputAt and bellAttention are owned by mu, like info.Attention.
	lastInputAt   time.Time
	bellAttention bool
	preview       Preview
	subs          map[*Subscriber]struct{}
	done          chan struct{}
	readerDone    chan struct{}
	cleanupDone   chan struct{}
	closeOnce     sync.Once
}

type startOptions struct {
	agentProtocol bool
	bellAttention bool
}

func start(id, cwd, preset, name string, argv []string, capacity int, options startOptions) (*Session, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = sessionEnv(os.Environ(), id, options.agentProtocol)
	f, err := startPTY(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return nil, err
	}
	pollable, err := pollablePTY(f)
	if err != nil {
		_ = f.Close()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		releaseLeader(cmd.Process.Pid)
		cleanupProcesses(cmd.Process.Pid, id)
		return nil, err
	}
	f = pollable
	now := time.Now()
	s := &Session{
		info: Info{
			ID:           id,
			Title:        filepath.Base(cwd),
			Cwd:          cwd,
			Preset:       preset,
			Cmd:          append([]string(nil), argv...),
			Status:       StatusRunning,
			CreatedAt:    now,
			LastActiveAt: now,
			Cols:         80,
			Rows:         24,
		},
		userTitle:     name,
		terminal:      f,
		cmd:           cmd,
		ring:          NewRingBuffer(capacity),
		subs:          make(map[*Subscriber]struct{}),
		done:          make(chan struct{}),
		readerDone:    make(chan struct{}),
		bellAttention: options.bellAttention,
	}
	s.signals = s.signalHandler()
	s.inputGate = make(chan struct{}, 1)
	s.inputGate <- struct{}{}
	s.cleanupDone = make(chan struct{})
	if name != "" {
		s.info.Title = name
	}
	go s.read()
	go s.wait()
	return s, nil
}

func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.infoLocked()
}

func (s *Session) infoLocked() Info {
	i := s.info
	i.Clients = len(s.subs)
	i.Cmd = append([]string(nil), i.Cmd...)
	if i.Workspace != nil {
		workspace := *i.Workspace
		i.Workspace = &workspace
	}
	if i.ExitCode != nil {
		code := *i.ExitCode
		i.ExitCode = &code
	}
	if i.Attention != nil {
		attention := *i.Attention
		i.Attention = &attention
	}
	return i
}
func (s *Session) Buffer() []byte        { return s.ring.Snapshot() }
func (s *Session) Done() <-chan struct{} { return s.done }
func Frame(kind byte, p []byte) []byte {
	frame := make([]byte, len(p)+1)
	frame[0] = kind
	copy(frame[1:], p)
	return frame
}
func (s *Session) stateLocked() []byte {
	data, _ := json.Marshal(s.infoLocked())
	return Frame(2, data)
}
func (s *Session) sendLocked(sub *Subscriber, p []byte) {
	if sub.failed {
		return
	}
	select {
	case sub.Out <- p:
	default:
		sub.failed = true
		close(sub.Slow)
	}
}
func (s *Session) broadcastLocked(p []byte) {
	for sub := range s.subs {
		s.sendLocked(sub, p)
	}
}

// Attach atomically captures history and registers the live subscriber: no replay gap.
func (s *Session) Attach() *Subscriber {
	s.mu.Lock()
	sub := &Subscriber{Out: make(chan []byte, 256), Slow: make(chan struct{})}
	s.subs[sub] = struct{}{}
	s.replayLocked(sub)
	s.mu.Unlock()
	return sub
}
func (s *Session) replayLocked(sub *Subscriber) {
	s.sendLocked(sub, s.ring.snapshotWithPrefix([]byte{1}))
	s.sendLocked(sub, s.stateLocked())
	if s.info.ExitCode != nil {
		data, _ := json.Marshal(map[string]int{"exitCode": *s.info.ExitCode})
		s.sendLocked(sub, Frame(3, data))
	}
}
func (s *Session) Replay(sub *Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		select {
		case <-sub.Out:
		default:
			s.replayLocked(sub)
			return
		}
	}
}
func (s *Session) Detach(sub *Subscriber) {
	s.mu.Lock()
	delete(s.subs, sub)
	s.mu.Unlock()
}
func (s *Session) Write(p []byte) error {
	return s.WriteContext(context.Background(), p)
}
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 || cols > 1000 || rows > 1000 {
		return errors.New("invalid terminal size")
	}
	s.mu.Lock()
	err := pty.Setsize(s.terminal, &pty.Winsize{Cols: cols, Rows: rows})
	if err == nil {
		s.info.Cols = cols
		s.info.Rows = rows
		s.info.Preview = s.preview.Resize(cols, rows)
	}
	s.mu.Unlock()
	return err
}
func (s *Session) read() {
	defer close(s.readerDone)
	buf := make([]byte, 32768)
	for {
		n, err := s.terminal.Read(buf)
		if n > 0 {
			p := buf[:n]
			s.mu.Lock()
			s.ring.Write(p)
			s.osc.Feed(p, s.signals)
			s.info.Preview = s.preview.Feed(p)
			s.info.LastActiveAt = time.Now()
			// Latched attention outlives redraws such as spinners and timers.
			if s.info.Attention != nil {
				s.info.Status = StatusWaiting
			} else {
				s.info.Status = StatusRunning
			}
			if len(s.subs) > 0 {
				s.broadcastLocked(Frame(0, p))
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}
func (s *Session) wait() {
	err := s.cmd.Wait()
	releaseLeader(s.cmd.Process.Pid)
	code := 0
	if err != nil {
		code = s.cmd.ProcessState.ExitCode()
		if code < 0 {
			if status, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
				code = 128 + int(status.Signal())
			}
		}
	} // Drain final PTY bytes before publishing exit.
	s.Close()
	<-s.cleanupDone
	select {
	case <-s.readerDone:
	case <-time.After(time.Second):
		_ = s.terminal.Close()
		<-s.readerDone
	}
	_ = s.terminal.Close()
	s.mu.Lock()
	s.info.Status = StatusExited
	s.info.Attention = nil
	s.info.ExitCode = &code
	s.broadcastLocked(s.stateLocked())
	data, _ := json.Marshal(map[string]int{"exitCode": code})
	s.broadcastLocked(Frame(3, data))
	close(s.done)
	s.mu.Unlock()
	slog.Info("session exited", "id", s.info.ID, "exitCode", code)
}
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGHUP)
		go func() {
			defer close(s.cleanupDone)
			cleanupProcesses(s.cmd.Process.Pid, s.info.ID)
		}()
	})
}
