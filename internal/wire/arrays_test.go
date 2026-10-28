package wire

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/tailscale"
	"github.com/rainchan/webmux/internal/workspace"
)

var updateArrays = flag.Bool("update-arrays", false, "re-record the response array fixture consumed by frontend Zod tests")

var chatTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// This fixture exercises real Go conversions with nil service arrays and is
// parsed by web/test/wireArrays.test.ts. Keep scalar validation constraints
// intact: command arrays still need a command, and nullable fields stay null.
func TestResponseArrayFixture(t *testing.T) {
	values := map[string]any{
		"sessions":     Sessions(nil),
		"snapshot":     SessionEvent(session.Event{Type: "snapshot"}),
		"session":      Session(session.Info{ID: "abcdefgh", Generation: "0123456789abcdef", Cmd: []string{"sh"}, Status: session.StatusRunning, Cols: 80, Rows: 24}),
		"repository":   Repository(workspace.Repository{Availability: "unavailable"}),
		"repositories": Repositories(nil),
		"branches":     Branches(workspace.Branches{Branches: []workspace.Branch{{Name: "main"}}}),
		"preview":      Preview(workspace.Preview{Worktree: workspace.Worktree{Kind: "main"}}),
		"launch":       LaunchSummaries([]launch.Summary{{ID: "abcdefgh", Name: "Shell", Cmd: []string{"sh"}}}),
		"launchConfig": LaunchConfig(launch.Config{ID: "abcdefgh", Name: "Shell", Cmd: []string{"sh"}}),
		"check":        CodeCheck(codeview.Check{Changes: "unchanged"}),
		"launches":     LaunchSummaries(nil),
		"push":         PushTestReport(push.TestReport{}),
		"listing":      CodeListing(codeview.Listing{}),
		"changes":      CodeChanges(codeview.Changes{}),
		"tailscale":    TailscaleStatus(tailscale.Status{Provider: "none", ConfiguredProvider: "auto"}),
		"chatSnapshot": ChatSnapshot(hybrid.Snapshot{State: hybrid.ChatState{Side: hybrid.SideACP, Agent: "omp", Plan: &hybrid.Plan{}}, Generation: "0123456789abcdef", From: 1, Next: 1}),
		"chatEvents": ChatEvents("0123456789abcdef", []hybrid.Event{
			{Seq: 1, Kind: hybrid.EventCommands, At: chatTime},
			{Seq: 2, Kind: hybrid.EventConfig, At: chatTime, Config: []hybrid.ConfigOption{{ID: "model", Name: "Model", Type: "select"}}},
			{Seq: 3, Kind: hybrid.EventRequest, At: chatTime, Request: &hybrid.Request{RequestID: "r1", Kind: hybrid.RequestForm, Form: &hybrid.Form{Message: "?"}}},
		}),
		"hybridRecords": HybridRecords([]hybrid.Record{{
			ID: "abcdefgh", Name: "omp", ConfigID: "abcdefgh", Agent: "omp", Cwd: "/w", Cmd: []string{"omp"},
			Side: hybrid.SideTUI, State: hybrid.StateClosed, CreatedAt: chatTime, UpdatedAt: chatTime,
		}}, func(string) bool { return false }),
	}
	got, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "response-arrays.json")
	if *updateArrays {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("response array fixture changed; review with Zod schemas, then go test ./internal/wire -run TestResponseArrayFixture -update-arrays")
	}
}
