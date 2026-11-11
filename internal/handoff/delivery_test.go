package handoff

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

type deliveryHarness struct {
	t                              *testing.T
	root, state, agentStore, agent string
	service                        *Service
	store                          *conversation.Service
	launches                       *launch.Service
	hybrids                        *hybrid.Service
	workspace                      *workspace.Service
	manager                        *session.Manager
	folders                        *artifacts.Service
	source                         conversation.Binding
	config                         launch.Config
}

func deliveryTest(t *testing.T, env map[string]string) *deliveryHarness {
	t.Helper()
	if testing.Short() {
		t.Skip("runs isolated fake agent")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &deliveryHarness{t: t, root: root, state: filepath.Join(root, "state"), agentStore: filepath.Join(root, "agent-store"), agent: filepath.Join(root, "bin", "omp")}
	cmd := exec.Command("go", "build", "-o", h.agent, "../acp/testdata/fakeagent")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake agent: %s %v", out, err)
	}
	h.open()
	environment := map[string]string{"FAKEAGENT_STORE": h.agentStore, "FAKEAGENT_TURN_DELAY": "2ms"}
	for key, value := range env {
		environment[key] = value
	}
	h.config, err = h.launches.Create(launch.Input{Name: "target", Cmd: []string{h.agent}, Env: environment, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	sourceConfig, err := h.launches.Create(launch.Input{Name: "source", Cmd: []string{h.agent}, Env: map[string]string{"FAKEAGENT_STORE": h.agentStore, "FAKEAGENT_TURN_DELAY": "2ms"}, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.hybrids.Create(context.Background(), hybrid.CreateInput{Cwd: h.root, ConfigID: sourceConfig.ID, Cmd: sourceConfig.Cmd, Env: sourceConfig.Env, StartSide: hybrid.SideACP})
	if err != nil {
		t.Fatal(err)
	}
	h.source, err = h.store.Binding(context.Background(), source.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.store.StartRun(context.Background(), h.source.ProviderThreadID, "source-prompt")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ role, text string }{{"user", "original constraint 中文🙂"}, {"agent", "partial result"}} {
		_, err = h.store.WriteActivity(context.Background(), conversation.ActivityInput{ProviderThreadID: h.source.ProviderThreadID, RunID: run.ID, LocalKey: entry.role, Kind: "message", Role: entry.role, Status: "completed", Source: "local", Text: entry.text})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := h.store.FinishRun(context.Background(), run.ID, "cancelled"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.close)
	return h
}

func (h *deliveryHarness) open() {
	h.t.Helper()
	h.store = conversation.Open(conversation.Options{StateDir: h.state})
	if err := h.store.Unavailable(); err != nil {
		h.t.Fatal(err)
	}
	h.launches = launch.Open(h.state)
	h.manager = session.NewManager(session.Options{DefaultCwd: h.root})
	h.folders = artifacts.New(artifacts.Options{StateDir: h.state, TempDir: h.root, Manager: h.manager})
	h.hybrids = hybrid.New(hybrid.Options{StateDir: h.state, Manager: h.manager, Conversations: h.store, Uploads: h.folders.UploadPath})
	h.folders.Start(func(id string) bool { _, err := h.hybrids.Record(id); return err == nil })
	h.workspace = workspace.New(h.state, h.manager, h.launches, h.root)
	h.workspace.UseHybrid(h.hybrids)
	h.workspace.UseArtifacts(h.folders)
	h.service = New(Options{Artifacts: h.folders, Conversations: h.store, Launch: h.launches, Hybrid: h.hybrids, Workspace: h.workspace, TokenCap: 16000, MaxPromptBytes: hybrid.MaxPromptBytes, MaxPromptWireBytes: hybrid.MaxPromptWireBytes})
}

func (h *deliveryHarness) close() {
	if h.store == nil {
		return
	}
	h.service.Close()
	h.hybrids.Close()
	h.folders.Close()
	h.workspace.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.manager.Shutdown(ctx)
	h.launches.Close()
	if err := h.store.Close(); err != nil {
		h.t.Error(err)
	}
	h.store = nil
}

func (h *deliveryHarness) preview(location conversation.PreviewLocation) (conversation.Preview, MessageRequest) {
	h.t.Helper()
	p, err := h.service.Preview(context.Background(), h.source.ConversationID, Request{ClientRequestID: "preview-command", ConfigID: h.config.ID, Fork: true, UserText: "continue this work", Location: location})
	if err != nil {
		h.t.Fatal(err)
	}
	return p, MessageRequest{ClientRequestID: "message-command", HandoffID: p.ID, Revision: p.Revision, Text: p.Request.UserText, ConfigID: p.Request.ConfigID, AgentConfig: p.Request.AgentConfig, Location: p.Request.Location, Attachments: p.Request.Attachments}
}

func (h *deliveryHarness) waitStatus(id, status string) conversation.Execution {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		e, err := h.service.Delivery(context.Background(), id)
		if err == nil && e.Status == status {
			return e
		}
		if err != nil || (e.Status != "pending" && status != "inline") {
			h.t.Fatalf("delivery: %+v %v", e, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	e, err := h.service.Delivery(context.Background(), id)
	h.t.Fatalf("waiting %s: %+v %v", status, e, err)
	return e
}

func (h *deliveryHarness) promptCalls() int {
	h.t.Helper()
	body, err := os.ReadFile(filepath.Join(h.agentStore, "events.log"))
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.Count(string(body), "call session/prompt")
}

func TestInlineDeliveryUsesOneRuntimePromptAndOriginalVisibleUserText(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	first, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sub, err := h.hybrids.Subscribe(e.RuntimeID, nil)
		if err == nil {
			running := sub.Snapshot.State.Turn.Running
			sub.Close()
			if !running {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	binding, err := h.store.Binding(ctx, e.RuntimeID)
	if err != nil || binding.ConversationID != p.TargetConversationID || binding.ProviderThreadID != p.TargetProviderThreadID {
		t.Fatalf("identity: %+v %v", binding, err)
	}
	history, err := h.store.Activities(ctx, p.TargetConversationID, 0, 100, 2000)
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, part := range history.Items {
		if part.Activity.Role == "user" {
			users++
			if part.Activity.Text != "continue this work" {
				t.Fatal("history leaked inline envelope")
			}
		}
	}
	if users != 1 || h.promptCalls() != 1 {
		t.Fatalf("users=%d prompts=%d", users, h.promptCalls())
	}
	h.close()
	h.open()
	again, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil || again.ID != first.ID || again.RuntimeID != e.RuntimeID || again.Status != "inline" || h.promptCalls() != 1 {
		t.Fatalf("restart duplicate: %+v %v", again, err)
	}
	in.Text = "changed"
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); !errors.Is(err, conversation.ErrConflict) {
		t.Fatal("different payload accepted", err)
	}
}

func TestDeliveryCrashRecoveryNeverRepeatsResourcesOrUnknownPrompt(t *testing.T) {
	for _, phase := range []string{"receipt", "worktree_created", "session_created", "before_prompt", "after_prompt"} {
		t.Run(phase, func(t *testing.T) {
			h := deliveryTest(t, nil)
			ctx := context.Background()
			location := conversation.PreviewLocation{}
			if phase == "worktree_created" {
				repoPath := filepath.Join(h.root, "repo")
				if err := os.Mkdir(repoPath, 0700); err != nil {
					t.Fatal(err)
				}
				for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial"}} {
					cmd := exec.Command("git", args...)
					cmd.Dir = repoPath
					cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("git: %s %v", out, err)
					}
				}
				repo, _, err := h.workspace.Register(ctx, repoPath)
				if err != nil {
					t.Fatal(err)
				}
				location = conversation.PreviewLocation{RepositoryID: repo.ID, NewWorktreeBase: "main", NewWorktreeBranch: "handoff-test"}
			}
			p, in := h.preview(location)
			h.service.pauseAfterPhase = func(got string) bool { return got == phase }
			e, err := h.service.Deliver(ctx, h.source.ConversationID, in)
			if err != nil {
				t.Fatal(err)
			}
			h.service.workers.Wait()
			h.close()
			h.open()
			again, err := h.service.Deliver(ctx, h.source.ConversationID, in)
			if err != nil {
				t.Fatal(err)
			}
			if again.ID != e.ID || again.RuntimeID != e.RuntimeID {
				t.Fatal("new resource reservation")
			}
			h.service.workers.Wait()
			if phase == "after_prompt" {
				got, err := h.service.Delivery(ctx, p.ID)
				if err != nil || (got.Status != "unknown" && got.Status != "inline") || h.promptCalls() > 1 {
					t.Fatalf("ambiguous intent resent: %+v %v", got, err)
				}
			} else {
				h.waitStatus(p.ID, "inline")
				if h.promptCalls() != 1 {
					t.Fatalf("prompt count=%d", h.promptCalls())
				}
				ids, err := h.store.RuntimeIDs(ctx, p.TargetConversationID)
				if err != nil || len(ids) != 1 || ids[0] != e.RuntimeID {
					t.Fatalf("runtime duplicated: %v %v", ids, err)
				}
			}
		})
	}
}

func TestProviderAuthRefusalDoesNotClaimConsumption(t *testing.T) {
	h := deliveryTest(t, map[string]string{"FAKEAGENT_FAIL": "session/prompt=-32000"})
	p, in := h.preview(conversation.PreviewLocation{})
	if _, err := h.service.Deliver(context.Background(), h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "failed")
	if e.ErrorClass != "provider_refused" {
		t.Fatalf("refusal: %+v", e)
	}
	if calls := h.promptCalls(); calls != 1 {
		t.Fatal(calls)
	}
}

func TestUnknownInlineConsumptionReconcilesOnlyFromNativeLoad(t *testing.T) {
	h := deliveryTest(t, map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"})
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(p.ID, "unknown")
	h.close()
	h.open()
	e, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil || e.Status != "unknown" || h.promptCalls() != 1 {
		t.Fatalf("unknown resent: %+v %v", e, err)
	}
	cfg := h.config
	delete(cfg.Env, "FAKEAGENT_FAIL")
	if _, err := h.launches.Update(cfg.ID, launch.Input{Name: cfg.Name, Cmd: cfg.Cmd, Env: cfg.Env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "explicit-reconcile", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	got := h.waitStatus(p.ID, "inline")
	if got.ID != e.ID || h.promptCalls() != 1 {
		t.Fatal("load reconciliation sent a prompt")
	}
	// Provider really saved the full wire prompt; WebMux exposes only the new
	// user text after validating the exact native identity and payload hash.
	record, err := h.hybrids.Record(e.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(h.agentStore, "sessions", record.AgentSessionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Messages []struct{ Role, Text string } }
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Messages) != 1 || !strings.Contains(data.Messages[0].Text, "original constraint 中文🙂") {
		t.Fatal("provider did not receive history")
	}
}

func TestExplicitRetryReusesWorktreeCreatedBeforeFailure(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	repoPath := filepath.Join(h.root, "retry-repo")
	if err := os.Mkdir(repoPath, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	repo, _, err := h.workspace.Register(ctx, repoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, in := h.preview(conversation.PreviewLocation{RepositoryID: repo.ID, NewWorktreeBase: "main", NewWorktreeBranch: "retry-tree"})
	h.service.pauseAfterPhase = func(phase string) bool { return phase == "worktree_created" }
	first, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	h.service.workers.Wait()
	if err := h.store.FinishExecution(ctx, first.ID, "failed", "test_preparation_failure"); err != nil {
		t.Fatal(err)
	}
	h.service.pauseAfterPhase = nil
	in.ClientRequestID = "explicit-retry"
	in.Retry = true
	retry, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	got := h.waitStatus(p.ID, "inline")
	if got.RuntimeID != first.RuntimeID || retry.ID == first.ID || h.promptCalls() != 1 {
		t.Fatalf("retry resources: %+v", got)
	}
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil || strings.Count(string(out), "worktree ") != 2 {
		t.Fatalf("duplicate worktrees: %s %v", out, err)
	}
}

func TestForgottenSourceUnknownStaysBlockedUntilActualTargetReplay(t *testing.T) {
	h := deliveryTest(t, map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"})
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "unknown")
	sourceIDs, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.hybrids.Forget(ctx, sourceIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Execution(ctx, e.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatal("source operation retained", err)
	}
	h.close()
	h.open()
	if _, held, err := h.store.RuntimeDispatch(ctx, e.RuntimeID); err != nil || !held {
		t.Fatal("unknown bypassed on restart", held, err)
	}
	cfg := h.config
	delete(cfg.Env, "FAKEAGENT_FAIL")
	if _, err := h.launches.Update(cfg.ID, launch.Input{Name: cfg.Name, Cmd: cfg.Cmd, Env: cfg.Env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "forgotten-source-proof", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	gate, held, err := h.store.RuntimeDispatch(ctx, e.RuntimeID)
	if err != nil || held || gate.Status != "inline" || h.promptCalls() != 1 {
		t.Fatal("target replay failed or sent again", gate, held, err)
	}
	if _, err := h.store.Execution(ctx, e.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatal("proof recreated source operation", err)
	}
	if _, err := h.hybrids.Prompt(ctx, e.RuntimeID, hybrid.PromptInput{ClientRequestID: "next-after-proof", Text: "new explicit task"}); err != nil {
		t.Fatal("exact proof did not release live actor", err)
	}
	waitIdle(t, h.hybrids, e.RuntimeID)
}
