package hybrid

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
	"strconv"
	"strings"
)

// activityRecorder is actor-owned and receives ACP values before translator
// truncation. It never reads chatLog or uses chat generation/sequence as IDs.
type activityRecorder struct {
	thread, run, completedRun string
	replay                    bool
	replayCapture             bool
	replayReceipts            map[string]bool
	failed                    bool
	next                      int
	tools                     map[string]recordedTool
	compactions               map[string]recordedCompaction
	message                   struct {
		role, provider, key string
		open                bool
	}
}

type recordedCompaction struct{ run, key, status string }

type recordedTool struct {
	run, key, label, kind, status, input string
	exitCode                             *int
}

func (a *actor) activityFailure() {
	a.activity.failed = true
	a.svc.log.Warn("conversation activity write failed", "id", a.id)
	if a.svc.conversations != nil && a.activity.thread != "" {
		// A failed database cannot necessarily record its own gap. The metadata
		// warning above remains observable and crash recovery retains draft tails.
		if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "storage_failure"); err != nil {
			a.svc.log.Warn("conversation gap could not be persisted", "id", a.id)
		}
	}
}

func (a *actor) activityThread() error {
	if a.svc.conversations == nil {
		return nil
	}
	b, err := a.svc.conversations.Binding(context.Background(), a.id)
	if err != nil {
		return fmt.Errorf("%w: durable history unavailable", ErrUnavailable)
	}
	a.activity.thread = b.ProviderThreadID
	a.conversationID = b.ConversationID
	return nil
}

func (a *actor) beginActivityRun(text string, attachments []Attachment) error {
	return a.beginActivityRunID(text, attachments, rand.Text())
}

func (a *actor) beginActivityRunID(text string, attachments []Attachment, promptID string) error {
	if !a.svc.conversations.HistoryEnabled() {
		return nil
	}
	if err := a.activityThread(); err != nil {
		return err
	}
	r, err := a.svc.conversations.StartRun(context.Background(), a.activity.thread, promptID)
	if err != nil {
		return fmt.Errorf("%w: durable run unavailable", ErrUnavailable)
	}
	a.activity.run = r.ID
	a.activity.failed = false
	a.activity.replay = false
	a.activity.message.open = false
	if err := a.writeActivity(conversation.ActivityInput{LocalKey: r.ID + ":start", Kind: "turn_start", Status: "started"}); err != nil {
		return err
	}
	metadata := activityAttachments(attachments)
	return a.writeActivity(conversation.ActivityInput{LocalKey: r.ID + ":user", Kind: "message", Role: "user", Status: "completed", Text: text, Attachments: metadata})
}

func (a *actor) writeActivity(in conversation.ActivityInput) error {
	return a.writeActivityRun(in, false)
}

func (a *actor) writeActivityRun(in conversation.ActivityInput, keepRun bool) error {
	if !a.svc.conversations.HistoryEnabled() || (a.activity.replay && !a.activity.replayCapture) {
		return nil
	}
	if err := a.activityThread(); err != nil {
		return err
	}
	in.ProviderThreadID = a.activity.thread
	if in.RunID == "" && !keepRun {
		in.RunID = a.activity.run
	}
	if in.Source == "" {
		in.Source = "local"
		if a.activity.replay {
			in.Source = "replay"
		}
	}
	if _, err := a.svc.conversations.WriteActivity(context.Background(), in); err != nil {
		return fmt.Errorf("%w: durable activity unavailable", ErrUnavailable)
	}
	return nil
}

func (a *actor) activityKey(prefix string) string {
	a.activity.next++
	run := a.activity.run
	if run == "" {
		run = rand.Text()
	}
	return run + ":" + prefix + ":" + strconv.Itoa(a.activity.next)
}

