package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
)

const chatTimeout = 20 * time.Second

var fakeAgent struct {
	once sync.Once
	base string
	env  []string
	dir  string
	err  error
}

// fakeAgentPath builds the ACP fake agent once, linked under both profile
// names so agentprofile recognises it.
func fakeAgentPath(t testing.TB, profile string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the fake agent")
	}
	fakeAgent.once.Do(func() {
		fakeAgent.dir, fakeAgent.err = os.MkdirTemp(fakeAgent.base, "api-fakeagent-")
		if fakeAgent.err != nil {
			return
		}
		// A separate file per profile, not a hard link: macOS reports a
		// hard-linked executable under either name, breaking the root check.
		for _, name := range []string{agentprofile.OMP, agentprofile.OpenCode} {
			build := exec.Command("go", "build", "-o", filepath.Join(fakeAgent.dir, name), "../acp/testdata/fakeagent")
			build.Env = fakeAgent.env
			if out, err := build.CombinedOutput(); err != nil {
				fakeAgent.err = fmt.Errorf("go build: %w\n%s", err, out)
				return
			}
		}
	})
	if fakeAgent.err != nil {
		t.Fatal(fakeAgent.err)
	}
	return filepath.Join(fakeAgent.dir, profile)
}

// fakeChat starts a chat-side hybrid session of the fake agent; env adds
// FAKEAGENT_* settings.
func fakeChat(t testing.TB, hybrids *hybrid.Service, cwd, profile string, env ...string) string {
	t.Helper()
	vars := map[string]string{"FAKEAGENT_STORE": t.TempDir(), "FAKEAGENT_PROFILE": profile, "FAKEAGENT_TURN_DELAY": "5ms"}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		vars[k] = v
	}
	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()
	s, err := hybrids.Create(ctx, hybrid.CreateInput{
		Cwd: cwd, Name: "chat", ConfigID: "cfgtest01", Cmd: []string{fakeAgentPath(t, profile)}, Env: vars, StartSide: hybrid.SideACP,
	})
	if err != nil {
		t.Fatalf("create chat session: %v", err)
	}
	id := s.Info().ID
	// Sessions start in auto; a fake that asks for permission is there to
	// show the request, so put it in ask mode.
	if vars["FAKEAGENT_PERMISSION"] == "1" {
		if err := hybrids.SetPermissionMode(ctx, id, hybrid.PermissionModeInput{ClientRequestID: chatRID(), Mode: hybrid.PermissionAsk}); err != nil {
			t.Fatalf("ask mode: %v", err)
		}
	}
	return id
}

var chatRequestIDs atomic.Int64

func chatRID() string { return fmt.Sprintf("api-req-%06d", chatRequestIDs.Add(1)) }

