package push

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/hybrid"
)

// A permission request on a hybrid session's chat side raises permission
// attention through the session list, which pushes like a terminal agent's.
func TestServicePushesChatPermission(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the fake ACP agent")
	}
	agent := filepath.Join(t.TempDir(), agentprofile.OMP)
	if out, err := exec.Command("go", "build", "-o", agent, "../acp/testdata/fakeagent").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	m := newManager(t)
	hybrids := hybrid.New(hybrid.Options{StateDir: t.TempDir(), Manager: m})
	t.Cleanup(hybrids.Close)
	server := newPushServer(t, nil)
	s := startService(t, m, server, nil)
	p256dh, auth, ua := browserKeys(t)
	if _, err := s.Register(server.URL+"/device", p256dh, auth, "https://term.example.com"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	created, err := hybrids.Create(ctx, hybrid.CreateInput{
		Cwd: t.TempDir(), Name: "chat", Cmd: []string{agent}, StartSide: hybrid.SideACP,
		Env: map[string]string{"FAKEAGENT_STORE": t.TempDir(), "FAKEAGENT_PERMISSION": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Info().ID
	if err := hybrids.SetPermissionMode(ctx, id, hybrid.PermissionModeInput{ClientRequestID: "push-ask-1", Mode: hybrid.PermissionAsk}); err != nil {
		t.Fatal(err)
	}
	if _, err := hybrids.Prompt(ctx, id, hybrid.PromptInput{ClientRequestID: "push-prompt-1", Text: "read it"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-server.hits:
	case <-time.After(sendTimeout + 10*time.Second):
		t.Fatal("permission was never pushed")
	}
	var message struct {
		Notification struct{ Title, Navigate string } `json:"notification"`
		WebMux       struct {
			SessionID string `json:"sessionId"`
			Kind      string `json:"kind"`
			Source    string `json:"source"`
		} `json:"webmux"`
	}
	got := server.requests()
	if err := json.Unmarshal(decryptForTest(t, got[0].body, ua, auth), &message); err != nil {
		t.Fatal(err)
	}
	if message.WebMux.SessionID != id || message.WebMux.Kind != "permission" || message.WebMux.Source != "agent" ||
		message.Notification.Title != "chat" || message.Notification.Navigate != "https://term.example.com/s/"+id {
		t.Fatalf("message %+v", message)
	}
}
