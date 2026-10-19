import type { Meta, StoryObj } from "@storybook/svelte-vite";
import OverflowScenarios from "./OverflowScenarios.svelte";
import { longError } from "./fixtures";

const meta = {
  title: "UI/Overflow",
  component: OverflowScenarios,
  tags: ["overflow"],
} satisfies Meta<typeof OverflowScenarios>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SessionRow: Story = { args: { scenario: "session" } };
export const WorkspaceGroup: Story = { args: { scenario: "workspace" } };
export const QuickCommand: Story = { args: { scenario: "command" } };
export const QuickCommandError: Story = {
  args: { scenario: "command", saveError: longError },
};
export const CloseSession: Story = { args: { scenario: "close" } };
export const DraftComposer: Story = { args: { scenario: "composer" } };
export const CodeChanges: Story = { args: { scenario: "changes" } };
export const WorkspaceDialog: Story = {
  args: { scenario: "workspace-dialog" },
};