// command sends a chat command as the authenticated browser would.
func (h *hybridAPI) command(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	r := httptest.NewRequest(method, "http://webmux.example"+path, bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w
}

func (h *hybridAPI) expectCommand(method, path string, body any, status int) []byte {
	h.t.Helper()
	w := h.command(method, path, body)
	if w.Code != status {
		h.t.Fatalf("%s %s: status %d want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w.Body.Bytes()
}

// expectConflict expects a 409 HybridConflict with reason.
func (h *hybridAPI) expectConflict(method, path string, body any, reason string) {
	h.t.Helper()
	var conflict contract.HybridConflict
	if err := json.Unmarshal(h.expectCommand(method, path, body, http.StatusConflict), &conflict); err != nil {
		h.t.Fatal(err)
	}
	if conflict.Reason != reason || conflict.Error == "" {
		h.t.Fatalf("%s %s: conflict %+v, want reason %s", method, path, conflict, reason)
	}
}

func (h *hybridAPI) serve() *httptest.Server {
	server := httptest.NewServer(h.handler)
	h.t.Cleanup(server.Close)
	return server
}

// observe follows session id's chat in-process.
type observer struct {
	t      testing.TB
	sub    *hybrid.Subscription
	events []hybrid.Event
	state  hybrid.ChatState
}

func observe(t testing.TB, hybrids *hybrid.Service, id string) *observer {
	t.Helper()
	sub, err := hybrids.Subscribe(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return &observer{t: t, sub: sub, events: sub.Events, state: sub.Snapshot.State}
}

func (o *observer) until(what string, ok func(*observer) bool) {
	o.t.Helper()
	timeout := time.After(chatTimeout)
	for !ok(o) {
		select {
		case f := <-o.sub.Out:
			o.events = append(o.events, f.Events...)
			if f.State != nil {
				o.state = *f.State
			}
		case <-o.sub.Done:
			o.t.Fatalf("observer ended (%v) waiting for %s", o.sub.Err(), what)
		case <-timeout:
			o.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func (o *observer) count(kind string) int {
	n := 0
	for _, e := range o.events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func dialChat(t testing.TB, server *httptest.Server, path string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path, &websocket.DialOptions{Subprotocols: tokenProtocols("secret"), HTTPHeader: header})
	if conn != nil {
		conn.SetReadLimit(8 << 20)
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	return conn, res, err
}

func readChat(t testing.TB, conn *websocket.Conn) contract.ChatFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read chat frame: %v", err)
	}
	var f contract.ChatFrame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func closeStatus(t testing.TB, conn *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func TestChatSocketAuthOriginAndMissingSessions(t *testing.T) {
	h := newHybridAPI(t, nil)
	server := h.serve()
	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OMP)
	path := "/ws/s/" + id + "/chat"

	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()
	_, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path, nil)
	if err == nil || res == nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("without token: %v %+v", err, res)
	}
	_, res, err = dialChat(t, server, path, http.Header{"Origin": {"https://evil.example"}})
	if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin: %v %+v", err, res)
	}

	ordinary, err := h.manager.Create(session.CreateInput{Cwd: h.cwd, Cmd: []string{"sh", "-c", "sleep 60"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"zzzzzzzz", ordinary.Info().ID} {
		conn, _, err := dialChat(t, server, "/ws/s/"+missing+"/chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		if code := closeStatus(t, conn); code != 4404 {
			t.Fatalf("chat socket of %s closed with %d", missing, code)
		}
	}

	// A session on the terminal side still has its chat channel.
	var info contract.SessionInfo
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("omp", true)), 201), &info); err != nil {
		t.Fatal(err)
	}
	conn, _, err := dialChat(t, server, "/ws/s/"+info.ID+"/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := readChat(t, conn)
	if snapshot.Type != contract.ChatFrameSnapshot || snapshot.State == nil || snapshot.State.Side != contract.HybridSideTUI || snapshot.Generation == "" {
		t.Fatalf("terminal-side snapshot %+v", snapshot)
	}
	if events := readChat(t, conn); events.Type != contract.ChatFrameEvents {
		t.Fatalf("second frame %+v", events)
	}
}

func TestChatSocketResumeSendsOnlyTail(t *testing.T) {
	h := newHybridAPI(t, nil)
	server := h.serve()
	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OMP)
	watch := observe(t, h.hybrids, id)
	watch.until("start events", func(o *observer) bool { return len(o.state.Commands) > 0 && len(o.state.Config) > 0 })
	path := "/ws/s/" + id + "/chat"

	first, _, err := dialChat(t, server, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := readChat(t, first)
	backlog := readChat(t, first)
	if snapshot.Type != contract.ChatFrameSnapshot || backlog.Type != contract.ChatFrameEvents || len(backlog.Items) == 0 {
		t.Fatalf("first connection %+v %+v", snapshot, backlog)
	}
	last := backlog.Items[len(backlog.Items)-1].Seq
	if last != snapshot.Next-1 || backlog.From != snapshot.From {
		t.Fatalf("backlog %d..%d, snapshot %+v", backlog.From, last, snapshot)
	}
	_ = first.Close(websocket.StatusNormalClosure, "")

	h.expectCommand("POST", "/api/sessions/"+id+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "while away"}, 200)
	watch.until("turn end", func(o *observer) bool { return o.count(hybrid.EventTurnEnd) == 1 })

	again, _, err := dialChat(t, server, fmt.Sprintf("%s?generation=%s&seq=%d", path, snapshot.Generation, last), nil)
	if err != nil {
		t.Fatal(err)
	}
	resumed := readChat(t, again)
	tail := readChat(t, again)
	if resumed.Reset || resumed.Generation != snapshot.Generation || tail.From != last+1 || len(tail.Items) == 0 || tail.Items[0].Seq != last+1 {
		t.Fatalf("resume snapshot %+v tail from %d", resumed, tail.From)
	}
	kinds := make([]string, len(tail.Items))
	for i, e := range tail.Items {
		kinds[i] = e.Kind
	}
	if kinds[0] != contract.ChatEventTurnStart || !strings.Contains(strings.Join(kinds, " "), contract.ChatEventTurnEnd) {
		t.Fatalf("tail kinds %v", kinds)
	}

	stale, _, err := dialChat(t, server, path+"?generation=0123456789abcdef&seq=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	reset := readChat(t, stale)
	replay := readChat(t, stale)
	if !reset.Reset || replay.Items[0].Seq != reset.From {
		t.Fatalf("stale resume %+v replay from %d", reset, replay.From)
	}
}

func TestChatCommands(t *testing.T) {
	h := newHybridAPI(t, nil)
	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	watch := observe(t, h.hybrids, id)
	base := "/api/sessions/" + id + "/chat/"

	h.expectCommand("POST", base+"prompts", map[string]any{"text": "hi"}, 400)
	h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: " \n "}, 400)
	h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: strings.Repeat("é", hybrid.MaxPromptBytes/2+1)}, 400)
	h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "hi", Mode: "shout"}, 400)

	sent := contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "hello"}
	first := h.expectCommand("POST", base+"prompts", sent, 200)
	if retry := h.expectCommand("POST", base+"prompts", sent, 200); !bytes.Equal(retry, first) || string(bytes.TrimSpace(first)) != `{"queued":false}` {
		t.Fatalf("prompt %s, retry %s", first, retry)
	}
	var queued contract.ChatPromptResult
	if err := json.Unmarshal(h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "later", Mode: "auto"}, 200), &queued); err != nil {
		t.Fatal(err)
	}
	if !queued.Queued || queued.ItemID == "" {
		t.Fatalf("busy prompt %+v", queued)
	}
	item := base + "queue/" + queued.ItemID
	edited := "later, edited"
	h.expectCommand("PATCH", item, contract.ChatQueueRequest{ClientRequestID: chatRID(), Text: &edited, Move: "up"}, 204)
	watch.until("edited queue", func(o *observer) bool { return len(o.state.Queue.Items) == 1 && o.state.Queue.Items[0].Text == edited })
	h.expectCommand("DELETE", item, contract.ChatQueueRequest{ClientRequestID: chatRID()}, 204)
	h.expectConflict("PATCH", item, contract.ChatQueueRequest{ClientRequestID: chatRID(), Text: &edited}, "sent")
	h.expectConflict("POST", item+"/send-now", contract.ChatQueueRequest{ClientRequestID: chatRID()}, "sent")
	h.expectCommand("POST", base+"queue/q99/send-now", contract.ChatQueueRequest{ClientRequestID: chatRID()}, 404)

	// omp steers the running turn.
	if body := h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "nudge", Mode: "steer"}, 200); string(bytes.TrimSpace(body)) != `{"queued":false}` {
		t.Fatalf("steer %s", body)
	}
	smart, bogus := "smart", "bogus"
	h.expectCommand("PUT", base+"config/model", contract.ChatConfigRequest{ClientRequestID: chatRID(), Value: &smart}, 204)
	h.expectCommand("PUT", base+"config/model", contract.ChatConfigRequest{ClientRequestID: chatRID(), Value: &bogus}, 400)
	watch.until("model smart", func(o *observer) bool {
		for _, c := range o.state.Config {
			if c.ID == "model" {
				return c.Value == smart
			}
		}
		return false
	})

	watch.until("permission request", func(o *observer) bool { return len(o.state.Requests) == 1 })
	request := base + "requests/" + watch.state.Requests[0].RequestID
	h.expectCommand("POST", request, contract.ChatAnswerRequest{ClientRequestID: chatRID(), Outcome: "selected", OptionID: "nope"}, 400)
	answer := contract.ChatAnswerRequest{ClientRequestID: chatRID(), Outcome: "selected", OptionID: "allow"}
	h.expectCommand("POST", request, answer, 204)
	h.expectCommand("POST", request, answer, 204)
	h.expectConflict("POST", request, contract.ChatAnswerRequest{ClientRequestID: chatRID(), Outcome: "cancelled"}, "answered")
	h.expectCommand("POST", base+"requests/r99", contract.ChatAnswerRequest{ClientRequestID: chatRID(), Outcome: "cancelled"}, 404)
	h.expectCommand("POST", request, map[string]any{"clientRequestId": chatRID(), "outcome": "accept", "content": map[string]any{"x": map[string]any{}}}, 400)
	watch.until("turn end", func(o *observer) bool { return o.count(hybrid.EventTurnEnd) == 1 })

	for _, path := range []string{"cancel", "queue/resume", "queue/clear"} {
		h.expectCommand("POST", base+path, contract.ChatCommandRequest{ClientRequestID: chatRID()}, 204)
	}
	h.expectCommand("POST", "/api/sessions/zzzzzzzz/chat/cancel", contract.ChatCommandRequest{ClientRequestID: chatRID()}, 404)
}

