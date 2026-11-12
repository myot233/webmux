import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { z } from "zod";
import * as S from "../src/contracts/schemas";

const directory = fileURLToPath(
  new URL("../../internal/api/testdata/wire/", import.meta.url),
);
// A terminal close is not a JSON frame; its snapshot pins the code and reason
// that src/ws.ts treats as terminal (4409: the session moved to its other view).
const TerminalMovedSchema = z.object({
  code: z.literal(4409),
  reason: z.string().startsWith("session moved"),
});
const groups: Array<[z.ZodType, string[]]> = [
  [
    S.HandoffDraftSchema,
    ["handoff-draft-empty", "handoff-draft-saved", "handoff-draft-delivery"],
  ],
  [
    S.HandoffPreviewSchema,
    ["handoff-preview-empty", "handoff-preview-manual", "handoff-preview-get"],
  ],
  [S.ErrorSchema, ["handoff-preview-conflict"]],
  [S.HandoffSelectionSchema, ["handoff-selection-empty"]],
  [
    S.HandoffDeliverySchema,
    [
      "handoff-delivery-pending",
      "handoff-delivery-failed",
      "handoff-delivery-retry",
    ],
  ],
  [
    S.ConversationActivitiesSchema,
    ["conversation-activities", "conversation-activities-empty"],
  ],
  [S.ConversationActivitySchema, ["conversation-activity"]],
  [S.ChatFrameSchema, ["ws-chat-events", "ws-chat-snapshot", "ws-chat-state"]],
  [S.LaunchOptionsSchema, ["launch-config-options"]],
  [
    S.SessionInputResultSchema,
    ["chat-prompt-queued", "chat-prompt-sent", "session-input"],
  ],
  [
    S.SessionConflictSchema,
    [
      "chat-conflict-answered",
      "chat-conflict-wrong-view",
      "hybrid-conflict-not-eligible",
      "hybrid-resume-config-missing",
      "hybrid-resume-session-missing",
    ],
  ],
  [S.ChatSupportSchema, ["launch-hybrid-support"]],
  [S.ArchivedSessionsSchema, ["sessions-archived"]],
  [S.ViewChangeResultSchema, ["hybrid-view"]],
  [
    S.NoContentSchema,
    [
      "chat-answer",
      "chat-permission-mode",
      "session-forget",
      "session-interrupt",
    ],
  ],
  [S.CodeChangesSchema, ["code-changes"]],
  [S.CodeCheckSchema, ["code-check"]],
  [S.CodeContextSchema, ["code-context", "code-context-artifacts"]],
  [S.ArtifactsFolderSchema, ["artifacts-ensure", "artifacts-ensure-code"]],
  [S.CodeDiffSchema, ["code-diff"]],
  [S.CodeFileSchema, ["code-file", "code-file-pdf", "code-snapshot-file"]],
  [S.CodeListingSchema, ["code-files", "code-tree"]],
  [S.CodeLocationSchema, ["code-locate", "code-locate-pdf"]],
  [S.ConfigSchema, ["config"]],
  [S.DirectoriesSchema, ["fs-dirs-default", "fs-dirs-empty", "fs-dirs-prefix"]],
  [
    S.ErrorSchema,
    [
      "settings-conflict",
      "session-details-missing",
      "session-labels-invalid",
      "session-attention-invalid",
    ],
  ],
  [S.SessionDetailsSchema, ["session-details"]],
  [S.AgentGuidePromptSchema, ["session-agent-guide"]],
  [
    S.NoContentSchema,
    [
      "fs-mkdir",
      "launch-delete",
      "push-unsubscribe",
      "session-attention-clear",
    ],
  ],
  [S.HealthSchema, ["health"]],
  [
    S.LaunchConfigSchema,
    ["launch-create-no-env", "launch-create", "launch-get", "launch-update"],
  ],
  [
    S.LaunchConfigsSchema,
    [
      "launch-list-chat-options",
      "launch-list-empty",
      "launch-list",
      "launch-presets-restore",
    ],
  ],
  [S.LaunchPresetsSchema, ["launch-presets"]],
  [
    S.PushStatusSchema,
    [
      "push-status-disabled",
      "push-status-quick-tunnel",
      "push-status-tailscale",
      "push-status",
    ],
  ],
  [S.PushSubscriptionResultSchema, ["push-subscribe-gone", "push-subscribe"]],
  [S.PushTestResultSchema, ["push-test"]],
  [S.RepositoriesSchema, ["repositories-empty", "repositories"]],
  [
    S.CreateSessionResultSchema,
    [
      "session-create",
      "session-create-cat",
      "session-create-exiting",
      "session-create-from-config",
      "session-create-new-worktree",
      "session-create-new-worktree-retry",
      "session-create-workspace",
    ],
  ],
  [
    S.CreateSessionFailureSchema,
    [
      "error-400-session-config-empty",
      "error-400-session-conflict",
      "error-400-session-create-chat-field",
      "error-400-session-create-not-chat",
      "error-409-session-create-connector-missing",
      "error-409-session-create-worktree",
      "error-400-session-env",
      "error-400-session-json",
      "error-404-session-config",
      "error-503-session-config",
      "hybrid-conflict-connector-missing",
      "session-create-labels-invalid",
    ],
  ],
  [S.BranchesSchema, ["repository-branches"]],
  [S.CommitsSchema, ["repository-commits"]],
  [S.RemotesSchema, ["repository-remotes", "repository-remotes-empty"]],
  [S.FetchRemotesResponseSchema, ["repository-fetch"]],
  [S.SwitchPreviewSchema, ["worktree-switch-preview"]],
  [S.RefreshBranchBaseResponseSchema, ["repository-branch-refresh"]],
  [
    S.RepositorySchema,
    [
      "repository-register-again",
      "repository-register-instruction",
      "repository-register",
      "repository-register-release",
      "repository-update-instruction",
      "repository-update",
      "repository-worktrees",
    ],
  ],
  [
    S.NoContentSchema,
    ["repository-unregister", "session-delete-close", "session-delete-remove"],
  ],
  [
    S.SessionInfoSchema,
    [
      "hybrid-resume",
      "session-get",
      "session-labels",
      "session-attention",
      "session-resize",
      "session-resize-take",
      "session-get-hybrid",
      "session-get-hybrid-work",
      "ws-terminal-state-exited",
      "ws-terminal-state-hybrid",
      "ws-terminal-state",
    ],
  ],
  [z.array(S.SessionInfoSchema), ["sessions-list-empty", "sessions-list"]],
  [
    S.SettingsSnapshotSchema,
    ["settings-get", "settings-patch", "settings-reset"],
  ],
  [S.SettingsEventSchema, ["ws-settings-snapshot", "ws-settings-update"]],
  [
    S.TunnelSettingsSchema,
    ["tunnel-enabled", "tunnel-get", "tunnel-reset", "tunnel-settings"],
  ],
  [S.UploadSchema, ["upload"]],
  [
    S.VoiceSettingsSchema,
    ["voice-settings-get", "voice-settings-put", "voice-settings-reset"],
  ],
  [S.P2PStatusSchema, ["p2p-get", "p2p-settings-put", "p2p-settings-reset"]],
  [S.P2PSignalResponseSchema, ["p2p-signal-offer"]],
  [
    S.WorktreeSchema,
    [
      "worktree-create",
      "worktree-create-release",
      "worktree-create-batch",
      "hybrid-rebuild",
    ],
  ],
  [S.RemovalPreviewSchema, ["worktree-removal-preview"]],
  [S.BatchRemovalPreviewSchema, ["worktree-batch-removal-preview"]],
  [S.BatchRemoveResultSchema, ["worktree-batch-remove"]],
  [S.ReleasePreviewSchema, ["worktree-release-preview"]],
  [S.WorktreeUsageSchema, ["worktree-usage"]],
  [S.ReleaseResultSchema, ["worktree-release"]],
  [S.SwitchPreviewSchema, ["worktree-switch-preview"]],
  [S.RebuildPreviewSchema, ["hybrid-rebuild-preview"]],
  [
    S.SessionEventSchema,
    [
      "ws-sessions-created",
      "ws-sessions-removed",
      "ws-sessions-forwards",
      "ws-sessions-snapshot-empty",
      "ws-sessions-snapshot",
      "ws-sessions-snapshot-forwards",
      "ws-sessions-updated",
    ],
  ],
  [
    S.ForwardSchema,
    ["forward-create", "forward-create-options", "forward-update"],
  ],
  [S.ForwardsSchema, ["forwards-list", "forwards-list-empty"]],
  [S.ForwardTicketSchema, ["forward-ticket"]],
  [S.NoContentSchema, ["forward-delete"]],
  [
    S.ControlStateSchema,
    [
      "ws-terminal-control-claimed",
      "ws-terminal-control-initial",
      "ws-terminal-control-observer",
      "ws-terminal-control-resize-controller",
      "ws-terminal-control-resize-take",
      "ws-terminal-control-uncontrolled",
    ],
  ],
  [S.ExitSchema, ["ws-terminal-exit"]],
  [TerminalMovedSchema, ["ws-terminal-moved"]],
  [
    S.StreamPositionSchema,
    ["ws-terminal-position-live", "ws-terminal-position-reset"],
  ],
  [
    S.SubmissionStatusSchema,
    [
      "ws-terminal-submission-failed",
      "ws-terminal-submission-missing",
      "ws-terminal-submission-received",
      "ws-terminal-submission-written",
    ],
  ],
  // add-branch-sync-actions
  [S.FastForwardPreviewSchema, ["branch-fast-forward-preview"]],
  [S.PushPreviewSchema, ["branch-push-preview"]],
  [S.PushResultSchema, ["branch-push"]],
  [S.BranchDeletePreviewSchema, ["branch-delete-preview"]],
  [S.BranchDeleteResultSchema, ["branch-delete"]],
];
const snapshotSchemas = new Map<string, z.ZodType>();
for (const [schema, names] of groups)
  for (const name of names) snapshotSchemas.set(`${name}.json`, schema);

