import type { Meta, StoryObj } from "@storybook/svelte-vite";
import CodeDiff from "./CodeDiff.svelte";

const meta = {
  title: "Workspace/Code diff",
  component: CodeDiff,
} satisfies Meta<typeof CodeDiff>;

export default meta;
type Story = StoryObj<typeof meta>;
export const Review: Story = {};
