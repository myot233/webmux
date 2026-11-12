package wire

import (
	"strconv"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/handoff"
)

func HandoffRequest(r contract.HandoffPreviewRequest) handoff.Request {
	return handoff.Request{ClientRequestID: r.ClientRequestID, ConfigID: r.ConfigID, TargetProviderThreadID: r.TargetProviderThreadID, AgentConfig: r.AgentConfig, UserText: r.UserText, Fork: r.Fork, ForceFresh: r.Fresh, Location: conversation.PreviewLocation(r.Location), Attachments: each(r.Attachments, func(a contract.ChatAttachment) conversation.PreviewAttachment {
		kind := a.Kind
		if kind == "" {
			kind = "file"
		}
		return conversation.PreviewAttachment{Reference: a.Path, Name: a.Name, MimeType: a.MimeType, Size: a.Size, Kind: kind}
	})}
}

func HandoffPreview(p conversation.Preview) contract.HandoffPreview {
	state := p.State
	if p.Stale && (state == "ready" || state == "over_budget") {
		state = "stale"
	}
	// Draft creation, retrieval and editing all return bounded metadata. The
	// selection endpoint recovers the complete lists by activity position.
	selectedCount, omittedCount := p.SelectedCount, p.OmittedCount
	p.SelectedItemIDs = p.SelectedItemIDs[:min(len(p.SelectedItemIDs), 100)]
	p.OmittedItemIDs = p.OmittedItemIDs[:min(len(p.OmittedItemIDs), 100)]
	return contract.HandoffPreview{SourceLocation: contract.HandoffLocation(p.Request.SourceLocation), ID: p.ID, SourceConversationID: p.SourceConversationID, TargetConversationID: p.TargetConversationID, TargetProviderThreadID: p.TargetProviderThreadID, ConfigID: p.Request.ConfigID, AgentName: p.Request.AgentName, AgentConfig: requiredMap(p.Request.AgentConfig), UserText: p.Request.UserText, Fresh: p.Request.ForceFresh, Fork: p.Request.Fork, Location: contract.HandoffLocation(p.Request.Location), Attachments: each(p.Request.Attachments, func(a conversation.PreviewAttachment) contract.ChatAttachment {
		return contract.ChatAttachment{Path: a.Reference, Name: a.Name, MimeType: a.MimeType, Size: a.Size, Kind: a.Kind}
	}), Strategy: p.Strategy, State: state, SourcePosition: strconv.FormatInt(p.SourcePosition, 10), SourceRunOrdinal: strconv.FormatInt(p.SourceRunOrdinal, 10), FromRunOrdinal: strconv.FormatInt(p.FromRunOrdinal, 10), ToRunOrdinal: strconv.FormatInt(p.ToRunOrdinal, 10), Revision: p.Revision, HistoryOrigin: p.HistoryOrigin, ReconcileRequired: p.ReconcileRequired, Stale: p.Stale, Manual: p.ManualText != nil, RenderedText: p.RenderedText, SelectedItemIDs: required(p.SelectedItemIDs), OmittedItemIDs: required(p.OmittedItemIDs), OmittedItems: each(p.OmittedItemIDs, func(id string) contract.HandoffOmission {
		reason := "budget"
		if p.ManualText != nil {
			reason = "manual_context"
		}
		return contract.HandoffOmission{ActivityID: id, Reason: reason}
	}), SelectedCount: selectedCount, OmittedCount: omittedCount, SelectionTruncated: selectedCount > len(p.SelectedItemIDs) || omittedCount > len(p.OmittedItemIDs), Budget: contract.HandoffBudget(p.Budget), NativeAvailable: p.Request.NativePlanned && p.ManualText == nil, HistoryToolAvailable: p.HistoryToolAvailable}
}

func HandoffSelection(p conversation.SelectionPage) contract.HandoffSelection {
	return contract.HandoffSelection{
		SourceLocation:       contract.HandoffLocation(p.SourceLocation),
		SourceConversationID: p.SourceConversationID, SourcePosition: strconv.FormatInt(p.SourcePosition, 10), Revision: p.Revision,
		HistoryOrigin: p.HistoryOrigin, ReconcileRequired: p.ReconcileRequired, Manual: p.Manual,
		SelectedCount: p.SelectedCount, OmittedCount: p.OmittedCount, NextPosition: strconv.FormatInt(p.NextPosition, 10), Done: p.Done,
		Items: each(p.Items, func(item conversation.SelectionItem) contract.HandoffSelectionItem {
			reason := ""
			if !item.Selected {
				reason = "budget"
				if p.Manual {
					reason = "manual_context"
				}
			}
			return contract.HandoffSelectionItem{ActivityID: item.ActivityID, Position: strconv.FormatInt(item.Position, 10), Selected: item.Selected, Reason: reason}
		}),
	}
}

func HandoffMessage(r contract.ConversationMessageRequest) handoff.MessageRequest {
	request := HandoffRequest(contract.HandoffPreviewRequest{Attachments: r.Attachments})
	return handoff.MessageRequest{ClientRequestID: r.ClientRequestID, HandoffID: r.HandoffID, Revision: r.Revision, Text: r.Text, ConfigID: r.ConfigID, AgentConfig: r.AgentConfig, Location: conversation.PreviewLocation(r.Location), Attachments: request.Attachments, Retry: r.Retry}
}

func HandoffDelivery(e conversation.Execution) contract.HandoffDelivery {
	status, promptStatus := e.Status, e.Status
	if e.Status == "inline" {
		promptStatus = "consumed"
		if e.HistoryStatus == "injected" {
			status = "injected"
		}
	}
	historyStatus := e.HistoryStatus
	if historyStatus == "" {
		historyStatus = "none"
	}
	return contract.HandoffDelivery{ClientRequestID: e.ClientRequestID, Retry: e.Retry, ExecutionID: e.ID, DeliveryID: e.DeliveryID, HandoffID: e.HandoffID, SourceConversationID: e.SourceConversationID, TargetConversationID: e.TargetConversationID, TargetProviderThreadID: e.TargetProviderThreadID, SessionID: e.RuntimeID, Status: status, HistoryStatus: historyStatus, PromptStatus: promptStatus, Phase: e.Phase, Step: e.Step, ErrorClass: e.ErrorClass, WorktreeID: e.WorktreeID, WorktreePath: e.WorktreePath, NativeContinue: e.NativeContinue, ReplacementHandoffID: e.ReplacementHandoffID, UpdatedAt: e.UpdatedAt}
}
