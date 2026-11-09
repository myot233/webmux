import type { Meta, StoryObj } from "@storybook/svelte-vite";
import UsageUpdates from "./UsageUpdates.svelte";
const meta = {
  title: "Chat/Usage updates",
  component: UsageUpdates,
} satisfies Meta<typeof UsageUpdates>;
export default meta;
export const Reports: StoryObj<typeof meta> = {};
