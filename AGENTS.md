# Repository Guidelines

WebMux is a single-user Linux/macOS web terminal for persistent PTY sessions, including shells and CLI agents; supported agents also run as hybrid TUI/ACP-chat sessions. Browser disconnection does not end a session; server restart does. The Go binary embeds the browser app. This is not a process sandbox, durable session store, or tmux wrapper: commands run as the service user. Keep deployments on loopback or a trusted VPN; never expose an unrestricted shell publicly.

**This file has a hard 10 KiB budget**, enforced by `internal/archtest` (`TestAgentsGuideSize`). Keep only rules needed on almost every change here; put the rest in `docs/agent-rules/` and link it. Index: [docs/agent-rules.md](docs/agent-rules.md).

## Architecture

- `cmd/webmux` loads TOML, converts it to `session.Options` (via `cmd/internal/assemble`), constructs every service (`session.Manager`, launch, workspace, codeview, files, push) and injects them into `api.New` as `api.Deps`. Only `cmd/` constructs services or converts configuration into service options. Keep explicit constructor-based wiring.
- Layers are enforced by `internal/archtest` (rules R0–R11 in `docs/architecture.md`; manifest in `internal/archtest/layers_test.go`, where every new `internal/` package must be classified).
  - Controllers: `internal/api`, `hub`, `wire`. Services: `session`, `workspace`, `launch`, `push`, `codeview`, `files`, and others. Shared: `contract`, `config`, `runtime`, `statefile`, `fetch`, `pathutil`, `attentionkind`. Out-of-server clients: `client`, `attach`, `tray/...`, `tunnel`, `service`, and others.
  - Shared packages and clients never import services or controllers. Only `cmd/` imports `internal/api`.
