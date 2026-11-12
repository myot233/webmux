package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/forward"
	"github.com/rainchan/webmux/internal/session"
)

func TestListCarriesForwards(t *testing.T) {
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	forwards := forward.New(forward.Config{Reachable: func(context.Context, int) bool { return false }, ProbeInterval: time.Hour})
	defer forwards.Close()
	if _, err := forwards.Add(t.Context(), forward.Input{Port: 3000}); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(ended)
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		List(r.Context(), m, forwards, NewWebSocketConn(c))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	read := func() contract.SessionEvent {
		t.Helper()
		_, data, err := client.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var raw struct {
			Type     string             `json:"type"`
			Forwards []contract.Forward `json:"forwards"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		return contract.SessionEvent{Type: raw.Type, Forwards: raw.Forwards}
	}

	snapshot := read()
	if snapshot.Type != "snapshot" || len(snapshot.Forwards) != 1 || snapshot.Forwards[0].Port != 3000 {
		t.Fatalf("snapshot %+v", snapshot)
	}
	// Changes coalesce: two quick changes may arrive as one event, which
	// always carries the latest list.
	if _, err := forwards.Add(t.Context(), forward.Input{Port: 5173}); err != nil {
		t.Fatal(err)
	}
	if err := forwards.Remove(3000); err != nil {
		t.Fatal(err)
	}
	for {
		event := read()
		if event.Type != "forwards" {
			t.Fatalf("unexpected %s event", event.Type)
		}
		if len(event.Forwards) == 1 && event.Forwards[0].Port == 5173 {
			break
		}
	}

	_ = client.Close(websocket.StatusNormalClosure, "")
	select {
	case <-ended:
	case <-ctx.Done():
		t.Fatal("List did not return after the client closed")
	}
}
