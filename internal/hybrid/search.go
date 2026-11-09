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
		a.log.mu.Lock()
		if a.log.state.Side != SideACP || a.log.ended {
			a.log.mu.Unlock()
			continue
		}
		events := append([]Event(nil), a.log.events...)
		a.log.mu.Unlock()
		type messageKey struct{ id, role string }
		text := make(map[messageKey]*strings.Builder)
		for _, ev := range events {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			m := ev.Message
			if m == nil || (m.Role != "user" && m.Role != "agent") {
				continue
			}
			key := messageKey{m.MessageID, m.Role}
			if text[key] == nil {
				text[key] = new(strings.Builder)
			}
			text[key].WriteString(m.Text)
		}
		matched := 0
		for i := len(events) - 1; i >= 0 && matched < chatSearchLimit; i-- {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ev := events[i]
			m := ev.Message
			if m == nil {
				continue
			}
			key := messageKey{m.MessageID, m.Role}
			message := text[key]
			if message == nil {
				continue
			}
			delete(text, key)
			if excerpt, ok := matchExcerpt(message.String(), query); ok {
				matched++
				results = append(results, MessageMatch{a.id, m.MessageID, m.Role, excerpt, ev.At})
			}
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
