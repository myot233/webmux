package hybrid

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/acp"
)

// maxFieldBytes caps one text field of an event (a message chunk, a tool
// output, a summary), so one huge value cannot fill the log; longer text
// is cut at a UTF-8 boundary and flagged where the event has a flag.
const maxFieldBytes = 256 << 10

// capText returns s cut to maxFieldBytes and whether it was cut.
func capText(s string) (string, bool) {
	if len(s) <= maxFieldBytes {
		return s, false
	}
	cut := maxFieldBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func capped(s string) string {
	s, _ = capText(s)
	return s
}

func cappedPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := capped(*s)
	return &v
}

// translator turns ACP session updates into normalized events. It holds
// the little state normalization needs: which message consecutive chunks
// without a messageId belong to, and the summary of each compaction.
// It is owned by the actor.
type translator struct {
	nextMessage int
	last        struct {
		open        bool
		role, agent string // agent: the ACP messageId, empty if none
		id          string
	}
	compactions map[string]*Compaction
}

// breakMessage ends the current message: the next chunk without a
// messageId starts a new one.
func (t *translator) breakMessage() { t.last.open = false }

// messageID names the timeline message a chunk belongs to: the agent's
// messageId, or a server-assigned ID shared by consecutive chunks of one
// role. One ACP message can carry both thought and text chunks under the
// same messageId (omp's replay does); the timeline shows them as separate
// entries, so roles other than the agent's reply are qualified.
func (t *translator) messageID(role, agentID string) string {
	if agentID != "" {
		id := agentID
		if role != RoleAgent {
			id = role + ":" + agentID
		}
		t.last.open, t.last.role, t.last.agent, t.last.id = true, role, agentID, id
		return id
	}
	if t.last.open && t.last.role == role && t.last.agent == "" {
		return t.last.id
	}
	t.nextMessage++
	id := "wm" + strconv.Itoa(t.nextMessage)
	t.last.open, t.last.role, t.last.agent, t.last.id = true, role, "", id
	return id
}

// userMessage is the event for a prompt the user sent.
func (t *translator) userMessage(text string, attachments []Attachment) Event {
	t.breakMessage()
	text, cut := capText(text)
	return Event{
		Kind: EventMessage,
		Message: &MessageChunk{
			MessageID: t.messageID(RoleUser, ""), Role: RoleUser, Text: text, Truncated: cut,
			Attachments: attachments,
		},
	}
}

