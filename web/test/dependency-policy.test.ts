import { spawnSync } from "node:child_process";
import {
  mkdtempSync,
  mkdirSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { ICruiseResult } from "dependency-cruiser";
import { describe, expect, test } from "vitest";

const root = fileURLToPath(new URL("..", import.meta.url));
const cli = resolve(
  root,
  "node_modules/dependency-cruiser/bin/dependency-cruiser.mjs",
);
const config = resolve(root, ".dependency-cruiser.cjs");

function violations(
  from: string,
  to: string,
  options: { typeOnly?: boolean; cycle?: boolean; missing?: boolean } = {},
): string[] {
  const directory = mkdtempSync(join(tmpdir(), "webmux-dependencies-"));
  function put(file: string, source: string) {
    const target = join(directory, file);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, source);
  }
  function specifier(source: string, target: string) {
    const path = relative(dirname(source), target).replaceAll("\\", "/");
    return path.startsWith(".") ? path : `./${path}`;
  }
  try {
    // Svelte compilation emits runtime imports; resolve the real installed package.
    symlinkSync(
      resolve(root, "node_modules"),
      join(directory, "node_modules"),
      "dir",
    );
    put(
      "tsconfig.json",
      JSON.stringify({ compilerOptions: { module: "esnext" } }),
    );
    // A Svelte-only fixture still needs one compiler-recognized input.
    put("src/app.d.ts", "export {};");
    const statement = options.typeOnly
      ? `import type { Marker } from ${JSON.stringify(specifier(from, to))};\nexport type Copy = Marker;`
      : `import ${JSON.stringify(specifier(from, to))};`;
    put(
      from,
      from.endsWith(".svelte")
        ? `<script lang="ts">${statement}</script><div>Fixture</div>`
        : statement,
    );
    if (!options.missing) {
      put(
        to,
        to.endsWith(".svelte")
          ? "<div>Fixture</div>"
          : options.cycle
            ? `import ${JSON.stringify(specifier(to, from))};`
            : "export interface Marker { value: string }",
      );
    }
    const result = spawnSync(
      process.execPath,
      [cli, "src", "--config", config, "--output-type", "json"],
      { cwd: directory, encoding: "utf8", timeout: 20_000 },
    );
    if (result.error) throw result.error;
    expect(result.stderr, "dependency-cruiser must parse the fixture").toBe("");
    expect([0, 1]).toContain(result.status);
    const graph = JSON.parse(result.stdout) as ICruiseResult;
    return graph.summary.violations.map((violation) => violation.rule.name);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

describe("dependency boundary policy", () => {
  test.each([
    [
      "src/contracts/wire.ts",
      "src/platform/api.ts",
      "contracts-are-independent",
    ],
    [
      "src/platform/api.ts",
      "src/domains/sessions/state.ts",
      "platform-does-not-know-domains",
    ],
    [
      "src/platform/api.ts",
      "src/lib/components/ui/button/Button.svelte",
      "platform-does-not-render",
    ],
    ["src/lib/utils.ts", "src/platform/api.ts", "lib-is-framework-free"],
    [
      "src/lib/components/ui/button/index.ts",
      "src/domains/settings/state.ts",
      "ui-is-a-leaf",
    ],
    [
      "src/domains/sessions/model.ts",
      "src/hooks/sessions/controller.ts",
      "domain-does-not-render",
    ],
    [
      "src/domains/sessions/model.ts",
      "src/app/bootstrap.ts",
      "domain-does-not-render",
    ],
    [
      "src/domains/sessions/model.ts",
      "src/components/sessions/Session.svelte",
      "domain-does-not-render",
    ],
    [
      "src/domains/terminal/model.ts",
      "src/lib/components/ui/button/Button.svelte",
      "domain-does-not-render",
    ],
    [
      "src/hooks/sessions/controller.ts",
      "src/app/bootstrap.ts",
      "hooks-do-not-know-composition",
    ],
    [
      "src/hooks/sessions/controller.ts",
      "src/components/sessions/Session.svelte",
      "hooks-do-not-render",
    ],
    [
      "src/components/shared/widget.ts",
      "src/app/bootstrap.ts",
      "components-do-not-know-app",
    ],
    [
      "src/components/shared/widget.ts",
      "src/pages/Settings.svelte",
      "components-do-not-know-pages",
    ],
    [
      "src/domains/settings/model.ts",
      "src/pages/Settings.svelte",
      "only-app-composes-pages",
    ],
  ])("rejects %s → %s (%s)", (from, to, rule) => {
    expect(violations(from, to)).toContain(rule);
  });

  test.each([
    ["settings", "sessions"],
    ["sessions", "workspace"],
    ["workspace", "terminal"],
    ["terminal", "code"],
    ["notifications", "workspace"],
    ["code", "chat"],
    ["chat", "sessions"],
  ])("rejects unapproved topic coupling %s → %s", (from, to) => {
    expect(
      violations(`src/domains/${from}/model.ts`, `src/domains/${to}/model.ts`),
    ).toContain(`${from}-dependency-boundary`);
  });

  test.each([
    ["src/platform/api.ts", "src/contracts/wire.ts"],
    ["src/domains/sessions/model.ts", "src/platform/api.ts"],
    ["src/domains/workspace/model.ts", "src/domains/sessions/model.ts"],
    [
      "src/hooks/sessions/controller.ts",
      "src/domains/sessions/state.svelte.ts",
    ],
    ["src/components/shared/widget.ts", "src/hooks/shared/viewport.svelte.ts"],
    ["src/App.svelte", "src/pages/Settings.svelte"],
  ])("allows downward composition %s → %s", (from, to) => {
    expect(violations(from, to)).toEqual([]);
  });

  test("rejects upward dependencies even when they are type-only", () => {
    expect(
      violations("src/contracts/wire.ts", "src/domains/sessions/model.ts", {
        typeOnly: true,
      }),
    ).toContain("contracts-are-independent");
  });

  test("rejects cycles between domain topics", () => {
    expect(
      violations(
        "src/domains/sessions/model.ts",
        "src/domains/workspace/model.ts",
        {
          cycle: true,
        },
      ),
    ).toContain("no-circular");
  });

  test("rejects stale imports after a move", () => {
    expect(
      violations("src/domains/sessions/model.ts", "src/session-schemas.ts", {
        missing: true,
      }),
    ).toContain("no-unresolved");
  });
});
