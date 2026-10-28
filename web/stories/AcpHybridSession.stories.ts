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