// update translates one session/update; unknown kinds yield nothing.
func (t *translator) update(u acp.Update) []Event {
	switch u := u.(type) {
	case *acp.UserMessageChunk:
		return t.chunk(RoleUser, acp.ContentChunk(*u))
	case *acp.AgentMessageChunk:
		return t.chunk(RoleAgent, acp.ContentChunk(*u))
	case *acp.AgentThoughtChunk:
		return t.chunk(RoleThought, acp.ContentChunk(*u))
	}
	t.breakMessage()
	switch u := u.(type) {
	case *acp.ToolCall:
		tool := Tool{ToolCallID: u.ToolCallID, Title: cappedPtr(&u.Title), Status: toolStatus(&u.Status), Content: toolContents(u.Content), Locations: locations(u.Locations)}
		if u.Kind != "" {
			kind := u.Kind
			tool.Kind = &kind
		}
		if tool.Content == nil {
			tool.Content = []Content{}
		}
		if tool.Locations == nil {
			tool.Locations = []Location{}
		}
		return []Event{{Kind: EventTool, Tool: &tool}}
	case *acp.ToolCallUpdate:
		tool := Tool{ToolCallID: u.ToolCallID, Kind: cappedPtr(u.Kind), Title: cappedPtr(u.Title), Status: toolStatus(u.Status)}
		if u.Content != nil {
			tool.Content = toolContents(u.Content)
		}
		if u.Locations != nil {
			tool.Locations = locations(u.Locations)
		}
		return []Event{{Kind: EventTool, Tool: &tool}}
	case *acp.Plan:
		plan := Plan{Entries: make([]PlanEntry, len(u.Entries))}
		for i, e := range u.Entries {
			status := e.Status
			if status != "in_progress" && status != "completed" {
				status = "pending"
			}
			plan.Entries[i] = PlanEntry{Content: capped(e.Content), Priority: e.Priority, Status: status}
		}
		return []Event{{Kind: EventPlan, Plan: &plan}}
	case *acp.AvailableCommandsUpdate:
		commands := make([]Command, len(u.AvailableCommands))
		for i, c := range u.AvailableCommands {
			commands[i] = Command{Name: c.Name, Description: capped(c.Description)}
			if c.Input != nil {
				commands[i].Hint = capped(c.Input.Hint)
			}
		}
		return []Event{{Kind: EventCommands, Commands: commands}}
	case *acp.ConfigOptionUpdate:
		return []Event{{Kind: EventConfig, Config: configOptions(u.ConfigOptions)}}
	case *acp.SessionInfoUpdate:
		if !u.Title.Set {
			return nil
		}
		title := ""
		if u.Title.Value != nil {
			title = capped(*u.Title.Value)
		}
		return []Event{{Kind: EventInfo, Info: &Info{Title: &title}}}
	case *acp.UsageUpdate:
		usage := Usage{Used: u.Used, Size: u.Size}
		if u.Cost != nil {
			cost := u.Cost.Amount
			usage.Cost, usage.Currency = &cost, u.Cost.Currency
		}
		return []Event{{Kind: EventUsage, Usage: &usage}}
	case *acp.Notice:
		level := u.Severity
		if level != acp.NoticeWarning && level != acp.NoticeError {
			level = acp.NoticeInfo
		}
		text := u.Title
		if u.Description != "" {
			text += "\n" + u.Description
		}
		return []Event{{Kind: EventNotice, Notice: &Notice{Level: level, Text: capped(text)}}}
	case *acp.CompactionUpdate:
		c := t.compaction(u.CompactionID)
		c.Status = compactionStatus(u.Status)
		if u.Summary != nil {
			c.Summary = capped(blocksText(u.Summary))
		}
		out := *c
		return []Event{{Kind: EventCompaction, Compaction: &out}}
	case *acp.CompactionSummaryChunk:
		c := t.compaction(u.CompactionID)
		c.Summary = capped(c.Summary + blocksText([]acp.ContentBlock{u.Content}))
		out := *c
		return []Event{{Kind: EventCompaction, Compaction: &out}}
	}
	return nil
}

func (t *translator) chunk(role string, c acp.ContentChunk) []Event {
	text, cut := capText(blocksText([]acp.ContentBlock{c.Content}))
	return []Event{{Kind: EventMessage, Message: &MessageChunk{MessageID: t.messageID(role, c.MessageID), Role: role, Text: text, Truncated: cut}}}
}

func (t *translator) compaction(id string) *Compaction {
	if t.compactions == nil {
		t.compactions = make(map[string]*Compaction)
	}
	c := t.compactions[id]
	if c == nil {
		c = &Compaction{ID: id, Status: "running"}
		t.compactions[id] = c
	}
	return c
}

// blocksText renders message content as Markdown text: text as is, a
// resource link as a link, anything else as a placeholder naming its type.
func blocksText(blocks []acp.ContentBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		switch block.Type {
		case acp.ContentText:
			b.WriteString(block.Text)
		case acp.ContentResourceLink:
			name := block.Name
			if name == "" {
				name = block.URI
			}
			fmt.Fprintf(&b, "[%s](%s)", name, block.URI)
		default:
			fmt.Fprintf(&b, "［暂不支持的内容：%s］", block.Type)
		}
	}
	return b.String()
}

func compactionStatus(s string) string {
	switch s {
	case acp.CompactionCompleted:
		return "completed"
	case acp.CompactionFailed, acp.CompactionCancelled:
		return "failed"
	default:
		return "running"
	}
}

func toolStatus(s *string) *string {
	if s == nil {
		return nil
	}
	switch *s {
	case acp.ToolPending, acp.ToolInProgress, acp.ToolCompleted, acp.ToolFailed:
		v := *s
		return &v
	}
	return nil
}

func toolContents(in []acp.ToolCallContent) []Content {
	if in == nil {
		return nil
	}
	out := make([]Content, len(in))
	for i, c := range in {
		out[i] = toolContent(c)
	}
	return out
}