func TestChatPermissionMode(t *testing.T) {
	h := newHybridAPI(t, nil)
	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	watch := observe(t, h.hybrids, id)
	path := "/api/sessions/" + id + "/chat/permission-mode"

	h.expectCommand("PUT", path, contract.ChatPermissionModeRequest{ClientRequestID: chatRID(), Mode: "never"}, 400)
	h.expectCommand("PUT", path, map[string]any{"clientRequestId": chatRID(), "mode": "auto", "extra": true}, 400)
	auto := contract.ChatPermissionModeRequest{ClientRequestID: chatRID(), Mode: contract.ChatPermissionAuto}
	if watch.state.PermissionMode != hybrid.PermissionAsk {
		t.Fatalf("mode after asking %q", watch.state.PermissionMode)
	}
	h.expectCommand("PUT", path, auto, 204)
	h.expectCommand("PUT", path, auto, 204)
	watch.until("auto mode", func(o *observer) bool { return o.state.PermissionMode == hybrid.PermissionAuto })
	h.expectCommand("POST", "/api/sessions/"+id+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "hello"}, 200)
	watch.until("turn end", func(o *observer) bool { return o.count(hybrid.EventTurnEnd) == 1 })
	if n := watch.count(hybrid.EventRequest); n != 0 {
		t.Fatalf("auto mode opened %d requests", n)
	}
	h.expectCommand("PUT", "/api/sessions/zzzzzzzz/chat/permission-mode", contract.ChatPermissionModeRequest{ClientRequestID: chatRID(), Mode: contract.ChatPermissionAsk}, 404)
}

