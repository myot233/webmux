import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  resolve: {
    alias: { $lib: fileURLToPath(new URL("./src/lib", import.meta.url)) },
  },
  build: { outDir: "dist", emptyOutDir: true },
  // The terminal page is lazy. Without pre-bundling, the dev server discovers
  // xterm on the first terminal visit, re-optimizes and force-reloads the page.
  optimizeDeps: {
    include: [
      "@xterm/xterm",
      "@xterm/addon-fit",
      "@xterm/addon-webgl",
      "paneforge",
      "@codemirror/state",
      "@codemirror/view",
      "@codemirror/search",
      "@codemirror/language",
      "@codemirror/lang-javascript",
      "@codemirror/lang-python",
      "@codemirror/lang-json",
      "@codemirror/lang-go",
      "@codemirror/lang-rust",
      "@codemirror/lang-html",
      "@codemirror/lang-css",
      "diff",
    ],
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:7681",
      "/ws": { target: "ws://127.0.0.1:7681", ws: true },
    },
  },
});
