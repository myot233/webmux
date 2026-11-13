package hybrid

import (
	"context"
	"sort"
	"strings"
	"time"
)

const chatSearchLimit = 30

type MessageMatch struct {
	SessionID string
	MessageID string
	Role      string
	Excerpt   string
	At        time.Time
}

// SearchMessages scans bounded, already-cached live chat logs. It never
// starts an agent or reads provider transcripts.
func (s *Service) SearchMessages(ctx context.Context, query string) ([]MessageMatch, error) {
	s.mu.Lock()
	actors := make([]*actor, 0, len(s.actors))
	for _, a := range s.actors {
		actors = append(actors, a)
	}
	s.mu.Unlock()

	query = strings.ToLower(query)
	results := make([]MessageMatch, 0, chatSearchLimit)
	for _, a := range actors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matches, err := a.log.searchMessages(ctx, a.id, query)
		if err != nil {
			return nil, err
		}
		results = append(results, matches...)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].At.Equal(results[j].At) {
			if results[i].SessionID == results[j].SessionID {
				return results[i].MessageID < results[j].MessageID
			}
			return results[i].SessionID < results[j].SessionID
		}
		return results[i].At.After(results[j].At)
	})
	results = results[:min(len(results), chatSearchLimit)]
	return results, nil
}

type searchChunk struct {
	id   string
	role string
	text string
	at   time.Time
}

func (l *chatLog) snapshotMessageChunks() []searchChunk {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state.Side != SideACP || l.ended || len(l.events) == 0 {
		return nil
	}
	chunks := make([]searchChunk, 0, len(l.events))
	for _, event := range l.events {
		m := event.Message
		if m == nil || (m.Role != "user" && m.Role != "agent") {
			continue
		}
		chunks = append(chunks, searchChunk{
			id:   m.MessageID,
			role: m.Role,
			text: m.Text,
			at:   event.At,
		})
	}
	return chunks
}

func (l *chatLog) searchMessages(ctx context.Context, sessionID, query string) ([]MessageMatch, error) {
	chunks := l.snapshotMessageChunks()
	if len(chunks) == 0 {
		return nil, nil
	}
	type messageKey struct{ id, role string }
	type messageText struct {
		first  string
		chunks *strings.Builder
	}
	text := make(map[messageKey]messageText)
	for _, m := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := messageKey{m.id, m.role}
		value, exists := text[key]
		if !exists {
			text[key] = messageText{first: m.text}
			continue
		}
		if value.chunks == nil {
			value.chunks = new(strings.Builder)
			value.chunks.WriteString(value.first)
			text[key] = value
		}
		value.chunks.WriteString(m.text)
	}
	var results []MessageMatch
	for i := len(chunks) - 1; i >= 0 && len(results) < chatSearchLimit; i-- {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m := chunks[i]
		key := messageKey{m.id, m.role}
		value, exists := text[key]
		if !exists {
			continue
		}
		delete(text, key)
		message := value.first
		if value.chunks != nil {
			message = value.chunks.String()
		}
		if excerpt, ok := matchExcerpt(message, query); ok {
			results = append(results, MessageMatch{sessionID, m.id, m.role, excerpt, m.at})
		}
	}
	return results, nil
}

func matchExcerpt(text, query string) (string, bool) {
	// Rune offsets stay valid even when lowercasing changes the byte length.
	lower := strings.ToLower(text)
	i := strings.Index(lower, query)
	if i < 0 {
		return "", false
	}
	left := len([]rune(lower[:i]))
	runes := []rune(text)
	start := max(0, left-45)
	end := min(len(runes), left+len([]rune(query))+90)
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + strings.Join(strings.Fields(string(runes[start:end])), " ") + suffix, true
}
