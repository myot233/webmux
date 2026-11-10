package hybrid

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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
//
// Over budget, the log first shortens the tool output of old events (see
// degradeLocked) and only then drops the oldest events. A finished run of
// message chunks is stored as one event (see sealLocked), so sequence
// numbers in the log have gaps; spans tells which sequences an event
// stands for.
type chatLog struct {
	mu         sync.Mutex
	capacity   int
	generation string
	events     []Event
	sizes      []int
	spans      []span
	// degraded is how many leading events have no tool output left to
	// shorten, so each append resumes the scan where the last one stopped.
	degraded   int
	bytes      int
	next       uint64
	truncated  bool
	state      ChatState
	subs       map[*Subscription]struct{}
	ended      bool
	nextPrompt *nextPromptJob
}

// span records what a stored event stands for. A merged run of message
// chunks keeps the sequence of its last chunk as Event.Seq, the sequence of
// its first chunk, and the end offset of every chunk's text, so a client
// that applied only the first chunks is sent just the rest. Chunk k of a
// run has sequence first+k: the log only ever loses events at its start.
type span struct {
	first uint64
	ends  []int
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
		l.sealLocked(e)
		e.Seq, e.At = l.next, now
		l.next++
		size := eventSize(e)
		l.events = append(l.events, e)
		l.sizes = append(l.sizes, size)
		l.spans = append(l.spans, span{first: e.Seq})
		l.bytes += size
		stored[i] = e
	}
	if l.bytes > l.capacity {
		l.degradeLocked()
	}
	for l.bytes > l.capacity && len(l.events) > 1 {
		l.bytes -= l.sizes[0]
		l.events[0] = Event{}
		l.events, l.sizes, l.spans = l.events[1:], l.sizes[1:], l.spans[1:]
		l.degraded = max(l.degraded-1, 0)
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
	l.events, l.sizes, l.spans, l.degraded, l.bytes, l.truncated = nil, nil, nil, 0, 0, false
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
	return l.spans[0].first
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
	partial := false
	if resume != nil {
		if resume.Generation == l.generation && resume.Seq >= from && resume.Seq <= l.next {
			// Sequences have gaps where chunks were merged: find the first
			// event that stands for something the client has not applied.
			start = sort.Search(len(l.events), func(i int) bool { return l.events[i].Seq >= resume.Seq })
			partial = start < len(l.events) && l.spans[start].first < resume.Seq
		} else {
			snap.Reset = true
		}
	}
	events := append([]Event(nil), l.events[start:]...)
	if partial {
		events[0] = l.remainderLocked(start, resume.Seq)
	}
	sub := &Subscription{Snapshot: snap, Events: events, log: l, out: make(chan Frame, subscriberQueue), done: make(chan struct{})}
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

const (
	// degradedToolBytes bounds each text of a tool content block that the
	// log shortened to stay within its budget.
	degradedToolBytes = 2 << 10
	// protectedDivisor: the newest capacity/protectedDivisor bytes of the
	// log keep their full tool output.
	protectedDivisor = 4
)

// chunkKey identifies the message a chunk extends. Only agent and thought
// chunks without attachments qualify for merging.
func chunkKey(e Event) (id, role string, ok bool) {
	m := e.Message
	if e.Kind != EventMessage || m == nil || len(m.Attachments) > 0 || (m.Role != RoleAgent && m.Role != RoleThought) {
		return "", "", false
	}
	return m.MessageID, m.Role, true
}

// sealLocked merges the run of message chunks at the end of the log into
// one event once next, the event about to be stored, no longer extends it.
// A run still streaming stays as separate chunks.
func (l *chatLog) sealLocked(next Event) {
	n := len(l.events)
	if n < 2 || l.spans[n-1].ends != nil {
		return
	}
	id, role, ok := chunkKey(l.events[n-1])
	if !ok {
		return
	}
	if nid, nrole, nok := chunkKey(next); nok && nid == id && nrole == role {
		return
	}
	start := n - 1
	for start > 0 && l.spans[start-1].ends == nil {
		pid, prole, pok := chunkKey(l.events[start-1])
		if !pok || pid != id || prole != role {
			break
		}
		start--
	}
	if start < n-1 {
		l.mergeLocked(start)
	}
}

// mergeLocked replaces the chunks from index start to the end of the log by
// one event: the last chunk's sequence, the first chunk's time, the texts
// joined. Chunks are never modified; the merged event is new.
func (l *chatLog) mergeLocked(start int) {
	run := l.events[start:]
	var text strings.Builder
	ends := make([]int, 0, len(run))
	cut := false
	for _, e := range run {
		text.WriteString(e.Message.Text)
		ends = append(ends, text.Len())
		cut = cut || e.Message.Truncated
	}
	first, last := run[0], run[len(run)-1]
	merged := Event{Seq: last.Seq, Kind: EventMessage, At: first.At, Message: &MessageChunk{
		MessageID: first.Message.MessageID, Role: first.Message.Role, Text: text.String(), Truncated: cut,
	}}
	size := eventSize(merged)
	for _, s := range l.sizes[start:] {
		l.bytes -= s
	}
	l.bytes += size
	firstSeq := l.spans[start].first
	clear(l.events[start:])
	l.events = append(l.events[:start], merged)
	l.sizes = append(l.sizes[:start], size)
	l.spans = append(l.spans[:start], span{first: firstSeq, ends: ends})
	// The merged event is a message with no tool output to shorten, so it
	// counts as scanned; the scan position must not point past the log.
	l.degraded = min(l.degraded, start+1)
}

// remainderLocked returns merged event i without the chunks the client
// already applied, those before sequence from.
func (l *chatLog) remainderLocked(i int, from uint64) Event {
	e, sp := l.events[i], l.spans[i]
	applied := int(from - sp.first)
	m := *e.Message
	m.Text = m.Text[sp.ends[applied-1]:]
	e.Message = &m
	return e
}

// degradeLocked shortens the tool output of the oldest events until the log
// fits its budget or only the protected newest part is left. Titles,
// statuses, locations and sequences stay; a client that already received an
// event keeps what it got, and a client that has not gets the short version.
func (l *chatLog) degradeLocked() {
	limit := l.capacity / protectedDivisor
	boundary, kept := len(l.events), 0
	for boundary > 0 && kept+l.sizes[boundary-1] <= limit {
		boundary--
		kept += l.sizes[boundary]
	}
	// An event that alone exceeds the window is not inside it, so it can be
	// shortened: keeping the conversation matters more than one large output.
	i := l.degraded
	for ; i < boundary && l.bytes > l.capacity; i++ {
		if e, ok := degradeTool(l.events[i]); ok {
			size := eventSize(e)
			l.bytes += size - l.sizes[i]
			l.sizes[i], l.events[i] = size, e
		}
	}
	l.degraded = max(l.degraded, i)
}

// degradeTool returns e with every tool content text cut to
// degradedToolBytes and flagged truncated, and whether anything was cut. e
// itself is left alone.
func degradeTool(e Event) (Event, bool) {
	t := e.Tool
	if t == nil {
		return e, false
	}
	var content []Content
	for i, c := range t.Content {
		short, cut := degradeContent(c)
		if !cut {
			continue
		}
		if content == nil {
			content = slices.Clone(t.Content)
		}
		content[i] = short
	}
	if content == nil {
		return e, false
	}
	short := *t
	short.Content = content
	e.Tool = &short
	return e, true
}

func degradeContent(c Content) (Content, bool) {
	var cutText, cutNew, cutOld bool
	c.Text, cutText = cutBytes(c.Text, degradedToolBytes)
	c.NewText, cutNew = cutBytes(c.NewText, degradedToolBytes)
	if c.OldText != nil {
		var old string
		if old, cutOld = cutBytes(*c.OldText, degradedToolBytes); cutOld {
			c.OldText = &old
		}
	}
	cut := cutText || cutNew || cutOld
	c.Truncated = c.Truncated || cut
	return c, cut
}

// cutBytes returns s cut to at most n bytes at a UTF-8 boundary, and whether
// it was cut.
func cutBytes(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n], true
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
		s(t.QueueItemID)
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
