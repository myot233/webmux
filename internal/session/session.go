package session

import (
	"maps"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/vtpreview"
)

type Preview = vtpreview.Preview

type Status = contract.SessionStatus

const (
	StatusRunning = contract.SessionStatusRunning
	StatusWaiting = contract.SessionStatusWaiting
	StatusIdle    = contract.SessionStatusIdle
	StatusExited  = contract.SessionStatusExited
)

type Workspace struct {
	RepositoryID string
	WorktreeID   string
	WorktreePath string
}

// ReportedCwd is the directory a program reported through OSC 7. It is a
// navigation hint only; session ownership, file access roots and command
// execution stay bound to the immutable launch directory and Workspace.
type ReportedCwd struct {
	Path   string
	Host   string
	Remote bool
	Since  time.Time
}

type Info struct {
	ID string
	// Generation distinguishes this run from any other session that could
	// carry the same ID. Output offsets and submissions are bound to it.
	Generation string
	Title      string
	// PlaceholderTitle says Title is still derived from Cwd: no name, OSC
	// title or driver overlay title has replaced it.
	PlaceholderTitle bool
	Cwd              string
	Cmd              []string
	Status           Status
	ExitCode         *int
	Preview          string
	CreatedAt        time.Time
	LastActiveAt     time.Time
	Cols             uint16
	Rows             uint16
	Clients          int
	Workspace        *Workspace
	Attention        *Attention
	ReportedCwd      *ReportedCwd
	// ConfigID is the saved launch config used at creation, if any.
	ConfigID string
	// Hybrid is set only on a shell whose driver published an overlay.
	Hybrid *HybridInfo
	// ListeningPorts are the TCP ports the current run's process tree
	// listens on, refreshed every 2 seconds (see ListeningPortsSupported).
	ListeningPorts []ListeningPort
	// ParentID is the session whose process created this one through the
	// local API; empty otherwise. It lives in memory only.
	ParentID string
	// Labels are caller-defined key-value pairs, in memory only.
	Labels map[string]string
}

// Message is one item queued for a terminal subscriber, in protocol order.
// Frame is raw output (type byte 0) or a replay snapshot (type byte 1),
// shared between subscribers and never modified. Otherwise exactly one of
// the other fields is set; the controller encodes it for the client.
type Message struct {
	Frame      []byte
	Info       *Info
	Exit       *int
	Position   *Position
	Submission *SubmissionStatus
	Control    *ControlState
	// Moved ends a terminal connection because its run ended for a switch;
	// it carries the side reported by the shell's overlay.
	Moved *string
}

// Subscriber is one terminal connection's queue. Whoever receives from Out
// must pass each message to Release so the byte budget reflects the backlog.
type Subscriber struct {
	Out    chan Message
	Slow   chan struct{}
	failed bool
	client string
	label  string
	seq    uint64
	// queued counts frame bytes sent to Out and not yet released.
	queued atomic.Int64
}

// Absorb output bursts without blocking the PTY reader; a subscriber that
// falls behind by more messages or bytes than this disconnects. Bursts of
// small reads need many slots, so the byte budget bounds memory instead.
const (
	terminalOutputQueueCapacity = 1024
	terminalOutputQueueBytes    = 8 << 20
)

// Release records that m has left the queue.
func (sub *Subscriber) Release(m Message) {
	sub.queued.Add(-int64(len(m.Frame)))
}

// AttachOptions identifies the browser behind a connection and where its
// terminal already is. A matching resume point replays only the missing tail.
type AttachOptions struct {
	Client string
	Label  string
	Resume *ResumePoint
}
type ResumePoint struct {
	Generation string
	Offset     uint64
}
type Session struct {
	mu        sync.Mutex
	info      Info
	userTitle string
	// run is the current or last PTY run, owned by mu. Only a shell can be
	// without one: before its first run or after EndRun.
	run     *run
	osc     OSCScanner
	signals *SignalHandler
	// lastInputAt and bellAttention are owned by mu, like info.Attention.
	lastInputAt   time.Time
	bellAttention bool
	// program, structured and replies are owned by mu. program is the OSC
	// 7501 root record; structured records that the process reported a 7501
	// status or a Warp cli-agent event, so the waiting heuristics no longer
	// apply. replies holds session-written query replies that the next
	// holder of the run's inputGate writes before its own input.
	program    *programRecord
	structured bool
	// cliAttention is attention a script raised while a driver's overlay
	// reports the shell (no run); it is owned by mu and replaces the
	// overlay's attention until the driver reports a new state.
	cliAttention       *Attention
	programStatus      bool
	attentionPolicy    AttentionPolicy
	warpRootSessionID  string
	lastEventSessionID string
	turnStartedAt      time.Time
	pendingAttention   *Attention
	doneDelayTimer     *time.Timer
	replies            [][]byte
	// identity holds the current run's identity signals, owned by mu.
	identity Identity
	preview  Preview
	subs     map[*Subscriber]struct{}
	nextSeq  uint64
	control  *Controller
	// controlSub is the controlling client's authoritative connection: the
	// most recent one. A duplicated browser tab can carry the same client ID;
	// only one of its connections may resize the PTY or answer queries.
	controlSub   *Subscriber
	repaintTimer *time.Timer
	submissions  submissionLog
	// done closes when the session ends: with its run for an ordinary
	// session, on removal or shutdown for a shell.
	done    chan struct{}
	options startOptions
	shellState
}

