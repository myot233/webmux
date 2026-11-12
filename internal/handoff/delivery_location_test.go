package handoff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
)

func TestForkUsesSelectedExistingWorktreeDirectory(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	directory := filepath.Join(h.root, "existing-target")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = directory
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	repository, _, err := h.workspace.Register(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	p, in := h.preview(conversation.PreviewLocation{RepositoryID: repository.ID, WorktreeID: repository.Worktrees[0].ID})
	if p.Request.Location.Cwd != directory {
		t.Fatalf("preview cwd = %q, want target %q", p.Request.Location.Cwd, directory)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	record, err := h.hybrids.Record(e.RuntimeID)
	if err != nil || record.Cwd != directory {
		t.Fatalf("target runtime: %+v %v", record, err)
	}
}

func TestExplicitRetryReattemptsArchivedResumeAfterTransientFailure(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	h.service.pauseAfterPhase = func(phase string) bool { return phase == "session_created" }
	first, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	h.service.workers.Wait()
	if err := h.manager.Close(first.RuntimeID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		record, err := h.hybrids.Record(first.RuntimeID)
		if err == nil && record.State == hybrid.StateClosed && record.PID == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("target did not close: %+v %v", record, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	record, err := h.hybrids.Record(first.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(h.agentStore, "sessions", record.AgentSessionID+".json")
	if err := os.Rename(native, native+".unavailable"); err != nil {
		t.Fatal(err)
	}
	h.service.pauseAfterPhase = nil
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(p.ID, "failed")
	if err := os.Rename(native+".unavailable", native); err != nil {
		t.Fatal(err)
	}
	in.ClientRequestID, in.Retry = "retry-after-native-store-restored", true
	retry, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	got := h.waitStatus(p.ID, "inline")
	if retry.ID == first.ID || got.RuntimeID != first.RuntimeID || h.promptCalls() != 1 {
		t.Fatalf("retry: first=%+v retry=%+v final=%+v", first, retry, got)
	}
}
