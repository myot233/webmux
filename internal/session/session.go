package session

import (
	"context"
	"fmt"
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
	cliAttention  *Attention
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
		userTitle:     name,
		subs:          make(map[*Subscriber]struct{}),
		done:          make(chan struct{}),
		bellAttention: options.bellAttention,
		programStatus: options.programStatus,
		options:       options,
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
			s.sendLocked(sub, frameMessage(s.preview.RecoveryFrame()))
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
	if _, ok := s.subs[sub]; !ok {
		// MoveClients released this subscriber while the run goes on; its
		// queue still holds Moved, which the connection must receive.
		return
	}
	var statuses []Message
	for {
		select {
		case m := <-sub.Out:
			sub.Release(m)
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

// ScriptResize is a resize a script asks for over REST. Label names the
// controller while Take holds control, as ServerInput.Label does.
type ScriptResize struct {
	Cols, Rows int
	Take       bool
	Label      string
}

// ResizeScript resizes the PTY for a script (POST /api/sessions/{id}/resize).
// It is allowed while no client controls the terminal or the controller is
// itself server-side input. A browser or attach controller refuses it with
// ErrNotController unless in.Take is set: the script then takes control for
// the resize and releases it, as SubmitServer does. The check and the resize
// share one critical section, so a takeover cannot interleave between them.
// ErrExited means the run ended; ErrNoRun that the shell has no terminal run.
func (s *Session) ResizeScript(in ScriptResize) error {
	if err := ValidateSize(in.Cols, in.Rows); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	cols, rows := uint16(in.Cols), uint16(in.Rows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		return ErrNoRun
	}
	if s.info.ExitCode != nil {
		return ErrExited
	}
	if s.control == nil || s.controlSub == nil {
		return s.resizeLocked(cols, rows)
	}
	if !in.Take {
		return fmt.Errorf("%w (a browser or attached client controls it; take control for the resize with take, or webmux resize --take)", ErrNotController)
	}
	controller := &Controller{ClientID: serverClientPrefix + randomClientID(), Label: in.Label, Since: time.Now()}
	s.control, s.controlSub = controller, nil
	s.broadcastLocked(s.controlLocked())
	err := s.resizeLocked(cols, rows)
	s.control = nil
	s.broadcastLocked(s.controlLocked())
	return err
}

// ResizeFrom resizes the PTY only if the subscriber is the controlling
// connection, checked in the same critical section as the resize so that a
// takeover cannot interleave between them.
func (s *Session) ResizeFrom(sub *Subscriber, cols, rows uint16) error {
	if err := ValidateSize(int(cols), int(rows)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
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