type startOptions struct {
	agentProtocol   bool
	bellAttention   bool
	programStatus   bool
	attentionPolicy AttentionPolicy
	terminalVersion string
	submitDelay     time.Duration
	// serverEnv holds WEBMUX_SOCKET and WEBMUX_BIN entries ("NAME=value")
	// added to every process of the session.
	serverEnv []string
	// capacity is the scrollback ring size of every run.
	capacity int
	// env is the caller's explicit launch environment.
	env map[string]string
	// configID is the saved launch config used at creation, if any.
	configID string
	// cols and rows are the initial terminal size; zero is DefaultCols by
	// DefaultRows.
	cols, rows uint16
}

// size is the initial terminal size with the defaults filled in.
func (o startOptions) size() (cols, rows uint16) {
	cols, rows = o.cols, o.rows
	if cols == 0 || rows == 0 {
		return DefaultCols, DefaultRows
	}
	return cols, rows
}

func newSession(id, cwd, name string, options startOptions) *Session {
	cols, rows := options.size()
	now := time.Now()
	policy := options.attentionPolicy
	if policy.Mode == "" {
		policy = AttentionPolicy{
			Mode:          AttentionModeAll,
			ProgramStatus: options.programStatus,
			AgentProtocol: options.agentProtocol,
			Notify:        true,
			Bell:          options.bellAttention,
		}
	}
	s := &Session{
		info: Info{
			ID:               id,
			Title:            filepath.Base(cwd),
			PlaceholderTitle: name == "",
			Cwd:              cwd,
			CreatedAt:        now,
			LastActiveAt:     now,
			Cols:             cols,
			Rows:             rows,
			ConfigID:         options.configID,
		},
		userTitle:       name,
		subs:            make(map[*Subscriber]struct{}),
		done:            make(chan struct{}),
		bellAttention:   policy.Bell,
		programStatus:   policy.ProgramStatus,
		attentionPolicy: policy,
		turnStartedAt:   now,
		options:         options,
	}
	s.signals = s.signalHandler()
	if cols != DefaultCols || rows != DefaultRows {
		s.info.Preview = s.preview.Resize(cols, rows)
	}
	if name != "" {
		s.info.Title = name
	}
	return s
}

// start creates an ordinary session: exactly one run, ending with it.
func start(id, cwd, name string, argv []string, options startOptions) (*Session, error) {
	cols, rows := options.size()
	r, generation, err := launchRun(id, cwd, argv, cols, rows, options)
	if err != nil {
		return nil, err
	}
	s := newSession(id, cwd, name, options)
	s.installRunLocked(r, generation)
	s.info.Cmd = append([]string(nil), argv...)
	s.startRun(r)
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
	i.ListeningPorts = slices.Clone(i.ListeningPorts)
	i.Labels = maps.Clone(i.Labels)
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
	if i.ReportedCwd != nil {
		reported := *i.ReportedCwd
		i.ReportedCwd = &reported
	}
	if s.overlay != nil {
		s.overlay.applyTo(&i, s.run == nil, s.userTitle == "")
		if s.run == nil && s.cliAttention != nil && i.Status != StatusExited {
			attention := *s.cliAttention
			i.Attention, i.Status = &attention, StatusWaiting
		}
	}
	return i
}

// Buffer returns the current run's retained output, nil without a run.
func (s *Session) Buffer() []byte {
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	if r == nil {
		return nil
	}
	return r.ring.Snapshot()
}
func (s *Session) Done() <-chan struct{} { return s.done }

func (s *Session) stateLocked() Message {
	return infoMessage(s.infoLocked())
}
func (s *Session) sendLocked(sub *Subscriber, m Message) {
	if sub.failed {
		return
	}
	size := int64(len(m.Frame))
	// An empty queue always takes the next message, so a replay snapshot
	// larger than the budget still reaches a new connection.
	if queued := sub.queued.Load(); queued == 0 || queued+size <= terminalOutputQueueBytes {
		select {
		case sub.Out <- m:
			sub.queued.Add(size)
			return
		default:
		}
	}
	sub.failed = true
	close(sub.Slow)
}
func (s *Session) broadcastLocked(m Message) {
	for sub := range s.subs {
		s.sendLocked(sub, m)
	}
}
