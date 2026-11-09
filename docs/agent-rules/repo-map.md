# Repository Map

Detail moved out of `AGENTS.md`, which has a 10 KiB budget. Read it when you need to know where a change belongs or which file owns a behavior.

## Key Directories

- `cmd/webmux/`: CLI flags, dependency wiring, and server shutdown.
- `cmd/webmux-windows/`, `cmd/webmux-tray/`: Windows console client and Windows tray (WSL). `cmd/baseline/`: product-contract measurement tool. `cmd/internal/assemble/`: shared configuration-to-service-options wiring for the `cmd/` roots.
- `internal/session/`: PTY lifecycle, ring-buffer I/O pump, and session management.
  - `internal/vtpreview/`: stateless VT emulation for terminal preview and replay snapshots.
  - `internal/procpeek/`: cross-platform process tree traversal and listening port inspection (`_linux.go` reads `/proc`, `_darwin.go` uses sysctl/`proc_pidinfo`, shared logic in `_unix.go`).
- `internal/hybrid/`: hybrid (TUI ↔ ACP chat) sessions, one actor per session, records in `hybrid-sessions.json`.
  - `internal/acp/`: stdlib-only ACP v1 client. `internal/agentprofile/` (adapters `opencode`, `omp`, `codex`, `claudecode`, `agy`, `qoder`, `qodercn`; list in `agentprofile/agents`): agent-neutral profiles and registry.
- `internal/api/`, `internal/hub/`, `internal/wire/`: controllers.
  - HTTP/security boundaries, status mapping (`api/errors.go`), streaming transport, and service↔wire conversion.
  - No direct filesystem, process, or service package-level calls.
- `internal/contract/`, `internal/config/`: stdlib-only wire types and frame constants; configuration parsing and validation.
- Out-of-server clients: `internal/client/`, `internal/localclient/`, `internal/attach/`, `internal/clicmd/`, `internal/tray/`, `internal/wslclient/`. They reach the server only over the wire protocol.
  - Local API client and instance location: `client`, `localclient`. Native terminal attach: `attach`. Shared CLI commands (`run`/`attach`/`ls`/`open`/workspace/chat): `clicmd`.
  - Windows tray core, system operations (`tray/traysys`), UI (`tray/winui`), and the WSL bridge dialer: `tray`, `wslclient`.
- `internal/archtest/`: test-only layer manifest and architecture rules.
  - Classify every new `internal/` package in `layers_test.go`.
  - The fixture under `testdata/fixture` proves each rule.
  - `agents_size_test.go` enforces the `AGENTS.md` size budget.
- `internal/docguard/`: test-only documentation/spec size and tree-layout guard. Thresholds and exemptions live in [doc-governance.md](../doc-governance.md) and `doc_test.go`.
- `web/src/pages/`, `web/src/hooks/`: page state and resource-owning terminal/mobile behavior.
- `web/src/lib/components/ui/`: shadcn-svelte base components. Compose them in business components instead of inventing replacement dialogs or menus.
- `web/test/`: Node unit, browser component, and isolated backend E2E suites.
- `deploy/`: deployment examples.
- `docs/`, `openspec/`: user/contributor docs and specification artifacts. Obey [doc-governance.md](../doc-governance.md): 20–400 lines, at most 300 display columns per line, at most 48 KiB per file, at most 20 child directories.

`web/dist/`, `bin/`, and browser reports are generated outputs.
`acceptance.local/` is gitignored, machine-specific experimentation with fixed ports/tokens and live-session side effects. It is not the standard QA suite.

## Important Files

- `cmd/webmux/main.go`, `web/src/main.ts`, `web/src/App.svelte`: server/browser entry points and top-level lifecycle/routing.
- `internal/session/manager.go`, `session.go`, `input_write.go`, `ring.go`: state ownership, PTY lifecycle, cancellable input, and replay.
- `web/embed.go`: embeds `web/dist`. Rebuild the frontend before compiling Go to avoid missing or stale assets.
- `Makefile`, `go.mod`, `web/package.json`, `web/pnpm-lock.yaml`, `web/pnpm-workspace.yaml`: build commands, tool/dependency versions, and permitted install scripts.
- `web/vite.config.ts`, `web/tsconfig.json`, `web/components.json`, `.oxfmtrc.json`, `web/.oxlintrc.json`: proxy/alias, strict typing, UI generation, formatting, and lint configuration.
- Frontend architecture guard: `web/eslint.config.mjs`, `web/.dependency-cruiser.cjs`, `web/.jscpd.json`, `web/knip.json`, `web/test/architecture.test.ts`.
  - Baselines: `web/eslint-suppressions.json`, `web/.dependency-cruiser-known-violations.json`, `web/.jscpd-baseline.json`.
- `README.md`, `docs/` (usage, deployment, architecture, protocol), `CONTRIBUTING.md`, `SECURITY.md`: user and contributor documentation at overview level. Confirm implementation details against source.
  - Start from [docs/README.md](../README.md), the documentation hub; `docs/overview.md` holds the full feature introduction and `docs/faq.md` the FAQ and help channels that the root `README.md` summarizes.
  - `docs/specs.md` indexes every `openspec/specs/<capability>/spec.md`; add new capabilities there.
  - Maintain `docs/user-stories.md` (index) and `docs/user-stories/` (group files) under [user-stories-maintenance.md](user-stories-maintenance.md).
  - Follow [doc-governance.md](../doc-governance.md) for file size and tree layout, and update other documentation when relevant.
- `deploy/config.example.toml`, `deploy/webmux.service`, `deploy/com.webmux.plist`: TOML, systemd, and launchd examples.
- `openspec/config.yaml`: spec-driven workflow context.
  - Write new specification artifacts in Simplified Chinese, retaining English structural headings and SHALL/MUST keywords.
  - `.agents/`, `.claude/`, and `.omp/` contain generated workflow integrations.

## Runtime and Tooling Preferences

- Use Go 1.26+, Node.js 26.13.0 (LTS, pinned in `.nvmrc`), pnpm 12.11.2, and GNU Make.
- Use pnpm, not npm, Yarn, or Bun, to manage frontend dependencies. Retain the frozen lockfile workflow and the `esbuild`-only dependency build-script allowlist.
- Production needs only the Linux or macOS binary, a shell, and any chosen CLI agents. It does not require Node or Go.
- `deploy/webmux.service` (systemd) and `deploy/com.webmux.plist` (launchd) are the service examples.
- Release builds use `CGO_ENABLED=0`.
- `make check` needs golangci-lint v2.15.0. CI runs the same configured linters and `go vet ./...` after building embedded frontend assets.
- OpenSpec 1.13.0 is optional workflow tooling, not a frontend runtime dependency.
