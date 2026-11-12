package hybrid

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
)

func TestNextPromptUsesOnlyLatestTurn(t *testing.T) {
	now := time.Now()
	l := newChatLog(64<<10, ChatState{
		Side: SideACP,
		Turn: Turn{TurnID: "t2", EndedAt: &now, StopReason: StopError},
		Plan: &Plan{Entries: []PlanEntry{
			{Content: "old step", Status: "completed"},
			{Content: "verify fixture", Status: "pending"},
		}},
	})
	l.append(now,
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleUser, Text: "older prompt"}},
		Event{Kind: EventTurnStart, TurnStart: &TurnStart{TurnID: "t2"}},
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleUser, Text: "check fixture"}},
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleThought, Text: "private reasoning"}},
		Event{Kind: EventTool, Tool: &Tool{ToolCallID: "tool", Content: []Content{{Text: "raw tool output"}}}},
		Event{Kind: EventMessage, Message: &MessageChunk{Role: RoleAgent, Text: "the fixture failed"}},
		Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: "t2", StopReason: StopError, Error: "exit 1"}},
	)
	if job, _ := l.prepareNextPrompt("t1", now); job != nil {
		t.Fatal("previous turn accepted")
	}
	job, input := l.prepareNextPrompt("t2", now)
	want := &nextPromptContext{
		StopReason: StopError, User: "check fixture", Reply: "the fixture failed", Step: "verify fixture", Error: "exit 1",
	}
	if job == nil || !reflect.DeepEqual(input, want) {
		t.Fatalf("context = %+v, want %+v", input, want)
	}
	if duplicate, input := l.prepareNextPrompt("t2", now); duplicate != job || input != nil {
		t.Fatal("a second client started another generation")
	}
}

func TestNextPromptSkipsQuietTurns(t *testing.T) {
	now := time.Now()
	expired := now.Add(-3 * time.Minute)
	tests := []struct {
		name string
		turn Turn
	}{
		{name: "complete", turn: Turn{TurnID: "t1", EndedAt: &now, StopReason: acp.StopEndTurn}},
		{name: "cancelled", turn: Turn{TurnID: "t1", EndedAt: &now, StopReason: acp.StopCancelled}},
		{name: "expired", turn: Turn{TurnID: "t1", EndedAt: &expired, StopReason: StopError}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newChatLog(64<<10, ChatState{Side: SideACP, Turn: tt.turn})
			l.append(now, Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: tt.turn.TurnID}})
			if job, _ := l.prepareNextPrompt(tt.turn.TurnID, *tt.turn.EndedAt); job != nil {
				t.Fatal("ineligible turn started generation")
			}
		})
	}
}

func TestRunNextPrompt(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "fixed instruction and context",
			give: `input=$(cat); case "$input" in
			  *"Suggest at most one short next message"*'user: "test fixture"'*)
			    printf '%s\n' "Check the missing fixture." ;;
			esac`,
			want: "Check the missing fixture.",
		},
		{name: "no suggestion", give: "exit 0"},
		{name: "error", give: "echo ignored; exit 1"},
		{name: "multiline", give: "printf 'first\\nsecond\\n'"},
		{name: "slash command", give: "echo /clear"},
		{name: "too many characters", give: "echo " + strings.Repeat("你", nextPromptMaxRunes+1)},
		{name: "output budget", give: "echo " + strings.Repeat("a", nextPromptOutputBytes+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got := runNextPrompt(ctx, []string{"/bin/sh", "-c", tt.give}, nextPromptContext{User: "test fixture"})
			if got != tt.want {
				t.Fatalf("suggestion = %q, want %q", got, tt.want)
			}
		})
	}
}

