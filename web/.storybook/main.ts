import type { StorybookConfig } from "@storybook/svelte-vite";

const config: StorybookConfig = {
  stories: ["../stories/**/*.stories.ts"],
  framework: "@storybook/svelte-vite",
  core: { disableTelemetry: true },
  async viteFinal(config) {
    // Reuse the application's Svelte/Tailwind/alias setup, never its live API proxy.
    return {
      ...config,
      server: { ...config.server, proxy: undefined },
      preview: { ...config.preview, proxy: undefined },
    };
  },
};

export default config;
