import { z } from "zod";
import { api, APIError } from "../../platform/api";
import {
  remoteBranchBases,
  suggestWorktreeBranch,
} from "../../domains/workspace/branch-name";
import {
  activeHostId,
  isStoredMissing,
  store,
  stored,
} from "../../platform/storage";
import type { ConfigValue } from "../../platform/chat-api";
import { newRequestId } from "../../platform/ids";
import { selectableWorktree } from "../../domains/sessions/new-session-defaults";
import {
  BranchesSchema,
  ChatAttachmentSchema,
  ChatPermissionModeSchema,
  CreateSessionFailureSchema,
  CreateSessionRequestSchema,
  CreateSessionResultSchema,
  type ChatAttachment,
  type ChatConfig,
  type ChatPermissionMode,
  type CreateSessionFailure,
  type CreateSessionRequest,
  type Repository,
  type SessionView,
  type Worktree,
} from "../../contracts/schemas";

// Quick start's memory on this device: the repository and agent of the last
// successful start, and the unsent composer state. Both are validated on read
// and degrade to nothing when unreadable.

const TargetModeSchema = z.enum(["new", "existing"]);
export type QuickStartTargetMode = z.infer<typeof TargetModeSchema>;

const WorktreeChoiceSchema = z.object({
  repositoryId: z.string().min(1).max(256),
  worktreeId: z.string().min(1).max(256),
});

const ChoiceSchema = z.object({
  repositoryId: z.string().min(1).max(256),
  configId: z.string().regex(/^[a-z2-7]{8}$/),
  targetMode: TargetModeSchema.optional(),
  worktreeChoice: WorktreeChoiceSchema.optional(),
});
export type QuickStartChoice = z.infer<typeof ChoiceSchema>;

const ConfigValueSchema: z.ZodType<ConfigValue> = z.union([
  z.object({ value: z.string().max(1024) }),
  z.object({ enabled: z.boolean() }),
]);

const BaseChoiceSchema = z.object({
  repositoryId: z.string().min(1).max(256),
  name: z.string().min(1).max(256),
});

const DraftSchema = z.object({
  text: z.string().max(100_000).default(""),
  attachments: z.array(ChatAttachmentSchema).max(8).optional(),
  repositoryId: z.string().min(1).max(256).optional(),
  configId: z
    .string()
    .regex(/^[a-z2-7]{8}$/)
    .optional(),
  targetMode: TargetModeSchema.optional(),
  baseChoice: BaseChoiceSchema.optional(),
  worktreeChoice: WorktreeChoiceSchema.optional(),
  permissionMode: ChatPermissionModeSchema.optional(),
  edits: z
    .record(z.string(), z.record(z.string(), ConfigValueSchema))
    .optional(),
  instructionChoice: z
    .object({
      repositoryId: z.string().min(1).max(256),
      use: z.boolean(),
    })
    .optional(),
});
export type QuickStartDraft = {
  text: string;
  attachments?: ChatAttachment[];
  repositoryId?: string;
  configId?: string;
  targetMode?: QuickStartTargetMode;
  baseChoice?: { repositoryId: string; name: string };
  worktreeChoice?: { repositoryId: string; worktreeId: string };
  permissionMode?: ChatPermissionMode;
  edits?: Record<string, Record<string, ConfigValue>>;
  instructionChoice?: { repositoryId: string; use: boolean };
};

const memoryDrafts = new Map<string, QuickStartDraft>();
const memoryKey = () => activeHostId() ?? "";

function readJson<T extends z.ZodType>(key: string, schema: T) {
  try {
    return schema.safeParse(JSON.parse(stored(key, "null"))).data;
  } catch {
    return undefined;
  }
}

function emptyDraft(draft: QuickStartDraft) {
  return (
    !draft.text &&
    !draft.attachments?.length &&
    !draft.repositoryId &&
    !draft.configId &&
    !draft.targetMode &&
    !draft.baseChoice &&
    !draft.worktreeChoice &&
    (!draft.permissionMode || draft.permissionMode === "auto") &&
    (!draft.edits || Object.keys(draft.edits).length === 0) &&
    !draft.instructionChoice
  );
}

