package hybrid

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// subscriberQueue bounds the frames waiting for one chat subscriber; a
// subscriber that falls this far behind is dropped rather than slowing the
// session down.
const subscriberQueue = 256

// Reasons a subscription ends; Subscription.Err reports one of them.
var (
	// ErrSlow: the subscriber did not keep up and was dropped (WebSocket
	// close 4408); it reconnects with its resume point.
	ErrSlow = errors.New("聊天订阅跟不上事件，已断开")
	// ErrReset: the log started a new generation; reconnecting yields a
	// reset snapshot.
	ErrReset = errors.New("聊天事件日志已重置")
	// ErrEnded: the hybrid session is gone.
	ErrEnded = errors.New("混合会话已结束")
)

// ResumePoint is where a reconnecting client left off: the log generation
// and the next sequence it expects.
type ResumePoint struct {
	Generation string
	Seq        uint64
}

// Snapshot opens a subscription. From is the oldest cached sequence and
// Next the sequence the next event gets. Reset tells a resuming client to
// discard its timeline; Truncated that older events were dropped.
type Snapshot struct {
	State      ChatState
	Generation string
	From       uint64
	Next       uint64
	Reset      bool
	Truncated  bool
}

// Frame is one live update: Events of Generation, or the whole State.
type Frame struct {
	Generation string
	Events     []Event
	State      *ChatState
}

// Subscription is a gap-free view of a session's chat: Snapshot and Events
// (the log from the resume point) were taken together with the
// registration of Out, so the first frame on Out follows them directly.
// Done closes when the subscription ends; Err says why. Close releases it.
type Subscription struct {
	Snapshot Snapshot
	Events   []Event
	Out      <-chan Frame
	Done     <-chan struct{}

	log    *chatLog
	out    chan Frame
	done   chan struct{}
	reason error
}

// Err reports why Done closed: ErrSlow, ErrReset or ErrEnded; nil while
// the subscription is live or after Close.
func (s *Subscription) Err() error {
	s.log.mu.Lock()
	defer s.log.mu.Unlock()
	return s.reason
}

// Close unsubscribes. It is safe to call more than once.
func (s *Subscription) Close() {
	s.log.mu.Lock()
	defer s.log.mu.Unlock()
	s.log.dropLocked(s, nil)
}

// chatLog is a session's event log and published chat state. The actor is
// the only writer; subscribers read under mu, which is never held across
// blocking work: frames go out with non-blocking sends.
//
// The log is bounded by an estimate of the events' encoded size (see
// eventSize): the service never encodes client JSON itself. Events and
// state are immutable once stored; the actor builds new slices for every
// change, so readers share them without copying.
type chatLog struct {
	mu         sync.Mutex
	capacity   int
	generation string
	events     []Event
	sizes      []int
	bytes      int
	next       uint64
	truncated  bool
	state      ChatState
	subs       map[*Subscription]struct{}
	ended      bool
}

func newChatLog(capacity int, state ChatState) *chatLog {
	return &chatLog{capacity: capacity, generation: newGeneration(), next: 1, state: state, subs: make(map[*Subscription]struct{})}
}

func newGeneration() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return hex.EncodeToString(raw[:])
}

// append stores events, assigning sequence numbers and times, drops the
// oldest events beyond the byte budget (always keeping the newest), and
// publishes them.
func (l *chatLog) append(now time.Time, events ...Event) {
	if len(events) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return
	}
	stored := make([]Event, len(events))
	for i, e := range events {
		e.Seq, e.At = l.next, now
		l.next++
		size := eventSize(e)
		l.events = append(l.events, e)
		l.sizes = append(l.sizes, size)
		l.bytes += size
		stored[i] = e
	}
	for l.bytes > l.capacity && len(l.events) > 1 {
		l.bytes -= l.sizes[0]
		l.events[0] = Event{}
		l.events, l.sizes = l.events[1:], l.sizes[1:]
		l.truncated = true
	}
	l.publishLocked(Frame{Generation: l.generation, Events: stored})
}

// setState replaces the published state.
func (l *chatLog) setState(state ChatState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return
	}
	l.state = state
	l.publishLocked(Frame{State: &state})
}

// restart begins a new generation with an empty log (session/load replays
// the history into it). Live subscribers end with ErrReset and reconnect.
func (l *chatLog) restart() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.generation = newGeneration()
	l.events, l.sizes, l.bytes, l.truncated = nil, nil, 0, false
	for sub := range l.subs {
		l.dropLocked(sub, ErrReset)
	}
}

// end closes the log for good; subscribers end with ErrEnded.
func (l *chatLog) end() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ended = true
	for sub := range l.subs {
		l.dropLocked(sub, ErrEnded)
	}
}

func (l *chatLog) fromLocked() uint64 {
	if len(l.events) == 0 {
		return l.next
	}
	return l.events[0].Seq
}

