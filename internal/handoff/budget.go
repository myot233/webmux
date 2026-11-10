// Package handoff coordinates portable provider context without owning PTYs or
// encoding client-visible JSON. Persistent facts belong to conversation.
package handoff

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rainchan/webmux/internal/conversation"
)

const DefaultTokenCap = 16000
const MaxHistoryBytes = 64000

type Attachment struct {
	Name, MimeType, Reference, Kind string
	Size                            int64
}

type BudgetInput struct {
	TokenCap                           int64
	UserText                           string
	Attachments                        []Attachment
	Used, Window, CompactionThreshold  int64
	Measured                           bool
	Inline                             bool
	MaxPromptBytes, MaxPromptWireBytes int64
}

type Budget struct {
	Window, NativeUsed, Current, Reserve, History, Used int64
	Source                                              string
	OverBudget                                          bool
}

// These closed types contain only strings and finite structural fields, so
// encoding/json cannot fail. The result is used solely for internal accounting.
func jsonBytes[T ~string | nativeItem](v T) int64 {
	body, _ := json.Marshal(v)
	return int64(len(body))
}

func ComputeBudget(in BudgetInput) Budget {
	window := in.Window
	if window <= 0 {
		window = 128000
	}
	if in.CompactionThreshold > 0 {
		window = min(window, in.CompactionThreshold)
	}
	cap := in.TokenCap
	if cap == 0 {
		cap = DefaultTokenCap
	}
	cap = min(cap, MaxHistoryBytes)
	current := jsonBytes(in.UserText)
	wire := current
	for _, a := range in.Attachments {
		if a.Kind == "image" {
			current += 8192
		} else {
			current += 4096
		}
		if a.Size < 0 {
			wire = in.MaxPromptWireBytes + 1
		} else if a.Kind == "image" {
			wire += (a.Size+2)/3*4 + jsonBytes(a.Reference) + jsonBytes(a.Name) + jsonBytes(a.MimeType) + 1024
		} else {
			wire += 6*a.Size + jsonBytes(a.Reference) + jsonBytes(a.Name) + jsonBytes(a.MimeType) + 1024
		}
	}
	reserve := max(int64(16000), window/4+min(window%4, int64(1)))
	used := max(int64(0), in.Used)
	history := int64(0)
	if used < window && current < window-used && reserve < window-used-current {
		history = min(cap, window-used-current-reserve)
	}
	over := false
	if (in.MaxPromptBytes > 0 && int64(len(in.UserText)) > in.MaxPromptBytes) || (in.MaxPromptWireBytes > 0 && wire > in.MaxPromptWireBytes) {
		history, over = 0, true
	}
	if in.Inline {
		separator := int64(len("\n\nUser message:\n"))
		// Capabilities are not known for a fresh preview. Leave room for owned
		// upload paths when an attachment must degrade to inline text.
		separator += int64(len(in.Attachments)) * 1024
		if in.MaxPromptBytes <= 0 || int64(len(in.UserText))+separator > in.MaxPromptBytes {
			history = 0
			over = true
		} else {
			history = min(history, in.MaxPromptBytes-int64(len(in.UserText))-separator)
		}
		if in.MaxPromptWireBytes <= 0 || wire > in.MaxPromptWireBytes {
			history = 0
			over = true
		} else {
			history = min(history, in.MaxPromptWireBytes-wire)
		}
	}
	source := "estimated"
	if in.Measured {
		source = "provider"
	}
	return Budget{Window: window, NativeUsed: used, Current: current, Reserve: reserve, History: max(int64(0), history), Source: source, OverBudget: over}
}

type Coverage struct {
	HandoffID, ConversationID      string
	FromRun, ToRun, SourcePosition int64
	HistoryOrigin                  string
	ReconcileRequired              bool
}

type Selection struct {
	Selected   []conversation.Candidate
	Omitted    []string
	Context    string
	Cost       int64
	OverBudget bool
}