export const quickStartMemory = {
  choice: (): QuickStartChoice | undefined =>
    readJson("quickStart", ChoiceSchema),
  rememberChoice(choice: QuickStartChoice) {
    store("quickStart", JSON.stringify(choice));
  },
  loadDraft(): QuickStartDraft {
    const scope = memoryKey();
    const raw = stored("quick-start-draft", "");
    if (!raw) {
      // If storage was cleared externally (e.g. between tests), drop the
      // fallback only when localStorage is reachable and empty.
      if (isStoredMissing("quick-start-draft")) memoryDrafts.delete(scope);
      return memoryDrafts.get(scope) ?? { text: "" };
    }
    try {
      const parsed = DraftSchema.safeParse(JSON.parse(raw));
      return parsed.success ? parsed.data : { text: "" };
    } catch {
      return { text: "" };
    }
  },
  draft: (): string => quickStartMemory.loadDraft().text,
  saveDraftState(draft: QuickStartDraft) {
    if (emptyDraft(draft)) {
      quickStartMemory.clearDraft();
      return;
    }
    memoryDrafts.set(memoryKey(), draft);
    store("quick-start-draft", JSON.stringify(draft));
  },
  saveDraft(text: string) {
    quickStartMemory.saveDraftState({
      ...quickStartMemory.loadDraft(),
      text,
    });
  },
  clearDraft() {
    memoryDrafts.delete(memoryKey());
    store("quick-start-draft", null);
  },
};

/**
 * The remembered item when it is still offered and usable, else the first
 * usable one; undefined when none is.
 */
export function pickDefault<T extends { id: string }>(
  items: readonly T[],
  remembered: string | undefined,
  usable: (item: T) => boolean,
): T | undefined {
  const kept = items.find((item) => item.id === remembered);
  return kept && usable(kept) ? kept : items.find(usable);
}

/** The values the page shows for each option, as the request sends them. */
function configValues(options: readonly ChatConfig[]) {
  return Object.fromEntries(
    options.flatMap((option) =>
      option.type === "boolean"
        ? option.enabled === undefined
          ? []
          : [[option.id, String(option.enabled)]]
        : option.value === undefined
          ? []
          : [[option.id, option.value]],
    ),
  );
}

type QuickStartOutcome =
  | { ok: true; sessionId: string }
  | { ok: false; failure: CreateSessionFailure };

/**
 * Sends one quick start: a session created in a new worktree with its first
 * message. A dropped connection is retried once with the same request ID,
 * which the server answers with the first run's result.
 */
async function sendQuickStart(
  request: Omit<CreateSessionRequest, "clientRequestId">,
  clientRequestId = newRequestId(),
): Promise<QuickStartOutcome> {
  const body = { ...request, clientRequestId };
  for (let attempt = 0; ; attempt++) {
    try {
      const result = await api(
        "/sessions",
        CreateSessionResultSchema,
        "POST",
        body,
        undefined,
        CreateSessionRequestSchema,
      );
      return { ok: true, sessionId: result.session.id };
    } catch (err) {
      if (err instanceof TypeError && attempt === 0) continue;
      const failure =
        err instanceof APIError
          ? CreateSessionFailureSchema.safeParse(err.body).data
          : undefined;
      return {
        ok: false,
        failure: failure ?? {
          error: err instanceof Error ? err.message : String(err),
          step: "precheck",
        },
      };
    }
  }
}

export const failedStep: Record<CreateSessionFailure["step"], string> = {
  precheck: "无法开始",
  worktree: "创建 worktree 失败",
  session: "启动 agent 失败",
  config: "应用配置失败",
  prompt: "发送首条消息失败",
};

export type QuickStartBranches = z.infer<typeof BranchesSchema>;