func TestChatCommandConflicts(t *testing.T) {
	h := newHybridAPI(t, nil)
	var info contract.SessionInfo
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("omp", true)), 201), &info); err != nil {
		t.Fatal(err)
	}
	h.expectConflict("POST", "/api/sessions/"+info.ID+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "hi"}, "wrong_side")
	h.expectConflict("POST", "/api/sessions/"+info.ID+"/chat/cancel", contract.ChatCommandRequest{ClientRequestID: chatRID()}, "wrong_side")

	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OpenCode, "FAKEAGENT_PERMISSION=1")
	h.expectCommand("POST", "/api/sessions/"+id+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "busy"}, 200)
	h.expectConflict("POST", "/api/sessions/"+id+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "nudge", Mode: "steer"}, "steering_unavailable")

	// Every reason the chat commands report keeps its status and body.
	for _, row := range []struct {
		err    error
		status int
		reason string
	}{
		{hybrid.ErrSwitching, 409, "switching"},
		{fmt.Errorf("%w: x", hybrid.ErrAnswered), 409, "answered"},
		{hybrid.ErrSent, 409, "sent"},
		{hybrid.ErrWrongSide, 409, "wrong_side"},
		{hybrid.ErrSteeringUnavailable, 409, "steering_unavailable"},
		{hybrid.ErrLimit, 409, "limit"},
		{hybrid.ErrConflict, 409, ""},
		{&hybrid.IneligibleError{Reason: hybrid.ReasonOccupied}, 409, "not_eligible"},
		{hybrid.ErrAgentCall, 502, ""},
		{context.Canceled, 504, ""},
	} {
		w := httptest.NewRecorder()
		chatFailure(w, row.err)
		var body contract.HybridConflict
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != row.status || body.Reason != row.reason || body.Error == "" {
			t.Errorf("%v: %d %+v", row.err, w.Code, body)
		}
	}
}

