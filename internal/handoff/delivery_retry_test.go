package handoff

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

func refusedHandoff(t *testing.T, fork bool, extra map[string]string) (*deliveryHarness, conversation.Preview, MessageRequest, conversation.Execution, string) {
	t.Helper()
	authFile := filepath.Join(t.TempDir(), "authenticated")
	env := maps.Clone(extra)
	if env == nil {
		env = map[string]string{}
	}
	env["FAKEAGENT_AUTH_FILE"] = authFile
	h := deliveryTest(t, env)
	ctx := context.Background()
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "refused-preview", ConfigID: h.config.ID, UserText: "continue after authentication", Fork: fork})
	if err != nil {
		t.Fatal(err)
	}
	in := switchMessage(p, "refused-initial-delivery")
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "failed")
	if e.ErrorClass != "provider_refused" {
		t.Fatalf("expected definitive refusal: %+v", e)
	}
	waitIdle(t, h.hybrids, e.RuntimeID)
	return h, p, in, e, authFile
}

func TestForkRetryInstallsOwnerBeforeConfigurationAndDispatch(t *testing.T) {
	for _, status := range []string{"pending", "unknown"} {
		t.Run(status, func(t *testing.T) {
			var env map[string]string
			if status == "unknown" {
				env = map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"}
			}
			h, p, in, first, authFile := refusedHandoff(t, true, env)
			ctx := context.Background()
			if err := os.WriteFile(authFile, []byte("authenticated"), 0o600); err != nil {
				t.Fatal(err)
			}
			if status == "pending" {
				h.service.pauseAfterPhase = func(phase string) bool { return phase == "session_created" }
			}
			in.ClientRequestID, in.Retry = "refused-explicit-retry", true
			if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
				t.Fatal(err)
			}
			h.service.workers.Wait()
			e := h.waitStatus(p.ID, status)
			if e.ID == first.ID || e.RuntimeID != first.RuntimeID {
				t.Fatalf("retry did not retain runtime: first=%+v retry=%+v", first, e)
			}
			before := h.promptCalls()
			if _, err := h.hybrids.Prompt(ctx, e.RuntimeID, hybrid.PromptInput{ClientRequestID: "retry-blocks-ordinary", Text: "must remain gated"}); !errors.Is(err, hybrid.ErrSwitching) {
				t.Fatalf("%s retry admitted ordinary dispatch: %v", status, err)
			}
			if _, err := h.hybrids.Switch(ctx, e.RuntimeID, hybrid.SwitchInput{ClientRequestID: "retry-blocks-terminal", To: hybrid.SideTUI}); !errors.Is(err, hybrid.ErrSwitching) {
				t.Fatalf("%s retry admitted terminal: %v", status, err)
			}
			if h.promptCalls() != before {
				t.Fatal("blocked operation dispatched another provider prompt")
			}
		})
	}
}

func TestFailedDeliveryOwnerSurvivesArchiveAndRestart(t *testing.T) {
	for _, mode := range []string{"archive-chat", "archive-terminal", "restart"} {
		t.Run(mode, func(t *testing.T) {
			h, _, _, failed, _ := refusedHandoff(t, true, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if mode == "restart" {
				h.close()
				h.open()
			} else {
				if err := h.manager.Close(failed.RuntimeID); err != nil {
					t.Fatal(err)
				}
				for {
					rec, err := h.hybrids.Record(failed.RuntimeID)
					_, shellErr := h.manager.Get(failed.RuntimeID)
					if err == nil && rec.State == hybrid.StateClosed && errors.Is(shellErr, session.ErrNotFound) {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("failed target did not archive")
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			side := "acp"
			if mode == "archive-terminal" {
				side = "tui"
			}
			if _, err := h.workspace.ResumeHybrid(ctx, failed.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "resume-failed-owner", Side: side}); err != nil {
				t.Fatal(err)
			}
			if side == "tui" {
				if _, err := h.hybrids.Switch(ctx, failed.RuntimeID, hybrid.SwitchInput{ClientRequestID: "restored-owner-to-chat", To: hybrid.SideACP}); err != nil {
					t.Fatal(err)
				}
			}
			before := h.promptCalls()
			if _, err := h.hybrids.Prompt(ctx, failed.RuntimeID, hybrid.PromptInput{ClientRequestID: "restored-owner-blocks-prompt", Text: "must use explicit retry"}); !errors.Is(err, hybrid.ErrSwitching) {
				t.Fatalf("failed owner lost after %s: %v", mode, err)
			}
			if _, err := h.hybrids.Switch(ctx, failed.RuntimeID, hybrid.SwitchInput{ClientRequestID: "restored-owner-allows-recovery", To: hybrid.SideTUI}); err != nil {
				t.Fatalf("failed owner blocked explicit recovery after %s: %v", mode, err)
			}
			if h.promptCalls() != before {
				t.Fatal("resume/recovery automatically resent failed delivery")
			}
		})
	}
}

func TestSameConversationRefusalRetriesOnlyItsOwnAttemptHistory(t *testing.T) {
	for _, unrelated := range []bool{false, true} {
		t.Run(map[bool]string{false: "own-attempt", true: "unrelated-run"}[unrelated], func(t *testing.T) {
			h, p, in, failed, authFile := refusedHandoff(t, false, nil)
			ctx := context.Background()
			if err := os.WriteFile(authFile, []byte("external credential recovery"), 0o600); err != nil {
				t.Fatal(err)
			}
			if unrelated {
				if _, err := h.store.StartRun(ctx, h.source.ProviderThreadID, "unrelated-source-run"); err != nil {
					t.Fatal(err)
				}
			}
			in.ClientRequestID, in.Retry = "same-conversation-explicit-retry", true
			_, err := h.service.Deliver(ctx, h.source.ConversationID, in)
			if unrelated {
				if !errors.Is(err, conversation.ErrConflict) {
					t.Fatalf("unrelated source change admitted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("failed attempt's own history prevented retry: %v", err)
			}
			retried := h.waitStatus(p.ID, "inline")
			waitIdle(t, h.hybrids, retried.RuntimeID)
			frozen, err := h.store.Preview(ctx, p.ID)
			if err != nil || retried.RuntimeID != failed.RuntimeID || frozen.RenderedText != p.RenderedText || frozen.SourcePosition != p.SourcePosition || frozen.Revision != p.Revision {
				t.Fatalf("retry changed reviewed context/runtime: execution=%+v preview=%+v error=%v", retried, frozen, err)
			}
		})
	}
}