export function saveQuickStartDraftFromPage(params: {
  text: string;
  attachments: ChatAttachment[];
  repositoryChoice?: string;
  agentChoice?: string;
  targetModeChoice?: QuickStartTargetMode;
  baseChoice?: { repositoryId: string; name: string };
  worktreeChoice?: { repositoryId: string; worktreeId: string };
  permissionMode: ChatPermissionMode;
  edits: Record<string, Record<string, ConfigValue>>;
  instructionChoice?: { repositoryId: string; use: boolean };
  remembered?: QuickStartChoice;
}) {
  const {
    text,
    attachments,
    repositoryChoice,
    agentChoice,
    targetModeChoice,
    baseChoice,
    worktreeChoice,
    permissionMode,
    edits,
    instructionChoice,
    remembered,
  } = params;
  const edited = Object.keys(edits).length ? edits : undefined;
  const rememberedMode = remembered?.targetMode ?? "new";
  quickStartMemory.saveDraftState({
    text,
    ...(attachments.length ? { attachments } : {}),
    ...(repositoryChoice && repositoryChoice !== remembered?.repositoryId
      ? { repositoryId: repositoryChoice }
      : {}),
    ...(agentChoice && agentChoice !== remembered?.configId
      ? { configId: agentChoice }
      : {}),
    ...(targetModeChoice && targetModeChoice !== rememberedMode
      ? { targetMode: targetModeChoice }
      : {}),
    ...(baseChoice ? { baseChoice } : {}),
    ...(worktreeChoice &&
    (worktreeChoice.repositoryId !== remembered?.worktreeChoice?.repositoryId ||
      worktreeChoice.worktreeId !== remembered?.worktreeChoice?.worktreeId)
      ? { worktreeChoice }
      : {}),
    ...(permissionMode !== "auto" ? { permissionMode } : {}),
    ...(edited ? { edits: edited } : {}),
    ...(instructionChoice ? { instructionChoice } : {}),
  });
}

export function resolveQuickStartBranches(
  branchData: QuickStartBranches | undefined,
  repositoryId: string | undefined,
  baseChoice: { repositoryId: string; name: string } | undefined,
) {
  if (!branchData) {
    return {
      localBranches: [] as { name: string; head: string }[],
      remoteBranches: [] as { name: string; head: string }[],
      baseName: "",
      base: "",
    };
  }
  const localBranches = branchData.branches.map(({ name, head }) => ({
    name,
    head,
  }));
  const main = branchData.defaultBranch ?? "HEAD";
  if (
    branchData.defaultBase &&
    !localBranches.some((branch) => branch.name === main)
  )
    localBranches.unshift({ name: main, head: branchData.defaultBase });
  const remoteBranches = remoteBranchBases(
    branchData.bases,
    localBranches.map((branch) => branch.name),
  ).map(({ name, head }) => ({ name, head }));
  const baseBranches = [...localBranches, ...remoteBranches];
  const baseName =
    baseChoice?.repositoryId === repositoryId &&
    baseBranches.some((branch) => branch.name === baseChoice?.name)
      ? (baseChoice?.name ?? "")
      : branchData.defaultBase
        ? (branchData.defaultBranch ?? "HEAD")
        : "";
  const base =
    baseBranches.find((branch) => branch.name === baseName)?.head ?? "";
  return { localBranches, remoteBranches, baseName, base };
}

export function resolveQuickStartWorktree(
  repository: Repository | undefined,
  worktreeChoice: { repositoryId: string; worktreeId: string } | undefined,
) {
  const worktrees = repository?.worktrees ?? [];
  const defaultWorktree =
    worktrees.find(
      (item) => item.kind === "main" && selectableWorktree(item),
    ) ?? worktrees.find(selectableWorktree);
  let selectedWorktree = defaultWorktree;
  if (worktreeChoice?.repositoryId === repository?.id) {
    const kept = worktrees.find(
      (item) =>
        item.id === worktreeChoice?.worktreeId && selectableWorktree(item),
    );
    if (kept) selectedWorktree = kept;
  }
  return { worktrees, defaultWorktree, selectedWorktree };
}

