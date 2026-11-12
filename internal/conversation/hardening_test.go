package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSourceForgetRetainsTargetUnknownGateAndInlineOccupancy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	text := "historical packet\nUser message:\noriginal"
	e.Phase = "sending"
	e.NativeThreadID = "native"
	e.PromptHash = TextHash(text)
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("source operation survived", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	gate, held, err := s.RuntimeDispatch(ctx, e.RuntimeID)
	if err != nil || !held || gate.ID != e.ID || gate.Status != "unknown" {
		t.Fatal("unknown gate lost", gate, held, err)
	}
	if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "other-native", text); err != nil || matched {
		t.Fatal("wrong identity admitted", err)
	}
	if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used != 100 {
		t.Fatal("source forget lost occupancy", used, err)
	}
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: e.TargetConversationID, ClientRequestID: "new-target-preview", RequestHash: "preview", Request: PreviewRequest{ConfigID: "config", AgentName: "omp", TargetProviderThreadID: e.TargetProviderThreadID}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.CompletePreview(ctx, p.ID, PreviewResult{Budget: PreviewBudget{History: 16000}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: e.TargetConversationID, HandoffID: p.ID, ClientRequestID: "bypass-unknown", RequestHash: "new", Revision: p.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatal("new command bypassed forgotten-source unknown", err)
	}
	if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "native", text, "stable-user"); err != nil || !matched {
		t.Fatal("exact target proof failed", err)
	}
	if gate, held, err = s.RuntimeDispatch(ctx, e.RuntimeID); err != nil || held {
		t.Fatal("proof did not release gate", gate, held, err)
	}
	if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used != 100 {
		t.Fatal("confirmed occupancy lost", used, err)
	}
}

func TestForgottenSourceNativeUnknownNeedsWholeProofButNeverSendsUserTask(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	_, _, e, items := injectionTest(t, s)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveInjection(ctx, e.ID, "native", items); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if _, held, err := s.RuntimeDispatch(ctx, e.RuntimeID); err != nil || !held {
		t.Fatal("history unknown lost", held, err)
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items[:1]); !errors.Is(err, ErrConflict) {
		t.Fatal("partial proof", err)
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
		t.Fatal(err)
	}
	gate, held, err := s.RuntimeDispatch(ctx, e.RuntimeID)
	if err != nil || held || gate.CallStarted {
		t.Fatal("proof did not cancel unsent deleted operation", gate, held, err)
	}
	if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted operation recreated", err)
	}
}

func TestPublishedManagedRuntimePrunesButReservationSurvives(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRuntimes(ctx, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Binding(ctx, e.RuntimeID); err != nil {
		t.Fatal("unpublished reservation pruned", err)
	}
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: e.RuntimeID, Cwd: "/workspace", ConfigID: "config", AgentName: "omp", Status: "closed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRuntimes(ctx, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Binding(ctx, e.RuntimeID); !errors.Is(err, ErrNotFound) {
		t.Fatal("published managed orphan retained", err)
	}
}

func TestForgettingUnpublishedRuntimeLeavesRecoverableDeletionIntent(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRuntimeForgetting(ctx, e.RuntimeID); err != nil {
		t.Fatal(err)
	}
	// Simulate successful JSON removal and a crash before SQL deletion.
	if err := s.PruneRuntimes(ctx, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Binding(ctx, e.RuntimeID); !errors.Is(err, ErrNotFound) {
		t.Fatal("forget intent was confused with unpublished creation", err)
	}
}

func TestForgottenSourceRemovesUnexecutedForkDraftMetadata(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	p, _ := executionTest(t, s, b)
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation(ctx, p.TargetConversationID); !errors.Is(err, ErrNotFound) {
		t.Fatal("abandoned fork metadata survived", err)
	}
}

func TestSQLiteReadOnlyAndFullAbortBeforeActivityPublication(t *testing.T) {
	for _, fault := range []string{"read-only", "full"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			s := openTest(t, t.TempDir())
			b := bindTest(t, s, "source")
			statement := "PRAGMA query_only=ON"
			if fault == "full" {
				var pages int
				if err := s.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
					t.Fatal(err)
				}
				statement = fmt.Sprintf("PRAGMA max_page_count=%d", pages)
			}
			if _, err := s.db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			_, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, LocalKey: "cannot-commit", Kind: "message", Role: "user", Source: "local", Status: "completed", Text: strings.Repeat("private not logged ", 10000)})
			if !errors.Is(err, ErrUnavailable) {
				t.Fatal("storage failure did not fail closed", err)
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM activity WHERE position IS NOT NULL").Scan(&count); err != nil || count != 0 {
				t.Fatal("partial activity published", count, err)
			}
		})
	}
}

