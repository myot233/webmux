import type { Meta, StoryObj } from "@storybook/svelte-vite";
import DesignSystem from "./DesignSystem.svelte";

const meta = {
  title: "Design system",
  component: DesignSystem,
} satisfies Meta<typeof DesignSystem>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Foundations: Story = { args: { section: "foundations" } };
export const Actions: Story = { args: { section: "actions" } };
export const Forms: Story = { args: { section: "forms" } };
export const Feedback: Story = { args: { section: "feedback" } };
export const Surfaces: Story = { args: { section: "surfaces" } };
export const Overlays: Story = { args: { section: "overlays" } };
export const SessionStates: Story = { args: { section: "sessions" } };
