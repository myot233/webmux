package handoff

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/workspace"
)

func TestOwnedAttachmentFallbackReconcilesExactProviderTextAfterRestart(t *testing.T) {
	h := deliveryTest(t, map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"})
	ctx := context.Background()
	ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := h.folders.EnsureUploads(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(directory, "photo.png")
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jQ1cAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, data, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "attachment-preview", ConfigID: h.config.ID, Fork: true, UserText: "inspect the image", Attachments: []conversation.PreviewAttachment{{Reference: original, Kind: "image"}}})
	if err != nil {
		t.Fatal(err)
	}
	input := MessageRequest{ClientRequestID: "attachment-message", HandoffID: p.ID, Revision: 1, Text: p.Request.UserText, ConfigID: p.Request.ConfigID, AgentConfig: p.Request.AgentConfig, Location: p.Request.Location, Attachments: p.Request.Attachments}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, input); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "unknown")
	if len(e.Attachments) != 1 || e.Attachments[0].Reference == original || e.Attachments[0].Name != "photo.png" {
		t.Fatalf("attachment not transferred: %+v", e.Attachments)
	}
	copied := e.Attachments[0].Reference
	if _, err := h.folders.UploadPath(e.RuntimeID, copied); err != nil {
		t.Fatal("target rejects its own attachment", err)
	}
	body, err := os.ReadFile(filepath.Join(h.agentStore, "sessions", e.NativeThreadID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ Messages []struct{ Text string } }
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Messages) != 1 || !strings.Contains(saved.Messages[0].Text, copied) {
		t.Fatal("provider did not receive owned fallback path")
	}
	digest := sha256.Sum256([]byte(saved.Messages[0].Text))
	if e.PromptHash != hex.EncodeToString(digest[:]) {
		t.Fatal("digest excluded capability fallback")
	}
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}
	h.close()
	h.open()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, input); err != nil {
		t.Fatal(err)
	}
	cfg := h.config
	delete(cfg.Env, "FAKEAGENT_FAIL")
	if _, err := h.launches.Update(cfg.ID, launch.Input{Name: cfg.Name, Cmd: cfg.Cmd, Env: cfg.Env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "attachment-reconcile", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(p.ID, "inline")
	file, err := h.folders.ReadUpload(e.RuntimeID, copied)
	if err != nil || string(file.Data) != string(data) {
		t.Fatalf("target copy did not survive source loss: %+v %v", file, err)
	}
	page, err := h.store.Activities(ctx, p.TargetConversationID, 0, 100, 2000)
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, item := range page.Items {
		if item.Activity.Role != "user" {
			continue
		}
		users++
		if item.Activity.Text != "inspect the image" || len(item.Activity.Attachments) != 1 || item.Activity.Attachments[0].Reference != copied {
			t.Fatalf("replay lost normalized ownership: %+v", item.Activity)
		}
	}
	if users != 1 || h.promptCalls() != 1 {
		t.Fatalf("users=%d prompts=%d", users, h.promptCalls())
	}
	sub, err := h.hybrids.Subscribe(e.RuntimeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	attachments := 0
	for _, event := range sub.Events {
		if event.Message != nil && event.Message.Role == "user" {
			attachments += len(event.Message.Attachments)
			if strings.Contains(event.Message.Text, "WebMux handoff") {
				t.Fatal("envelope exposed on replay")
			}
		}
	}
	if err := h.hybrids.RemoveRecord(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	h.folders.Remove(ids[0])
	file, err = h.folders.ReadUpload(e.RuntimeID, copied)
	if err != nil || string(file.Data) != string(data) {
		t.Fatalf("forgetting the source removed target ownership: %v", err)
	}

	if attachments != 1 {
		t.Fatalf("replayed UI attachment count=%d", attachments)
	}
	sub.Close()
	h.close()
	h.open()
	if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, workspace.HybridResumeInput{ClientRequestID: "load-after-source-forgotten", Side: "acp"}); err != nil {
		t.Fatal(err)
	}
	again, err := h.hybrids.Subscribe(e.RuntimeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	attachments = 0
	for _, event := range again.Events {
		if event.Message != nil && event.Message.Role == "user" {
			attachments += len(event.Message.Attachments)
			if strings.Contains(event.Message.Text, "WebMux handoff") {
				t.Fatal("forgetting source exposed historical envelope")
			}
		}
	}
	if attachments != 1 || h.promptCalls() != 1 {
		t.Fatalf("source-owned replay metadata: attachments=%d calls=%d", attachments, h.promptCalls())
	}
}
