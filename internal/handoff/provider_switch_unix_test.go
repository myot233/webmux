//go:build linux || darwin

package handoff

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
	"golang.org/x/sys/unix"
)

func TestExternalCodexWriterDoesNotTriggerFreshFallback(t *testing.T) {
	h, original, _ := switchToB(t, nil)
	ctx := context.Background()
	cfg := codexLaunch(t, h)
	home := cfg.Env["CODEX_HOME"]
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

func codexLaunch(t *testing.T, h *deliveryHarness) launch.Config {
	t.Helper()
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
	cfg, err := h.launches.Create(launch.Input{Name: "Codex", Cmd: []string{filepath.Join(directory, "codex")}, Hybrid: true, Env: map[string]string{"PATH": directory + string(os.PathListSeparator) + os.Getenv("PATH"), "CODEX_PATH": filepath.Join(directory, "codex"), "CODEX_HOME": filepath.Join(h.root, "codex-home"), "FAKEAGENT_STORE": h.agentStore, "FAKEAGENT_TURN_DELAY": "2ms"}})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func claudeLaunch(t *testing.T, h *deliveryHarness) launch.Config {
	t.Helper()
	binary, err := os.ReadFile(h.agent)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(h.agent)
	for _, name := range []string{"claude", "claude-agent-acp"} {
		if err := os.WriteFile(filepath.Join(directory, name), binary, 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := h.launches.Create(launch.Input{Name: "Claude", Cmd: []string{filepath.Join(directory, "claude")}, Hybrid: true, Env: map[string]string{"PATH": directory + string(os.PathListSeparator) + os.Getenv("PATH"), "CLAUDE_CODE_EXECUTABLE": filepath.Join(directory, "claude"), "CLAUDE_CONFIG_DIR": filepath.Join(h.root, "claude-home"), "FAKEAGENT_STORE": h.agentStore, "FAKEAGENT_TURN_DELAY": "2ms"}})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func externalClaudeWriter(t *testing.T, h *deliveryHarness, cfg launch.Config, sid string) func() {
	t.Helper()
	manager := h.manager
	external, err := manager.Create(session.CreateInput{Cwd: h.root, Cmd: []string{cfg.Cmd[0], "--resume", sid}, Env: cfg.Env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close(external.Info().ID) })
	path := filepath.Join(cfg.Env["CLAUDE_CONFIG_DIR"], "sessions", strconv.Itoa(external.Identity().PID)+".json")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		body, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(body), sid) {
			break
		}
		select {
		case <-external.Done():
			t.Fatalf("external Claude exited before registration: %s", external.Buffer())
		case <-ctx.Done():
			t.Fatalf("external Claude did not register %s: %v", sid, err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	return func() {
		t.Helper()
		if err := manager.Close(external.Info().ID); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		select {
		case <-external.Done():
		case <-ctx.Done():
			t.Fatal("external Claude did not exit")
		}
	}
}

func TestProviderSwitchWithDeletedLaunchPreservesWriterChecks(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	cfg := claudeLaunch(t, h)
	home := cfg.Env["CLAUDE_CONFIG_DIR"]
	// The live actor must use its launch environment, not the service default.
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(h.root, "other-claude-home"))
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "deleted-launch-A-preview", ConfigID: cfg.ID, UserText: "A work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "deleted-launch-A-message")); err != nil {
		t.Fatal(err)
	}
	a := h.waitStatus(p.ID, "inline")
	waitIdle(t, h.hybrids, a.RuntimeID)
	original, err := h.hybrids.Record(a.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.launches.Delete(cfg.ID); err != nil {
		t.Fatal(err)
	}
	stopExternal := externalClaudeWriter(t, h, cfg, original.AgentSessionID)
	before := h.promptCalls()
	if err := h.service.quiesceProvider(ctx, a.RuntimeID); !errors.Is(err, hybrid.ErrExternalWriter) {
		t.Fatalf("deleted live launch lost external ownership: %v", err)
	}
	if rec, err := h.hybrids.Record(a.RuntimeID); err != nil || rec.State != hybrid.StateActive || h.promptCalls() != before {
		t.Fatalf("refused quiescence changed live writer: %+v %v", rec, err)
	}
	stopExternal()
	bp, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "deleted-launch-B-preview", ConfigID: h.config.ID, UserText: "B work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(bp, "deleted-launch-B-message")); err != nil {
		t.Fatal(err)
	}
	b := h.waitStatus(bp.ID, "inline")
	waitIdle(t, h.hybrids, b.RuntimeID)
	retired, err := h.hybrids.Record(a.RuntimeID)
	if err != nil || retired.State != hybrid.StateClosed || retired.PID != 0 || retired.ConfigID != original.ConfigID || retired.Agent != original.Agent || !slices.Equal(retired.Cmd, original.Cmd) || retired.AgentSessionID != original.AgentSessionID || retired.Cwd != original.Cwd {
		t.Fatalf("deleted launch retirement lost provenance: %+v %v", retired, err)
	}
	if _, err := h.manager.Get(a.RuntimeID); err == nil {
		t.Fatal("old writer shell was not removed")
	}

	// With no actor left, a deleted launch falls back to the service environment
	// but must still detect external native writers and propagate real failures.
	h.close()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	h.open()
	stopExternal = externalClaudeWriter(t, h, cfg, original.AgentSessionID)
	if err := h.service.quiesceProvider(ctx, a.RuntimeID); !errors.Is(err, hybrid.ErrExternalWriter) {
		t.Fatalf("deleted archived launch lost external ownership: %v", err)
	}
	stopExternal()
	c, err := h.launches.Create(launch.Input{Name: "C", Cmd: h.config.Cmd, Env: h.config.Env, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	cp, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "deleted-launch-C-preview", ConfigID: c.ID, UserText: "C work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(cp, "deleted-launch-C-message")); err != nil {
		t.Fatal(err)
	}
	active := h.waitStatus(cp.ID, "inline")
	waitIdle(t, h.hybrids, active.RuntimeID)
	records, err := h.hybrids.Records(false)
	if err != nil {
		t.Fatal(err)
	}
	writers := 0
	for _, rec := range records {
		if rec.State == hybrid.StateActive {
			writers++
			if rec.ID != active.RuntimeID || active.TargetConversationID != h.source.ConversationID || rec.Cwd != original.Cwd {
				t.Fatalf("unexpected active writer: %+v", rec)
			}
		}
	}
	if writers != 1 || h.promptCalls() != before+2 {
		t.Fatalf("writers=%d prompts=%d, expected %d", writers, h.promptCalls(), before+2)
	}
	h.launches.Close()
	if err := h.service.quiesceProvider(ctx, a.RuntimeID); !errors.Is(err, launch.ErrUnavailable) {
		t.Fatalf("launch storage error was suppressed: %v", err)
	}
}
