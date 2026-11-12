package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/session"
)

func TestRawOutputPagesPreserveRetainedSuffix(t *testing.T) {
	cfg := config.Config{Token: "secret", DefaultCwd: t.TempDir()}
	opts := testSessionOptions(cfg)
	opts.ScrollbackBytes = 8
	manager := session.NewManager(opts)
	defer manager.Shutdown(context.Background())
	handler := New(testDeps(t, Deps{Config: cfg, Manager: manager, Workspace: testWorkspace(t, manager, cfg.DefaultCwd)}))
	item, err := manager.Create(session.CreateInput{Cmd: []string{"/bin/sh", "-c", "printf abcdefghijklmnop"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-item.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("output process did not exit")
	}
	path := "/api/sessions/" + item.Info().ID + "/output"
	authBoundary(t, handler, "GET", path, "")
	var page contract.TerminalOutput
	response := workspaceRequest(t, handler, "GET", path+"?maxBytes=3", nil, 200)
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(page.Data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ijk" || !page.HistoryTruncated || !page.Limited || page.Start != "8" || page.NextOffset != "11" {
		t.Fatalf("page %+v data=%q", page, data)
	}
	response = workspaceRequest(t, handler, "GET", path+"?generation="+page.Generation+"&offset=11&maxBytes=8", nil, 200)
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	data, err = base64.StdEncoding.DecodeString(page.Data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "lmnop" || page.Reset || page.Limited {
		t.Fatalf("resume %+v data=%q", page, data)
	}
	if item.Info().Clients != 0 {
		t.Fatal("raw read attached terminal clients")
	}
	for _, query := range []string{"?offset=0", "?maxBytes=0", "?maxBytes=65537", "?maxBytes=1&maxBytes=2", "?generation=x&offset=1", "?generation=0123456789abcdef&offset=01", "?extra=x"} {
		workspaceRequest(t, handler, "GET", path+query, nil, 400)
	}
	workspaceRequest(t, handler, "GET", "/api/sessions/aaaaaaaa/output", nil, 404)
}
