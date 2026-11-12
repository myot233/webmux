package handoff

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
)

func TestDefinitiveDeliveryFailureAllowsTerminalAuthenticationAndExplicitRetry(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	authFile := filepath.Join(t.TempDir(), "authenticated")
	env := h.config.Env
	env["FAKEAGENT_AUTH_FILE"] = authFile
	if _, err := h.launches.Update(h.config.ID, launch.Input{Name: h.config.Name, Cmd: h.config.Cmd, Env: env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	p, in := h.preview(conversation.PreviewLocation{})
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	failed := h.waitStatus(p.ID, "failed")
	if failed.ErrorClass != "provider_refused" {
		t.Fatalf("not a definitive refusal: %+v", failed)
	}
	waitIdle(t, h.hybrids, failed.RuntimeID)
	original, err := h.hybrids.Record(failed.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.Prompt(ctx, failed.RuntimeID, hybrid.PromptInput{ClientRequestID: "ordinary-before-auth", Text: "must not replace failed handoff"}); !errors.Is(err, hybrid.ErrSwitching) {
		t.Fatalf("ordinary dispatch bypassed explicit retry: %v", err)
	}
	if _, err := h.hybrids.Switch(ctx, failed.RuntimeID, hybrid.SwitchInput{ClientRequestID: "failed-to-terminal", To: hybrid.SideTUI}); err != nil {
		t.Fatalf("definite failure prevented recovery: %v", err)
	}
	if _, err := h.hybrids.Input(ctx, failed.RuntimeID, hybrid.InputRequest{ClientRequestID: "authenticate-in-terminal", Text: "/login", Submit: true}); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(authFile); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal authentication did not complete")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := h.hybrids.Switch(ctx, failed.RuntimeID, hybrid.SwitchInput{ClientRequestID: "authenticated-to-chat", To: hybrid.SideACP}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.Prompt(ctx, failed.RuntimeID, hybrid.PromptInput{ClientRequestID: "ordinary-after-auth", Text: "must still use explicit retry"}); !errors.Is(err, hybrid.ErrSwitching) {
		t.Fatalf("recovery released ordinary dispatch gate: %v", err)
	}
	if h.promptCalls() != 1 {
		t.Fatalf("recovery automatically resent handoff: %d calls", h.promptCalls())
	}
	in.ClientRequestID, in.Retry = "explicit-after-auth", true
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	retried := h.waitStatus(p.ID, "inline")
	waitIdle(t, h.hybrids, retried.RuntimeID)
	restored, err := h.hybrids.Record(retried.RuntimeID)
	if err != nil || retried.ID == failed.ID || retried.RuntimeID != failed.RuntimeID || restored.AgentSessionID != original.AgentSessionID || h.promptCalls() != 2 {
		t.Fatalf("incorrect retry: failed=%+v retried=%+v record=%+v calls=%d error=%v", failed, retried, restored, h.promptCalls(), err)
	}
	first, err := h.service.options.Conversations.Execution(ctx, failed.ID)
	if err != nil || first.Status != "failed" || first.ErrorClass != "provider_refused" {
		t.Fatalf("retry rewrote refusal: %+v %v", first, err)
	}
}

func TestUnresolvedDeliveryCannotEnterTerminalRecovery(t *testing.T) {
	for _, status := range []string{"pending", "unknown"} {
		t.Run(status, func(t *testing.T) {
			var env map[string]string
			if status == "unknown" {
				env = map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"}
			}
			h := deliveryTest(t, env)
			if status == "pending" {
				h.service.pauseAfterPhase = func(phase string) bool { return phase == "session_created" }
			}
			ctx := context.Background()
			p, in := h.preview(conversation.PreviewLocation{})
			if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
				t.Fatal(err)
			}
			h.service.workers.Wait()
			e := h.waitStatus(p.ID, status)
			before := h.promptCalls()
			if _, err := h.hybrids.Switch(ctx, e.RuntimeID, hybrid.SwitchInput{ClientRequestID: "unresolved-to-terminal", To: hybrid.SideTUI}); !errors.Is(err, hybrid.ErrSwitching) {
				t.Fatalf("%s delivery entered unrestricted terminal: %v", status, err)
			}
			if _, err := h.hybrids.Prompt(ctx, e.RuntimeID, hybrid.PromptInput{ClientRequestID: "unresolved-ordinary-prompt", Text: "must remain blocked"}); !errors.Is(err, hybrid.ErrSwitching) {
				t.Fatalf("%s delivery admitted ordinary prompt: %v", status, err)
			}
			if h.promptCalls() != before {
				t.Fatal("blocked recovery dispatched a prompt")
			}
		})
	}
}
