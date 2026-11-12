package hybrid

import (
	"slices"
	"unicode/utf8"
)

const (
	// degradedToolBytes bounds each text of a tool content block that the
	// log shortened to stay within its budget.
	degradedToolBytes = 2 << 10
	// protectedDivisor: the newest capacity/protectedDivisor bytes of the
	// log keep their full tool output.
	protectedDivisor = 4
)

// degradeLocked shortens the tool output of the oldest events until the log
// fits its budget or only the protected newest part is left. Titles,
// statuses, locations and sequences stay; a client that already received an
// event keeps what it got, and a client that has not gets the short version.
func (l *chatLog) degradeLocked() {
	limit := l.capacity / protectedDivisor
	boundary, kept := len(l.events), 0
	for boundary > 0 && kept+l.sizes[boundary-1] <= limit {
		boundary--
		kept += l.sizes[boundary]
	}
	// An event that alone exceeds the window is not inside it, so it can be
	// shortened: keeping the conversation matters more than one large output.
	i := l.degraded
	for ; i < boundary && l.bytes > l.capacity; i++ {
		if e, ok := degradeTool(l.events[i]); ok {
			size := eventSize(e)
			l.bytes += size - l.sizes[i]
			l.sizes[i], l.events[i] = size, e
		}
	}
	l.degraded = max(l.degraded, i)
}

// degradeTool returns e with every tool content text cut to
// degradedToolBytes and flagged truncated, and whether anything was cut. e
// itself is left alone.
func degradeTool(e Event) (Event, bool) {
	t := e.Tool
	if t == nil {
		return e, false
	}
	var content []Content
	for i, c := range t.Content {
		short, cut := degradeContent(c)
		if !cut {
			continue
		}
		if content == nil {
			content = slices.Clone(t.Content)
		}
		content[i] = short
	}
	if content == nil {
		return e, false
	}
	short := *t
	short.Content = content
	e.Tool = &short
	return e, true
}

func degradeContent(c Content) (Content, bool) {
	var cutText, cutNew, cutOld bool
	c.Text, cutText = cutBytes(c.Text, degradedToolBytes)
	c.NewText, cutNew = cutBytes(c.NewText, degradedToolBytes)
	if c.OldText != nil {
		var old string
		if old, cutOld = cutBytes(*c.OldText, degradedToolBytes); cutOld {
			c.OldText = &old
		}
	}
	cut := cutText || cutNew || cutOld
	c.Truncated = c.Truncated || cut
	return c, cut
}

// cutBytes returns s cut to at most n bytes at a UTF-8 boundary, and whether
// it was cut.
func cutBytes(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n], true
}

// Per-field allowances of eventSize for keys, quotes and punctuation.
const (
	eventOverhead = 96
	fieldOverhead = 16
)

// eventSize estimates the encoded JSON size of e: every text field's
// length plus a fixed allowance per field and per event. Escaping is
// ignored, so text needing escapes is underestimated; the estimate only
// has to keep the log's memory proportional to scrollback_bytes.
func eventSize(e Event) int {
	n := eventOverhead + len(e.Kind)
	s := func(v string) { n += fieldOverhead + len(v) }
	sp := func(v *string) {
		if v != nil {
			s(*v)
		}
	}
	if m := e.Message; m != nil {
		s(m.MessageID)
		s(m.Role)
		s(m.Text)
	}
	if t := e.Tool; t != nil {
		s(t.ToolCallID)
		sp(t.Kind)
		sp(t.Title)
		sp(t.Status)
		for _, c := range t.Content {
			n += contentSize(c)
		}
		for _, loc := range t.Locations {
			s(loc.Path)
		}
	}
	if p := e.Plan; p != nil {
		for _, entry := range p.Entries {
			s(entry.Content)
			s(entry.Priority)
			s(entry.Status)
		}
	}
	for _, c := range e.Commands {
		s(c.Name)
		s(c.Description)
		s(c.Hint)
	}
	for _, c := range e.Config {
		n += configSize(c)
	}
	if e.Usage != nil {
		n += 4 * fieldOverhead
	}
	if e.Info != nil {
		sp(e.Info.Title)
	}
	if c := e.Compaction; c != nil {
		s(c.ID)
		s(c.Status)
		s(c.Summary)
	}
	if c := e.Notice; c != nil {
		s(c.Level)
		s(c.Text)
	}
	if t := e.TurnStart; t != nil {
		s(t.TurnID)
		s(t.PromptID)
		s(t.QueueItemID)
	}
	if t := e.TurnEnd; t != nil {
		s(t.TurnID)
		s(t.StopReason)
		s(t.Error)
	}
	if r := e.Request; r != nil {
		n += requestSize(*r)
	}
	if r := e.Resolved; r != nil {
		s(r.RequestID)
		s(r.Outcome)
		s(r.OptionID)
	}
	if m := e.Marker; m != nil {
		s(m.Kind)
		s(string(m.Side))
	}
	return n
}

func contentSize(c Content) int {
	n := 8 * fieldOverhead
	n += len(c.Type) + len(c.Text) + len(c.Path) + len(c.NewText) + len(c.URI) + len(c.Name) + len(c.MimeType) + len(c.Unsupported)
	if c.OldText != nil {
		n += len(*c.OldText)
	}
	return n
}

func configSize(c ConfigOption) int {
	n := 6*fieldOverhead + len(c.ID) + len(c.Name) + len(c.Category) + len(c.Type) + len(c.Value)
	for _, o := range c.Options {
		n += 3*fieldOverhead + len(o.Value) + len(o.Name) + len(o.Description)
	}
	return n
}

func requestSize(r Request) int {
	n := 5*fieldOverhead + len(r.RequestID) + len(r.Kind) + len(r.ToolCallID) + len(r.Title)
	for _, o := range r.Options {
		n += 3*fieldOverhead + len(o.OptionID) + len(o.Name) + len(o.Kind)
	}
	if f := r.Form; f != nil {
		n += fieldOverhead + len(f.Message)
		for _, field := range f.Fields {
			n += 6*fieldOverhead + len(field.Name) + len(field.Title) + len(field.Description) + len(field.Type) + len(field.Default)
			for i, v := range field.Enum {
				n += fieldOverhead + len(v)
				if i < len(field.EnumNames) {
					n += fieldOverhead + len(field.EnumNames[i])
				}
			}
		}
	}
	return n
}
