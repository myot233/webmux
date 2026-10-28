import {
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import globalSetup from "./e2e/global-setup.mjs";

describe("prepared E2E runtime", () => {
  let root: string;
  const contents = "#!/bin/sh\nexit 0\n";

  // The backend and, in agents/ beside it, the fake agent as omp and opencode.
  async function runtime(agents = ["omp", "opencode"]) {
    const binary = join(root, "webmux");
    await writeFile(binary, contents, { mode: 0o700 });
    await mkdir(join(root, "agents"));
    for (const name of agents)
      await writeFile(join(root, "agents", name), contents, { mode: 0o700 });
    vi.stubEnv("WEBMUX_E2E_BINARY", binary);
    return binary;
  }

  beforeEach(async () => {
    root = await mkdtemp(join(tmpdir(), "webmux-prepared-e2e-"));
    vi.stubEnv("WEBMUX_E2E_DIST", join(root, "missing-dist"));
    vi.stubEnv("WEBMUX_E2E_AGENTS", undefined);
  });

  afterEach(async () => {
    vi.unstubAllEnvs();
    await rm(root, { recursive: true, force: true });
  });

  test("uses the supplied executable and its fake agents without building or taking cleanup ownership", async () => {
    const binary = await runtime();

    const teardown = await globalSetup();

    expect(process.env.WEBMUX_E2E_BINARY).toBe(await realpath(binary));
    expect(process.env.WEBMUX_E2E_AGENTS).toBe(
      await realpath(join(root, "agents")),
    );
    expect(teardown).toBeUndefined();
    expect(await readFile(binary, "utf8")).toBe(contents);
  });

  test.each(["missing", "directory", "non-executable"])(
    "rejects a %s runtime instead of silently rebuilding",
    async (kind) => {
      const binary = kind === "directory" ? root : join(root, "webmux");
      if (kind === "non-executable")
        await writeFile(binary, "not executable", { mode: 0o600 });
      vi.stubEnv("WEBMUX_E2E_BINARY", binary);

      await expect(globalSetup()).rejects.toThrow("Invalid WEBMUX_E2E_BINARY");
    },
  );

  test("rejects a runtime without both fake agents", async () => {
    await runtime(["omp"]);

    await expect(globalSetup()).rejects.toThrow(
      "Missing fake agents for WEBMUX_E2E_BINARY",
    );
  });
});
