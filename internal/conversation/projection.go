package conversation

// HistoricalMessage is the portable projection used by selection and handoff.
// Exportable=false is explicit for control events and unsupported private kinds;
// callers must never reinterpret those records as user instructions.
type HistoricalMessage struct {
	ActivityID, ConversationID, ProviderThreadID, RunID string
	Position, RunOrdinal                                int64
	Kind, Role, Status, Text, Source                    string
	Exportable                                          bool
	ExclusionReason                                     string
	Attachments                                         []ActivityAttachment
}

func ProjectActivity(a Activity) HistoricalMessage {
	out := HistoricalMessage{ActivityID: a.ID, ConversationID: a.ConversationID, ProviderThreadID: a.ProviderThreadID, RunID: a.RunID, Position: a.Position, RunOrdinal: a.RunOrdinal, Kind: a.Kind, Role: a.Role, Status: a.Status, Text: a.Text, Source: a.Source, Attachments: a.Attachments}
	switch a.Kind {
	case "message":
		out.Exportable = a.Role == "user" || a.Role == "agent"
	case "command", "tool", "error", "interrupt", "file_change", "plan", "compaction":
		out.Exportable = true
	}
	if !out.Exportable {
		out.ExclusionReason = "not_portable"
		out.Text = ""
		out.Attachments = nil
	}
	return out
}
