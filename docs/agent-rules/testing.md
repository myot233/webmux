# Testing, QA, and UI Pull Requests

Detail moved out of `AGENTS.md`, which has a 10 KiB budget.

## Commands

```sh
make test                         # Full gate: Go race + all frontend suites
make test-fast                    # Go race + Vitest unit/browser (no Storybook/E2E/perf)
make test-unit                    # Vitest in Node
make test-browser                 # Vitest in headless Chromium
make test-e2e                     # Playwright: desktop + iPhone SE (320×568, the single phone baseline) Chromium
make cover                        # Go race + Vitest unit/browser coverage reports
make cover-go                     # Go race coverage (.cache/coverage/go.html)
make cover-web                    # Merged Vitest unit + browser coverage
make test-browsers-install        # Chromium (+ Linux libraries; may require sudo)
```

- Target a frontend test with `pnpm --dir web exec vitest run --config vitest.config.ts test/input.test.ts`.
- Target an E2E spec with `pnpm --dir web exec playwright test test/e2e/terminal.spec.ts`.
- Target a Go package with `go test -race ./internal/session`.

## Agent local validation

- Agents MUST launch independent local checks concurrently and collect every result before reporting success. Use the parallel Make targets for suites; for independent targeted commands, use concurrent tool calls or managed child processes and retain each exit status and log.
- Choose checks for the affected behavior first:
  - For Go-only edits with an existing embedded frontend, use targeted `go test -race` packages or `make test-go`.
  - For frontend unit/browser checks, use `make test-frontend`; for both stacks, use `make test-fast`.
  - Use `make test` when the full gate is needed.
- `make test`, `make test-fast` and `make test-frontend` already launch suites in parallel without a caller-supplied `-j`. `pnpm --dir web test` runs suites sequentially; prefer the Make entry points for local validation.
- Build shared outputs before starting consumers. Do not launch multiple builds writing `web/dist`, the E2E executable or Storybook output concurrently. Preserve the staged browser groups in `make test`, and run performance checks alone.
- Go already runs packages concurrently; `GO_TEST_PARALLEL` controls tests that call `t.Parallel()` within each package, not package concurrency. Do not add `t.Parallel()` indiscriminately to tests sharing process state, environment variables or filesystem fixtures.
- Keep the configured worker limits initially. If memory pressure or CPU contention requires fewer workers or serial execution, record the concrete reason in the validation report. Do not claim a speedup without a measured comparison.
- After unchanged checks pass, reuse their results; repeat them when subsequent edits, failures or unresolved concerns affect what they verified.

## What to cover

- Go tests use the standard `testing` package.
- Linux/macOS lifecycle tests cover child-process cleanup, waiting detection, and input cancellation. Subreaper reaping is Linux-only.
- macOS temp paths sit behind `/var -> /private/var`. Compare against canonical paths: packages' `TestMain` canonicalizes `TMPDIR`, and E2E uses `realpath`.
- Preserve behavioral coverage for authentication/Origin rejection, argv boundaries, replay continuity, exit ordering, cancellation, terminal input/rendering, and touch interactions.
- Coverage is reported without a hard numeric threshold:
  - Go race collects cross-package atomic statement coverage (`-coverpkg`, excluding test-only guard/fixture packages).
  - Vitest merges V8 coverage across the Node unit and headless Chromium browser suites.
  - CI publishes Markdown summaries to `$GITHUB_STEP_SUMMARY` and uploads HTML/profile artifacts.
- Chromium mobile emulation does not replace real-device iOS Safari/Android keyboard and native selection-menu checks.

## Test runtime

- Browser tests exercise real Svelte/Tailwind/xterm rendering without a backend.
- Local `make test` builds one production frontend and `.cache/webmux-e2e/webmux` once.
  - It also builds the fake ACP agent (`internal/acp/testdata/fakeagent`) under every agent and connector name in `web/test/e2e/fake-agent-names.txt`; Makefile, CI packaging and local Playwright share this list.
  - E2E/performance reuse them via `WEBMUX_E2E_BINARY`. Agents are found in `agents/` beside it.
  - Go and Vitest run in parallel first, then Storybook and E2E run in parallel. This keeps native gesture tests from competing with E2E.
  - Performance runs alone afterward to avoid CPU contention.
  - `make test-fast` runs Go and both Vitest suites in parallel while skipping Storybook, E2E and performance.