- `internal/session` owns PTYs, bounded byte scrollback, metadata, status detection, and process cleanup. `internal/api` owns REST routes, authentication, Origin checks, error-to-status mapping, and static serving. `internal/hub` bridges session subscriptions to WebSockets and encodes terminal JSON frames. `internal/files` owns directory completion, mkdir, and upload storage.
- `internal/hybrid` runs hybrid sessions (one actor each) and normalizes ACP updates into chat events; `internal/acp` is a stdlib-only ACP v1 client; `internal/agentprofile` holds per-agent profiles ([architecture](docs/architecture.md#混合会话)).
- REST handles configuration, session operations, and filesystem requests. `/ws/sessions` delivers a snapshot followed by list events; `/ws/s/{id}` carries terminal frames; `/ws/s/{id}/chat` carries chat events. Raw PTY bytes reach xterm without server-side terminal emulation; OSC/title and preview parsing are side channels.
- Replay capture and live subscription registration share a lock: preserve gap-free reattachment. Bounded queues disconnect slow clients rather than blocking PTY readers. Each connection has one data writer; cancelling a connection must not kill the persistent session.
- Backend payloads and `web/src/contracts/schemas.ts` form one contract. Every JSON body and frame is a type in `internal/contract`, converted from service types in `internal/wire`; services never encode client-visible JSON. When changing JSON fields, update schemas, contract types, hub/frontend frame handling and consumers together, then re-record `internal/api/testdata/wire` with `go test ./internal/api -run TestWireSnapshots -update`.

## Commands

Run from the repository root; the full list is in [conventions](docs/agent-rules/conventions.md#development-commands).

- `make web`: install frontend dependencies, run Svelte/TypeScript checks, build `web/dist`.
- `make build`: build the frontend, then the embedded Go binary `bin/webmux`. Rebuild the frontend first (`web/embed.go` embeds `web/dist`).
- `make check`: build the frontend, check test types and formatting, run the doc, persistence and architecture guards, Oxlint, `go vet` and `golangci-lint` (v2.15.0).
- `make format`: apply gofmt and Oxfmt.
- `pnpm --dir web check:arch`: frontend architecture guard; run it before finishing frontend work.
- `pnpm --dir web dev`: Vite at `http://127.0.0.1:5173`, proxying `/api` and `/ws` to backend port 7681. Run the backend separately and add `allowed_origins = ["http://127.0.0.1:5173"]` to its TOML or browser writes are rejected.
- `-config`, `-listen`, `-token` override startup settings.

## Conventions

- Go: follow [docs/go-style.md](docs/go-style.md) and add no lint warnings (`make go-lint`). Return contextual errors. Services report sentinel errors (for example `workspace.ErrConflict`); only `internal/api/errors.go` maps them to HTTP statuses.
- Concurrency: protect state with existing locks and never hold them across blocking network or PTY operations. Shared output chunks stay immutable. Exit publication follows PTY drain and process cleanup. Preserve cancellation/deadlines for PTY writes and join spawned connection goroutines.
- Execute commands as argv, not shell-expanded strings. Session creation takes either a saved launch `configId` or an explicit full `cmd`/`env`, never both. A new install seeds editable launch configs (`internal/launch/presets.go`); loading `.bashrc` needs an explicit `/usr/bin/env bash -ic` wrapper.
- Security: preserve authentication and Origin checks on API/WebSocket paths. Never log terminal input/output or expose tokens through configuration responses.
- Frontend: Svelte 5 runes per [docs/frontend-style.md](docs/frontend-style.md) (`$state` only for rendered state, `$derived` for computed values, `$effect` only for side effects with cleanup, typed `$props()`, `onclick` not `on:`). TypeScript is strict. Oxfmt: 80 columns, two spaces, semicolons, trailing commas.
- Validate HTTP responses, WebSocket JSON, and local settings with Zod before updating state; derive types with `z.infer`. Reuse `api.ts` and `ws.ts`; do not add parallel request or reconnect conventions. PTY data stays binary. Failures use `APIError`; HTTP 401 triggers `webmux:auth`.
- UI: look first in `web/src/lib/components/ui/`, then `web/src/components/`, then the shadcn-svelte registry. Add a variant or size to a design-system component instead of restyling it with `class`. Fix `check:arch` violations; never add `eslint-disable`/`jscpd:ignore` or grow a baseline ([docs/frontend-architecture.md](docs/frontend-architecture.md)).
- Any visible UI change: follow the `webmux-ui` skill (`.agents/skills/webmux-ui/SKILL.md`).
- Icons: Iconify with the `tabler` collection, bundled locally. Icon-only controls need an accessible name and a visible tooltip; decorative icons use `aria-hidden="true"` ([docs/design-system.md](docs/design-system.md#icons)).
- Names: PascalCase Svelte components, camelCase TypeScript functions, `*Schema` Zod contracts, Playwright `*.spec.ts`; files per [naming](docs/agent-rules/naming.md) (kebab-case TS, snake_case Go).
- Use pnpm (never npm, Yarn, or Bun) with the frozen lockfile. Go 1.27+, Node 26.13.0 (`.nvmrc`), GNU Make. Release builds use `CGO_ENABLED=0`.

## Testing

- `make test` is the full gate (Go race plus all frontend suites); `make test-fast` skips Storybook, E2E and performance.
- Target one Go package with `go test -race ./internal/session`, one Vitest file with `pnpm --dir web exec vitest run --config vitest.config.ts test/input.test.ts`, one E2E spec with `pnpm --dir web exec playwright test test/e2e/terminal.spec.ts`.
- Preserve coverage for authentication/Origin rejection, argv boundaries, replay continuity, exit ordering, cancellation, terminal input/rendering, and touch interactions.
- Do not connect tests to an existing developer service. macOS temp paths sit behind `/var -> /private/var`; compare canonical paths.
- Test runtime, CI layout and diagnostics: [testing](docs/agent-rules/testing.md).

## Collaboration and Pull Requests

- Deliver changes via a Pull Request from a dedicated branch off `origin/main` (use an isolated `git worktree` when the current checkout is on `main` or shared); never commit or push directly to `main` unless the user explicitly asks otherwise. Follow Conventional Commits (`feat(web): ...`, `fix: ...`).
- Follow every PR until all CI checks pass; fix CI failures on the branch until all checks are green.
- Once checks pass, leave the PR open for maintainer review and merge; do not merge unless the user explicitly asks you to.
- UI PRs require screenshots at desktop, tablet and phone sizes, before/after pairs for existing UI, and GIFs for interaction/motion changes. Follow [testing](docs/agent-rules/testing.md#ui-pull-request-evidence).

## Documentation and User Stories

- `docs/` and `openspec/` obey [docs/doc-governance.md](docs/doc-governance.md) (20–400 lines, ≤300 cols/line, ≤48 KiB, ≤20 child dirs) and the `webmux-docs` skill (`.agents/skills/webmux-docs/SKILL.md`). Write new OpenSpec artifacts in Simplified Chinese, keeping English structural headings and SHALL/MUST.
- Every implemented capability MUST have a story in `docs/user-stories/` (index: `docs/user-stories.md`). In the same change, update the affected stories when behavior changes: UI, APIs, CLI, configuration, security, lifecycle, deployment, contributor tooling. Pure refactors, formatting and dependency updates are exempt.
- Keep story IDs stable (never renumber or reuse; mark removed ones retired), keep both indexes and counts consistent, and never claim verification you did not run. Name the affected story IDs (or say there is no impact) in the delivery or PR summary. Full procedure: [user-stories-maintenance](docs/agent-rules/user-stories-maintenance.md).
- Every doc must be reachable by relative links from `README.md` or `AGENTS.md`, and relative links/anchors must resolve (`make docs-check`). Register new docs in [docs/README.md](docs/README.md) (specs in [docs/specs.md](docs/specs.md)); keep `README.md` a short landing page (budget in [doc-governance](docs/doc-governance.md#readme-预算)).
- Repository map (directories, important files): [repo-map](docs/agent-rules/repo-map.md).