func toolContent(c acp.ToolCallContent) Content {
	switch c.Type {
	case acp.ToolContentContent:
		if c.Content == nil {
			return Content{Type: "unsupported", Unsupported: c.Type}
		}
		return contentBlock(*c.Content)
	case acp.ToolContentDiff:
		out := Content{Type: "diff", Path: capped(c.Path)}
		var cut bool
		out.NewText, cut = capText(c.NewText)
		if c.OldText != nil {
			old, oldCut := capText(*c.OldText)
			out.OldText, cut = &old, cut || oldCut
		}
		out.Truncated = cut
		return out
	}
	return Content{Type: "unsupported", Unsupported: c.Type}
}

func contentBlock(b acp.ContentBlock) Content {
	switch b.Type {
	case acp.ContentText:
		text, cut := capText(b.Text)
		return Content{Type: "text", Text: text, Truncated: cut}
	case acp.ContentResourceLink:
		return Content{Type: "resource_link", URI: capped(b.URI), Name: capped(b.Name), MimeType: b.MimeType}
	}
	return Content{Type: "unsupported", Unsupported: b.Type}
}

func locations(in []acp.ToolCallLocation) []Location {
	if in == nil {
		return nil
	}
	out := make([]Location, len(in))
	for i, l := range in {
		out[i] = Location{Path: capped(l.Path)}
		if l.Line != nil {
			line := *l.Line
			out[i].Line = &line
		}
	}
	return out
}

// configOptions keeps the select and boolean options; other types cannot
// be rendered or set.
func configOptions(in []acp.ConfigOption) []ConfigOption {
	out := make([]ConfigOption, 0, len(in))
	for _, o := range in {
		c := ConfigOption{ID: o.ID, Name: o.Name, Category: o.Category, Type: o.Type, Options: []ConfigChoice{}}
		switch o.Type {
		case acp.ConfigBoolean:
			if o.Current.Bool == nil {
				continue
			}
			enabled := *o.Current.Bool
			c.Enabled = &enabled
		case acp.ConfigSelect:
			c.Value = o.Current.ID
			for _, choice := range o.Options {
				c.Options = append(c.Options, ConfigChoice{Value: choice.Value, Name: choice.Name, Description: choice.Description})
			}
		default:
			continue
		}
		out = append(out, c)
	}
	return out
}

// configValues is what the record keeps of the options: each value as
// text ("true"/"false" for booleans).
func configValues(options []ConfigOption) map[string]string {
	values := make(map[string]string, len(options))
	for _, o := range options {
		if o.Enabled != nil {
			values[o.ID] = strconv.FormatBool(*o.Enabled)
		} else {
			values[o.ID] = o.Value
		}
	}
	return values
}

// permissionRequest describes a session/request_permission.
func permissionRequest(id string, req acp.RequestPermissionRequest) Request {
	r := Request{RequestID: id, Kind: RequestPermission, ToolCallID: req.ToolCall.ToolCallID, Options: make([]PermissionOption, len(req.Options))}
	if req.ToolCall.Title != nil {
		r.Title = capped(*req.ToolCall.Title)
	}
	for i, o := range req.Options {
		r.Options[i] = PermissionOption{OptionID: o.OptionID, Name: capped(o.Name), Kind: o.Kind}
	}
	return r
}

// formRequest describes a form-mode elicitation. Fields are ordered by
// name; the schema's property map has no order.
func formRequest(id string, req acp.CreateElicitationRequest) Request {
	form := Form{Message: capped(req.Message), Fields: []FormField{}, Supported: true}
	if schema := req.RequestedSchema; schema != nil {
		names := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			p := schema.Properties[name]
			field := FormField{Name: name, Title: capped(p.Title), Description: capped(p.Description), Required: slices.Contains(schema.Required, name), Default: slices.Clone([]byte(p.Default))}
			switch {
			case p.Type == "boolean":
				field.Type = "boolean"
			case p.Type == "string" && len(p.OneOf) > 0:
				field.Type = "enum"
				for _, o := range p.OneOf {
					field.Enum = append(field.Enum, o.Const)
					field.EnumNames = append(field.EnumNames, o.Title)
				}
			case p.Type == "string" && len(p.Enum) > 0:
				field.Type, field.Enum = "enum", slices.Clone(p.Enum)
			case p.Type == "string":
				field.Type = "string"
			default:
				field.Type = "unsupported"
				form.Supported = false
			}
			form.Fields = append(form.Fields, field)
		}
	}
	return Request{RequestID: id, Kind: RequestForm, ToolCallID: req.ToolCallID, Form: &form}
}
