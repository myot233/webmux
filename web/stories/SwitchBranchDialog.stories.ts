import type { Meta, StoryObj } from "@storybook/svelte-vite";
import SwitchBranchDialog from "./SwitchBranchDialog.svelte";

const meta = {
  title: "Workspace/Switch branch",
  component: SwitchBranchDialog,
} satisfies Meta<typeof SwitchBranchDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Blockers: Story = { args: { scenario: "blockers" } };
export const Sessions: Story = { args: { scenario: "sessions" } };
export const LongNames: Story = { args: { scenario: "long-names" } };