// These endpoints belong to the native local client, not the browser contract.
const nonWebSnapshots = new Set([
  "local-status-tunnel-settings.json",
  "local-status-installed-version.json",
]);

function normalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalize);
  if (value && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, normalize(item)]),
    );
  if (typeof value !== "string") return value;
  const id = /^<(?:id|sessionId|configId)#(\d+)>$/.exec(value);
  if (id?.[1]) {
    const suffix = id[1].replace(/\d/g, (digit) =>
      "abcdefghij".charAt(Number(digit)),
    );
    return suffix.padStart(8, "a");
  }
  if (/^<generation#\d+>$/.test(value)) return "0123456789abcdef";
  if (value === "<time>") return "2026-01-02T03:04:05Z";
  return value.replaceAll("<tmp>", "/tmp");
}

const files = readdirSync(directory).filter((name) => name.endsWith(".json"));
describe("Go wire snapshots", () => {
  it("has an explicit schema decision for every snapshot", () => {
    for (const file of files)
      expect(
        snapshotSchemas.has(file) ||
          file.startsWith("error-") ||
          file.startsWith("tunnel-error-") ||
          nonWebSnapshots.has(file),
        file,
      ).toBe(true);
  });

  for (const file of files) {
    if (nonWebSnapshots.has(file)) continue;
    it(`${file} matches the frontend schema`, () => {
      const snapshot = JSON.parse(readFileSync(directory + file, "utf8")) as {
        status?: number;
        body: unknown;
      };
      const schema =
        snapshotSchemas.get(file) ??
        (file.startsWith("error-") || file.startsWith("tunnel-error-")
          ? S.ErrorSchema
          : undefined);
      expect(schema, file).toBeDefined();
      const body =
        snapshot.status === 204 && snapshot.body === null
          ? undefined
          : normalize(snapshot.body);
      expect(schema!.parse(body), file).toEqual(body);
    });
  }

  it("keeps document and change node IDs required, not merely accepted", () => {
    for (const schema of [S.CodeDocumentSchema, S.CodeChangeSchema])
      expect(schema.shape.nodeId.safeParse(undefined).success).toBe(false);
    for (const file of ["code-file.json", "code-changes.json"]) {
      const snapshot = JSON.parse(readFileSync(directory + file, "utf8")) as {
        body: { document?: object; entries?: object[] };
      };
      for (const value of snapshot.body.entries ?? [snapshot.body.document])
        expect(value).toHaveProperty("nodeId");
    }
  });
});