// Done signals that NextPrompt reached its per-request wait, so cancellation
// tests never depend on scheduler timing or a sleeping generator.
type nextPromptWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *nextPromptWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func blockedNextPromptService(t *testing.T) (*Service, time.Time, *os.File, *os.File, string, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	openFIFO := func(name string) *os.File {
		path := filepath.Join(dir, name)
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		return file
	}
	started, release := openFIFO("started"), openFIFO("release")
	calls := filepath.Join(dir, "calls")
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	log := newChatLog(64<<10, ChatState{
		Side: SideACP,
		Turn: Turn{TurnID: "t1", EndedAt: &now, StopReason: StopError},
	})
	log.append(now, Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: "t1", StopReason: StopError}})
	a := &actor{log: log, probeCtx: ctx}
	s := &Service{
		actors: map[string]*actor{"test": a},
		nextPromptCmd: []string{
			"/bin/sh", "-c",
			`cat >/dev/null; printf x >> "$1"; printf x > "$2"; IFS= read -r release < "$3"; printf '%s\n' 'Check the missing fixture.'`,
			"next-prompt-test", calls, started.Name(), release.Name(),
		},
		nextPromptSlots: make(chan struct{}, maxNextPromptProcesses),
	}
	t.Cleanup(func() {
		cancel()
		s.starting.Wait()
	})
	return s, now, started, release, calls, cancel
}

func waitNextPromptStarted(t *testing.T, file *os.File) {
	t.Helper()
	ready := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(file, make([]byte, 1))
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(testTimeout):
		t.Fatal("suggestion generator did not start")
	}
}

func TestNextPromptFirstWaiterCancellation(t *testing.T) {
	s, endedAt, started, release, calls, _ := blockedNextPromptService(t)
	type result struct {
		text string
		err  error
	}
	request := func(ctx context.Context) <-chan result {
		reply := make(chan result, 1)
		go func() {
			text, err := s.NextPrompt(ctx, "test", "t1", endedAt)
			reply <- result{text, err}
		}()
		return reply
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	first := request(firstCtx)
	waitNextPromptStarted(t, started)
	secondCtx := &nextPromptWaitContext{Context: context.Background(), waiting: make(chan struct{})}
	second := request(secondCtx)
	select {
	case <-secondCtx.waiting:
	case <-time.After(testTimeout):
		t.Fatal("second request did not join shared generation")
	}
	cancelFirst()
	select {
	case got := <-first:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("cancelled request = %+v", got)
		}
	case <-time.After(testTimeout):
		t.Fatal("cancelled request waited for the shared generator")
	}
	select {
	case got := <-second:
		t.Fatalf("live waiter returned before generation was released: %+v", got)
	default:
	}
	if _, err := io.WriteString(release, "continue\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-second:
		if got.err != nil || got.text != "Check the missing fixture." {
			t.Fatalf("live waiter = %+v", got)
		}
	case <-time.After(testTimeout):
		t.Fatal("live waiter did not receive shared suggestion")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if text, err := s.NextPrompt(ctx, "test", "t1", endedAt); err != nil || text != "Check the missing fixture." {
		t.Fatalf("cached suggestion = %q, %v", text, err)
	}
	if got, err := os.ReadFile(calls); err != nil || string(got) != "x" {
		t.Fatalf("generation invocations = %q, %v", got, err)
	}
}

func TestNextPromptSessionCancellationJoinsGenerator(t *testing.T) {
	s, endedAt, started, _, _, cancel := blockedNextPromptService(t)
	reply := make(chan error, 1)
	go func() {
		_, err := s.NextPrompt(context.Background(), "test", "t1", endedAt)
		reply <- err
	}()
	waitNextPromptStarted(t, started)
	cancel()
	joined := make(chan struct{})
	go func() {
		s.starting.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(testTimeout):
		t.Fatal("session cancellation did not join generator cleanup")
	}
	select {
	case err := <-reply:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(testTimeout):
		t.Fatal("session cancellation left a request waiting")
	}
	if len(s.nextPromptSlots) != 0 {
		t.Fatal("session cancellation retained a generation slot")
	}
}
