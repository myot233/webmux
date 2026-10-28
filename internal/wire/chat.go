package wire

import (
	"encoding/json"
	"fmt"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/workspace"
)

// ChatSnapshot is the first frame of a chat connection.
func ChatSnapshot(s hybrid.Snapshot) contract.ChatFrame {
	state := ChatState(s.State)
	return contract.ChatFrame{
		Type: contract.ChatFrameSnapshot, Generation: s.Generation, From: s.From, Next: s.Next,
		Reset: s.Reset, Truncated: s.Truncated, State: &state,
	}
}

// ChatEvents is an events frame; From is the first item's sequence.
func ChatEvents(generation string, events []hybrid.Event) contract.ChatFrame {
	f := contract.ChatFrame{Type: contract.ChatFrameEvents, Generation: generation, Items: each(events, ChatEvent)}
	if len(events) > 0 {
		f.From = events[0].Seq
	}
	return f
}

// ChatFrame converts one live frame: events, or the whole state.
func ChatFrame(f hybrid.Frame) contract.ChatFrame {
	if f.State != nil {
		state := ChatState(*f.State)
		return contract.ChatFrame{Type: contract.ChatFrameState, State: &state}
	}
	return ChatEvents(f.Generation, f.Events)
}

func ChatState(s hybrid.ChatState) contract.ChatState {
	out := contract.ChatState{
		Side: contract.HybridSide(s.Side), Phase: contract.HybridPhase(s.Phase), Dormant: s.Dormant,
		Agent: s.Agent, AgentVersion: s.AgentVersion, VerifiedBelow: s.VerifiedBelow,
		Eligibility: contract.ChatEligibility(s.Eligibility),
		Turn:        contract.ChatTurn(s.Turn),
		Requests:    each(s.Requests, chatRequest),
		Config:      each(s.Config, chatConfig),
		Commands:    each(s.Commands, chatCommand),
		Plan:        chatPlan(s.Plan),
		Usage:       chatUsage(s.Usage),
		Queue: contract.ChatQueue{Paused: s.Queue.Paused, Items: each(s.Queue.Items, func(it hybrid.QueueItem) contract.ChatQueueItem {
			return contract.ChatQueueItem(it)
		})},
		Steering: s.Steering, AuthRequired: s.AuthRequired, PermissionMode: contract.ChatPermissionAuto,
		SwitchError: s.SwitchError,
		Unsupported: required(s.Unsupported), QueueLost: s.QueueLost,
	}
	// The contract always names a mode; the zero value means auto.
	if s.PermissionMode == hybrid.PermissionAsk {
		out.PermissionMode = contract.ChatPermissionAsk
	}
	if s.AgentSessionID != nil {
		id := *s.AgentSessionID
		out.AgentSessionID = &id
	}
	if s.IDChange != nil {
		change := contract.ChatIDChange(*s.IDChange)
		out.IDChange = &change
	}
	if s.Recovery != nil {
		out.Recovery = &contract.ChatRecoveryTip{State: contract.HybridState(s.Recovery.State)}
	}
	return out
}

// ChatEvent converts one timeline event; only Kind's payload is set.
func ChatEvent(e hybrid.Event) contract.ChatEvent {
	out := contract.ChatEvent{Seq: e.Seq, Kind: e.Kind, At: e.At, Plan: chatPlan(e.Plan), Usage: chatUsage(e.Usage)}
	if e.Message != nil {
		m := contract.ChatMessageChunk(*e.Message)
		out.Message = &m
	}
	if e.Tool != nil {
		out.Tool = &contract.ChatTool{
			ToolCallID: e.Tool.ToolCallID, Kind: e.Tool.Kind, Title: e.Tool.Title, Status: e.Tool.Status,
			Content: patch(e.Tool.Content, chatContent), Locations: patch(e.Tool.Locations, chatLocation),
		}
	}
	if e.Kind == hybrid.EventCommands {
		out.Commands = each(e.Commands, chatCommand)
	}
	if e.Kind == hybrid.EventConfig {
		out.Config = each(e.Config, chatConfig)
	}
	if e.Info != nil {
		out.Info = &contract.ChatInfo{Title: e.Info.Title}
	}
	if e.Compaction != nil {
		c := contract.ChatCompaction(*e.Compaction)
		out.Compaction = &c
	}
	if e.Notice != nil {
		n := contract.ChatNotice(*e.Notice)
		out.Notice = &n
	}
	if e.TurnStart != nil {
		t := contract.ChatTurnStart(*e.TurnStart)
		out.TurnStart = &t
	}
	if e.TurnEnd != nil {
		t := contract.ChatTurnEnd(*e.TurnEnd)
		out.TurnEnd = &t
	}
	if e.Request != nil {
		r := chatRequest(*e.Request)
		out.Request = &r
	}
	if e.Resolved != nil {
		r := contract.ChatRequestOutcome(*e.Resolved)
		out.Resolved = &r
	}
	if e.Marker != nil {
		out.Marker = &contract.ChatMarker{Kind: e.Marker.Kind, Side: contract.HybridSide(e.Marker.Side)}
	}
	return out
}

// patch maps a tool field patch: nil (unchanged) stays nil.
func patch[S, D any](in []S, f func(S) D) []D {
	if in == nil {
		return nil
	}
	return each(in, f)
}

func chatContent(c hybrid.Content) contract.ChatContent { return contract.ChatContent(c) }

func chatLocation(l hybrid.Location) contract.ChatLocation { return contract.ChatLocation(l) }

func chatCommand(c hybrid.Command) contract.ChatCommand { return contract.ChatCommand(c) }