// subscribe takes the snapshot, the events after resume (all cached events
// when resume is nil or no longer in the log) and registers the live
// channel, all under one lock.
func (l *chatLog) subscribe(resume *ResumePoint) (*Subscription, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return nil, ErrNotFound
	}
	from := l.fromLocked()
	snap := Snapshot{State: l.state, Generation: l.generation, From: from, Next: l.next, Truncated: l.truncated}
	start := 0
	if resume != nil {
		if resume.Generation == l.generation && resume.Seq >= from && resume.Seq <= l.next {
			start = int(resume.Seq - from)
		} else {
			snap.Reset = true
		}
	}
	sub := &Subscription{Snapshot: snap, Events: append([]Event(nil), l.events[start:]...), log: l, out: make(chan Frame, subscriberQueue), done: make(chan struct{})}
	sub.Out, sub.Done = sub.out, sub.done
	l.subs[sub] = struct{}{}
	return sub, nil
}

func (l *chatLog) publishLocked(f Frame) {
	for sub := range l.subs {
		select {
		case sub.out <- f:
		default:
			l.dropLocked(sub, ErrSlow)
		}
	}
}

func (l *chatLog) dropLocked(sub *Subscription, reason error) {
	if _, ok := l.subs[sub]; !ok {
		return
	}
	delete(l.subs, sub)
	sub.reason = reason
	close(sub.done)
}

// Per-field allowances of eventSize for keys, quotes and punctuation.
const (
	eventOverhead = 96
	fieldOverhead = 16
)

// eventSize estimates the encoded JSON size of e: every text field's
// length plus a fixed allowance per field and per event. Escaping is
// ignored, so text needing escapes is underestimated; the estimate only
// has to keep the log's memory proportional to scrollback_bytes.
func eventSize(e Event) int {
	n := eventOverhead + len(e.Kind)
	s := func(v string) { n += fieldOverhead + len(v) }
	sp := func(v *string) {
		if v != nil {
			s(*v)
		}
	}
	if m := e.Message; m != nil {
		s(m.MessageID)
		s(m.Role)
		s(m.Text)
	}
	if t := e.Tool; t != nil {
		s(t.ToolCallID)
		sp(t.Kind)
		sp(t.Title)
		sp(t.Status)
		for _, c := range t.Content {
			n += contentSize(c)
		}
		for _, loc := range t.Locations {
			s(loc.Path)
		}
	}
	if p := e.Plan; p != nil {
		for _, entry := range p.Entries {
			s(entry.Content)
			s(entry.Priority)
			s(entry.Status)
		}
	}
	for _, c := range e.Commands {
		s(c.Name)
		s(c.Description)
		s(c.Hint)
	}
	for _, c := range e.Config {
		n += configSize(c)
	}
	if e.Usage != nil {
		n += 4 * fieldOverhead
	}
	if e.Info != nil {
		sp(e.Info.Title)
	}
	if c := e.Compaction; c != nil {
		s(c.ID)
		s(c.Status)
		s(c.Summary)
	}
	if c := e.Notice; c != nil {
		s(c.Level)
		s(c.Text)
	}
	if t := e.TurnStart; t != nil {
		s(t.TurnID)
		s(t.PromptID)
	}
	if t := e.TurnEnd; t != nil {
		s(t.TurnID)
		s(t.StopReason)
		s(t.Error)
	}
	if r := e.Request; r != nil {
		n += requestSize(*r)
	}
	if r := e.Resolved; r != nil {
		s(r.RequestID)
		s(r.Outcome)
		s(r.OptionID)
	}
	if m := e.Marker; m != nil {
		s(m.Kind)
		s(string(m.Side))
	}
	return n
}

func contentSize(c Content) int {
	n := 8 * fieldOverhead
	n += len(c.Type) + len(c.Text) + len(c.Path) + len(c.NewText) + len(c.URI) + len(c.Name) + len(c.MimeType) + len(c.Unsupported)
	if c.OldText != nil {
		n += len(*c.OldText)
	}
	return n
}

func configSize(c ConfigOption) int {
	n := 6*fieldOverhead + len(c.ID) + len(c.Name) + len(c.Category) + len(c.Type) + len(c.Value)
	for _, o := range c.Options {
		n += 3*fieldOverhead + len(o.Value) + len(o.Name) + len(o.Description)
	}
	return n
}

func requestSize(r Request) int {
	n := 5*fieldOverhead + len(r.RequestID) + len(r.Kind) + len(r.ToolCallID) + len(r.Title)
	for _, o := range r.Options {
		n += 3*fieldOverhead + len(o.OptionID) + len(o.Name) + len(o.Kind)
	}
	if f := r.Form; f != nil {
		n += fieldOverhead + len(f.Message)
		for _, field := range f.Fields {
			n += 6*fieldOverhead + len(field.Name) + len(field.Title) + len(field.Description) + len(field.Type) + len(field.Default)
			for i, v := range field.Enum {
				n += fieldOverhead + len(v)
				if i < len(field.EnumNames) {
					n += fieldOverhead + len(field.EnumNames[i])
				}
			}
		}
	}
	return n
}
