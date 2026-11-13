package hybrid

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSearchMessagesUsesCachedUserAndAgentText(t *testing.T) {
	t.Parallel()
	log := newChatLog(64*1024, ChatState{Side: SideACP})
	at := time.Unix(1700000000, 0)
	log.append(at,
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "1", Role: "user", Text: strings.Repeat("背景", 40) + "Git 状态"}},
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "2", Role: "thought", Text: "git hidden"}},
	)
	log.append(at.Add(time.Second),
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "3", Role: "agent", Text: "查看 gi"}},
	)
	log.append(at.Add(2*time.Second),
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "3", Role: "agent", Text: "t diff"}},
	)
	tui := newChatLog(64*1024, ChatState{Side: SideTUI})
	tui.append(at.Add(3*time.Second),
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "4", Role: "user", Text: "git in terminal"}},
	)
	s := Service{actors: map[string]*actor{
		"demo":     {id: "demo", log: log},
		"terminal": {id: "terminal", log: tui},
	}}
	items, err := s.SearchMessages(context.Background(), "GIT")
	if err != nil || len(items) != 2 || items[0].MessageID != "3" || items[0].Excerpt != "查看 git diff" ||
		items[1].MessageID != "1" || !strings.Contains(items[1].Excerpt, "Git 状态") ||
		!strings.HasPrefix(items[1].Excerpt, "…") {
		t.Fatalf("matches = %+v, err = %v", items, err)
	}
}

func TestSearchMessagesJoinsChunksByMessageAndRole(t *testing.T) {
	t.Parallel()
	log := newChatLog(64*1024, ChatState{Side: SideACP})
	at := time.Unix(1700000000, 0)
	log.append(at,
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "shared", Role: "user", Text: "alpha"}},
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "shared", Role: "agent", Text: "not a match"}},
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "shared", Role: "user", Text: "beta"}},
		Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "other", Role: "user", Text: "alpha"}},
	)
	s := Service{actors: map[string]*actor{"demo": {id: "demo", log: log}}}
	items, err := s.SearchMessages(context.Background(), "ALPHABETA")
	if err != nil || len(items) != 1 || items[0].MessageID != "shared" ||
		items[0].Role != "user" || items[0].Excerpt != "alphabeta" {
		t.Fatalf("matches = %+v, err = %v", items, err)
	}
}

func TestSearchMessagesFiltersBeforeGlobalLimit(t *testing.T) {
	t.Parallel()
	at := time.Unix(1700000000, 0)
	actors := make(map[string]*actor)
	for _, side := range []Side{SideACP, SideTUI} {
		log := newChatLog(64*1024, ChatState{Side: side})
		for i := range chatSearchLimit + 5 {
			when := at.Add(time.Duration(i) * time.Second)
			if side == SideTUI {
				when = when.Add(time.Hour)
			}
			log.append(when, Event{Kind: EventMessage, Message: &MessageChunk{
				MessageID: strconv.Itoa(i), Role: "user", Text: "needle",
			}})
		}
		id := string(side)
		actors[id] = &actor{id: id, log: log}
	}
	ended := newChatLog(64*1024, ChatState{Side: SideACP})
	ended.append(at.Add(2*time.Hour), Event{Kind: EventMessage, Message: &MessageChunk{
		MessageID: "ended", Role: "user", Text: "needle",
	}})
	ended.ended = true
	actors["ended"] = &actor{id: "ended", log: ended}
	newest := newChatLog(64*1024, ChatState{Side: SideACP})
	newest.append(at.Add(30*time.Minute), Event{Kind: EventMessage, Message: &MessageChunk{
		MessageID: "latest", Role: "agent", Text: "needle",
	}})
	actors["otherchat"] = &actor{id: "otherchat", log: newest}
	s := Service{actors: actors}
	items, err := s.SearchMessages(context.Background(), "needle")
	if err != nil || len(items) != chatSearchLimit {
		t.Fatalf("matches = %+v, err = %v", items, err)
	}
	if items[0].SessionID != "otherchat" || items[0].MessageID != "latest" {
		t.Fatalf("latest cross-session match = %+v", items[0])
	}
	for i, item := range items[1:] {
		if item.SessionID != string(SideACP) || item.MessageID != strconv.Itoa(chatSearchLimit+4-i) {
			t.Fatalf("match %d = %+v", i, item)
		}
	}
}

func TestSearchMessagesOnlyRetainsBoundedCache(t *testing.T) {
	t.Parallel()
	log := newChatLog(1, ChatState{Side: SideACP})
	at := time.Unix(1700000000, 0)
	log.append(at, Event{Kind: EventMessage, Message: &MessageChunk{
		MessageID: "old", Role: "user", Text: "evicted needle",
	}})
	log.append(at.Add(time.Second), Event{Kind: EventMessage, Message: &MessageChunk{
		MessageID: "new", Role: "user", Text: "retained needle",
	}})
	s := Service{actors: map[string]*actor{"demo": {id: "demo", log: log}}}
	items, err := s.SearchMessages(context.Background(), "needle")
	if err != nil || len(items) != 1 || items[0].MessageID != "new" {
		t.Fatalf("matches = %+v, err = %v", items, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.SearchMessages(ctx, "needle"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search = %v", err)
	}
}