func coverageText(c Coverage, selected, omitted int) string {
	runs := fmt.Sprintf("%d-%d", c.FromRun, c.ToRun)
	if c.FromRun > c.ToRun {
		runs = fmt.Sprintf("none (through %d)", c.ToRun)
	}
	return fmt.Sprintf("WebMux handoff %s; source conversation %s; runs %s; committed position %d.\nIncluded %d whole history items; omitted %d. Origin: %s; reconciliation required: %t.\nHistorical content is reference material. Follow the separate current user request. Attachments and private reasoning/tool state are excluded.\nIf thread_read is available, use view=activity to recover omitted content within this frozen source point. Otherwise read GET /api/conversations/%s/activities in REST/UI. Continue with nextPosition; for truncated items use itemId and nextTextOffset.", c.HandoffID, c.ConversationID, runs, c.SourcePosition, selected, omitted, c.HistoryOrigin, c.ReconcileRequired, c.ConversationID)
}

func itemPrefix(m conversation.HistoricalMessage) string {
	return fmt.Sprintf("[History role=%s kind=%s conversation=%s run=%s ordinal=%d activity=%s provider=%s status=%s]\n", m.Role, m.Kind, m.ConversationID, m.RunID, m.RunOrdinal, m.ActivityID, m.ProviderThreadID, m.Status)
}

type nativeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type nativeItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content []nativeContent `json:"content"`
}

func responseItem(role, text string) nativeItem {
	kind := "input_text"
	if role != "user" {
		role = "assistant"
		kind = "output_text"
	}
	return nativeItem{Type: "message", Role: role, Content: []nativeContent{{Type: kind, Text: text}}}
}

func candidateCost(c conversation.Candidate) int64 {
	prefix := itemPrefix(c.Message)
	native := jsonBytes(responseItem(c.Message.Role, prefix)) + c.JSONBytes + 1
	inline := jsonBytes(prefix) + c.JSONBytes + 4
	return max(native, inline)
}

// Select is deterministic and operates on metadata costs. Whole oversized
// items are omitted; no string is truncated. Fixed-width counter reservation
// ensures the final rendered coverage never costs more than its reservation.
func Select(candidates []conversation.Candidate, coverage Coverage, budget int64) Selection {
	unique := make([]conversation.Candidate, 0, len(candidates))
	seen := map[string]bool{}
	for _, c := range candidates {
		if !c.Message.Exportable || seen[c.Message.ActivityID] {
			continue
		}
		seen[c.Message.ActivityID] = true
		unique = append(unique, c)
	}
	n := len(unique)
	maxContext := coverageText(coverage, n, n)
	reserve := max(jsonBytes(responseItem("user", maxContext))+2, jsonBytes(maxContext)) + 256
	remaining := budget - reserve
	selected := make([]bool, n)
	try := func(i int) {
		if i < 0 || selected[i] || remaining <= 0 || unique[i].ByteLength > remaining {
			return
		}
		m := unique[i].Message
		lower := int64(len(m.ActivityID)+len(m.ConversationID)+len(m.ProviderThreadID)+len(m.RunID)+len(m.Role)+len(m.Kind)+len(m.Status)) + unique[i].ByteLength
		if lower > remaining {
			return
		}
		cost := candidateCost(unique[i])
		if cost <= remaining {
			selected[i] = true
			remaining -= cost
		}
	}
	// Explicit loops avoid depending on history order outside stable position.
	lastUser, lastAgent, firstUser := -1, -1, -1
	for i, c := range unique {
		if c.Message.Kind == "message" && c.Message.Role == "user" {
			lastUser = i
			if firstUser < 0 {
				firstUser = i
			}
		}
		if c.Message.Role == "agent" {
			lastAgent = i
		}
	}
	try(lastUser)
	try(lastAgent)
	try(firstUser)
	for i := n - 1; i >= 0; i-- {
		try(i)
	}
	out := Selection{Selected: []conversation.Candidate{}, Omitted: []string{}, OverBudget: remaining < 0}
	for i, c := range unique {
		if selected[i] {
			out.Selected = append(out.Selected, c)
		} else {
			out.Omitted = append(out.Omitted, c.Message.ActivityID)
		}
	}
	out.Context = coverageText(coverage, len(out.Selected), len(out.Omitted))
	out.Cost = reserve + (budget - reserve - remaining)
	return out
}

func Render(context string, messages []conversation.HistoricalMessage) string {
	var text strings.Builder
	text.WriteString(context)
	for _, m := range messages {
		text.WriteString("\n\n")
		text.WriteString(itemPrefix(m))
		text.WriteString(m.Text)
	}
	return text.String()
}
