import type { Meta, StoryObj } from "@storybook/svelte-vite";
import ComposerOptions from "./ComposerOptions.svelte";

export default {
  title: "Chat/Composer options",
  component: ComposerOptions,
} satisfies Meta<typeof ComposerOptions>;
export const Overflow: StoryObj<typeof ComposerOptions> = {};
