import type { Meta, StoryObj } from "@storybook/svelte-vite";
import HybridSession from "./acp/HybridSession.svelte";

// The session page of a hybrid session (issue #181): the chat view in the
// pane the terminal fills otherwise, the side switch in the header, switch
// progress and the D21 choice. Sample chat frames only: no ACP process, PTY
// or API request is involved.
const meta = {
  title: "Workspace/Hybrid session",
  component: HybridSession,
} satisfies Meta<typeof HybridSession>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Conversation: Story = { args: { scenario: "conversation" } };
export const LongConversation: Story = {
  args: { scenario: "long-conversation" },
};
export const PermissionRequest: Story = { args: { scenario: "permission" } };
export const CommandMenu: Story = { args: { scenario: "commands" } };
export const Uploading: Story = {
  args: { scenario: "conversation", uploading: true },
};
export const CancelledTurn: Story = { args: { scenario: "cancelled" } };
export const Compaction: Story = { args: { scenario: "compaction" } };
export const OpenCode: Story = {
  args: { scenario: "conversation", agent: "opencode" },
};
export const Switching: Story = { args: { scenario: "switching" } };
export const AwaitingChoice: Story = { args: { scenario: "awaiting-choice" } };
export const TerminalSide: Story = { args: { scenario: "terminal" } };
export const ConnectorMissing: Story = {
  args: { scenario: "connector-missing" },
};

// Claude Code uses the same generic states and components as every profile.
// These fixtures use its stable agent ID, executable and connector metadata.
export const ClaudeCode: Story = {
  args: { scenario: "conversation", agent: "claudecode" },
};
export const ClaudeConnectorMissing: Story = {
  args: { scenario: "connector-recheck", agent: "claudecode" },
};
export const ClaudeFirstMessage: Story = {
  args: { scenario: "first-message", agent: "claudecode" },
};
export const ClaudeAmbiguous: Story = {
  args: { scenario: "ambiguous", agent: "claudecode" },
};
export const ClaudeOccupied: Story = {
  args: { scenario: "occupied", agent: "claudecode" },
};
export const ClaudeModeFallback: Story = {
  args: { scenario: "mode-fallback", agent: "claudecode" },
};
export const ClaudeCommands: Story = {
  args: { scenario: "commands", agent: "claudecode" },
};
export const ClaudeRecoveryFailure: Story = {
  args: { scenario: "recovery-failure", agent: "claudecode" },
};
