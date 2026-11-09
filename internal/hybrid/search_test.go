package hybrid

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSearchMessagesUsesCachedUserAndAgentText(t *testing.T) {
	t.Parallel()
	log := newChatLog(64*1024, ChatState{})
	at := time.Unix(1700000000, 0)
	log.append(at,
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "1", Role: "user", Text: strings.Repeat("背景", 40) + "Git 状态"}},
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "2", Role: "thought", Text: "git hidden"}},
	)
	log.append(at.Add(time.Second),
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "3", Role: "agent", Text: "查看 git diff"}},
	)
	s := Service{actors: map[string]*actor{"demo": {id: "demo", log: log}}}
	items, err := s.SearchMessages(context.Background(), "GIT")
	if err != nil || len(items) != 2 || items[0].MessageID != "3" ||
		items[1].MessageID != "1" || !strings.Contains(items[1].Excerpt, "Git 状态") ||
		!strings.HasPrefix(items[1].Excerpt, "…") {
		t.Fatalf("matches = %+v, err = %v", items, err)
	}
}
