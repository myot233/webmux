import type { Meta, StoryObj } from "@storybook/svelte-vite";
import BranchOperations from "./BranchOperations.svelte";

const meta = {
  title: "Workspace/Branch operations",
  component: BranchOperations,
} satisfies Meta<typeof BranchOperations>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FastForward: Story = { args: { flow: "fast-forward" } };
export const FastForwardBlocked: Story = {
  args: { flow: "fast-forward-blocked" },
};
export const Publish: Story = { args: { flow: "publish" } };
export const PushBlocked: Story = { args: { flow: "push-blocked" } };
export const DeleteGroups: Story = { args: { flow: "delete" } };
