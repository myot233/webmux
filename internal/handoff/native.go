package handoff

import (
	"context"
	"errors"
	"strings"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/conversation"
)

func (s *Service) nativeContext(ctx context.Context, p conversation.Preview) (agentprofile.HistoricalContext, []conversation.InjectionItem, error) {
	// The preview owns its generated coverage. Reformatting after an upgrade
	// must not change the item hashes of an already reserved native injection.
	coverage := p.RenderedText
	if start := strings.Index(coverage, "\n\n[History role="); start >= 0 {
		coverage = coverage[:start]
	}
	if coverage == "" {
		return agentprofile.HistoricalContext{}, nil, conversation.ErrConflict
	}
	history := agentprofile.HistoricalContext{Items: []agentprofile.HistoricalItem{{ID: "coverage-" + p.ID, Role: "user", Text: coverage}}}
	for _, id := range p.SelectedItemIDs {
		part, err := s.options.Conversations.Activity(ctx, p.SourceConversationID, id, 0, conversation.MaxActivityBytes)
		if err != nil {
			return history, nil, err
		}
		message := conversation.ProjectActivity(part.Activity)
		if part.Truncated || part.Activity.Position > p.SourcePosition || !message.Exportable {
			return history, nil, conversation.ErrConflict
		}
		role := "user"
		if message.Role != "user" {
			role = "assistant"
		}
		history.Items = append(history.Items, agentprofile.HistoricalItem{ID: id, Role: role, Text: itemPrefix(message) + message.Text})
	}
	items := make([]conversation.InjectionItem, len(history.Items))
	for n, item := range history.Items {
		items[n] = conversation.InjectionItem{ID: item.ID, Role: item.Role, TextHash: conversation.TextHash(item.Text)}
	}
	return history, items, nil
}

func (s *Service) deliverNative(ctx context.Context, e conversation.Execution, p conversation.Preview, nativeID string) (bool, error) {
	history, items, err := s.nativeContext(ctx, p)
	if err != nil {
		return false, err
	}
	i, err := s.options.Conversations.ReserveInjection(ctx, e.ID, nativeID, items)
	if err != nil {
		return false, err
	}
	switch i.State {
	case "injected":
		return true, nil
	case "unsupported":
		return false, nil
	case "unknown":
		return false, conversation.ErrConflict
	}
	if i.CallStarted {
		return false, conversation.ErrConflict
	}
	accepted, err := s.options.Hybrid.InjectHistory(ctx, e.RuntimeID, e.ID, history)
	if err == nil && !accepted {
		err = s.options.Conversations.FinishInjection(ctx, e.DeliveryID, "unsupported")
	}
	return accepted, err
}

func (s *Service) historyOutcome(ctx context.Context, e conversation.Execution, err error) {
	i, readErr := s.options.Conversations.Injection(ctx, e.DeliveryID)
	status, class := "failed", "native_prepare"
	if readErr != nil && !errors.Is(readErr, conversation.ErrNotFound) {
		status, class = "unknown", "storage"
	} else if i.State == "unknown" || (i.CallStarted && i.State == "pending") {
		status, class = "unknown", "history_unknown"
	} else if i.State == "injected" {
		class = "native_reopen"
	}
	if status == "failed" {
		class = executionErrorClass(err, class)
	}
	_ = s.options.Conversations.FinishExecution(context.WithoutCancel(ctx), e.ID, status, class)
}
