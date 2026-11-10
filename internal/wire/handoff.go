package wire

import (
	"strconv"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/handoff"
)

func HandoffRequest(r contract.HandoffPreviewRequest) handoff.Request {
	return handoff.Request{ClientRequestID: r.ClientRequestID, ConfigID: r.ConfigID, TargetProviderThreadID: r.TargetProviderThreadID, AgentConfig: r.AgentConfig, UserText: r.UserText, Fork: r.Fork, Location: conversation.PreviewLocation(r.Location), Attachments: each(r.Attachments, func(a contract.ChatAttachment) conversation.PreviewAttachment {
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
	return contract.HandoffPreview{ID: p.ID, SourceConversationID: p.SourceConversationID, TargetConversationID: p.TargetConversationID, TargetProviderThreadID: p.TargetProviderThreadID, ConfigID: p.Request.ConfigID, AgentName: p.Request.AgentName, AgentConfig: requiredMap(p.Request.AgentConfig), UserText: p.Request.UserText, Fork: p.Request.Fork, Location: contract.HandoffLocation(p.Request.Location), Attachments: each(p.Request.Attachments, func(a conversation.PreviewAttachment) contract.ChatAttachment {
		return contract.ChatAttachment{Path: a.Reference, Name: a.Name, MimeType: a.MimeType, Size: a.Size, Kind: a.Kind}
	}), Strategy: p.Strategy, State: state, SourcePosition: strconv.FormatInt(p.SourcePosition, 10), SourceRunOrdinal: strconv.FormatInt(p.SourceRunOrdinal, 10), FromRunOrdinal: strconv.FormatInt(p.FromRunOrdinal, 10), ToRunOrdinal: strconv.FormatInt(p.ToRunOrdinal, 10), Revision: p.Revision, HistoryOrigin: p.HistoryOrigin, ReconcileRequired: p.ReconcileRequired, Stale: p.Stale, Manual: p.ManualText != nil, RenderedText: p.RenderedText, SelectedItemIDs: required(p.SelectedItemIDs), OmittedItemIDs: required(p.OmittedItemIDs), OmittedItems: each(p.OmittedItemIDs, func(id string) contract.HandoffOmission {
		reason := "budget"
		if p.ManualText != nil {
			reason = "manual_context"
		}
		return contract.HandoffOmission{ActivityID: id, Reason: reason}
	}), Budget: contract.HandoffBudget(p.Budget), NativeAvailable: false, HistoryToolAvailable: false}
}

func HandoffMessage(r contract.ConversationMessageRequest) handoff.MessageRequest {
	request := HandoffRequest(contract.HandoffPreviewRequest{Attachments: r.Attachments})
	return handoff.MessageRequest{ClientRequestID: r.ClientRequestID, HandoffID: r.HandoffID, Revision: r.Revision, Text: r.Text, ConfigID: r.ConfigID, AgentConfig: r.AgentConfig, Location: conversation.PreviewLocation(r.Location), Attachments: request.Attachments, Retry: r.Retry}
}

func HandoffDelivery(e conversation.Execution) contract.HandoffDelivery {
	return contract.HandoffDelivery{ExecutionID: e.ID, DeliveryID: e.DeliveryID, HandoffID: e.HandoffID, SourceConversationID: e.SourceConversationID, TargetConversationID: e.TargetConversationID, TargetProviderThreadID: e.TargetProviderThreadID, SessionID: e.RuntimeID, Status: e.Status, Phase: e.Phase, Step: e.Step, ErrorClass: e.ErrorClass, WorktreeID: e.WorktreeID, WorktreePath: e.WorktreePath, UpdatedAt: e.UpdatedAt}
}
