import type { Meta, StoryObj } from "@storybook/svelte-vite";
import HybridHome from "./acp/HybridHome.svelte";

// Hybrid sessions on the home page's workspace group (issue #181).
const meta = {
  title: "Workspace/Hybrid home",
  component: HybridHome,
} satisfies Meta<typeof HybridHome>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SessionCards: Story = {};
