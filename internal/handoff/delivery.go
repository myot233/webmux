package handoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/workspace"
)

type MessageRequest struct {
	ClientRequestID, HandoffID string
	Revision                   int64
	Text, ConfigID             string
	AgentConfig                map[string]string
	Location                   conversation.PreviewLocation
	Attachments                []conversation.PreviewAttachment
	Retry                      bool
}

func executionHash(source string, in MessageRequest) (string, error) {
	body, err := json.Marshal(struct {
		Source  string
		Request MessageRequest
	}{source, in})
	if err != nil {
		return "", conversation.ErrInvalid
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.workers.Wait()
}

// Deliver returns a durable attempt immediately. A repeated command observes
// that attempt and may resume preparation, but never re-enters a sending call.
func (s *Service) Deliver(ctx context.Context, source string, in MessageRequest) (conversation.Execution, error) {
	if !s.options.Conversations.HistoryEnabled() || s.options.Workspace == nil || s.options.Hybrid == nil {
		return conversation.Execution{}, conversation.ErrUnavailable
	}
	if err := validateRequest(Request{ClientRequestID: in.ClientRequestID, ConfigID: in.ConfigID, UserText: in.Text, AgentConfig: in.AgentConfig, Attachments: in.Attachments, Location: in.Location}); err != nil {
		return conversation.Execution{}, err
	}
	if in.AgentConfig == nil {
		in.AgentConfig = map[string]string{}
	}
	if in.Attachments == nil {
		in.Attachments = []conversation.PreviewAttachment{}
	}
	hash, err := executionHash(source, in)
	if err != nil {
		return conversation.Execution{}, err
	}
	receipt, err := s.options.Conversations.Receipt(ctx, source, in.ClientRequestID)
	if err == nil {
		if receipt.RequestHash != hash || receipt.ExecutionID == "" {
			return conversation.Execution{}, conversation.ErrConflict
		}
		e, err := s.options.Conversations.Execution(ctx, receipt.ExecutionID)
		if err == nil {
			s.startExecution(e)
		}
		return e, err
	}
	if !errors.Is(err, conversation.ErrNotFound) {
		return conversation.Execution{}, err
	}
	var p conversation.Preview
	if in.Retry {
		p, err = s.options.Conversations.DeliveryPreview(ctx, in.HandoffID)
	} else {
		p, err = s.Get(ctx, in.HandoffID)
	}
	if err != nil {
		return conversation.Execution{}, err
	}
	if p.SourceConversationID != source || p.Revision != in.Revision || p.Stale || p.Budget.OverBudget || p.Request.UserText != in.Text || p.Request.ConfigID != in.ConfigID || !maps.Equal(p.Request.AgentConfig, in.AgentConfig) || p.Request.Location != in.Location || !slices.Equal(p.Request.Attachments, in.Attachments) {
		return conversation.Execution{}, conversation.ErrConflict
	}
	if err := hybrid.ValidatePromptText(in.Text, len(in.Attachments)); err != nil {
		return conversation.Execution{}, err
	}
	e, err := s.options.Conversations.BeginExecution(ctx, conversation.ExecutionInput{SourceConversationID: source, HandoffID: in.HandoffID, ClientRequestID: in.ClientRequestID, RequestHash: hash, Revision: in.Revision, Retry: in.Retry})
	if err == nil {
		s.startExecution(e)
	}
	return e, err
}

func (s *Service) startExecution(e conversation.Execution) {
	if e.Status != "pending" || e.Phase == "sending" {
		return
	}
	s.mu.Lock()
	if s.closed || s.flights[e.ID] {
		s.mu.Unlock()
		return
	}
	if s.flights == nil {
		s.flights = map[string]bool{}
	}
	s.flights[e.ID] = true
	s.workers.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.workers.Done()
		defer func() { s.mu.Lock(); delete(s.flights, e.ID); s.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(s.ctx, workspace.CreateTimeout)
		defer cancel()
		s.driveExecution(ctx, e)
	}()
}

func (s *Service) pause(phase string) bool {
	return s.pauseAfterPhase != nil && s.pauseAfterPhase(phase)
}

func (s *Service) driveExecution(ctx context.Context, e conversation.Execution) {
	ctx = hybrid.ExecutionContext(ctx, e.ID)
	fail := func(class string) {
		_ = s.options.Conversations.FinishExecution(context.WithoutCancel(ctx), e.ID, "failed", class)
	}
	releaseWriter, err := s.options.Hybrid.ClaimConversation(e.TargetConversationID, e.ID)
	if err != nil {
		fail("writer_conflict")
		return
	}
	defer releaseWriter()
	checkpoint := func(phase, step string) bool {
		e.Phase, e.Step = phase, step
		return s.options.Conversations.CheckpointExecution(ctx, e) == nil && !s.pause(phase)
	}
	if s.pause("receipt") {
		return
	}
	p, err := s.options.Conversations.DeliveryPreview(ctx, e.HandoffID)
	if err != nil {
		fail("storage")
		return
	}
	config, err := s.options.Launch.Get(p.Request.ConfigID)
	if err != nil {
		fail("config_missing")
		return
	}
	fingerprint, err := configFingerprint(config)
	if err != nil || fingerprint != p.Request.ConfigFingerprint {
		fail("config_changed")
		return
	}
	// Only provenance-checked failed-attempt history can differ from the snapshot.
	if p.Stale {
		fail("source_changed")
		return
	}
	actualAttachments, err := s.attachmentSnapshot(ctx, p.SourceConversationID, p.Request.Attachments)
	if len(e.Attachments) == 0 && (err != nil || !slices.Equal(actualAttachments, p.Request.Attachments)) {
		fail("attachments_changed")
		return
	}
	if !p.Request.Fork {
		ids, err := s.options.Conversations.RuntimeIDs(ctx, p.SourceConversationID)
		if err != nil {
			fail("storage")
			return
		}
		for _, id := range ids {
			if id == e.RuntimeID {
				continue
			}
			if err := s.quiesceProvider(ctx, id); err != nil {
				fail(executionErrorClass(err, "writer_conflict"))
				return
			}
		}
	}
	if record, err := s.options.Hybrid.Record(e.RuntimeID); err == nil && record.State == hybrid.StateActive {
		if err := s.options.Hybrid.ClaimProvider(ctx, e.RuntimeID, e.ID); err != nil {
			fail(executionErrorClass(err, "writer_conflict"))
			return
		}
	}

	in := workspace.CreateInput{ConfigID: &p.Request.ConfigID, Side: string(hybrid.SideACP), Cwd: p.Request.Location.Cwd}
	location := p.Request.Location
	if location.NewWorktreeBranch != "" {
		in.Cwd = ""
		in.NewWorktree = &workspace.NewWorktree{RepositoryID: location.RepositoryID, Mode: "new", Branch: location.NewWorktreeBranch, Base: location.NewWorktreeBase}
	} else if location.RepositoryID != "" {
		in.Workspace = &workspace.Reference{RepositoryID: location.RepositoryID, WorktreeID: location.WorktreeID}
	}
	if e.Phase == "reserved" {
		plan, err := s.options.Workspace.PlanHandoff(ctx, in, e.DeliveryID)
		if err != nil {
			fail(executionErrorClass(err, "precheck"))
			return
		}
		e.WorktreePath, e.WorktreeBranch, e.WorktreeBase = plan.Path, plan.Branch, plan.Base
		if !checkpoint("worktree", workspace.StepWorktree) {
			return
		}
	}
	if in.NewWorktree != nil {
		tree, err := s.options.Workspace.EnsureHandoffWorktree(ctx, workspace.HandoffPlan{RepositoryID: location.RepositoryID, Path: e.WorktreePath, Branch: e.WorktreeBranch, Base: e.WorktreeBase})
		if err != nil {
			fail(executionErrorClass(err, "worktree"))
			return
		}
		e.WorktreeID = tree.ID
		if s.pause("worktree_created") {
			return
		}
		in.NewWorktree = nil
		in.Cwd = ""
		in.Workspace = &workspace.Reference{RepositoryID: location.RepositoryID, WorktreeID: tree.ID}
	}
	if e.Phase == "worktree" {
		if !checkpoint("session", workspace.StepSession) {
			return
		}
	}
	item, err := s.options.Workspace.EnsureHandoffSession(ctx, in, workspace.HandoffIdentity{RuntimeID: e.RuntimeID, ProviderThreadID: e.TargetProviderThreadID, ExecutionID: e.ID, RequireNativeResume: !p.Request.Fork})
	if err != nil {
		if !p.Request.Fork && (errors.Is(err, hybrid.ErrNativeResumeUnavailable) || errors.Is(err, hybrid.ErrSessionMissing) || errors.Is(err, hybrid.ErrProviderConfigChanged)) {
			if fallbackErr := s.prepareFreshFallback(ctx, e, p); fallbackErr != nil {
				fail(executionErrorClass(fallbackErr, "session"))
			}
			return
		}
		fail(executionErrorClass(err, "session"))
		return
	}
	if err := s.options.Hybrid.RefreshHistoryReader(ctx, e.RuntimeID, e.ID); err != nil {
		if !p.Request.Fork && errors.Is(err, hybrid.ErrNativeResumeUnavailable) {
			if fallbackErr := s.prepareFreshFallback(ctx, e, p); fallbackErr != nil {
				fail(executionErrorClass(fallbackErr, "session"))
			}
		} else {
			fail(executionErrorClass(err, "session"))
		}
		return
	}
	if !p.Request.Fork {
		if err := s.options.Conversations.ActivateProvider(ctx, p.TargetProviderThreadID); err != nil {
			fail("storage")
			return
		}
	}
	nativeContinue, err := s.options.Hybrid.NativeContinuation(ctx, e.RuntimeID)
	if err != nil {
		fail("session")
		return
	}
	e.NativeContinue = nativeContinue
	if s.pause("session_created") {
		return
	}
	if !checkpoint("session", workspace.StepConfig) {
		return
	}
	if warnings, err := s.options.Workspace.ConfigureHandoff(ctx, item.Info().ID, p.Request.AgentConfig, e.ID); err != nil || len(warnings) > 0 {
		fail("config")
		return
	}
	// Options selected during startup may change the actual model window. Keep
	// the selected whole items and refuse execution if they no longer fit.
	budget, err := s.budget(ctx, p)
	if e.HistoryStatus != "injected" && (err != nil || budget.OverBudget || p.Budget.Used > budget.History) {
		if err == nil && !p.Request.Fork && !p.Request.ForceFresh {
			if fallbackErr := s.prepareFreshFallback(ctx, e, p); fallbackErr != nil {
				fail(executionErrorClass(fallbackErr, "over_budget"))
			}
			return
		}
		fail("over_budget")
		return
	}
	attachments, err := s.ownAttachments(ctx, p, e)
	if err != nil {
		fail("attachments_changed")
		return
	}
	if s.pause("attachments_owned") {
		return
	}

	if !checkpoint("ready", workspace.StepPrompt) {
		return
	}
	rec, err := s.options.Hybrid.Record(e.RuntimeID)
	if err != nil || rec.AgentSessionID == "" {
		fail(executionErrorClass(err, "session"))
		return
	}
	wireText := InlinePrompt(p)
	if p.Request.NativePlanned && p.ManualText == nil {
		if s.pause("before_native") {
			return
		}
		accepted, err := s.deliverNative(ctx, e, p, rec.AgentSessionID)
		if err != nil {
			s.historyOutcome(context.WithoutCancel(ctx), e, err)
			return
		}
		if accepted {
			wireText = p.Request.UserText
		} else {
			inline := p
			inline.Request.NativePlanned = false
			budget, err := s.budget(ctx, inline)
			if err != nil || budget.OverBudget || p.Budget.Used > budget.History {
				fail("inline_over_budget")
				return
			}
		}
		if s.pause("native_injected") {
			return
		}
	}
	digest := sha256.Sum256([]byte(wireText))
	e.NativeThreadID, e.PromptHash = rec.AgentSessionID, hex.EncodeToString(digest[:])
	latest, err := s.options.Conversations.DeliveryPreview(ctx, p.ID)
	if err != nil || latest.Stale || latest.Revision != p.Revision {
		fail("source_changed")
		return
	}
	if !checkpoint("sending", workspace.StepPrompt) {
		return
	}
	if s.pause("before_prompt") {
		return
	}
	allowed := map[string]bool{}
	for _, a := range attachments {
		allowed[a.Path] = true
	}

	_, err = s.options.Hybrid.PromptContext(ctx, e.RuntimeID, hybrid.ContextPrompt{ExecutionID: e.ID, Text: p.Request.UserText, WireText: wireText, Attachments: attachments, CheckAttachment: func(path string) (string, error) {
		if !allowed[path] {
			return "", conversation.ErrInvalid
		}
		return path, nil
	}})
	if s.pause("after_prompt") {
		return
	}
	if err != nil {
		// call() can time out after enqueueing the actor command. That outcome is
		// ambiguous; a validation/actor refusal returned synchronously is unsent.
		status, class := "failed", "prompt_refused"
		saved, readErr := s.options.Conversations.Execution(context.WithoutCancel(ctx), e.ID)
		if readErr != nil || saved.CallStarted || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status, class = "unknown", "transport"
		}
		_ = s.options.Conversations.FinishExecution(context.WithoutCancel(ctx), e.ID, status, class)
	}
}

func (s *Service) Delivery(ctx context.Context, id string) (conversation.Execution, error) {
	if !s.options.Conversations.HistoryEnabled() {
		return conversation.Execution{}, conversation.ErrUnavailable
	}
	return s.options.Conversations.Delivery(ctx, id)
}

func executionErrorClass(err error, fallback string) string {
	if errors.Is(err, hybrid.ErrExternalWriter) {
		return "external_writer"
	}
	if errors.Is(err, hybrid.ErrBusy) {
		return "unfinished_work"
	}
	var connector *hybrid.ConnectorMissingError
	if errors.As(err, &connector) {
		return "connector_missing"
	}
	if errors.Is(err, workspace.ErrConflict) {
		return "location_conflict"
	}
	if errors.Is(err, hybrid.ErrConfigMissing) {
		return "config_missing"
	}
	if errors.Is(err, hybrid.ErrSessionMissing) {
		return "native_session_missing"
	}
	return fallback
}
