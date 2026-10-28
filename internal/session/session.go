package session

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

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
	Generation   string
	Title        string
	Cwd          string
	Cmd          []string
	Status       Status
	ExitCode     *int
	Preview      string
	CreatedAt    time.Time
	LastActiveAt time.Time
	Cols         uint16
	Rows         uint16
	Clients      int
	Workspace    *Workspace
	Attention    *Attention
	ReportedCwd  *ReportedCwd
	// ConfigID is the saved launch config used at creation, if any.
	ConfigID string
	// Hybrid is set only on a shell whose driver published an overlay.
	Hybrid *HybridInfo
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

type Subscriber struct {
	Out    chan Message
	Slow   chan struct{}
	failed bool
	client string
	label  string
	seq    uint64
}

// Absorb output bursts without blocking the PTY reader; slow subscribers disconnect.
const terminalOutputQueueCapacity = 256

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
	program       *programRecord
	structured    bool
	programStatus bool
	replies       [][]byte
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
	terminalVersion string
	// capacity is the scrollback ring size of every run.
	capacity int
	// env is the caller's explicit launch environment.
	env map[string]string
	// configID is the saved launch config used at creation, if any.
	configID string
}

func newSession(id, cwd, name string, options startOptions) *Session {
	now := time.Now()
	s := &Session{
		info: Info{
			ID:           id,
			Title:        filepath.Base(cwd),
			Cwd:          cwd,
			CreatedAt:    now,
			LastActiveAt: now,
			Cols:         80,
			Rows:         24,
			ConfigID:     options.configID,
		},
		userTitle:     name,
		subs:          make(map[*Subscriber]struct{}),
		done:          make(chan struct{}),
		bellAttention: options.bellAttention,
		programStatus: options.programStatus,
		options:       options,
	}
	s.signals = s.signalHandler()
	if name != "" {
		s.info.Title = name
	}
	return s
}

// start creates an ordinary session: exactly one run, ending with it.
func start(id, cwd, name string, argv []string, options startOptions) (*Session, error) {
	r, generation, err := launchRun(id, cwd, argv, 80, 24, options)
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
		s.overlay.applyTo(&i, s.run == nil)
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
	select {
	case sub.Out <- m:
	default:
		sub.failed = true
		close(sub.Slow)
	}
}
func (s *Session) broadcastLocked(m Message) {
	for sub := range s.subs {
		s.sendLocked(sub, m)
	}
}

// Attach registers an anonymous subscriber with a full replay.
func (s *Session) Attach() (*Subscriber, error) {
	return s.AttachClient(AttachOptions{})
}

// AttachClient atomically captures history and registers the live subscriber:
// no replay gap. The first client of a session without a controller takes
// control; everyone else observes until they send user input or claim it.
// A shell without a run has no terminal to attach to: ErrNoRun.
func (s *Session) AttachClient(o AttachOptions) (*Subscriber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		return nil, ErrNoRun
	}
	if o.Client == "" {
		o.Client = randomClientID()
	}
	s.nextSeq++
	sub := &Subscriber{Out: make(chan Message, terminalOutputQueueCapacity), Slow: make(chan struct{}), client: o.Client, label: o.Label, seq: s.nextSeq}
	if s.control == nil {
		s.setControlLocked(sub)
	} else if s.control.ClientID == sub.client {
		// A reconnect supersedes the connection it replaces.
		s.controlSub = sub
		if s.control.Label != sub.label {
			s.control.Label = sub.label
			s.broadcastLocked(s.controlLocked())
		}
	}
	s.subs[sub] = struct{}{}
	s.replayLocked(sub, o.Resume, nil)
	return sub, nil
}

// Position precedes replayed bytes. "append" continues a terminal at Start
// with the bytes up to End sent as ordinary output; "reset" is followed by a
// snapshot covering Start to End. A truncated reset discards incomplete history
// and starts empty at End, initialized only with synthetic persistent-mode bytes.
// Those synthetic bytes do not correspond to stream offsets.
type Position struct {
	Generation string
	Mode       string
	Start      uint64
	End        uint64
	Truncated  bool
}

