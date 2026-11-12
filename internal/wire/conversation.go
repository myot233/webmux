package wire

import (
	"strconv"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
)

func ConversationActivity(p conversation.ActivityPart) contract.ConversationActivity {
	a := p.Activity
	return contract.ConversationActivity{Exportable: p.Exportable, ExclusionReason: p.ExclusionReason, ActivityID: a.ID, ConversationID: a.ConversationID, ProviderThreadID: a.ProviderThreadID, RunID: a.RunID, Position: strconv.FormatInt(a.Position, 10), RunOrdinal: strconv.FormatInt(a.RunOrdinal, 10), Kind: a.Kind, Role: a.Role, Status: a.Status, Text: a.Text, Source: a.Source, ProviderItemID: a.ProviderItemID, ByteLength: strconv.FormatInt(a.ByteLength, 10), NextTextOffset: strconv.FormatInt(p.NextTextOffset, 10), Truncated: p.Truncated, CreatedAt: a.CreatedAt, Attachments: each(a.Attachments, func(v conversation.ActivityAttachment) contract.ConversationAttachment {
		return contract.ConversationAttachment{Name: v.Name, MimeType: v.MimeType, Reference: v.Reference, Size: v.Size}
	})}
}

func ConversationActivities(p conversation.ActivityPage) contract.ConversationActivities {
	return contract.ConversationActivities{Items: each(p.Items, ConversationActivity), NextPosition: strconv.FormatInt(p.NextPosition, 10), Gaps: each(p.Gaps, func(g conversation.HistoryGap) contract.ConversationGap {
		return contract.ConversationGap{ID: g.ID, ProviderThreadID: g.ProviderThreadID, Reason: g.Reason, CreatedAt: g.CreatedAt}
	}), ReconcileRequired: p.ReconcileRequired, HistoryOrigin: p.HistoryOrigin}
}

func ConversationContext(c conversation.Conversation) contract.ConversationContext {
	return contract.ConversationContext{ID: c.ID, Cwd: c.Cwd, RepositoryID: c.RepositoryID, WorktreeID: c.WorktreeID}
}
