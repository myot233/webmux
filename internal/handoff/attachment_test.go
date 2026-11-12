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