// replayLocked queues the grid, position, stream and state for sub. Frames in
// held follow the state; the exit frame stays last because the connection
// closes once it is sent.
func (s *Session) replayLocked(sub *Subscriber, resume *ResumePoint, held []Message) {
	// The grid comes first: an observer must size its terminal before it
	// parses cursor-addressed history, or that history lands on a wrong grid.
	s.sendLocked(sub, s.controlLocked())
	ring := s.run.ring
	start, end := ring.Range()
	position := Position{Generation: s.info.Generation, Mode: "reset", Start: start, End: end, Truncated: start > 0}
	if resume != nil && resume.Generation == s.info.Generation {
		if tail, ok := ring.sinceWithPrefix([]byte{0}, resume.Offset); ok {
			position.Mode, position.Start = "append", resume.Offset
			s.sendLocked(sub, positionMessage(position))
			if len(tail) > 1 {
				s.sendLocked(sub, frameMessage(tail))
			}
		}
	}
	if position.Mode == "reset" {
		if position.Truncated {
			// A raw suffix cannot restore cursor, modes or parser state. It
			// may even start inside a control sequence or UTF-8 character.
			// Later live bytes may still finish a sequence begun before
			// this cut; a redraw is best-effort, not VT state serialization.
			position.Start = end
			s.sendLocked(sub, positionMessage(position))
			s.sendLocked(sub, frameMessage(s.preview.recoveryFrame()))
		} else {
			s.sendLocked(sub, positionMessage(position))
			s.sendLocked(sub, frameMessage(ring.snapshotWithPrefix([]byte{1})))
		}
	}
	s.sendLocked(sub, s.stateLocked())
	for _, m := range held {
		s.sendLocked(sub, m)
	}
	if s.info.ExitCode != nil {
		s.sendLocked(sub, exitMessage(*s.info.ExitCode))
	}
	if position.Mode == "reset" && position.Truncated {
		// Subscription and replay are already queued under mu, so any
		// response follows them without a replay/live-output gap. A brief
		// PTY width change requests a redraw even from programs that ignore
		// same-size SIGWINCH, without changing the logical grid or controller.
		s.repaintLocked()
	}
}

// Replay discards queued stream frames, which the new snapshot supersedes,
// but keeps submission statuses: they are not part of the stream and a lost
// one would leave a draft locked.
func (s *Session) Replay(sub *Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		// A run that ended for a switch already released its subscribers.
		return
	}
	var statuses []Message
	for {
		select {
		case m := <-sub.Out:
			if m.Submission != nil {
				statuses = append(statuses, m)
			}
		default:
			s.replayLocked(sub, nil, statuses)
			return
		}
	}
}
func (s *Session) Detach(sub *Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, sub)
	// The newest remaining connection of the same client inherits control.
	var heir *Subscriber
	for other := range s.subs {
		if other.client == sub.client && (heir == nil || other.seq > heir.seq) {
			heir = other
		}
	}
	if heir == nil {
		s.submissions.forgetWatcher(sub.client)
	}
	if s.controlSub != sub {
		return
	}
	// Another connection of the same client takes over; otherwise control is
	// not handed to the remaining observers: their layout must not change
	// because another device went away. The next input claims it.
	s.controlSub = heir
	if heir == nil {
		s.control = nil
		s.broadcastLocked(s.controlLocked())
		return
	}
	// The promoted connection's earlier resizes were refused; the control
	// frame makes it send its grid again.
	s.sendLocked(heir, s.controlLocked())
}
func (s *Session) Write(p []byte) error {
	return s.WriteContext(context.Background(), p)
}
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 || cols > 1000 || rows > 1000 {
		return errors.New("invalid terminal size")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resizeLocked(cols, rows)
}

// ResizeFrom resizes the PTY only if the subscriber is the controlling
// connection, checked in the same critical section as the resize so that a
// takeover cannot interleave between them.
func (s *Session) ResizeFrom(sub *Subscriber, cols, rows uint16) error {
	if cols == 0 || rows == 0 || cols > 1000 || rows > 1000 {
		return errors.New("invalid terminal size")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controlSub != sub {
		return ErrNotController
	}
	return s.resizeLocked(cols, rows)
}

func (s *Session) resizeLocked(cols, rows uint16) error {
	if cols == s.info.Cols && rows == s.info.Rows {
		return nil
	}
	// Same-logical-size attach notifications are intentionally no-ops above:
	// restoring immediately would coalesce the recovery width change away.
	err := s.resizePTYLocked(cols, rows)
	if err == nil {
		if s.repaintTimer != nil {
			s.repaintTimer.Stop()
			s.repaintTimer = nil
		}
		s.info.Cols = cols
		s.info.Rows = rows
		s.info.Preview = s.preview.Resize(cols, rows)
		// Observers follow the controller's grid instead of fitting their own.
		s.broadcastLocked(s.controlLocked())
	}
	return err
}

// repaintLocked briefly changes the PTY width, then restores the logical grid.
// The kernel notifies the actual foreground job on each size change. There is
// no synthetic input, control transfer, or transient control-grid broadcast.
func (s *Session) repaintLocked() {
	r := s.run
	if s.repaintTimer != nil || s.info.ExitCode != nil || r == nil || r.terminal == nil || r.cmd == nil || r.cmd.Process == nil || s.info.Cols == 0 || s.info.Rows == 0 {
		return
	}
	cols := s.info.Cols - 1
	if cols == 0 {
		cols = 2
	}
	if err := s.resizePTYLocked(cols, s.info.Rows); err != nil {
		return
	}
	s.info.Preview = s.preview.Resize(cols, s.info.Rows)
	var timer *time.Timer
	timer = time.AfterFunc(100*time.Millisecond, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.repaintTimer != timer {
			return
		}
		s.repaintTimer = nil
		if s.info.ExitCode != nil || s.run != r {
			return
		}
		if err := s.resizePTYLocked(s.info.Cols, s.info.Rows); err == nil {
			s.info.Preview = s.preview.Resize(s.info.Cols, s.info.Rows)
		}
	})
	s.repaintTimer = timer
}
