package api

import (
	"bytes"
	"context"
	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/session"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReplayRetainsNewestBytesAfterBufferWrap(t *testing.T) {
	presets := []session.Preset{{Name: "output", Cmd: []string{"/bin/bash", "-c", "printf abcde"}}}
	m := session.NewManager(session.Options{ScrollbackBytes: 4, DefaultCwd: t.TempDir(), Presets: presets})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer m.Shutdown(ctx)
	s, err := m.Create("", "output", nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-ctx.Done():
		t.Fatal("output process did not exit")
	}
	server := httptest.NewServer(New(config.Config{}, m, testWorkspace(t, m, t.TempDir())))
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/s/"+s.Info().ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	kind, p, err := conn.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || !bytes.Equal(p, []byte{1, 'b', 'c', 'd', 'e'}) {
		t.Fatalf("wrapped replay: kind=%v bytes=%v err=%v", kind, p, err)
	}
}
