import { z } from "zod";
import { store, stored } from "../../platform/api";
import {
  ChatAttachmentSchema,
  HandoffLocationSchema,
  type HandoffPreview,
  type HandoffDraft,
} from "../../contracts/schemas";

const ChoiceSchema = z.object({
  locationKind: z
    .enum(["directory", "worktree", "new-worktree"])
    .default("directory"),
  configId: z.string(),
  model: z.string(),
  task: z.string().max(1 << 20),
  location: HandoffLocationSchema,
  attachments: z.array(ChatAttachmentSchema).max(64).default([]),
});
const RecoverySchema = z.object({
  handoffId: z.string(),
  revision: z.number().int().positive(),
  task: z.string().max(1 << 20),
  // Only unsaved user edits are backed up. Never cache unedited history.
  context: z
    .string()
    .max(1 << 20)
    .optional(),
});
export type HandoffChoice = z.infer<typeof ChoiceSchema>;
export type HandoffRecovery = z.infer<typeof RecoverySchema>;
export const emptyLocation = () => ({
  cwd: "",
  repositoryId: "",
  worktreeId: "",
  newWorktreeBase: "",
  newWorktreeBranch: "",
});

export function handoffChoicesDiffer(
  preview: HandoffPreview | null,
  configId: string,
  model: string,
  attachments: HandoffChoice["attachments"],
  location: HandoffChoice["location"],
): boolean {
  return (
    preview !== null &&
    (configId !== preview.configId ||
      model !== (preview.agentConfig.model ?? "") ||
      JSON.stringify(attachments) !== JSON.stringify(preview.attachments) ||
      JSON.stringify(location) !== JSON.stringify(preview.location))
  );
}

function read<T extends z.ZodType>(key: string, schema: T): z.infer<T> | null {
  try {
    return schema.parse(JSON.parse(stored(key)));
  } catch {
    return null;
  }
}
export function handoffDraftStorage(source: string, fork: boolean) {
  const key = `handoff.${source}.${fork ? "fork" : "switch"}`;
  return {
    choice: () => read(`${key}.choice`, ChoiceSchema),
    saveChoice: (value: HandoffChoice) =>
      store(`${key}.choice`, JSON.stringify(ChoiceSchema.parse(value))),
    recovery: () => read(`${key}.recovery`, RecoverySchema),
    saveRecovery: (value: HandoffRecovery | null) =>
      store(
        `${key}.recovery`,
        value === null ? null : JSON.stringify(RecoverySchema.parse(value)),
      ),
    clear: () => {
      store(`${key}.choice`, null);
      store(`${key}.recovery`, null);
    },
  };
}

export function rememberHandoffDraft(
  storage: ReturnType<typeof handoffDraftStorage>,
  snapshot: HandoffChoice & {
    orphan: boolean;
    unpreviewed: boolean;
    recovery: HandoffRecovery | null;
    preview: HandoffPreview | null;
    context: string;
    dirty: boolean;
    choiceChanged: boolean;
    conflict: boolean;
  },
) {
  const {
    task,
    configId,
    model,
    location,
    locationKind,
    attachments,
    orphan,
    unpreviewed,
    recovery,
    preview,
    context,
    dirty,
    choiceChanged,
    conflict,
  } = snapshot;
  if (task.length <= 1 << 20)
    storage.saveChoice({
      configId,
      model,
      task,
      location,
      locationKind,
      attachments,
    });
  if (orphan && recovery) {
    if (task.length > 1 << 20 || context.length > 1 << 20) return;
    storage.saveRecovery({
      ...recovery,
      task,
      ...(recovery.context !== undefined ? { context } : {}),
    });
    return;
  }
  // Pre-preview choices have no history/revision of their own. Preserve their
  // choice backup without claiming another device's preview as local recovery.
  if (
    preview &&
    !unpreviewed &&
    (dirty || choiceChanged || conflict) &&
    task.length <= 1 << 20 &&
    context.length <= 1 << 20
  )
    storage.saveRecovery({
      handoffId: preview.id,
      // Unresolved edits retain their original base, not the remote preview's.
      revision:
        conflict && recovery?.handoffId === preview.id
          ? recovery.revision
          : preview.revision,
      task,
      ...(context !== preview.renderedText || (conflict && preview.manual)
        ? { context }
        : {}),
    });
  else if (!dirty && !choiceChanged) storage.saveRecovery(null);
}

export function handoffLocationKind(
  location: HandoffChoice["location"],
): HandoffChoice["locationKind"] {
  return location.newWorktreeBranch
    ? "new-worktree"
    : location.repositoryId
      ? "worktree"
      : "directory";
}

export function handoffSourceLocation(
  source: HandoffDraft["source"],
): HandoffChoice["location"] {
  return {
    ...emptyLocation(),
    cwd: source.cwd,
    repositoryId: source.repositoryId,
    worktreeId: source.worktreeId,
  };
}

export function hasConflictingHandoffWork(
  choice: HandoffChoice | null,
  draft: HandoffDraft,
): boolean {
  if (!choice || !draft.preview) return false;
  const sourceLocation = handoffSourceLocation(draft.source);
  const meaningful =
    choice.configId ||
    choice.model ||
    choice.task ||
    choice.attachments.length ||
    choice.locationKind !== handoffLocationKind(sourceLocation) ||
    JSON.stringify(choice.location) !== JSON.stringify(sourceLocation);
  const preview = draft.preview;
  return !!(
    meaningful &&
    (choice.task !== preview.userText ||
      handoffChoicesDiffer(
        preview,
        choice.configId,
        choice.model,
        choice.attachments,
        choice.location,
      ) ||
      choice.locationKind !== handoffLocationKind(preview.location))
  );
}
