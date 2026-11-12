package wire

import (
	"reflect"
	"testing"

	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

// These pairs are converted with a direct type conversion, which is only safe
// while the exported fields match in name and order. Field renames must update
// the converter at the same time.
func TestDirectConversionsKeepFieldOrder(t *testing.T) {
	pairs := []struct {
		name     string
		service  any
		contract any
	}{
		{"Capability", workspace.Capability{}, contract.WorkspaceCapability{}},
		{"WorktreeChanges", workspace.WorktreeChanges{}, contract.WorktreeChanges{}},
		{"Commit", workspace.Commit{}, contract.Commit{}},
		{"Base", workspace.Base{}, contract.BranchBase{}},
		{"Remote", workspace.Remote{}, contract.Remote{}},
		{"FetchResult", workspace.FetchResult{}, contract.FetchResult{}},
		{"Reference", workspace.Reference{}, contract.WorkspaceReference{}},
		{"CreateRequest", workspace.CreateRequest{}, contract.CreateWorktreeRequest{}},
		{"RemoveRequest", workspace.RemoveRequest{}, contract.RemoveWorktreeRequest{}},
		{"BatchRemoveRequest", workspace.BatchRemoveRequest{}, contract.BatchRemoveWorktreesRequest{}},
		{"BatchRemoveItemResult", workspace.BatchRemoveItemResult{}, contract.BatchRemoveItemResult{}},
		{"ReleaseRequest", workspace.ReleaseRequest{}, contract.ReleaseWorktreeRequest{}},
		{"SwitchTarget", workspace.SwitchTarget{}, contract.SwitchTarget{}},
		{"SwitchTargetInfo", workspace.SwitchTargetInfo{}, contract.SwitchTargetInfo{}},
		{"Issue", workspace.Issue{}, contract.Issue{}},
		{"SwitchRequest", workspace.SwitchRequest{}, contract.SwitchRequest{}},
		{"FastForwardRequest", workspace.FastForwardRequest{}, contract.FastForwardRequest{}},
		{"PushRequest", workspace.PushRequest{}, contract.PushRequest{}},
		{"BranchDeleteRequest", workspace.BranchDeleteRequest{}, contract.BranchDeleteRequest{}},
		{"BranchDeleteItemResult", workspace.BranchDeleteItemResult{}, contract.BranchDeleteItemResult{}},
		{"IgnoredEntry", workspace.IgnoredEntry{}, contract.IgnoredEntry{}},
		{"RebuildRequest", workspace.RebuildRequest{}, contract.RebuildRequest{}},
		{"HybridRelease", hybrid.Release{}, contract.HybridRelease{}},
		{"CodeContext", codeview.Context{}, contract.CodeContext{}},
		{"CodeNode", codeview.Node{}, contract.CodeNode{}},
		{"CodeLocation", codeview.Location{}, contract.CodeLocation{}},
		{"CodeChange", codeview.Change{}, contract.CodeChange{}},
		{"CodeCheck", codeview.Check{}, contract.CodeCheck{}},
		{"CodeCheckRequest", codeview.CheckRequest{}, contract.CodeCheckRequest{}},
		{"CodeUsage", codeview.Usage{}, contract.CodeUsage{}},
		{"Position", session.Position{}, contract.StreamPosition{}},
		{"Workspace", session.Workspace{}, contract.SessionWorkspace{}},
		{"ReportedCwd", session.ReportedCwd{}, contract.SessionReportedCwd{}},
		{"Controller", session.Controller{}, contract.TerminalController{}},
		{"PushTestReport", push.TestReport{}, contract.PushTestReport{}},
		{"ChatEligibility", hybrid.Eligibility{}, contract.ChatEligibility{}},
		{"ChatTurn", hybrid.Turn{}, contract.ChatTurn{}},
		{"ChatIDChange", hybrid.IDChange{}, contract.ChatIDChange{}},
		{"ChatContent", hybrid.Content{}, contract.ChatContent{}},
		{"ChatLocation", hybrid.Location{}, contract.ChatLocation{}},
		{"ChatPlanEntry", hybrid.PlanEntry{}, contract.ChatPlanEntry{}},
		{"ChatCommand", hybrid.Command{}, contract.ChatCommand{}},
		{"ChatConfigChoice", hybrid.ConfigChoice{}, contract.ChatConfigChoice{}},
		{"ChatUsage", hybrid.Usage{}, contract.ChatUsage{}},
		{"ChatCompaction", hybrid.Compaction{}, contract.ChatCompaction{}},
		{"ChatNotice", hybrid.Notice{}, contract.ChatNotice{}},
		{"ChatTurnStart", hybrid.TurnStart{}, contract.ChatTurnStart{}},
		{"ChatTurnEnd", hybrid.TurnEnd{}, contract.ChatTurnEnd{}},
		{"ChatOption", hybrid.PermissionOption{}, contract.ChatOption{}},
		{"ChatRequestOutcome", hybrid.RequestOutcome{}, contract.ChatRequestOutcome{}},
		{"ChatQueueRequest", hybrid.QueueEdit{}, contract.ChatQueueRequest{}},
		{"AgentCandidate", hybrid.Candidate{}, contract.AgentCandidate{}},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			service := exportedFields(reflect.TypeOf(pair.service))
			wire := exportedFields(reflect.TypeOf(pair.contract))
			if !reflect.DeepEqual(service, wire) {
				t.Fatalf("fields = %v, contract = %v", service, wire)
			}
		})
	}
}

func exportedFields(t reflect.Type) []string {
	fields := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		if field := t.Field(i); field.IsExported() {
			fields = append(fields, field.Name)
		}
	}
	return fields
}
