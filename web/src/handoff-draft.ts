import { z } from "zod";
import { store, stored } from "./api";
import { ChatAttachmentSchema, HandoffLocationSchema } from "./schemas";

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
