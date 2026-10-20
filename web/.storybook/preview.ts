import type { Preview } from "@storybook/svelte-vite";
import "../src/styles.css";

const preview: Preview = {
  parameters: { layout: "fullscreen" },
  initialGlobals: { theme: "light" },
  globalTypes: {
    theme: {
      description: "Application theme",
      toolbar: { icon: "circlehollow", items: ["light", "dark"] },
    },
  },
  decorators: [
    (story, { globals }) => {
      document.documentElement.classList.toggle(
        "dark",
        globals.theme === "dark",
      );
      return story();
    },
  ],
};

export default preview;