- Workers share only the immutable executables. Each has an independent backend, HOME/config/workdirs, fake-agent session stores (the agents come first on each backend's `PATH`) and dynamic ports.
  - Vitest browser scales to four workers locally, E2E to eight, and Storybook to six. CI keeps its existing worker counts. `WEBMUX_E2E_WORKERS` overrides E2E everywhere.
  - Session, process and hybrid-record cleanup is automatic.
- Do not connect tests to an existing developer service.
- Tests may run inside a WebMux session: command tests clear the inherited session marker. The few integration tests that need a genuinely external parent process skip explicitly in that environment instead of misattributing the outer session to their test server.

## CI

- CI classifies changed paths in the `Classify changes` job (`.github/scripts/classify-changes.sh`).
  - Docs-only changes run the formatting, documentation, persistence and agent/web static guards without the test suites.
  - Go-only changes skip Vitest, Storybook and performance. Web-only changes skip Go race, the Windows cross-compile and the Windows installer smoke. E2E still runs for either.
  - Go wire snapshots also select Vitest because the frontend Zod tests consume them. `test-classify-changes.sh` covers this cross-stack rule.
  - Unknown paths, `.github/**`, `Makefile`, module/lockfile changes and installer tooling force a full run. Do not use `on.pull_request.paths`: the required `Test result` check must always report.
  - The job validates the base itself, forces a full run for any `.github/` change, and executes the base revision's classifier so a PR cannot weaken the rules that skip its own tests.
  - Every push to main forces the complete suite (classification is skipped), so a merged commit normally has a full result.
    GitHub keeps at most one pending run per concurrency group, so a rapid burst of merges can still cancel an intermediate pending run; the release falls back to `full: true` for such a commit.
  - `release.yml` first runs the `verify` job (`.github/scripts/require-main-test.sh`) for the tagged commit.
    - Reuse: the commit is on main and its `push` Test run passed with no skipped job. `Checks` is skipped.
    - Fallback: not on main, no run, a failed or partial run, or the run did not finish within the wait budget. `Checks` calls `test.yml` with `full: true` itself.
    - `sign` and `publish` need one of the two to have passed, so a tag never releases without the complete suite.
  - The release `Build` job only builds, checksums and verifies the real artifacts (including the deb/rpm install check); it does not rerun Go tests, which `Go race` already covers.
  - Stable and dev release builds install NSIS through `.github/scripts/install-nsis.sh`, using an isolated HTTPS kernel.org Ubuntu source and Ubuntu's archive signing key; the runner's global sources are unchanged, and index download failures stop the build.
- CI builds the Linux E2E executable, fake agents and frontend output once in the `E2E runtime` job (`.github/scripts/package-e2e-runtime.mjs`).
  - Go race and every E2E shard depend on that job and reuse its same-run artifact, including the locked Playwright runtime.
- `.github/workflows/test.yml` runs parallel jobs:
  - Ubuntu: formatting/architecture guard/frontend build, Go race, Vitest, Storybook, single-worker 4x CPU performance, and four E2E shards (desktop and iPhone SE phone projects).
  - E2E shards reuse the executable and Playwright packages; they skip pnpm/frontend dependencies and Go setup.
  - No macOS runner: GitHub's macOS arm64 capacity left jobs queued until they failed. The Ubuntu Go job vets the darwin build (`GOOS=darwin go vet`); macOS runtime behaviour (process tree, `sysctl`, `/private/var` paths) is not tested in CI, so run `make test` on a Mac when changing it.
  - `Test result` aggregates every job, accepts intentionally `skipped` results, and cross-checks that every job the classifier selected actually succeeded.
- Every browser job installs Chromium through `.github/actions/playwright`, which caches the Linux apt packages because a slow Ubuntu mirror can otherwise consume a job's whole timeout.
- Go modules and the Go build/test cache use separate keys: modules follow `go.mod`/`go.sum`, while the build cache rolls over weekly (and on module changes) so `main` refreshes it without saving a full copy per Go commit.
- PR runs restore caches read-only and superseded PR runs are cancelled. Caches are saved from `main`.
- Failure diagnostics live in `web/playwright-report/`, `web/test-results/`, and `web/.vitest/attachments/` (uploaded per job). Open the E2E HTML report with `pnpm --dir web test:e2e:report`.

## UI pull request evidence

- PRs that change visible UI (layout, styling, components, or interaction states) MUST embed screenshots of the final implementation in the PR description.
  - Capture the running app or its real Storybook components. Mockups and test summaries do not replace screenshots.
- Every browser UI PR MUST include all three representative viewports in the same PR description. Dimensions are in CSS pixels.
  - Desktop (1280 × 900)
  - Tablet (768 × 800)
  - Phone (390 × 844)
  - Include additional viewports when the change affects layouts these sizes do not cover.
- For changes to an existing interface, show a before/after pair at each required viewport. Capture the PR base and proposed change with the same theme, sample data, and interaction state.
  - Cover both light and dark when both are affected.
  - Label the viewport, theme, and sample data.
- Interaction or animation changes MUST also embed a GIF of the final implementation in the PR description. This is the standard UI delivery workflow, not an optional follow-up.
  - Record the running app or real Storybook component at normal speed, showing the initial state, the action and the settled result; include closing/reversing when relevant.
  - Keep the loop short and readable, label viewport/theme/demo data, and cover each changed interaction. Use isolated data and a publicly readable GitHub attachment or committed asset URL.
  - Inspect the GIF for missing frames, clipping and timing, verify it animates in the published PR, and refresh it after motion changes. GIFs complement the required screenshots; static-only changes do not need a GIF.
- Show before/after views for changes to an existing interface.
  - Include the affected light/dark themes and desktop/mobile layouts when the change spans them.
  - Label the viewport and any sample data.
- Use GitHub attachments or committed image URLs that reviewers can open. Local filesystem paths and localhost URLs are not valid PR image links.
- Check that the images render in the published PR and refresh them when later changes alter the UI.
- Use isolated fixtures or demo sessions without credentials or private terminal output.
