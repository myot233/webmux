package handoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
)

type Options struct {
	Hybrid                                       *hybrid.Service
	Artifacts                                    *artifacts.Service
	Conversations                                *conversation.Service
	Launch                                       *launch.Service
	TokenCap, MaxPromptBytes, MaxPromptWireBytes int64
}

type Service struct{ options Options }

func New(o Options) *Service {
	if o.TokenCap == 0 {
		o.TokenCap = DefaultTokenCap
	}
	return &Service{options: o}
}

type Request struct {
	ClientRequestID, ConfigID, TargetProviderThreadID string
	AgentConfig                                       map[string]string
	UserText                                          string
	Fork                                              bool
	Location                                          conversation.PreviewLocation
	Attachments                                       []conversation.PreviewAttachment
}

func requestHash(source string, in Request) (string, error) {
	// Only typed snapshot fields enter the hash. It never contains launch env,
	// file bytes or a transient source point that would change retry identity.
	body, err := json.Marshal(struct {
		Source, Command string
		Request         Request
	}{source, "handoff-preview", in})
	if err != nil {
		return "", conversation.ErrInvalid
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func configFingerprint(c launch.Config) (string, error) {
	// The digest, not the environment values or argv, is persisted with a draft.
	body, err := json.Marshal(struct {
		Cmd    []string
		Env    map[string]string
		Hybrid bool
	}{c.Cmd, c.Env, c.Hybrid})
	if err != nil {
		return "", conversation.ErrInvalid
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func validateRequest(in Request) error {
	if len(in.ClientRequestID) < 8 || len(in.ClientRequestID) > 64 || in.ConfigID == "" || !utf8.ValidString(in.UserText) || len(in.UserText) > 1<<20 || len(in.Attachments) > 64 || len(in.AgentConfig) > 64 {
		return conversation.ErrInvalid
	}
	for _, r := range in.ClientRequestID {
		if r != '-' && r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return conversation.ErrInvalid
		}
	}
	for key, value := range in.AgentConfig {
		if len(key) > 256 || len(value) > 8192 {
			return conversation.ErrInvalid
		}
	}
	var metadata int
	for _, a := range in.Attachments {
		metadata += len(a.Name) + len(a.MimeType) + len(a.Reference)
		if metadata > 8192 || a.Size < 0 || a.Size > 100<<20 || (a.Kind != "image" && a.Kind != "file") {
			return conversation.ErrInvalid
		}
	}
	if in.Location.NewWorktreeBase != "" && in.Location.NewWorktreeBranch == "" {
		return conversation.ErrInvalid
	}
	return nil
}

func (s *Service) Preview(ctx context.Context, source string, in Request) (conversation.Preview, error) {
	if s.options.Conversations == nil || s.options.Launch == nil || s.options.TokenCap < 1024 || s.options.TokenCap > 64000 {
		return conversation.Preview{}, conversation.ErrUnavailable
	}
	if err := validateRequest(in); err != nil {
		return conversation.Preview{}, err
	}
	if in.AgentConfig == nil {
		in.AgentConfig = map[string]string{}
	}
	if in.Attachments == nil {
		in.Attachments = []conversation.PreviewAttachment{}
	}
	hash, err := requestHash(source, in)
	if err != nil {
		return conversation.Preview{}, err
	}
	receipt, err := s.options.Conversations.Receipt(ctx, source, in.ClientRequestID)
	if err == nil {
		if receipt.RequestHash != hash {
			return conversation.Preview{}, conversation.ErrConflict
		}
		preview, err := s.options.Conversations.Preview(ctx, receipt.HandoffID)
		if err != nil {
			return preview, err
		}
		if preview.State != "preparing" {
			return s.Get(ctx, preview.ID)
		}
		return s.finishPreview(ctx, preview)
	}
	if !errors.Is(err, conversation.ErrNotFound) {
		return conversation.Preview{}, err
	}
	config, err := s.options.Launch.Get(in.ConfigID)
	if err != nil {
		return conversation.Preview{}, err
	}
	snapshotAttachments, err := s.attachmentSnapshot(ctx, source, in.Attachments)
	if err != nil {
		return conversation.Preview{}, err
	}
	support := config.HybridSupport()
	if !config.Hybrid || !support.Supported {
		return conversation.Preview{}, conversation.ErrInvalid
	}
	fingerprint, err := configFingerprint(config)
	if err != nil {
		return conversation.Preview{}, err
	}
	agentConfig := maps.Clone(in.AgentConfig)
	var targetBaseline map[string]string
	if in.TargetProviderThreadID != "" {
		target, err := s.options.Conversations.ProviderThread(ctx, in.TargetProviderThreadID)
		if err != nil {
			return conversation.Preview{}, err
		}
		if target.ConversationID != source || target.ConfigID != in.ConfigID || target.AgentName != support.Agent {
			return conversation.Preview{}, conversation.ErrConflict
		}
		agentConfig = maps.Clone(target.Config)
		targetBaseline = maps.Clone(target.Config)
		for key, value := range in.AgentConfig {
			agentConfig[key] = value
		}
	}
	request := conversation.PreviewRequest{ConfigID: in.ConfigID, AgentName: support.Agent, ConfigFingerprint: fingerprint, TargetProviderThreadID: in.TargetProviderThreadID, AgentConfig: agentConfig, TargetConfigBaseline: targetBaseline, UserText: in.UserText, Fork: in.Fork, Location: in.Location, Attachments: snapshotAttachments}
	preview, err := s.options.Conversations.ReservePreview(ctx, conversation.PreviewReservation{SourceConversationID: source, ClientRequestID: in.ClientRequestID, RequestHash: hash, Request: request})
	if err != nil {
		return preview, err
	}
	if preview.State != "preparing" {
		return s.Get(ctx, preview.ID)
	}
	return s.finishPreview(ctx, preview)
}

func (s *Service) budget(ctx context.Context, p conversation.Preview) (Budget, error) {
	target, err := s.options.Conversations.ProviderThread(ctx, p.TargetProviderThreadID)
	if err != nil {
		return Budget{}, err
	}
	used := target.ContextUsage
	if !target.UsageMeasured {
		used, err = s.options.Conversations.EstimatedNativeUsage(ctx, target.ID)
		if err != nil {
			return Budget{}, err
		}
	}
	window, threshold := target.ContextWindow, target.AutoCompactThreshold
	if !maps.Equal(target.Config, p.Request.AgentConfig) {
		window, threshold = 0, 0
	}
	attachments := make([]Attachment, len(p.Request.Attachments))
	for i, a := range p.Request.Attachments {
		attachments[i] = Attachment{Name: a.Name, MimeType: a.MimeType, Reference: a.Reference, Kind: a.Kind, Size: a.Size}
	}
	return ComputeBudget(BudgetInput{TokenCap: s.options.TokenCap, UserText: p.Request.UserText, Attachments: attachments, Used: used, Window: window, CompactionThreshold: threshold, Measured: target.UsageMeasured, Inline: true, MaxPromptBytes: s.options.MaxPromptBytes, MaxPromptWireBytes: s.options.MaxPromptWireBytes}), nil
}

func previewCoverage(p conversation.Preview) Coverage {
	return Coverage{HandoffID: p.ID, ConversationID: p.SourceConversationID, FromRun: p.FromRunOrdinal, ToRun: p.ToRunOrdinal, SourcePosition: p.SourcePosition, HistoryOrigin: p.HistoryOrigin, ReconcileRequired: p.ReconcileRequired}
}

func (s *Service) finishPreview(ctx context.Context, p conversation.Preview) (conversation.Preview, error) {
	budget, err := s.budget(ctx, p)
	if err != nil {
		return p, err
	}
	candidates, err := s.options.Conversations.Candidates(ctx, p.SourceConversationID, p.SourcePosition, max(int64(0), p.FromRunOrdinal-1))
	if err != nil {
		return p, err
	}
	selection := Select(candidates, previewCoverage(p), budget.History)
	messages := make([]conversation.HistoricalMessage, 0, len(selection.Selected))
	ids := make([]string, 0, len(selection.Selected))
	for _, candidate := range selection.Selected {
		part, err := s.options.Conversations.Activity(ctx, p.SourceConversationID, candidate.Message.ActivityID, 0, conversation.MaxActivityBytes)
		if err != nil {
			return p, err
		}
		if part.Truncated || part.Activity.Position > p.SourcePosition {
			return p, conversation.ErrConflict
		}
		message := conversation.ProjectActivity(part.Activity)
		if !message.Exportable {
			return p, conversation.ErrConflict
		}
		messages = append(messages, message)
		ids = append(ids, message.ActivityID)
	}
	budget.Used = selection.Cost
	budget.OverBudget = budget.OverBudget || selection.OverBudget
	text := Render(selection.Context, messages)
	if jsonBytes(text) > selection.Cost {
		return p, conversation.ErrUnavailable
	}
	return s.options.Conversations.CompletePreview(ctx, p.ID, conversation.PreviewResult{RenderedText: text, Budget: conversation.PreviewBudget(budget), SelectedItemIDs: ids, OmittedItemIDs: selection.Omitted})
}

func (s *Service) Get(ctx context.Context, id string) (conversation.Preview, error) {
	if s.options.Conversations == nil {
		return conversation.Preview{}, conversation.ErrUnavailable
	}
	p, err := s.options.Conversations.Preview(ctx, id)
	if err != nil || p.State == "preparing" {
		return p, err
	}
	// A saved conclusion is stale when its target constraints change. Keep the
	// original budget and edited text visible until the user regenerates it.
	current, err := s.budget(ctx, p)
	if err != nil {
		return p, err
	}
	saved := p.Budget
	if current.Window != saved.Window || current.NativeUsed != saved.NativeUsed || current.Current != saved.Current || current.Reserve != saved.Reserve || current.History != saved.History || current.Source != saved.Source {
		p.Stale = true
	}
	if p.Request.TargetProviderThreadID != "" {
		target, err := s.options.Conversations.ProviderThread(ctx, p.TargetProviderThreadID)
		if err != nil {
			return p, err
		}
		// Both the original baseline and the explicitly requested final values
		// are expected. Other changes invalidate the draft even when an unknown
		// model window produces the same conservative budget totals.
		if !maps.Equal(target.Config, p.Request.TargetConfigBaseline) && !maps.Equal(target.Config, p.Request.AgentConfig) {
			p.Stale = true
		}
	}
	config, err := s.options.Launch.Get(p.Request.ConfigID)
	if errors.Is(err, launch.ErrNotFound) {
		p.Stale = true
		return p, nil
	}
	if err != nil {
		return p, err
	}
	fingerprint, err := configFingerprint(config)
	if err != nil {
		return p, err
	}
	if fingerprint != p.Request.ConfigFingerprint {
		p.Stale = true
	}
	return p, nil
}

func (s *Service) Edit(ctx context.Context, id string, revision int64, text string) (conversation.Preview, error) {
	if revision < 1 {
		return conversation.Preview{}, conversation.ErrInvalid
	}
	p, err := s.Get(ctx, id)
	if err != nil {
		return p, err
	}
	if p.Revision != revision {
		return p, conversation.ErrConflict
	}
	if len(text) > 1<<20 || !utf8.ValidString(text) {
		return p, conversation.ErrInvalid
	}
	budget, err := s.budget(ctx, p)
	if err != nil {
		return p, err
	}
	context := coverageText(previewCoverage(p), 0, len(p.SelectedItemIDs)+len(p.OmittedItemIDs))
	budget.Used = jsonBytes(context+"\n\nManual context:\n"+text) + 256
	budget.OverBudget = budget.OverBudget || budget.Used > budget.History
	updated, err := s.options.Conversations.EditPreview(ctx, id, revision, text, conversation.PreviewBudget(budget))
	if err != nil {
		return updated, err
	}
	return s.Get(ctx, updated.ID)
}

// InlineContext adds immutable attribution around an edited draft. Native
// injection remains disabled for manual_context even when an adapter supports it.
func InlineContext(p conversation.Preview) string {
	if p.ManualText == nil {
		return p.RenderedText
	}
	return coverageText(previewCoverage(p), 0, len(p.OmittedItemIDs)) + "\n\nManual context:\n" + *p.ManualText
}

func InlinePrompt(p conversation.Preview) string {
	return strings.TrimSuffix(InlineContext(p), "\n") + "\n\nUser message:\n" + p.Request.UserText
}