func (a *actor) recordUpdate(u acp.Update) {
	if !a.svc.conversations.HistoryEnabled() || (a.activity.replay && !a.activity.replayCapture) {
		return
	}
	if u, ok := u.(*acp.UsageUpdate); ok {
		if err := a.activityThread(); err != nil {
			a.activityFailure()
			return
		}
		if err := a.svc.conversations.ObserveUsage(context.Background(), a.activity.thread, a.agentSessionID, u.Used, u.Size); err != nil {
			a.svc.log.Warn("conversation context usage unavailable", "id", a.id)
		}
		return
	}
	source := "live"
	if a.activity.replay {
		source = "replay"
	}
	var chunk *acp.ContentChunk
	role := ""
	switch u := u.(type) {
	case *acp.AgentMessageChunk:
		a.acknowledgeCurrentRun()
		if a.turn != nil && a.turn.executionID != "" {
			a.finishContextExecution(a.turn, "inline", "")
		}
		c := acp.ContentChunk(*u)
		chunk = &c
		role = "agent"
	case *acp.UserMessageChunk:
		if !a.activity.replay {
			return
		}
		c := acp.ContentChunk(*u)
		chunk = &c
		role = "user"
	case *acp.AgentThoughtChunk:
		return
	}
	if chunk != nil {
		m := &a.activity.message
		if !m.open || m.role != role || m.provider != chunk.MessageID {
			m.open = true
			m.role = role
			m.provider = chunk.MessageID
			m.key = a.activityKey("message")
			if chunk.MessageID != "" {
				m.key = a.activity.run + ":message:" + role + ":" + chunk.MessageID
			}
		}
		in := conversation.ActivityInput{LocalKey: m.key, ProviderItemID: chunk.MessageID, Kind: "message", Role: role, Status: "in_progress", Source: source}
		if chunk.Content.Type == acp.ContentText {
			in.Text = chunk.Content.Text
		} else {
			reference := chunk.Content.URI
			if reference == "" {
				reference = "provider-item:" + chunk.MessageID
			}
			name := chunk.Content.Name
			if name == "" {
				name = chunk.Content.Type
			}
			size := int64(-1)
			if chunk.Content.Size != nil {
				size = *chunk.Content.Size
			}
			in.Attachments = []conversation.ActivityAttachment{{Name: name, MimeType: chunk.Content.MimeType, Reference: reference, Size: size}}
			if a.activity.replay {
				if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "replay_ambiguous"); err != nil {
					a.activityFailure()
				}
			}
		}
		err := a.writeActivity(in)
		if err != nil {
			a.activityFailure()
		}
		return
	}
	a.activity.message.open = false
	var in conversation.ActivityInput
	keepRun := false
	switch u := u.(type) {

	case *acp.ToolCall:
		keepRun = true
		if a.activity.tools == nil {
			a.activity.tools = map[string]recordedTool{}
		}
		kind := "tool"
		if u.Kind == "execute" {
			kind = "command"
		}
		if u.Kind == "edit" {
			kind = "file_change"
		}
		t := recordedTool{run: a.activity.run, key: a.activity.run + ":tool:" + u.ToolCallID, label: u.Title, kind: kind, status: u.Status, input: normalizedCommand(u.RawInput), exitCode: normalizedExit(u.RawOutput)}
		if t.status == "" {
			t.status = "pending"
		}
		a.activity.tools[u.ToolCallID] = t
		heading := recordedToolHeading(t)
		in = conversation.ActivityInput{Heading: &heading, RunID: t.run, LocalKey: t.key, ProviderItemID: u.ToolCallID, Kind: t.kind, Role: "agent", Status: t.status, Text: activityToolText("", u.Content), Replace: true}
	case *acp.ToolCallUpdate:
		keepRun = true
		if a.activity.tools == nil {
			a.activity.tools = map[string]recordedTool{}
		}
		t, ok := a.activity.tools[u.ToolCallID]
		if !ok {
			t = recordedTool{run: a.activity.run, key: a.activity.run + ":tool:" + u.ToolCallID, kind: "tool", status: "pending"}
		}
		if u.Title != nil {
			t.label = *u.Title
		}
		if u.Kind != nil && *u.Kind == "execute" {
			t.kind = "command"
		}
		status := t.status
		if u.Status != nil && *u.Status != "" {
			status = *u.Status
			t.status = status
		}
		if u.Kind != nil && *u.Kind == "edit" {
			t.kind = "file_change"
		}
		if u.RawInput != nil {
			t.input = normalizedCommand(u.RawInput)
		}
		if u.RawOutput != nil {
			t.exitCode = normalizedExit(u.RawOutput)
		}
		a.activity.tools[u.ToolCallID] = t
		text := ""
		if u.Content != nil {
			text = activityToolText("", u.Content)
		}
		var heading *string
		if !ok || u.Title != nil || u.RawInput != nil || u.RawOutput != nil {
			value := recordedToolHeading(t)
			heading = &value
		}
		in = conversation.ActivityInput{Heading: heading, RunID: t.run, LocalKey: t.key, ProviderItemID: u.ToolCallID, Kind: t.kind, Role: "agent", Status: status, Text: text, Replace: u.Content != nil}
	case *acp.Plan:
		var body strings.Builder
		for _, entry := range u.Entries {
			fmt.Fprintf(&body, "[%s] %s\n", entry.Status, entry.Content)
		}
		in = conversation.ActivityInput{LocalKey: a.activity.run + ":plan", Kind: "plan", Role: "agent", Status: "completed", Text: body.String(), Replace: true}
	case *acp.CompactionUpdate:
		keepRun = true
		c := a.compaction(u.CompactionID)
		if u.Status != "" {
			c.status = u.Status
		}
		a.activity.compactions[u.CompactionID] = c
		in = conversation.ActivityInput{RunID: c.run, LocalKey: c.key, ProviderItemID: u.CompactionID, Kind: "compaction", Role: "agent", Status: c.status, Text: activityBlocks(u.Summary), Replace: u.Summary != nil}
	case *acp.CompactionSummaryChunk:
		keepRun = true
		c := a.compaction(u.CompactionID)
		in = conversation.ActivityInput{RunID: c.run, LocalKey: c.key, ProviderItemID: u.CompactionID, Kind: "compaction", Role: "agent", Status: c.status, Text: activityBlocks([]acp.ContentBlock{u.Content})}
	case *acp.Notice:
		if u.Severity != acp.NoticeError {
			return
		}
		in = conversation.ActivityInput{LocalKey: a.activityKey("error"), Kind: "error", Role: "system", Status: "error", Text: u.Title + "\n" + u.Description}
	default:
		return
	}
	in.Source = source
	if err := a.writeActivityRun(in, keepRun); err != nil {
		a.activityFailure()
	} else if (in.RunID == "" || in.RunID != a.activity.run) && (in.Status == "completed" || in.Status == "failed") {
		delete(a.activity.tools, in.ProviderItemID)
		delete(a.activity.compactions, in.ProviderItemID)
	}
}

