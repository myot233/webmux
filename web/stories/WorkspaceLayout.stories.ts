import type { Meta, StoryObj } from "@storybook/svelte-vite";
import WorkspaceLayout from "./WorkspaceLayout.svelte";

const meta = {
  title: "Workspace/Flat layout",
  component: WorkspaceLayout,
} satisfies Meta<typeof WorkspaceLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SingleSession: Story = { args: { multiple: false } };
export const MultipleSessions: Story = { args: { multiple: true } };
export const WorktreeActions: Story = { args: { withWorktree: true } };