// TestHybridSwitch covers POST /api/sessions/{id}/switch: a refused switch
// names its eligibility reason, a switch to the terminal returns the
// queued prompts as the requester's draft, and bad bodies are 400.
func TestHybridSwitch(t *testing.T) {
	h := newHybridAPI(t, nil)
	var info contract.SessionInfo
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("omp", true)), 201), &info); err != nil {
		t.Fatal(err)
	}
	path := "/api/sessions/" + info.ID + "/switch"
	var conflict contract.HybridConflict
	if err := json.Unmarshal(h.expectCommand("POST", path, contract.HybridSwitchRequest{ClientRequestID: chatRID(), To: contract.HybridSideACP}, 409), &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.Reason != "not_eligible" || conflict.Eligibility != hybrid.ReasonUnknownAgent {
		t.Fatalf("conflict %+v", conflict)
	}
	h.expectCommand("POST", path, contract.HybridSwitchRequest{ClientRequestID: chatRID(), To: "chat"}, 400)
	h.expectCommand("POST", path, contract.HybridSwitchRequest{ClientRequestID: chatRID(), Choice: "later"}, 400)
	h.expectCommand("POST", path, contract.HybridSwitchRequest{ClientRequestID: chatRID(), Choice: "force"}, 409)
	h.expectCommand("POST", "/api/sessions/zzzzzzzz/switch", contract.HybridSwitchRequest{ClientRequestID: chatRID(), To: contract.HybridSideTUI}, 404)

	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OMP, "FAKEAGENT_TURN_DELAY=400ms")
	base := "/api/sessions/" + id + "/chat/"
	h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "busy"}, 200)
	h.expectCommand("POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: "later"}, 200)
	var result contract.HybridSwitchResult
	if err := json.Unmarshal(h.expectCommand("POST", "/api/sessions/"+id+"/switch", contract.HybridSwitchRequest{ClientRequestID: chatRID(), To: contract.HybridSideTUI}, 200), &result); err != nil {
		t.Fatal(err)
	}
	if result.Side != contract.HybridSideTUI || result.Draft != "later" || result.Dropped == nil || result.Unsupported == nil {
		t.Fatalf("switch result %+v", result)
	}
	h.expectCommand("DELETE", "/api/sessions/"+id, nil, 204)
}

// smallBuffers shrinks the send buffer of accepted connections.
type smallBuffers struct{ net.Listener }

func (l smallBuffers) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.SetWriteBuffer(4096)
	}
	return c, err
}

// A chat client that stops reading is dropped with 4408 once its frames
// back up, while the session and other clients carry on.
func TestChatSocketSlowClient(t *testing.T) {
	h := newHybridAPI(t, nil)
	server := httptest.NewUnstartedServer(h.handler)
	server.Listener = smallBuffers{server.Listener}
	server.Start()
	t.Cleanup(server.Close)
	id := fakeChat(t, h.hybrids, h.cwd, agentprofile.OMP, "FAKEAGENT_TURN_DELAY=1ms")

	// Small socket buffers on both ends make the server's writes block soon.
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if tcp, ok := c.(*net.TCPConn); ok {
			_ = tcp.SetReadBuffer(4096)
		}
		return c, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()
	slow, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/s/"+id+"/chat", &websocket.DialOptions{
		Subprotocols: tokenProtocols("secret"),
		HTTPClient:   &http.Client{Transport: &http.Transport{DialContext: dial}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = slow.CloseNow() }()
	slow.SetReadLimit(8 << 20)

	// Each turn carries a large prompt and its echo and publishes a dozen
	// frames: far more than the subscriber queue once writes stall.
	text := strings.Repeat("x", hybrid.MaxPromptBytes-1)
	for range 30 {
		h.expectCommand("POST", "/api/sessions/"+id+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: chatRID(), Text: text}, 200)
	}
	watch := observe(t, h.hybrids, id)
	watch.until("all turns", func(o *observer) bool { return len(o.state.Queue.Items) == 0 && !o.state.Turn.Running })

	if code := closeStatus(t, slow); code != 4408 {
		t.Fatalf("slow client closed with %d, want 4408", code)
	}
	// The session is unaffected: a new connection gets the current state.
	fresh, _, err := dialChat(t, server, "/ws/s/"+id+"/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot := readChat(t, fresh); snapshot.State == nil || snapshot.State.Turn.Running || len(snapshot.State.Queue.Items) != 0 {
		t.Fatalf("snapshot after the slow client %+v", snapshot)
	}
}