export async function executeQuickStart(params: {
  repository: Repository;
  agentId: string;
  startSide: SessionView;
  text: string;
  useInstruction: boolean;
  attachments: ChatAttachment[];
  options: readonly ChatConfig[];
  permissionMode: ChatPermissionMode;
  targetMode: QuickStartTargetMode;
  selectedWorktree: Worktree | undefined;
  branchData: QuickStartBranches | undefined;
  base: string;
  commitAttachments: (items: ChatAttachment[]) => void;
  restoreAttachments: (items: ChatAttachment[]) => void;
  onSuccess: (sessionId: string) => void;
}): Promise<CreateSessionFailure | undefined> {
  const {
    repository,
    agentId,
    startSide,
    text,
    useInstruction,
    attachments,
    options,
    permissionMode,
    targetMode,
    selectedWorktree,
    branchData,
    base,
    commitAttachments,
    restoreAttachments,
    onSuccess,
  } = params;
  const chat = startSide === "chat";
  const sharedRequest = {
    configId: agentId,
    view: startSide,
    prompt: text,
    useInitInstruction: !!repository.initInstruction && useInstruction,
    // The server fills {{dir}} and {{webmux}} in the message once the
    // session exists, as the quick command menu inserts them.
    expandPrompt: true,
    ...(attachments.length ? { attachments } : {}),
    // A terminal start takes no agent options or permission mode.
    ...(chat
      ? {
          agentConfig: configValues(options),
          permissionMode,
        }
      : {}),
  };

  if (targetMode === "existing" && selectedWorktree) {
    const outcome = await sendQuickStart({
      workspace: {
        repositoryId: repository.id,
        worktreeId: selectedWorktree.id,
      },
      ...sharedRequest,
    });
    if (outcome.ok) {
      commitAttachments(attachments);
      store("quickStartSide", startSide);
      quickStartMemory.rememberChoice({
        repositoryId: repository.id,
        configId: agentId,
        targetMode: "existing",
        worktreeChoice: {
          repositoryId: repository.id,
          worktreeId: selectedWorktree.id,
        },
      });
      quickStartMemory.clearDraft();
      onSuccess(outcome.sessionId);
      return undefined;
    }
    if (outcome.failure.step === "prompt") commitAttachments(attachments);
    else restoreAttachments(attachments);
    return outcome.failure;
  }

  const data = branchData!;
  const taken = data.branches.map((branch) => branch.name);
  const paths = repository.worktrees.map((tree) => tree.path);
  let avoid: string | undefined;
  // A branch or directory that appeared since the list was read is a
  // conflict; draw another name and send again under a new request ID.
  let lastFailure: CreateSessionFailure | undefined;
  for (let attempt = 0; attempt < 3; attempt++) {
    const branch = suggestWorktreeBranch({
      branchNames: taken,
      worktreePaths: paths,
      avoid,
    });
    const outcome = await sendQuickStart({
      newWorktree: {
        repositoryId: repository.id,
        mode: "new",
        base,
        branch,
      },
      ...sharedRequest,
    });
    if (outcome.ok) {
      commitAttachments(attachments);
      store("quickStartSide", startSide);
      quickStartMemory.rememberChoice({
        repositoryId: repository.id,
        configId: agentId,
      });
      quickStartMemory.clearDraft();
      onSuccess(outcome.sessionId);
      return undefined;
    }
    if (
      outcome.failure.step !== "worktree" ||
      outcome.failure.reason !== "conflict"
    ) {
      // Before the first prompt the files are still staged and can go
      // again; from then on they belong to the created session.
      if (outcome.failure.step === "prompt") commitAttachments(attachments);
      else restoreAttachments(attachments);
      return outcome.failure;
    }
    taken.push(branch);
    avoid = branch;
    lastFailure = outcome.failure;
  }
  restoreAttachments(attachments);
  return lastFailure;
}