func chatConfig(c hybrid.ConfigOption) contract.ChatConfig {
	return contract.ChatConfig{
		ID: c.ID, Name: c.Name, Category: c.Category, Type: c.Type, Value: c.Value, Enabled: c.Enabled,
		Options: each(c.Options, func(o hybrid.ConfigChoice) contract.ChatConfigChoice { return contract.ChatConfigChoice(o) }),
	}
}

func chatPlan(p *hybrid.Plan) *contract.ChatPlan {
	if p == nil {
		return nil
	}
	return &contract.ChatPlan{Entries: each(p.Entries, func(e hybrid.PlanEntry) contract.ChatPlanEntry { return contract.ChatPlanEntry(e) })}
}

func chatUsage(u *hybrid.Usage) *contract.ChatUsage {
	if u == nil {
		return nil
	}
	out := contract.ChatUsage(*u)
	return &out
}

func chatRequest(r hybrid.Request) contract.ChatRequest {
	out := contract.ChatRequest{
		RequestID: r.RequestID, Kind: r.Kind, ToolCallID: r.ToolCallID, Title: r.Title,
		Options: patch(r.Options, func(o hybrid.PermissionOption) contract.ChatOption { return contract.ChatOption(o) }),
	}
	if r.Form != nil {
		out.Form = &contract.ChatForm{
			Message: r.Form.Message, Supported: r.Form.Supported,
			Fields: each(r.Form.Fields, func(f hybrid.FormField) contract.ChatFormField {
				return contract.ChatFormField{
					Name: f.Name, Title: f.Title, Description: f.Description, Type: f.Type, Required: f.Required,
					Enum: f.Enum, EnumNames: f.EnumNames, Default: json.RawMessage(f.Default),
				}
			}),
		}
	}
	return out
}

func ChatPromptResult(r hybrid.PromptResult) contract.ChatPromptResult {
	return contract.ChatPromptResult{Queued: r.Queued, ItemID: r.ItemID}
}

func ChatPromptInput(r contract.ChatPromptRequest) hybrid.PromptInput {
	return hybrid.PromptInput(r)
}

func ChatQueueEdit(r contract.ChatQueueRequest) hybrid.QueueEdit {
	return hybrid.QueueEdit(r)
}

func ChatConfigInput(configID string, r contract.ChatConfigRequest) hybrid.ConfigInput {
	return hybrid.ConfigInput{ClientRequestID: r.ClientRequestID, ConfigID: configID, Value: r.Value, Enabled: r.Enabled}
}

func ChatPermissionModeInput(r contract.ChatPermissionModeRequest) hybrid.PermissionModeInput {
	return hybrid.PermissionModeInput{ClientRequestID: r.ClientRequestID, Mode: hybrid.PermissionMode(r.Mode)}
}

func HybridSwitchInput(r contract.HybridSwitchRequest) hybrid.SwitchInput {
	return hybrid.SwitchInput{ClientRequestID: r.ClientRequestID, To: hybrid.Side(r.To), Choice: r.Choice}
}

func HybridSwitchResult(r hybrid.SwitchResult) contract.HybridSwitchResult {
	return contract.HybridSwitchResult{Side: contract.HybridSide(r.Side), Draft: r.Draft, Dropped: r.Dropped, Unsupported: r.Unsupported}
}

func HybridResumeInput(r contract.HybridResumeRequest) workspace.HybridResumeInput {
	return workspace.HybridResumeInput{
		ClientRequestID: r.ClientRequestID, Side: string(r.Side), AgentSessionID: r.AgentSessionID, ConfirmWithoutEnv: r.ConfirmWithoutEnv,
	}
}

// HybridRecord converts one record; configMissing tells whether its launch
// config was deleted. The activity before an interruption, option values
// and process identity stay on the server.
func HybridRecord(r hybrid.Record, configMissing bool) contract.HybridRecord {
	out := contract.HybridRecord{
		ID: r.ID, Name: r.Name, ConfigID: r.ConfigID, ConfigMissing: configMissing, Agent: r.Agent, Cwd: r.Cwd,
		Cmd: required(r.Cmd), Side: contract.HybridSide(r.Side), PreviousIDs: required(r.PreviousIDs),
		State: contract.HybridState(r.State), Activity: r.Activity, QueuedCount: r.QueuedCount, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.AgentSessionID != "" {
		id := r.AgentSessionID
		out.AgentSessionID = &id
	}
	if r.InterruptedAt != nil {
		at := *r.InterruptedAt
		out.InterruptedAt = &at
	}
	return out
}

// HybridRecords converts a record list; configMissing reports whether the
// launch config with an ID was deleted.
func HybridRecords(list []hybrid.Record, configMissing func(configID string) bool) []contract.HybridRecord {
	return each(list, func(r hybrid.Record) contract.HybridRecord { return HybridRecord(r, configMissing(r.ConfigID)) })
}

func HybridCandidates(list []hybrid.Candidate) []contract.HybridCandidate {
	return each(list, func(c hybrid.Candidate) contract.HybridCandidate { return contract.HybridCandidate(c) })
}

// ChatAnswer converts a request answer; hybrid.FormValue checks each form
// value.
func ChatAnswer(r contract.ChatAnswerRequest) (hybrid.Answer, error) {
	out := hybrid.Answer{Outcome: r.Outcome, OptionID: r.OptionID}
	if r.Content == nil {
		return out, nil
	}
	out.Content = make(map[string]any, len(r.Content))
	for name, raw := range r.Content {
		v, err := hybrid.FormValue(raw)
		if err != nil {
			return hybrid.Answer{}, fmt.Errorf("%w: %s", err, name)
		}
		out.Content[name] = v
	}
	return out, nil
}
