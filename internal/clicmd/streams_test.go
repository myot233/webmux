package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

func TestStripper(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain\n", "plain\n"},
		{"crlf\r\nnext\n", "crlf\nnext\n"},
		{"pty\r\r\nline\n", "pty\nline\n"},
		{"progress 10%\rprogress 99%\rdone\n", "done\n"},
		{"\x1b[31mred\x1b[0m \x1b[1;2Hmoved\n", "red moved\n"},
		{"title\x1b]0;window\x07 osc\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\\n", "title osclink\n"},
		{"dcs\x1bPq#0;2\x1b\\ done\n", "dcs done\n"},
		{"abc\bd\n", "abd\n"},
		{"\x1b(Bcharset\x1bcreset\n", "charsetreset\n"},
		{"tab\there\x07bell\n", "tab\therebell\n"},
		{"中文\b字\n", "中字\n"},
		{"unfinished", "unfinished"},
	} {
		var out bytes.Buffer
		s := newStripper(&out)
		// Byte by byte: state carries across writes.
		for i := range len(tc.in) {
			_, _ = s.Write([]byte{tc.in[i]})
		}
		_ = s.Flush()
		if out.String() != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, out.String(), tc.want)
		}
	}
}

func TestParseStates(t *testing.T) {
	states, err := parseStates("idle, attention")
	if err != nil || strings.Join(states, ",") != "idle,attention" {
		t.Fatalf("%v %v", states, err)
	}
	if _, err := parseStates("idle,done"); err == nil {
		t.Fatal("unknown state accepted")
	}
}

// A --changed wait must see a session return to its subscription state after
// leaving it: a shell at its prompt is waiting, runs a command, and is
// waiting again.
func TestMatchingChangedAcceptsRoundTrip(t *testing.T) {
	w := &sessionWatch{subscription: &sessionobserve.SessionSubscription{}}
	opts := waitOptions{ids: []string{"a"}, states: []string{"waiting"}, changed: true}
	baseline := map[string]string{"a": string(contract.SessionStatusWaiting)}
	set := func(status contract.SessionStatus) {
		if err := w.subscription.Apply(contract.SessionEvent{Type: "updated", Session: &contract.SessionInfo{ID: "a", Status: status}}); err != nil {
			t.Fatal(err)
		}
	}
	set(contract.SessionStatusWaiting)
	if len(w.matching(opts, baseline)) != 0 {
		t.Fatal("the subscription state matched without a change")
	}
	set(contract.SessionStatusRunning)
	if len(w.matching(opts, baseline)) != 0 {
		t.Fatal("a state outside --until matched")
	}
	set(contract.SessionStatusWaiting)
	if len(w.matching(opts, baseline)) != 1 {
		t.Fatal("returning to the subscription state after a change did not match")
	}
}

// Server paths stay POSIX on every client OS.
func TestServerPathHelpers(t *testing.T) {
	for _, tc := range []struct {
		root, dir string
		in        bool
	}{
		{"/w/app", "/w/app", true},
		{"/w/app", "/w/app/src", true},
		{"/w/app", "/w/app2", false},
		{"/w/app", "/w", false},
		{"/w/app/", "/w/app/x/../y", true},
		{"/", "/anything", true},
		{"", "/w", false},
	} {
		if got := within(tc.root, tc.dir); got != tc.in {
			t.Errorf("within(%q, %q) = %v", tc.root, tc.dir, got)
		}
	}
	repo := contract.Repository{AnchorPath: "/home/u/src/app"}
	if got := defaultWorktreePath(repo, "feat/x"); got != "/home/u/src/app.worktrees/x" {
		t.Errorf("default worktree path %q", got)
	}
	if !isDriveLetter("C") || isDriveLetter("repo") || isDriveLetter("1") {
		t.Error("drive letters")
	}
}

func TestSessionWaitPoliciesUseSharedFacts(t *testing.T) {
	exitCode := 7
	tests := []struct {
		name    string
		options waitOptions
		frames  []contract.SessionEvent
		stable  bool
		wantIDs string
		wantErr error
	}{
		{
			name: "all sessions", options: waitOptions{ids: []string{"a", "b"}, states: []string{"idle"}},
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "idle"}, {ID: "b", Status: "running"}}}, {Type: "updated", Session: &contract.SessionInfo{ID: "b", Status: "idle"}}},
			wantIDs: "a,b",
		},
		{
			name: "any session", options: waitOptions{ids: []string{"a", "b"}, states: []string{"idle"}, any: true},
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "running"}, {ID: "b", Status: "idle"}}}},
			wantIDs: "b",
		},
		{
			name: "changed round trip", options: waitOptions{ids: []string{"a"}, states: []string{"waiting"}, changed: true},
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "waiting"}}}, {Type: "updated", Session: &contract.SessionInfo{ID: "a", Status: "running"}}, {Type: "updated", Session: &contract.SessionInfo{ID: "a", Status: "waiting"}}},
			wantIDs: "a",
		},
		{
			name: "stable resets when condition stops holding", options: waitOptions{ids: []string{"a"}, states: []string{"idle"}, stable: 70 * time.Millisecond}, stable: true,
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "idle"}}}, {Type: "updated", Session: &contract.SessionInfo{ID: "a", Status: "running"}}, {Type: "updated", Session: &contract.SessionInfo{ID: "a", Status: "idle"}}},
			wantIDs: "a",
		},
		{
			name: "idle is not exit and actual exit code survives", options: waitOptions{ids: []string{"a"}, states: []string{"exited"}},
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "idle"}}}, {Type: "updated", Session: &contract.SessionInfo{ID: "a", Status: "exited", ExitCode: &exitCode}}},
			wantIDs: "a",
		},
		{
			name: "removed is not exit", options: waitOptions{ids: []string{"a"}, states: []string{"exited"}},
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "running"}}}, {Type: "removed", ID: "a"}},
			wantErr: sessionobserve.ErrSessionRemoved,
		},
		{
			name: "idle times out waiting for exit", options: waitOptions{ids: []string{"a"}, states: []string{"exited"}},
			frames:  []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "a", Status: "idle"}}}},
			wantErr: errTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := observationClient(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = conn.CloseNow() }()
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				for i, frame := range tt.frames {
					if tt.stable && i > 0 {
						select {
						case <-time.After(20 * time.Millisecond):
						case <-ctx.Done():
							return
						}
					}
					data, err := json.Marshal(frame)
					if err != nil {
						t.Error(err)
						return
					}
					if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
						return
					}
				}
				_, _, _ = conn.Read(ctx)
			})
			watch, err := watchSessions(context.Background(), client)
			if err != nil {
				t.Fatal(err)
			}
			defer watch.subscription.Close()
			timeout := time.Second
			if errors.Is(tt.wantErr, errTimeout) {
				timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			started := time.Now()
			matched, err := watch.waitFor(ctx, tt.options)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("matched %+v, error %v; want %v", matched, err, tt.wantErr)
			}
			var ids []string
			for _, info := range matched {
				ids = append(ids, info.ID)
				if info.Status == contract.SessionStatusExited && (info.ExitCode == nil || *info.ExitCode != 7) {
					t.Fatalf("lost process exit code: %+v", info)
				}
			}
			if strings.Join(ids, ",") != tt.wantIDs {
				t.Fatalf("matched %v, want %s", ids, tt.wantIDs)
			}
			if tt.stable && time.Since(started) < 100*time.Millisecond {
				t.Fatal("stability timer ignored the intervening running state")
			}
		})
	}
}
