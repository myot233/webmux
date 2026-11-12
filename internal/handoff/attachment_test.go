package handoff

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rainchan/webmux/internal/conversation"
)

func TestStagedDraftAttachmentsSurviveRestartRegenerationAndSourceForget(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	// History sources need not have a live session or an artifacts folder.
	h.close()
	h.open()
	ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	stage, err := h.folders.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(stage, "proof.txt")
	data := []byte("durable handoff attachment 中文🙂\n")
	if err := os.WriteFile(original, data, 0600); err != nil {
		t.Fatal(err)
	}
	request := Request{ClientRequestID: "durable-preview", ConfigID: h.config.ID, Fork: true, UserText: "inspect the proof", Attachments: []conversation.PreviewAttachment{{Reference: original, Kind: "file"}}}
	p, err := h.service.Preview(ctx, h.source.ConversationID, request)
	if err != nil {
		t.Fatal(err)
	}
	owned := p.Request.Attachments[0].Reference
	if owned == original {
		t.Fatal("persistent draft still references ephemeral staging")
	}
	if _, err := h.folders.UploadPath(ids[0], owned); err != nil {
		t.Fatal("draft is not owned by its source", err)
	}
	if body, err := os.ReadFile(original); err != nil || !bytes.Equal(body, data) {
		t.Fatal("snapshot stole another form's staged upload", err)
	}
	// A second client's draft shares a safe immutable source upload, not a
	// move or an overwrite of the first revision's attachment.
	request.ClientRequestID = "second-durable-preview"
	second, err := h.service.Preview(ctx, h.source.ConversationID, request)
	if err != nil || second.Request.Attachments[0].Reference != owned {
		t.Fatal("second client lost staged attachment", err)
	}
	if h.promptCalls() != 0 {
		t.Fatal("preview sent a prompt")
	}
	h.close()
	h.open()
	if _, err := os.Lstat(original); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("startup did not clear original staging", err)
	}
	latest, err := h.store.LatestPreview(ctx, h.source.ConversationID, true)
	if err != nil || latest.ID != second.ID {
		t.Fatal("restart did not discover saved draft", err)
	}
	p, err = h.service.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	request.ClientRequestID = "regenerated-durable-preview"
	request.Attachments = p.Request.Attachments
	p, err = h.service.Preview(ctx, h.source.ConversationID, request)
	if err != nil || p.Request.Attachments[0].Reference != owned || p.Request.Attachments[0].Name != "proof.txt" {
		t.Fatal("regeneration lost attachment identity", err)
	}
	if h.promptCalls() != 0 {
		t.Fatal("restart or regeneration sent a prompt")
	}
	input := MessageRequest{ClientRequestID: "send-durable-attachment", HandoffID: p.ID, Revision: p.Revision, Text: p.Request.UserText, ConfigID: p.Request.ConfigID, AgentConfig: p.Request.AgentConfig, Location: p.Request.Location, Attachments: p.Request.Attachments}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, input); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	if len(e.Attachments) != 1 || e.Attachments[0].Reference == owned || h.promptCalls() != 1 {
		t.Fatal("send did not create independent target ownership", e)
	}
	copied := e.Attachments[0].Reference
	if _, err := h.folders.UploadPath(e.RuntimeID, owned); err == nil {
		t.Fatal("target accepted source-owned path")
	}
	if _, err := h.folders.UploadPath(ids[0], copied); err == nil {
		t.Fatal("source accepted target-owned path")
	}
	if body, err := os.ReadFile(copied); err != nil || !bytes.Equal(body, data) {
		t.Fatal("target bytes changed", err)
	}
	if err := h.hybrids.Forget(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	h.folders.Remove(ids[0])
	if _, err := os.Lstat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("forgotten source retained draft upload", err)
	}
	if body, err := os.ReadFile(copied); err != nil || !bytes.Equal(body, data) {
		t.Fatal("forgetting source removed target bytes", err)
	}
}

