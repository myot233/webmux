package session

import (
	"regexp"
	"testing"
	"time"
)

func TestStatusBoundaries(t *testing.T) {
	now := time.Now()
	patterns := []*regexp.Regexp{regexp.MustCompile(`[$?]$`)}
	for _, row := range []struct {
		age             time.Duration
		line            string
		exited, blocked bool
		want            Status
	}{{time.Second, "", false, false, StatusRunning}, {2 * time.Second, "", false, false, StatusWaiting}, {31 * time.Second, "done", false, false, StatusIdle}, {31 * time.Second, "Continue?", false, false, StatusWaiting}, {31 * time.Second, "", false, true, StatusWaiting}, {time.Second, "$", true, true, StatusExited}} {
		got := classify(now, now.Add(-row.age), row.line, row.exited, row.blocked, 30*time.Second, patterns)
		if got != row.want {
			t.Fatalf("%+v got %s", row, got)
		}
	}
}
func TestPreviewSplitUnicodeAndANSI(t *testing.T) {
	var p Preview
	raw := []byte("\x1b[32m中文预览\x1b[0m\r\x1b]2;hidden title\aContinue? ")
	for _, b := range raw {
		p.Feed([]byte{b})
	}
	if got := p.last; got != "Continue?" {
		t.Fatal(got)
	}
}

func TestWorkspaceMetadataParticipatesInDisplayComparison(t *testing.T) {
	a := Info{}
	b := Info{Workspace: &Workspace{RepositoryID: "repository", WorktreeID: "tree", WorktreePath: "/tree"}}
	if sameDisplay(a, b) {
		t.Fatal("association change was invisible")
	}
	a.Workspace = &Workspace{RepositoryID: "repository", WorktreeID: "tree", WorktreePath: "/tree"}
	if !sameDisplay(a, b) {
		t.Fatal("equal workspace values were compared by pointer")
	}
	b.Workspace.WorktreePath = "/moved"
	if sameDisplay(a, b) {
		t.Fatal("workspace path change was invisible")
	}
}
