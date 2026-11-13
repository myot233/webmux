package sessionobserve

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/contract"
)

// TurnSelector selects exactly one kind of identity. PromptID is the direct
// input receipt; QueueItemID follows the turn that eventually sends that item.
type TurnSelector struct {
	TurnID      string
	PromptID    string
	QueueItemID string
	FirstTurn   bool
}

type AttachPhase uint8

const (
	AfterSubmit AttachPhase = iota
	BeforeSubmit
)

// TurnPolicy contains only differences between observation callers. ReplyLimit
// is in bytes; zero keeps the full reply. Events are allocated only on request.
// InitialOnly returns after applying the initial replay, without waiting live.
type TurnPolicy struct {
	AttachPhase   AttachPhase
	StopAtRequest bool
	ReplyLimit    int
	CollectEvents bool
	InitialOnly   bool
}

type TurnObservation struct {
	TurnID            string
	QueueItemID       string
	Phase             string
	StopReason        string
	Error             string
	Reply             string
	ReplyTruncated    bool
	HistoryIncomplete bool
	Unfinished        bool
	Requests          []contract.ChatRequest
	Events            []contract.ChatEvent
}

func (s TurnSelector) validate() error {
	count := 0
	for _, value := range []string{s.TurnID, s.PromptID, s.QueueItemID} {
		if value != "" {
			count++
		}
	}
	if s.FirstTurn {
		count++
	}
	if count != 1 {
		return errors.New("select exactly one turnId, promptId, queueItemId or firstTurn")
	}
	return nil
}

func (s TurnSelector) matches(start contract.ChatTurnStart) bool {
	return s.TurnID != "" && s.TurnID == start.TurnID ||
		s.PromptID != "" && s.PromptID == start.PromptID ||
		s.QueueItemID != "" && s.QueueItemID == start.QueueItemID || s.FirstTurn
}

// WaitTurn consumes the preserved replay before following live frames. An
// error always accompanies the latest partial observation, never an invented
// completion. It does not interrupt or otherwise write to the session.
func (s *ChatSubscription) WaitTurn(ctx context.Context, selector TurnSelector, policy TurnPolicy) (TurnObservation, error) {
	t := turnTracker{
		selector: selector, policy: policy,
		result:   TurnObservation{TurnID: selector.TurnID, QueueItemID: selector.QueueItemID, Phase: "pending", Requests: []contract.ChatRequest{}},
		messages: make(map[string]struct{}), open: make(map[string]contract.ChatRequest),
	}
	if err := selector.validate(); err != nil {
		return t.result, err
	}
	if policy.ReplyLimit < 0 {
		return t.result, errors.New("reply limit must not be negative")
	}
	if selector.FirstTurn {
		frame, err := s.initialSnapshot(ctx)
		if err != nil {
			return t.result, err
		}
		if frame.Truncated || frame.From > 1 {
			return t.result, fmt.Errorf("%w: first turn cannot be identified; do not substitute a later turn", ErrHistoryUnavailable)
		}
	}
	primeErr := s.Prime(ctx)
	initial := s.initial
	s.initial = nil
	for i, frame := range initial {
		done, err := t.apply(frame, s.primed && i == len(initial)-1)
		if err != nil || done {
			return t.observation(), err
		}
	}
	if primeErr != nil {
		if t.tracked == "" && t.state != nil && t.selector.TurnID != "" && t.state.Turn.TurnID == t.selector.TurnID {
			t.incomplete()
		}
		return t.observation(), primeErr
	}
	if policy.InitialOnly {
		return t.observation(), nil
	}
	for {
		frame, err := s.Read(ctx)
		if err != nil {
			return t.observation(), err
		}
		done, err := t.apply(frame, false)
		if err != nil || done {
			return t.observation(), err
		}
	}
}

type replyMessage struct {
	text    strings.Builder
	clipped bool
}

type turnTracker struct {
	selector      TurnSelector
	policy        TurnPolicy
	result        TurnObservation
	state         *contract.ChatState
	tracked       string
	collecting    bool
	replayed      bool
	queueSeen     bool
	messages      map[string]struct{}
	lastMessageID string
	lastMessage   replyMessage
	open          map[string]contract.ChatRequest
	opened        []string
}

func (t *turnTracker) observation() TurnObservation {
	t.result.Reply = t.lastMessage.text.String()
	t.result.Requests = t.result.Requests[:0]
	for _, id := range t.opened {
		if request, ok := t.open[id]; ok {
			t.result.Requests = append(t.result.Requests, request)
		}
	}
	return t.result
}

func (t *turnTracker) incomplete() {
	t.result.HistoryIncomplete = true
	t.result.ReplyTruncated = true
}

func (t *turnTracker) requests(requests []contract.ChatRequest) {
	clear(t.open)
	t.opened = t.opened[:0]
	for _, request := range requests {
		t.openRequest(request)
	}
}

func (t *turnTracker) openRequest(request contract.ChatRequest) {
	if !slices.Contains(t.opened, request.RequestID) {
		t.opened = append(t.opened, request.RequestID)
	}
	t.open[request.RequestID] = request
}

func (t *turnTracker) match(id string, started bool) {
	t.tracked = id
	t.result.TurnID = id
	t.result.Phase = "running"
	t.collecting = true
	if !started {
		t.incomplete()
	}
	if t.state != nil && t.state.Turn.TurnID == id {
		// Snapshot requests are current at the replay boundary. Replayed
		// request.close events below override them before deciding to stop.
		t.requests(t.state.Requests)
	}
}

