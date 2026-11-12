import type { Meta, StoryObj } from "@storybook/svelte-vite";
import RepositoryPanel from "./RepositoryPanel.svelte";

const meta = {
  title: "Workspace/Repository panel",
  component: RepositoryPanel,
} satisfies Meta<typeof RepositoryPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Overview: Story = { args: { tab: "overview" } };
export const Worktrees: Story = { args: { tab: "worktrees" } };
export const Branches: Story = { args: { tab: "branches" } };
export const Remotes: Story = { args: { tab: "remotes" } };
export const WorktreeSync: Story = { args: { sync: true } };
export const Fetching: Story = { args: { fetchReply: "pending" } };
export const FetchPartial: Story = { args: { fetchReply: "partial" } };