func TestProcessCrashAfterClaimBeforeConsumptionCommitStaysUnknown(t *testing.T) {
	if dir := os.Getenv("WEBMUX_CONSUMPTION_COMMIT_CRASH"); dir != "" {
		s := Open(Options{StateDir: dir})
		if s.Unavailable() != nil {
			os.Exit(40)
		}
		b, err := s.Binding(context.Background(), "source")
		if err != nil {
			os.Exit(41)
		}
		r, err := s.Receipt(context.Background(), b.ConversationID, "message-command")
		if err != nil {
			os.Exit(41)
		}
		s.beforeCommit = func() error { os.Exit(42); return nil }
		_ = s.FinishExecution(context.Background(), r.ExecutionID, "inline", "")
		os.Exit(43)
	}
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	e.Phase = "sending"
	e.NativeThreadID = "native"
	e.PromptHash = TextHash("packet")
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessCrashAfterClaimBeforeConsumptionCommitStaysUnknown$")
	cmd.Env = append(os.Environ(), "WEBMUX_CONSUMPTION_COMMIT_CRASH="+dir)
	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 42 {
		t.Fatal("unexpected child crash", err)
	}
	s = openTest(t, dir)
	got, err := s.Execution(ctx, e.ID)
	if err != nil || got.Status != "unknown" {
		t.Fatal("uncommitted success became known", got, err)
	}
	if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); !errors.Is(err, ErrConflict) {
		t.Fatal("claimed call repeated after crash", err)
	}
	receipt, err := s.Receipt(ctx, b.ConversationID, "message-command")
	if err != nil || receipt.ExecutionID != e.ID {
		t.Fatal("new attempt allocated", receipt, err)
	}
}

func TestEstimatedNativeUsageIncludesPreviouslyConsumedAttachments(t *testing.T) {
	s := openTest(t, t.TempDir())
	ctx := context.Background()
	b := bindTest(t, s, "source")
	_, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, LocalKey: "with-files", Kind: "message", Role: "user", Source: "local", Status: "completed", Text: "body", Attachments: []ActivityAttachment{{Name: "image.png", MimeType: "image/png", Reference: "target-owned:image"}, {Name: "file.txt", MimeType: "text/plain", Reference: "target-owned:file"}}})
	if err != nil {
		t.Fatal(err)
	}
	used, err := s.EstimatedNativeUsage(ctx, b.ProviderThreadID)
	if err != nil || used != escapedTextBytes("body")+256+8192+4096 {
		t.Fatal("prior attachment occupancy was omitted", used, err)
	}
}

