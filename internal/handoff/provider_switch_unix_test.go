//go:build linux || darwin

package handoff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rainchan/webmux/internal/launch"
	"golang.org/x/sys/unix"
)

func TestExternalCodexWriterDoesNotTriggerFreshFallback(t *testing.T) {
	h, original, _ := switchToB(t, nil)
	ctx := context.Background()
	binary, err := os.ReadFile(h.agent)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(h.agent)
	for _, name := range []string{"codex", "codex-acp"} {
		if err := os.WriteFile(filepath.Join(directory, name), binary, 0700); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(h.root, "codex-home")
	cfg, err := h.launches.Create(launch.Input{Name: "Codex", Cmd: []string{filepath.Join(directory, "codex")}, Hybrid: true, Env: map[string]string{"PATH": directory + string(os.PathListSeparator) + os.Getenv("PATH"), "CODEX_PATH": filepath.Join(directory, "codex"), "CODEX_HOME": home, "FAKEAGENT_STORE": h.agentStore, "FAKEAGENT_TURN_DELAY": "2ms"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "preview-Codex", ConfigID: cfg.ID, UserText: "Codex work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "message-Codex")); err != nil {
		t.Fatal(err)
	}
	codex := h.waitStatus(p.ID, "inline")
	waitIdle(t, h.hybrids, codex.RuntimeID)
	record, err := h.hybrids.Record(codex.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	back, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "preview-original", ConfigID: original.ConfigID, UserText: "OMP work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(back, "message-original")); err != nil {
		t.Fatal(err)
	}
	active := h.waitStatus(back.ID, "inline")
	waitIdle(t, h.hybrids, active.RuntimeID)
	if err := os.MkdirAll(filepath.Join(home, "thread-writer-locks"), 0700); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(home, "thread-writer-locks", record.AgentSessionID+".lock"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Flock(int(lock.Fd()), unix.LOCK_UN) }()
	target, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "preview-held-Codex", ConfigID: cfg.ID, UserText: "return Codex"})
	if err != nil {
		t.Fatal(err)
	}
	before := h.promptCalls()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(target, "message-held-Codex")); err != nil {
		t.Fatal(err)
	}
	failed := h.waitStatus(target.ID, "failed")
	if failed.ErrorClass != "external_writer" || failed.ReplacementHandoffID != "" || h.promptCalls() != before {
		t.Fatalf("forced external writer: %+v", failed)
	}
}