func (t *turnTracker) apply(frame contract.ChatFrame, replayEnd bool) (bool, error) {
	if frame.Type == contract.ChatFrameSnapshot && t.selector.FirstTurn && (frame.Truncated || frame.From > 1) {
		return false, fmt.Errorf("%w: first turn cannot be identified; do not substitute a later turn", ErrHistoryUnavailable)
	}
	if frame.State != nil {
		t.state = frame.State
		if t.tracked != "" && frame.State.Turn.TurnID == t.tracked {
			t.requests(frame.State.Requests)
		} else if t.tracked == "" && t.selector.TurnID != "" && frame.State.Turn.TurnID == t.selector.TurnID {
			// Show matching current facts even if the initial replay times out,
			// but do not attribute its unidentified prefix messages to a turn.
			if frame.State.Turn.Running {
				t.result.Phase = "running"
			}
			t.requests(frame.State.Requests)
		}
	}
	for _, event := range frame.Items {
		if event.Kind == contract.ChatEventTurnStart && event.TurnStart != nil {
			start := event.TurnStart
			if t.tracked == "" && t.selector.matches(*start) {
				t.match(start.TurnID, true)
				if t.result.QueueItemID == "" {
					t.result.QueueItemID = start.QueueItemID
				}
			}
			t.collecting = t.tracked != "" && start.TurnID == t.tracked
		}
		matchedEnd := event.Kind == contract.ChatEventTurnEnd && event.TurnEnd != nil &&
			(t.tracked != "" && event.TurnEnd.TurnID == t.tracked || t.selector.TurnID != "" && event.TurnEnd.TurnID == t.selector.TurnID)
		if matchedEnd && t.tracked == "" {
			t.match(event.TurnEnd.TurnID, false)
		}
		if t.policy.CollectEvents && (t.collecting || matchedEnd) {
			t.result.Events = append(t.result.Events, event)
		}
		if matchedEnd {
			t.result.Phase = "ended"
			t.result.StopReason = event.TurnEnd.StopReason
			t.result.Error = event.TurnEnd.Error
			t.result.Unfinished = event.TurnEnd.Unfinished
			clear(t.open)
			return true, nil
		}
		// A retained close can also resolve a request in the matching
		// snapshot when that turn's start has already been evicted.
		if event.Kind == contract.ChatEventResolved && event.Resolved != nil {
			if t.collecting || t.tracked == "" && t.state != nil && t.state.Turn.TurnID == t.selector.TurnID {
				delete(t.open, event.Resolved.RequestID)
			}
		}
		if !t.collecting {
			continue
		}
		switch event.Kind {
		case contract.ChatEventMessage:
			if event.Message != nil && event.Message.Role == "agent" {
				t.appendReply(*event.Message)
			}
		case contract.ChatEventRequest:
			if event.Request != nil {
				t.openRequest(*event.Request)
			}
		case contract.ChatEventTurnEnd:
			// A different turn's end cannot finish the selected turn.
			t.collecting = false
		}
	}
	if replayEnd {
		t.replayed = true
		if t.tracked == "" && t.selector.TurnID != "" && t.state != nil && t.state.Turn.TurnID == t.selector.TurnID {
			// Keep any request.close already applied to snapshot requests.
			t.tracked = t.selector.TurnID
			t.result.Phase = "running"
			t.collecting = true
			t.incomplete()
		}
	}
	// Apply turn events before queue absence, including frames carrying both
	// state and events: consuming a queue item normally removes it.
	if t.tracked == "" && t.selector.QueueItemID != "" && t.state != nil {
		present := false
		for _, item := range t.state.Queue.Items {
			if item.ID == t.selector.QueueItemID {
				present = true
				break
			}
		}
		if present {
			t.queueSeen = true
			t.result.Phase = "queued"
		} else if t.queueSeen && frame.State != nil {
			return false, fmt.Errorf("%w: %s", ErrQueueRemoved, t.selector.QueueItemID)
		}
	}
	if replayEnd && t.tracked == "" && t.policy.AttachPhase == AfterSubmit && !t.queueSeen {
		return false, fmt.Errorf("%w: no matching retained turn or current state", ErrHistoryUnavailable)
	}
	if t.replayed && t.tracked != "" && t.state != nil && t.state.Turn.TurnID == t.tracked {
		if !t.state.Turn.Running && t.state.Turn.StopReason != "" {
			t.result.Phase = "ended"
			t.result.StopReason = t.state.Turn.StopReason
			t.result.Unfinished = t.state.Turn.Unfinished
			t.incomplete()
			clear(t.open)
			return true, nil
		}
		t.result.Unfinished = t.state.Turn.Unfinished
	}
	if t.replayed && t.tracked != "" {
		t.result.Phase = "running"
		if len(t.open) != 0 {
			t.result.Phase = "waiting_for_user"
			if t.policy.StopAtRequest {
				return true, nil
			}
		}
	}
	return false, nil
}

func (t *turnTracker) appendReply(chunk contract.ChatMessageChunk) {
	if _, ok := t.messages[chunk.MessageID]; !ok {
		t.messages[chunk.MessageID] = struct{}{}
		t.lastMessageID = chunk.MessageID
		// Only the last agent message can be returned. Keep earlier IDs to
		// recognize interleaved chunks, without retaining discarded bodies.
		t.lastMessage = replyMessage{}
	}
	if chunk.Truncated {
		t.incomplete()
	}
	message := &t.lastMessage
	if chunk.MessageID != t.lastMessageID || message.clipped {
		return
	}
	text := chunk.Text
	if t.policy.ReplyLimit > 0 {
		remaining := t.policy.ReplyLimit - message.text.Len()
		if len(text) > remaining {
			text = text[:remaining]
			for !utf8.ValidString(text) {
				text = text[:len(text)-1]
			}
			message.clipped = true
			t.result.ReplyTruncated = true
		}
	}
	message.text.WriteString(text)
}