func TestAttachmentSnapshotRejectsUnapprovedAndForeignFiles(t *testing.T) {
	h := deliveryTest(t, nil)
	stage, err := h.folders.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(h.root, "unapproved.txt")
	if err := os.WriteFile(outside, []byte("not uploaded"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(stage, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	foreign, _ := h.preview(conversation.PreviewLocation{})
	input := MessageRequest{ClientRequestID: "create-foreign-owner", HandoffID: foreign.ID, Revision: foreign.Revision, Text: foreign.Request.UserText, ConfigID: foreign.Request.ConfigID, AgentConfig: foreign.Request.AgentConfig, Location: foreign.Request.Location, Attachments: foreign.Request.Attachments}
	if _, err := h.service.Deliver(context.Background(), h.source.ConversationID, input); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(foreign.ID, "inline")
	dir, err := h.folders.EnsureUploads(e.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "foreign.txt")
	if err := os.WriteFile(other, []byte("other conversation"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, link, other} {
		if _, err := h.service.attachmentSnapshot(context.Background(), h.source.ConversationID, []conversation.PreviewAttachment{{Reference: path, Kind: "file"}}); !errors.Is(err, conversation.ErrInvalid) {
			t.Fatalf("accepted unapproved path %s: %v", path, err)
		}
	}
}

func TestStagedPreviewsWithSameBasenameKeepEveryRevision(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	dir, err := h.folders.EnsureUploads(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	ordinary := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(ordinary, []byte("ordinary source report"), 0600); err != nil {
		t.Fatal(err)
	}
	makePreview := func(key, text string) conversation.Preview {
		t.Helper()
		stage, err := h.folders.EnsureStaging()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(stage, "report.txt")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: key, ConfigID: h.config.ID, Fork: true, UserText: "inspect the report", Attachments: []conversation.PreviewAttachment{{Reference: path, Kind: "file"}}})
		if err != nil {
			t.Fatal("same-basename preview failed", err)
		}
		return p
	}
	first := makePreview("first-report-preview", "first staged report")
	repeat := makePreview("repeat-report-preview", "first staged report")
	if first.Request.Attachments[0].Reference != repeat.Request.Attachments[0].Reference {
		t.Fatal("unchanged content was copied twice")
	}
	h.close()
	h.open()
	revised := makePreview("revised-report-preview", "revised staged report")
	firstPath, revisedPath := first.Request.Attachments[0].Reference, revised.Request.Attachments[0].Reference
	if firstPath == revisedPath {
		t.Fatal("revised content replaced old snapshot")
	}
	for _, p := range []conversation.Preview{first, repeat, revised} {
		saved, err := h.service.Get(ctx, p.ID)
		if err != nil || saved.Stale || saved.Request.Attachments[0].Name != "report.txt" || saved.Request.Attachments[0].MimeType != p.Request.Attachments[0].MimeType {
			t.Fatalf("persisted preview metadata changed: %+v %v", saved, err)
		}
	}
	regenerated, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "regenerated-report-preview", ConfigID: h.config.ID, Fork: true, UserText: revised.Request.UserText, Attachments: revised.Request.Attachments})
	if err != nil || regenerated.Request.Attachments[0] != revised.Request.Attachments[0] {
		t.Fatal("regeneration changed attachment metadata", err)
	}
	if h.promptCalls() != 0 {
		t.Fatal("preview or restart sent a prompt")
	}
	input := MessageRequest{ClientRequestID: "send-revised-report", HandoffID: regenerated.ID, Revision: regenerated.Revision, Text: regenerated.Request.UserText, ConfigID: regenerated.Request.ConfigID, AgentConfig: regenerated.Request.AgentConfig, Location: regenerated.Request.Location, Attachments: regenerated.Request.Attachments}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, input); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(regenerated.ID, "inline")
	if h.promptCalls() != 1 || len(e.Attachments) != 1 || e.Attachments[0].Name != "report.txt" {
		t.Fatal("explicit send did not preserve attachment", e)
	}
	for path, want := range map[string]string{ordinary: "ordinary source report", firstPath: "first staged report", revisedPath: "revised staged report", e.Attachments[0].Reference: "revised staged report"} {
		if body, err := os.ReadFile(path); err != nil || string(body) != want {
			t.Fatalf("lost retained bytes at %s: %q %v", path, body, err)
		}
	}
}

func TestEqualContentStagedAttachmentsRemainDistinctThroughDelivery(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	stage, err := h.folders.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("identical bytes, distinct attachment names 中文🙂\n")
	names := []string{"before.txt", "after.txt"}
	request := Request{ClientRequestID: "equal-content-preview", ConfigID: h.config.ID, Fork: true, UserText: "compare both named files"}
	for _, name := range names {
		path := filepath.Join(stage, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		request.Attachments = append(request.Attachments, conversation.PreviewAttachment{Reference: path, Kind: "file"})
	}
	first, err := h.service.Preview(ctx, h.source.ConversationID, request)
	if err != nil {
		t.Fatal(err)
	}
	h.close()
	h.open()
	request.ClientRequestID = "regenerate-equal-content-preview"
	request.Attachments = first.Request.Attachments
	regenerated, err := h.service.Preview(ctx, h.source.ConversationID, request)
	if err != nil {
		t.Fatal("accepted equal-content attachments cannot be regenerated after restart", err)
	}
	if len(regenerated.Request.Attachments) != len(names) {
		t.Fatal("regeneration collapsed distinct named attachments", regenerated.Request.Attachments)
	}
	sourcePaths := make([]string, len(names))
	for i, a := range regenerated.Request.Attachments {
		if a.Name != names[i] || a.Reference != first.Request.Attachments[i].Reference {
			t.Fatal("regeneration changed saved attachment identity", a)
		}
		sourcePaths[i] = a.Reference
		if body, err := os.ReadFile(a.Reference); err != nil || !bytes.Equal(body, data) {
			t.Fatal("saved attachment bytes changed", a.Name, err)
		}
	}
	if sourcePaths[0] == sourcePaths[1] || h.promptCalls() != 0 {
		t.Fatal("distinct attachments collapsed or preview sent a prompt", sourcePaths)
	}
	input := MessageRequest{ClientRequestID: "send-equal-content-attachments", HandoffID: regenerated.ID, Revision: regenerated.Revision, Text: regenerated.Request.UserText, ConfigID: regenerated.Request.ConfigID, AgentConfig: regenerated.Request.AgentConfig, Location: regenerated.Request.Location, Attachments: regenerated.Request.Attachments}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, input); err != nil {
		t.Fatal(err)
	}
	execution := h.waitStatus(regenerated.ID, "inline")
	if len(execution.Attachments) != len(names) || h.promptCalls() != 1 {
		t.Fatal("explicit send lost named attachments", execution)
	}
	for i, a := range execution.Attachments {
		if a.Name != names[i] || a.Reference == sourcePaths[i] {
			t.Fatal("target did not retain independent named ownership", a)
		}
		if _, err := h.folders.UploadPath(execution.RuntimeID, a.Reference); err != nil {
			t.Fatal("attachment is not target-owned", err)
		}
		if body, err := os.ReadFile(a.Reference); err != nil || !bytes.Equal(body, data) {
			t.Fatal("delivered attachment bytes changed", a.Name, err)
		}
	}
	if execution.Attachments[0].Reference == execution.Attachments[1].Reference {
		t.Fatal("target collapsed the two named files")
	}
}
