import { describe, expect, it } from "vitest";
import {
  ConversationCursorSchema,
  ConversationMessageRequestSchema,
  HandoffPreviewRequestSchema,
  HandoffEditRequestSchema,
  HandoffSelectionSchema,
} from "../src/contracts/schemas";

describe("persistent history cursors", () => {
  it("preserves int64 positions beyond JavaScript's integer precision", () => {
    for (const value of ["0", "1", "9007199254740993", "9223372036854775807"]) {
      expect(ConversationCursorSchema.parse(value)).toBe(value);
    }
  });

  it("rejects ambiguous, malformed and overflowing representations", () => {
    for (const value of [
      "",
      "01",
      "-1",
      "+1",
      "1e3",
      "1.0",
      "0 OR 1=1",
      "9223372036854775808",
    ]) {
      expect(ConversationCursorSchema.safeParse(value).success).toBe(false);
    }
  });
});

describe("handoff preview inputs", () => {
  it("keeps editable context separate from the current request", () => {
    expect(
      HandoffPreviewRequestSchema.parse({
        clientRequestId: "preview-123",
        configId: "cfg",
        userText: "new task",
      }).userText,
    ).toBe("new task");
    expect(
      HandoffEditRequestSchema.parse({ revision: 2, text: "manual history" }),
    ).toEqual({ revision: 2, text: "manual history" });
  });
  it("rejects invalid optimistic revisions and missing command IDs", () => {
    expect(
      HandoffEditRequestSchema.safeParse({ revision: 0, text: "text" }).success,
    ).toBe(false);
    expect(
      HandoffPreviewRequestSchema.safeParse({
        configId: "cfg",
        userText: "task",
      }).success,
    ).toBe(false);
  });
});

it("requires an explicit delivery revision and retry decision", () => {
  const input = {
    clientRequestId: "message-command",
    handoffId: "handoff",
    revision: 1,
    text: "new request",
    configId: "config",
    agentConfig: {},
    location: {
      cwd: "",
      repositoryId: "",
      worktreeId: "",
      newWorktreeBase: "",
      newWorktreeBranch: "",
    },
    attachments: [],
    retry: false,
  };
  expect(ConversationMessageRequestSchema.safeParse(input).success).toBe(true);
  expect(
    ConversationMessageRequestSchema.safeParse({ ...input, revision: 0 })
      .success,
  ).toBe(false);
  expect(
    ConversationMessageRequestSchema.safeParse({ ...input, retry: undefined })
      .success,
  ).toBe(false);
});

it("reads bounded selection pages without truncating global counts", () => {
  const item = {
    activityId: "activity",
    position: "9007199254740993",
    selected: false,
    reason: "manual_context",
  };
  const page = {
    sourceLocation: {
      cwd: "/source",
      repositoryId: "",
      worktreeId: "",
      newWorktreeBase: "",
      newWorktreeBranch: "",
    },
    sourceConversationId: "source",
    sourcePosition: "9007199254740993",
    revision: 2,
    historyOrigin: "complete",
    reconcileRequired: false,
    manual: true,
    selectedCount: 0,
    omittedCount: 40000,
    items: [item],
    nextPosition: item.position,
    done: false,
  };
  expect(HandoffSelectionSchema.parse(page).omittedCount).toBe(40000);
  expect(
    HandoffSelectionSchema.safeParse({
      ...page,
      items: Array.from({ length: 101 }, () => item),
    }).success,
  ).toBe(false);
});
