package handoff

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/conversation"
)

func candidate(id, role, text string) conversation.Candidate {
	m := conversation.HistoricalMessage{ActivityID: id, ConversationID: "conversation", RunID: "run", ProviderThreadID: "provider", Kind: "message", Role: role, Status: "completed", Exportable: true, Text: text}
	return conversation.Candidate{Message: m, ByteLength: int64(len(text)), JSONBytes: jsonBytes(text) - 2}
}

func TestBudgetAccountsForRequestAttachmentsUsageAndReserve(t *testing.T) {
	tests := []struct {
		name                      string
		give                      BudgetInput
		history, reserve, current int64
		source                    string
	}{
		{"unknown", BudgetInput{UserText: "hello"}, 16000, 32000, 7, "estimated"},
		{"known usage", BudgetInput{TokenCap: 64000, UserText: "hello", Used: 80000, Window: 128000, Measured: true}, 15993, 32000, 7, "provider"},
		{"attachments", BudgetInput{UserText: "", Attachments: []Attachment{{Kind: "image"}, {Kind: "file"}}}, 16000, 32000, 12290, "estimated"},
		{"small window", BudgetInput{UserText: "hello", Window: 8000}, 0, 16000, 7, "estimated"},
		{"threshold", BudgetInput{UserText: "hello", Window: 128000, CompactionThreshold: 20000}, 3993, 16000, 7, "estimated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := ComputeBudget(tt.give)
			if b.History != tt.history || b.Reserve != tt.reserve || b.Current != tt.current || b.Source != tt.source {
				t.Fatalf("budget=%+v", b)
			}
		})
	}
}

func TestInlineBudgetPreservesRequestAtExactPromptBoundary(t *testing.T) {
	const limit = 64 << 10
	separator := len("\n\nUser message:\n")
	for _, n := range []int{limit - separator - 1, limit - separator, limit - separator + 1, limit} {
		request := strings.Repeat("x", n)
		b := ComputeBudget(BudgetInput{UserText: request, Inline: true, MaxPromptBytes: limit, MaxPromptWireBytes: 14 << 20})
		if b.OverBudget != (n+separator > limit) {
			t.Fatalf("boundary %d: %+v", n, b)
		}
		if int64(len(request))+int64(separator)+b.History > limit && !b.OverBudget {
			t.Fatalf("inline overflow: %+v", b)
		}
	}
	b := ComputeBudget(BudgetInput{Inline: true, MaxPromptBytes: limit, MaxPromptWireBytes: 100, Attachments: []Attachment{{Kind: "image", Size: 100}}})
	if !b.OverBudget {
		t.Fatal("wire overflow accepted")
	}
}

func TestSelectorPriorityOrderWholeItemsAndMonotoneCost(t *testing.T) {
	all := []conversation.Candidate{candidate("first", "user", "original"), candidate("old", "agent", "old result"), candidate("latest-user", "user", "current work"), candidate("latest-agent", "agent", "result")}
	coverage := Coverage{HandoffID: "handoff", ConversationID: "source", FromRun: 1, ToRun: 4, SourcePosition: 4, HistoryOrigin: "complete"}
	full := Select(all, coverage, 10000)
	if len(full.Selected) != 4 || len(full.Omitted) != 0 {
		t.Fatalf("short history lost: %+v", full)
	}
	reserve := Select(all, coverage, 0).Cost
	priority := Select(all, coverage, reserve+candidateCost(all[2]))
	if len(priority.Selected) != 1 || priority.Selected[0].Message.ActivityID != "latest-user" {
		t.Fatalf("latest user was not first priority: %+v", priority)
	}
	var previous int64
	for budget := int64(0); budget < 5000; budget++ {
		selected := Select(all, coverage, budget)
		if !selected.OverBudget && selected.Cost > budget {
			t.Fatalf("cost exceeds budget: %d %d", selected.Cost, budget)
		}
		if selected.Cost < previous {
			t.Fatalf("selected cost is not monotone at %d", budget)
		}
		previous = selected.Cost
		for _, m := range selected.Selected {
			if m.Message.Text != all[indexOf(all, m.Message.ActivityID)].Message.Text {
				t.Fatal("item truncated")
			}
		}

	}
}

func indexOf(all []conversation.Candidate, id string) int {
	for i, c := range all {
		if c.Message.ActivityID == id {
			return i
		}
	}
	return -1
}

func TestSelectorOversizedItemsEscapingAndDuplicateIDs(t *testing.T) {
	coverage := Coverage{HandoffID: "handoff", ConversationID: "source"}
	oversized := candidate("huge", "user", strings.Repeat("中文🙂", 10000))
	small := candidate("small", "agent", "\"<>&\n\u2028🙂")
	selected := Select([]conversation.Candidate{oversized, small, small}, coverage, 3000)
	if len(selected.Selected) != 1 || selected.Selected[0].Message.ActivityID != "small" || len(selected.Omitted) != 1 || selected.Omitted[0] != "huge" {
		t.Fatalf("oversized/duplicate handling: %+v", selected)
	}
	rendered := Render(selected.Context, []conversation.HistoricalMessage{small.Message})
	if jsonBytes(rendered) > selected.Cost {
		t.Fatalf("serialized text cost exceeded declaration: %d/%d", jsonBytes(rendered), selected.Cost)
	}
}

func TestSelectorCoverageDigitsAndSerializedCost(t *testing.T) {
	for _, count := range []int{9, 10, 99, 100, 999, 1000} {
		all := make([]conversation.Candidate, count)
		for i := range all {
			all[i] = candidate(fmt.Sprint(i), "agent", "<>&\"\n中文🙂")
		}
		selected := Select(all, Coverage{HandoffID: "handoff", ConversationID: "source"}, 64000)
		messages := make([]conversation.HistoricalMessage, len(selected.Selected))
		for i, c := range selected.Selected {
			messages[i] = c.Message
		}
		if cost := jsonBytes(Render(selected.Context, messages)); cost > selected.Cost || selected.Cost > 64000 {
			t.Fatalf("%d counter boundary: %d / %d", count, cost, selected.Cost)
		}
	}
}

func BenchmarkSelectorTenThousand(b *testing.B) {
	all := make([]conversation.Candidate, 10000)
	for i := range all {
		all[i] = candidate(fmt.Sprint(i), "agent", strings.Repeat("x", 2000))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Select(all, Coverage{HandoffID: "handoff", ConversationID: "source"}, 16000)
	}
}