func TestForgottenSourceDispatchMigration(t *testing.T) {
	for _, version := range []int{10, 11} {
		for _, replaced := range []bool{false, true} {
			for _, state := range []string{"confirmed", "prompt-unknown", "native-pending", "native-unknown", "native-confirmed", "unsent", "failed"} {
				t.Run(fmt.Sprintf("v%d/replaced=%t/%s", version, replaced, state), func(t *testing.T) {
					ctx := context.Background()
					dir := t.TempDir()
					s := openTest(t, dir)
					b := bindTest(t, s, "source")
					_, e := executionTest(t, s, b)
					if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
						t.Fatal(err)
					}
					if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: e.RuntimeID, Cwd: "/workspace", AgentSessionID: "native", Status: "closed"}); err != nil {
						t.Fatal(err)
					}
					packet := "legacy envelope\nUser message:\noriginal"
					items := []InjectionItem{{ID: "legacy-item", Role: "user", TextHash: TextHash("legacy history")}}
					if strings.HasPrefix(state, "native-") {
						if _, err := s.ReserveInjection(ctx, e.ID, "native", items); err != nil {
							t.Fatal(err)
						}
						if err := s.ClaimInjection(ctx, e.DeliveryID); err != nil {
							t.Fatal(err)
						}
						if state == "native-unknown" {
							if err := s.FinishInjection(ctx, e.DeliveryID, "unknown"); err != nil {
								t.Fatal(err)
							}
						}
						if state == "native-confirmed" {
							if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
								t.Fatal(err)
							}
						}
					} else if state != "unsent" && state != "failed" {
						e.Phase, e.NativeThreadID, e.PromptHash = "sending", "native", TextHash(packet)
						if err := s.CheckpointExecution(ctx, e); err != nil {
							t.Fatal(err)
						}
						if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
							t.Fatal(err)
						}
						if state == "confirmed" {
							r, err := s.StartRun(ctx, e.TargetProviderThreadID, e.ID)
							if err != nil {
								t.Fatal(err)
							}
							if err := s.AcknowledgeRun(ctx, r.ID); err != nil {
								t.Fatal(err)
							}
							if err := s.FinishExecution(ctx, e.ID, "inline", ""); err != nil {
								t.Fatal(err)
							}
						}
					}
					if state == "failed" {
						if err := s.FinishExecution(ctx, e.ID, "failed", "connector_missing"); err != nil {
							t.Fatal(err)
						}
					}
					if replaced {
						if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: e.RuntimeID, Cwd: "/workspace", AgentSessionID: "replacement-native", Status: "closed"}); err != nil {
							t.Fatal(err)
						}
					}
					current, err := s.Binding(ctx, e.RuntimeID)
					if err != nil || (replaced && current.ProviderThreadID == e.TargetProviderThreadID) {
						t.Fatal("native replacement did not move binding", current, err)
					}
					// Recreate the actual v10 layout and its source cascade deletion.
					if _, err := s.db.Exec(`DROP TRIGGER dispatch_receipt_insert; DROP TRIGGER dispatch_receipt_update;
 DROP TRIGGER dispatch_receipt_delete; DROP TRIGGER dispatch_receipt_history; DROP TABLE dispatch_receipt;
 DROP INDEX delivery_target; ALTER TABLE runtime_binding DROP COLUMN published; PRAGMA user_version=10;`); err != nil {
						t.Fatal(err)
					}
					var legacyObjects int
					if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name IN
 ('dispatch_receipt','dispatch_receipt_insert','dispatch_receipt_update','dispatch_receipt_delete','dispatch_receipt_history','delivery_target')`).Scan(&legacyObjects); err != nil || legacyObjects != 0 {
						t.Fatal("v10 fixture retained v11 objects", legacyObjects, err)
					}
					if err := s.db.QueryRow("SELECT count(*) FROM pragma_table_info('runtime_binding') WHERE name='published'").Scan(&legacyObjects); err != nil || legacyObjects != 0 {
						t.Fatal("v10 fixture retained v11 runtime column", legacyObjects, err)
					}
					if _, err := s.db.Exec("DELETE FROM conversation WHERE id=?", b.ConversationID); err != nil {
						t.Fatal(err)
					}
					if version == 11 {
						// Model the shipped v11 migration's retained facts: changed
						// bindings skip inline owners and misclassify old native calls
						// as unsent on the replacement. Ordinary owners already work.
						if _, err := s.db.Exec(dispatchReceiptSchema); err != nil {
							t.Fatal(err)
						}
						if !replaced || (state != "confirmed" && state != "prompt-unknown") {
							thread, native, delivery := current.ProviderThreadID, "native", e.ID
							status, phase, started, cost := "cancelled", "ready", false, 0
							if replaced {
								native = "replacement-native"
							} else {
								switch state {
								case "confirmed", "prompt-unknown":
									status, phase, started, cost = "unknown", "sending", true, 64000
									if state == "confirmed" {
										status = "inline"
									}
								case "native-pending", "native-unknown":
									status, delivery = "unknown", e.DeliveryID
								}
							}
							if _, err := s.db.Exec("INSERT INTO dispatch_receipt VALUES(?,?,?,?,?,?,?,?,?)", e.ID, thread, e.RuntimeID, native, delivery, status, phase, started, cost); err != nil {
								t.Fatal(err)
							}
						}
					}
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					s = openTest(t, dir)
					// Current-version repair is idempotent too.
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					s = openTest(t, dir)
					gate, held, err := s.RuntimeDispatch(ctx, e.RuntimeID)
					wantHeld := state == "prompt-unknown" || state == "native-pending" || state == "native-unknown"
					historical := state == "confirmed" || state == "prompt-unknown" || state == "native-pending" || state == "native-unknown"
					wantThread := current.ProviderThreadID
					if historical {
						wantThread = e.TargetProviderThreadID
					}
					if err != nil || held != wantHeld || gate.RuntimeID != e.RuntimeID || gate.TargetProviderThreadID != wantThread {
						t.Fatal("legacy owner cannot resume exact target", gate, held, err)
					}
					if historical && gate.NativeThreadID != "native" {
						t.Fatal("receipt adopted replacement native identity", gate)
					}
					if state == "confirmed" || state == "prompt-unknown" {
						if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used < 64000 {
							t.Fatal("legacy envelope occupancy was underestimated", used, err)
						}
					}
					if state == "confirmed" && gate.Status != "inline" {
						t.Fatal("confirmed target became unknown", gate)
					}
					switch state {
					case "prompt-unknown":
						if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "wrong-native", packet); err != nil || matched {
							t.Fatal("wrong proof", matched, err)
						}
						if replaced {
							if _, matched, err := s.NormalizeInlineUser(ctx, current.ProviderThreadID, "replacement-native", packet); err != nil || matched {
								t.Fatal("replacement writer reconciled historical prompt", matched, err)
							}
						}
						if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "native", packet); err != nil || !matched {
							t.Fatal("legacy exact packet cannot reconcile", matched, err)
						}
					case "native-pending", "native-unknown":
						if err := s.ReconcileInjection(ctx, e.DeliveryID, "replacement-native", items); !errors.Is(err, ErrConflict) {
							t.Fatal("replacement writer reconciled historical injection", err)
						}
						if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
							t.Fatal("legacy native proof cannot reconcile", err)
						}
					}
					if _, held, err := s.RuntimeDispatch(ctx, e.RuntimeID); err != nil || held {
						t.Fatal("legacy proof did not release gate", held, err)
					}
					if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
						t.Fatal("migration recreated deleted source operation", err)
					}
					if _, err := s.Conversation(ctx, b.ConversationID); !errors.Is(err, ErrNotFound) {
						t.Fatal("migration recreated forgotten source", err)
					}
					if binding, err := s.Binding(ctx, e.RuntimeID); err != nil || binding != current {
						t.Fatal("repair rebound current writer to historical target", binding, err)
					}
				})
			}
		}
	}
}

func TestCaptureOptOutFollowsReplacementNativeIdentityAtomically(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := Open(Options{StateDir: dir, DisableCapture: true})
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Unavailable(); err != nil {
		t.Fatal(err)
	}
	first := bindTest(t, s, "runtime")
	s.beforeCommit = func() error { return errors.New("replacement commit failed") }
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "runtime", Cwd: "/workspace", AgentSessionID: "private-replacement"}); err == nil {
		t.Fatal("replacement published without its privacy marker")
	}
	s.beforeCommit = nil
	if binding, err := s.Binding(ctx, "runtime"); err != nil || binding != first {
		t.Fatal("failed replacement changed binding", binding, err)
	}
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "runtime", Cwd: "/workspace", AgentSessionID: "private-replacement"}); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.Binding(ctx, "runtime")
	if err != nil || replacement.ProviderThreadID == first.ProviderThreadID {
		t.Fatal("native replacement did not allocate identity", replacement, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = Open(Options{StateDir: dir})
	if err := s.Unavailable(); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.ReplayCaptureAllowed(ctx, replacement.ProviderThreadID); err != nil || allowed {
		t.Fatal("replacement opted-out history would be imported", allowed, err)
	}
	// Metadata reconciliation after re-enable must not erase the marker, even
	// when the replacement itself was first observed after the opt-out ended.
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "runtime", Cwd: "/workspace", AgentSessionID: "late-replacement"}); err != nil {
		t.Fatal(err)
	}
	late, err := s.Binding(ctx, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.ReplayCaptureAllowed(ctx, late.ProviderThreadID); err != nil || allowed {
		t.Fatal("late replacement lost opt-out lineage", allowed, err)
	}
	if _, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: late.ProviderThreadID, LocalKey: "new-live", Kind: "message", Role: "user", Source: "local", Status: "completed", Text: "new public live message"}); err != nil {
		t.Fatal("re-enable blocked new live capture", err)
	}
}
