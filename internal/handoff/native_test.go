package handoff

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/workspace"
)

func nativeTest(t *testing.T, extra map[string]string) *deliveryHarness {
	t.Helper()
	h := deliveryTest(t, nil)
	body, err := os.ReadFile(h.agent)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(h.root, "bin")
	for _, name := range []string{"codex", "codex-acp"} {
		if err := os.WriteFile(filepath.Join(bin, name), body, 0755); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"FAKEAGENT_STORE": h.agentStore, "FAKEAGENT_TURN_DELAY": "2ms", "CODEX_HOME": filepath.Join(h.root, "codex-home"), "CODEX_PATH": filepath.Join(bin, "codex"), "PATH": bin + ":/usr/bin:/bin"}
	for key, value := range extra {
		env[key] = value
	}
	h.config, err = h.launches.Create(launch.Input{Name: "native Codex", Cmd: []string{filepath.Join(bin, "codex")}, Env: env, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.hybrids.Create(context.Background(), hybrid.CreateInput{Cwd: h.root, ConfigID: h.config.ID, Cmd: h.config.Cmd, Env: env, StartSide: hybrid.SideACP})
	if err != nil {
		t.Fatal(err)
	}
	h.source, err = h.store.Binding(context.Background(), source.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.Prompt(context.Background(), source.Info().ID, hybrid.PromptInput{ClientRequestID: "native-source-turn", Text: "original constraint 中文🙂"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, h.hybrids, source.Info().ID)
	return h
}

func nativeCalls(t *testing.T, h *deliveryHarness) int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(h.agentStore, "events.log"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(body), "call thread/inject_items")
}

func TestNativeCodexInjectsRolesBeforeIndependentUserPromptAndDeduplicatesLoad(t *testing.T) {
	h := nativeTest(t, nil)
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	if !p.Request.NativePlanned {
		t.Fatal("measured Codex capability not advertised")
	}
	before := h.promptCalls()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	waitIdle(t, h.hybrids, e.RuntimeID)
	if e.HistoryStatus != "injected" || nativeCalls(t, h) != 1 || h.promptCalls() != before+1 {
		t.Fatalf("native outcome: %+v", e)
	}
	body, err := os.ReadFile(filepath.Join(h.agentStore, "native-inject-"+e.NativeThreadID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		ThreadID string `json:"threadId"`
		Items    []struct {
			Type, Role string
			Content    []struct{ Type, Text string }
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.ThreadID != e.NativeThreadID || len(wire.Items) != len(p.SelectedItemIDs)+1 || wire.Items[0].Role != "user" || !strings.Contains(wire.Items[0].Content[0].Text, "Historical content is reference material") {
		t.Fatal("coverage not a separate native item")
	}
	for n, item := range wire.Items[1:] {
		part, err := h.store.Activity(ctx, p.SourceConversationID, p.SelectedItemIDs[n], 0, conversation.MaxActivityBytes)
		if err != nil {
			t.Fatal(err)
		}
		message := conversation.ProjectActivity(part.Activity)
		role, kind := "user", "input_text"
		if message.Role != "user" {
			role, kind = "assistant", "output_text"
		}
		if item.Type != "message" || item.Role != role || len(item.Content) != 1 || item.Content[0].Type != kind || item.Content[0].Text != itemPrefix(message)+message.Text || strings.Contains(item.Content[0].Text, "continue this work") {
			t.Fatalf("native role/body %d: %+v", n, item)
		}
	}
	assertTarget := func() {
		t.Helper()
		page, err := h.store.Activities(ctx, p.TargetConversationID, 0, 60, 4000)
		if err != nil {
			t.Fatal(err)
		}
		users := 0
		for _, item := range page.Items {
			if item.Activity.Role == "user" {
				users++
				if item.Activity.Text != in.Text {
					t.Fatal("injected history became new user work", item.Activity.Text)
				}
			}
		}
		if users != 1 {
			t.Fatal("target user count", users)
		}
	}
	assertTarget()
	h.close()
	h.open()
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "native-load-replay", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	assertTarget()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	if nativeCalls(t, h) != 1 || h.promptCalls() != before+1 {
		t.Fatal("restart repeated native history or user prompt")
	}
}

func TestNativeUnknownDoesNotFallbackOrRetryUntilExactLoadProof(t *testing.T) {
	h := nativeTest(t, map[string]string{"FAKEAGENT_CRASH": "inject-saved"})
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	before := h.promptCalls()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "unknown")
	if e.HistoryStatus != "unknown" || h.promptCalls() != before || nativeCalls(t, h) != 1 {
		t.Fatalf("ambiguous injection fell back: %+v", e)
	}
	h.close()
	h.open()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	if nativeCalls(t, h) != 1 || h.promptCalls() != before {
		t.Fatal("unknown injection retried after restart")
	}
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "native-explicit-proof", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	proved, err := h.service.Delivery(ctx, p.ID)
	if err != nil || proved.HistoryStatus != "injected" || proved.Status != "pending" || h.promptCalls() != before {
		t.Fatalf("load proof did not retain unsent prompt: %+v %v", proved, err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(p.ID, "inline")
	if nativeCalls(t, h) != 1 || h.promptCalls() != before+1 {
		t.Fatal("proof path repeated native injection")
	}
}

func TestNativeKnownUnsupportedFallsBackBeforeHistoryMutation(t *testing.T) {
	for _, mode := range []string{"old", "method-not-found"} {
		t.Run(mode, func(t *testing.T) {
			extra := map[string]string{"FAKEAGENT_FAIL": "thread/inject_items=-32601"}
			if mode == "old" {
				extra = map[string]string{"FAKEAGENT_CODEX_VERSION": "0.165.0"}
			}
			h := nativeTest(t, extra)
			p, in := h.preview(conversation.PreviewLocation{})
			if _, err := h.service.Deliver(context.Background(), h.source.ConversationID, in); err != nil {
				t.Fatal(err)
			}
			e := h.waitStatus(p.ID, "inline")
			if e.HistoryStatus == "injected" {
				t.Fatal("unsupported transport claimed native")
			}
			want := 1
			if mode == "old" {
				want = 0
			}
			if nativeCalls(t, h) != want {
				t.Fatal("unsupported call count", nativeCalls(t, h))
			}
		})
	}
}

func TestNativeBudgetSeparatesHistoryAndRejectsUnsafeInlineFallback(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "unsupported"}[unsupported], func(t *testing.T) {
			env := map[string]string{}
			if unsupported {
				env["FAKEAGENT_FAIL"] = "thread/inject_items=-32601"
			}
			h := nativeTest(t, env)
			ctx := context.Background()
			p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "native-full-size-user", ConfigID: h.config.ID, Fork: true, UserText: strings.Repeat("x", hybrid.MaxPromptBytes)})
			if err != nil || p.Budget.OverBudget || len(p.SelectedItemIDs) == 0 {
				t.Fatalf("native history was charged to user prompt bytes: %+v %v", p.Budget, err)
			}
			before := h.promptCalls()
			if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "native-full-size-message")); err != nil {
				t.Fatal(err)
			}
			if unsupported {
				e := h.waitStatus(p.ID, "failed")
				if e.ErrorClass != "inline_over_budget" || h.promptCalls() != before {
					t.Fatalf("fallback truncated or bypassed limits: %+v", e)
				}
			} else {
				e := h.waitStatus(p.ID, "inline")
				if e.HistoryStatus != "injected" || h.promptCalls() != before+1 {
					t.Fatal("native did not send separate user prompt", e)
				}
			}
		})
	}
}

