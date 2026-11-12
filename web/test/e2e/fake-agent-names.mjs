import { readFileSync } from "node:fs";

export const agentNames = readFileSync(
  new URL("./fake-agent-names.txt", import.meta.url),
  "utf8",
)
  .trim()
  .split(/\s+/);
