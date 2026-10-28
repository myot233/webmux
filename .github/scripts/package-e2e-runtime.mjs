import { execFileSync } from "node:child_process";
import { copyFile, cp, mkdir, realpath } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";

const destination = resolve(process.argv[2]);
const web = resolve("web");
const playwright = await realpath(join(web, "node_modules/playwright"));
const require = createRequire(join(playwright, "package.json"));
await mkdir(join(destination, "web/node_modules/@playwright"), {
  recursive: true,
});
// Playwright's runner has only these three runtime packages. Copy the exact
// versions installed by the frozen pnpm lockfile, following pnpm's links.
for (const name of ["@playwright/test", "playwright", "playwright-core"]) {
  await cp(
    name === "playwright-core"
      ? dirname(require.resolve("playwright-core/package.json"))
      : await realpath(join(web, "node_modules", name)),
    join(destination, "web/node_modules", name),
    { recursive: true, dereference: true },
  );
}
await cp(join(web, "dist"), join(destination, "web/dist"), {
  recursive: true,
});

// Each runner's shards get the backend and, in agents/ beside it, the fake
// agent under the names agentprofile recognises (as `make e2e-runtime`
// lays out .cache/webmux-e2e), so they need no Go toolchain.
function build(platform, arch, output, pkg) {
  execFileSync("go", ["build", "-trimpath", "-o", output, pkg], {
    env: { ...process.env, CGO_ENABLED: "0", GOOS: platform, GOARCH: arch },
    stdio: "inherit",
  });
}
for (const [platform, arch] of [
  ["linux", "amd64"],
  ["darwin", "arm64"],
]) {
  const runtime = join(destination, ".ci-runtime", platform);
  const agents = join(runtime, "agents");
  await mkdir(agents, { recursive: true });
  build(platform, arch, join(runtime, "webmux"), "./cmd/webmux");
  build(
    platform,
    arch,
    join(agents, "omp"),
    "./internal/acp/testdata/fakeagent",
  );
  await copyFile(join(agents, "omp"), join(agents, "opencode"));
}