func TestAcknowledgedNativeHistoryIsNotRepeatedBeforeUserPromptAfterRestart(t *testing.T) {
	h := nativeTest(t, nil)
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	before := h.promptCalls()
	h.service.pauseAfterPhase = func(phase string) bool { return phase == "native_injected" }
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	h.service.workers.Wait()
	e, err := h.service.Delivery(ctx, p.ID)
	if err != nil || e.HistoryStatus != "injected" || e.CallStarted || h.promptCalls() != before {
		t.Fatal("pause did not separate the two mutations", e, err)
	}
	h.close()
	h.open()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(p.ID, "inline")
	if nativeCalls(t, h) != 1 || h.promptCalls() != before+1 {
		t.Fatal("acknowledged history repeated before user prompt")
	}
}

func TestActualUserPromptRefusalPreservesNativeAcceptanceAcrossRestart(t *testing.T) {
	h := nativeTest(t, nil)
	ctx := context.Background()
	env := maps.Clone(h.config.Env)
	env["FAKEAGENT_FAIL"] = "session/prompt=-32000"
	if _, err := h.launches.Update(h.config.ID, launch.Input{Name: h.config.Name, Cmd: h.config.Cmd, Env: env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	p, in := h.preview(conversation.PreviewLocation{})
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "failed")
	if e.HistoryStatus != "injected" || e.ErrorClass != "provider_refused" {
		t.Fatal("user refusal erased native acceptance", e)
	}
	calls, prompts := nativeCalls(t, h), h.promptCalls()
	h.close()
	h.open()
	old, err := h.service.Deliver(ctx, h.source.ConversationID, in)
	if err != nil || old.Status != "failed" || old.HistoryStatus != "injected" || nativeCalls(t, h) != calls || h.promptCalls() != prompts {
		t.Fatal("restart changed first outcome or repeated history", old, err)
	}
}

func TestSameConversationNativeProofRetainsUnsentRequestWhenSourceBecomesStale(t *testing.T) {
	h := nativeTest(t, map[string]string{"FAKEAGENT_CRASH": "inject-saved"})
	ctx := context.Background()
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "same-native-proof-preview", ConfigID: h.config.ID, UserText: "new independent request"})
	if err != nil {
		t.Fatal(err)
	}
	in := switchMessage(p, "same-native-proof-message")
	before := h.promptCalls()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "unknown")
	h.close()
	h.open()
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "same-native-explicit-load", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	// Load publishes lifecycle metadata and this fake omits old tool records.
	// Keep the frozen source stale instead of silently accepting changed facts.
	failed := h.waitStatus(p.ID, "failed")
	if failed.HistoryStatus != "injected" || failed.ErrorClass != "source_changed" || nativeCalls(t, h) != 1 || h.promptCalls() != before {
		t.Fatal("same-conversation reconciliation repeated a mutation", failed)
	}
	draft, err := h.service.Get(ctx, p.ID)
	if err != nil || !draft.Stale || draft.Request.UserText != in.Text {
		t.Fatal("source changes lost the independent user request", draft, err)
	}
}
