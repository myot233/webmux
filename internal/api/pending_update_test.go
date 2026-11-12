package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/session"
)

func TestConfigPendingUpdate(t *testing.T) {
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	for _, row := range []struct {
		name      string
		installed func() string
		want      *contract.PendingUpdate
	}{
		{"not wired", nil, nil},
		{"nothing installed", func() string { return "" }, nil},
		{"installed", func() string { return "0.5.0" }, &contract.PendingUpdate{InstalledVersion: "0.5.0", Launcher: contract.LauncherLaunchd}},
	} {
		t.Run(row.name, func(t *testing.T) {
			h := New(testDeps(t, Deps{Manager: m, InstalledVersion: row.installed, Launcher: contract.LauncherLaunchd}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:7681/api/config", nil))
			var got contract.ServerConfig
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
				t.Fatalf("HTTP %d %v: %s", w.Code, err, w.Body.String())
			}
			if (got.PendingUpdate == nil) != (row.want == nil) || got.PendingUpdate != nil && *got.PendingUpdate != *row.want {
				t.Fatalf("pendingUpdate = %+v, want %+v", got.PendingUpdate, row.want)
			}
		})
	}
}

func TestWireSnapshotsInstalledVersion(t *testing.T) {
	rec := &wireRecorder{t: t, norm: newWireNormalizer(), seen: map[string]bool{}}
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	local := NewLocal(testDeps(t, Deps{Manager: m}), Local{Status: func() contract.StatusV1 {
		return contract.StatusV1{V: 1, State: contract.StateRunning, Record: true, LocalAPI: true,
			Version: "0.4.0", Launcher: contract.LauncherSystemd, InstalledVersion: "0.5.0",
			Drift: []string{}, Warnings: []string{}, URLs: contract.URLs{Remote: []contract.RemoteURL{}}}
	}})
	w := httptest.NewRecorder()
	local.ServeHTTP(w, httptest.NewRequest("GET", contract.PathStatus, nil))
	rec.record("local-status-installed-version", w.Code, w.Body.Bytes())
}
