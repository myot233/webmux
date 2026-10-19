export const stories = [
  "session-row",
  "workspace-group",
  "quick-command",
  "quick-command-error",
  "close-session",
  "draft-composer",
  "code-changes",
  "workspace-dialog",
] as const;

export type Story = (typeof stories)[number];
