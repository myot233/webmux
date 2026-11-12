package mcpcontrol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/localclient"
)

func TestActionsForwardStableReceipts(t *testing.T) {
	var bodies []string
	c := testClient(t, func(r *http.Request) string {
		if r.Method == http.MethodPost {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			bodies = append(bodies, string(data))
			if !strings.Contains(string(data), `"clientRequestId":"stable-request"`) {
				t.Fatal("request id lost")
			}
		}
		switch r.URL.Path {
		case "/api/sessions":
			return `{"session":{"id":"created","view":"chat"},"configWarnings":[],"initialInput":{"view":"chat","turnId":"first-turn"}}`
		case "/api/sessions/created/details":
			return `{"link":"http://localhost:7681/s/created"}`
		case "/api/sessions/created/input":
			return `{"view":"chat","queued":true,"itemId":"queue-one","position":2}`
		case "/api/sessions/created/interrupt":
			return ""
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return ""
		}
	})
	raw := json.RawMessage(`{"clientRequestId":"stable-request","configId":"config","view":"chat","prompt":"task","newWorktree":{"repositoryId":"repo","mode":"new","base":"main"}}`)
	result, err := c.launch(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"turnId":"first-turn"`) || !strings.Contains(string(data), `http://localhost:7681/s/created`) {
		t.Fatalf("launch receipt %s", data)
	}
	if _, err := c.launch(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if bodies[0] != bodies[1] {
		t.Fatal("retry changed request")
	}
	result, err = c.send(context.Background(), json.RawMessage(`{"sessionId":"created","request":{"clientRequestId":"stable-request","text":"next","mode":"auto"}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"itemId":"queue-one"`) || !strings.Contains(string(data), `"position":2`) {
		t.Fatalf("send receipt %s", data)
	}
	if _, err := c.interrupt(context.Background(), json.RawMessage(`{"sessionId":"created","clientRequestId":"stable-request"}`)); err != nil {
		t.Fatal(err)
	}
}
func TestActionsRejectMissingIdentity(t *testing.T) {
	c := testClient(t, func(*http.Request) string { t.Fatal("invalid mutation reached API"); return "" })
	for _, call := range []func(context.Context, json.RawMessage) (any, error){c.launch, c.send, c.interrupt} {
		for _, raw := range []string{`{}`, `null`, `{"sessionId":"../token","clientRequestId":"stable-request"}`, `{"clientRequestId":"short"}`} {
			if _, err := call(context.Background(), json.RawMessage(raw)); err == nil {
				t.Fatalf("accepted %s", raw)
			}
		}
	}
}
func TestLaunchPartialFailurePreservesCreatedResources(t *testing.T) {
	c := Client{API: &localclient.Client{HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 409, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"prompt failed","step":"prompt","sessionId":"created","worktree":{"id":"tree","path":"/work/tree"}}`))}, nil
	})}}}
	_, err := c.launch(context.Background(), json.RawMessage(`{"clientRequestId":"stable-request","cmd":["bash"]}`))
	if err == nil || !strings.Contains(err.Error(), `"sessionId":"created"`) || !strings.Contains(err.Error(), `"step":"prompt"`) {
		t.Fatalf("partial failure lost: %v", err)
	}
}

func TestLaunchDoesNotInventInitialTurn(t *testing.T) {
	c := testClient(t, func(r *http.Request) string {
		if r.Method == "POST" {
			return `{"session":{"id":"idle","view":"chat"},"configWarnings":[]}`
		}
		return `{"session":{"id":"idle"}}`
	})
	for _, raw := range []string{`{"clientRequestId":"empty-request","configId":"config","view":"chat","prompt":"  "}`, `{"clientRequestId":"empty-request","configId":"config","view":"chat","useInitInstruction":true,"workspace":{"repositoryId":"repo","worktreeId":"tree"}}`} {
		value, err := c.launch(context.Background(), json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := value.(map[string]any)["wait"]; exists {
			t.Fatal("unsubmitted prompt returned a wait selector")
		}
	}
}