func (a *actor) compaction(id string) recordedCompaction {
	if a.activity.compactions == nil {
		a.activity.compactions = map[string]recordedCompaction{}
	}
	c, ok := a.activity.compactions[id]
	if !ok {
		c = recordedCompaction{run: a.activity.run, key: a.activity.run + ":compaction:" + id, status: "in_progress"}
		a.activity.compactions[id] = c
	}
	return c
}

func (a *actor) beginActivityReplay() {
	if a.svc.conversations == nil {
		return
	}
	if err := a.activityThread(); err != nil {
		a.activityFailure()
		return
	}
	a.activity.replayReceipts = map[string]bool{}
	allowed, err := a.svc.conversations.ReplayCaptureAllowed(context.Background(), a.activity.thread)
	a.activity.replayCapture = allowed && err == nil
	if err != nil {
		a.activityFailure()
	}
	if !a.activity.replayCapture {
		a.activity.replay = true
		a.startNativeReplay()
		return
	}
	r, err := a.svc.conversations.StartRun(context.Background(), a.activity.thread, rand.Text())
	if err != nil {
		a.activityFailure()
		return
	}
	a.activity.run = r.ID
	a.activity.failed = false
	a.activity.tools = map[string]recordedTool{}
	a.activity.compactions = map[string]recordedCompaction{}
	a.activity.replay = true
	a.activity.message.open = false
	a.startNativeReplay()
}

func (a *actor) finishActivityReplay() {
	a.flushReplayUser()
	if a.svc.conversations == nil || !a.activity.replay {
		return
	}
	if a.replayNative.current.DeliveryID != "" {
		a.nativeReplayGap()
	}
	if a.activity.failed {
		if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "storage_failure"); err != nil {
			a.activityFailure()
			return
		}
	}
	if !a.activity.replayCapture {
		a.activity.run = ""
		a.activity.replay = false
		a.activity.message.open = false
		return
	}
	if err := a.svc.conversations.ReconcileReplay(context.Background(), a.activity.thread, a.activity.run); err != nil {
		a.activityFailure()
	}
	a.activity.run = ""
	a.activity.replay = false
	a.activity.message.open = false
}

func (a *actor) finishActivityRun(stop string) {
	a.activity.completedRun = a.activity.run
	if a.svc.conversations == nil || a.activity.run == "" || a.activity.replay {
		return
	}
	if a.activity.failed {
		if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "storage_failure"); err != nil {
			a.activityFailure()
			return
		}
	}
	kind := "turn_end"
	if stop == acp.StopCancelled {
		kind = "interrupt"
	}
	if err := a.writeActivity(conversation.ActivityInput{LocalKey: a.activity.run + ":end", Kind: kind, Status: stop}); err != nil {
		a.activityFailure()
	}
	if err := a.svc.conversations.FinishRun(context.Background(), a.activity.run, stop); err != nil {
		a.activityFailure()
	}
	for id, t := range a.activity.tools {
		if t.run == a.activity.run && t.status != "in_progress" && t.status != "pending" {
			delete(a.activity.tools, id)
		}
	}
	for id, c := range a.activity.compactions {
		if c.status == "completed" || c.status == "failed" {
			delete(a.activity.compactions, id)
		}
	}
	a.activity.run = ""
	a.activity.message.open = false
}

func (a *actor) recordTUIGap() {
	if a.svc.conversations == nil {
		return
	}
	if err := a.activityThread(); err != nil {
		a.activityFailure()
		return
	}
	if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "tui"); err != nil {
		a.activityFailure()
	}
}

// Only explicitly normalized command/exit fields are portable. Environment,
// provider private state and every other raw field are discarded.
func normalizedCommand(raw json.RawMessage) string {
	var in struct {
		Command json.RawMessage `json:"command"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	var text string
	if json.Unmarshal(in.Command, &text) == nil {
		return text
	}
	var argv []string
	if json.Unmarshal(in.Command, &argv) != nil {
		return ""
	}
	for i := range argv {
		argv[i] = strconv.Quote(argv[i])
	}
	return strings.Join(argv, " ")
}

func normalizedExit(raw json.RawMessage) *int {
	var in struct {
		ExitCode *int `json:"exitCode"`
		Snake    *int `json:"exit_code"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return nil
	}
	if in.ExitCode != nil {
		return in.ExitCode
	}
	return in.Snake
}
