package hybrid

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
)

func TestNextPromptUsesOnlyLatestTurn(t *testing.T) {
	now := time.Now()
	l := newChatLog(64<<10, ChatState{
		Side: SideACP,
		Turn: Turn{TurnID: "t2", EndedAt: &now, StopReason: StopError},
		Plan: &Plan{Entries: []PlanEntry{
			{Content: "old step", Status: "completed"},
			{Content: "verify fixture", Status: "pending"},
		}},
	})
	l.append(now,
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleUser, Text: "older prompt"}},
		Event{Kind: EventTurnStart, TurnStart: &TurnStart{TurnID: "t2"}},
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleUser, Text: "check fixture"}},
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleThought, Text: "private reasoning"}},
		Event{Kind: EventTool, Tool: &Tool{ToolCallID: "tool", Content: []Content{{Text: "raw tool output"}}}},
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleAgent, Text: "the fixture failed"}},
		Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: "t2", StopReason: StopError, Error: "exit 1"}},
	)
	if job, _ := l.prepareNextPrompt("t1", now); job != nil {
		t.Fatal("previous turn accepted")
	}
	job, input := l.prepareNextPrompt("t2", now)
	want := &nextPromptContext{
		StopReason: StopError, User: "check fixture", Reply: "the fixture failed", Step: "verify fixture", Error: "exit 1",
	}
	if job == nil || !reflect.DeepEqual(input, want) {
		t.Fatalf("context = %+v, want %+v", input, want)
	}
	if duplicate, input := l.prepareNextPrompt("t2", now); duplicate != job || input != nil {
		t.Fatal("a second client started another generation")
	}
}

func TestNextPromptSkipsQuietTurns(t *testing.T) {
	now := time.Now()
	expired := now.Add(-3 * time.Minute)
	tests := []struct {
		name string
		turn Turn
	}{
		{name: "complete", turn: Turn{TurnID: "t1", EndedAt: &now, StopReason: acp.StopEndTurn}},
		{name: "cancelled", turn: Turn{TurnID: "t1", EndedAt: &now, StopReason: acp.StopCancelled}},
		{name: "expired", turn: Turn{TurnID: "t1", EndedAt: &expired, StopReason: StopError}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newChatLog(64<<10, ChatState{Side: SideACP, Turn: tt.turn})
			l.append(now, Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: tt.turn.TurnID}})
			if job, _ := l.prepareNextPrompt(tt.turn.TurnID, *tt.turn.EndedAt); job != nil {
				t.Fatal("ineligible turn started generation")
			}
		})
	}
}

func TestRunNextPrompt(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "fixed instruction and context",
			give: `input=$(cat); case "$input" in
			  *"Suggest at most one short next message"*'user: "test fixture"'*)
			    printf '%s\n' "Check the missing fixture." ;;
			esac`,
			want: "Check the missing fixture.",
		},
		{name: "no suggestion", give: "exit 0"},
		{name: "error", give: "echo ignored; exit 1"},
		{name: "multiline", give: "printf 'first\\nsecond\\n'"},
		{name: "slash command", give: "echo /clear"},
		{name: "too many characters", give: "echo " + strings.Repeat("你", nextPromptMaxRunes+1)},
		{name: "output budget", give: "echo " + strings.Repeat("a", nextPromptOutputBytes+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got := runNextPrompt(ctx, []string{"/bin/sh", "-c", tt.give}, nextPromptContext{User: "test fixture"})
			if got != tt.want {
				t.Fatalf("suggestion = %q, want %q", got, tt.want)
			}
		})
	}
}
