# File Naming

A reader should be able to predict a file's name from what it owns. Names state the noun or responsibility, never a vague word.

- `internal/archtest` (`TestGoFileNames`, `TestSourceFileSize`) enforces Go file naming and the `< 500` line production source budget.
- `web/test/architecture.test.ts` (`file naming`) enforces frontend file naming and the `< 500` line production source budget; the rest is review judgement.

## Frontend (`web/`)

| Kind                     | Rule                                                                            | Example                                          |
| ------------------------ | ------------------------------------------------------------------------------- | ------------------------------------------------ |
| Svelte component         | PascalCase, `<Family><Part>.svelte`; one family prefix per feature              | `CodeFileTree.svelte`, `TerminalHeader.svelte`   |
| Non-component TypeScript | kebab-case, `<domain>-<role>.ts`                                                | `chat-api.ts`, `quick-commands.ts`               |
| Rune state               | `x.svelte.ts` holds `$state`; the pure logic lives in `x.ts` with the same stem | `quick-commands.ts` + `quick-commands.svelte.ts` |
| Unit test                | `web/test/<source-stem>.test.ts`                                                | `draft-storage.test.ts`                          |
| Browser test, story      | `<Component>.test.ts` or `<Component>.stories.ts`; helpers stay kebab-case      | `CodePanel.test.ts`, `terminal-fixture.ts`       |
| shadcn-svelte base       | generated under `src/lib/components/ui/`; keep the registry's names             | `button/button.svelte`                           |

- Never mix camelCase into TypeScript file names, even for a single file. Identifiers inside the file stay camelCase.
- Group by prefix: all chat modules start with `chat-`, all code-reader modules with `code-`. Add a new member to the existing family instead of inventing a new prefix.
- Hooks that own resources or coordinate domains live in `src/hooks/<topic>/`; page-level composition lives in `src/pages/`.
  Pure algorithms and domain state live together in `src/domains/<topic>/`; wire schemas live in `src/contracts/` and generic browser I/O in `src/platform/`. See [frontend architecture](../frontend-architecture.md).

## Go (`internal/`, `cmd/`)

| Kind        | Rule                                                                                                                                   | Example                                           |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| Source file | lowercase words joined by `_`, named for the noun or responsibility it owns                                                            | `ring.go`, `input_write.go`, `listening_ports.go` |
| Family      | `<family>_<part>.go` so related files sort together                                                                                    | `cleanup_linux.go`, `chat_config_queue.go`        |
| Platform    | `_linux`, `_darwin`, `_windows` are build constraints; `_unix` and `_other` need a `//go:build` line                                   | `proc_unix.go`, `proc_other.go`                   |
| Types       | `types.go` only when a package has several small exported types shared by its files; a type with behavior goes in the file of its noun | `workspace/types.go`                              |
| Test        | `<source-stem>_test.go` for that file's cases; a cross-file behavior may use a topic name                                              | `ring_test.go`, `security_test.go`                |

- Forbidden words anywhere in a name: `util`, `utils`, `helper`, `helpers`, `misc`, `more`, `common`, `stuff`, `extra`, `new`, `old`. Test fixtures are `*_fixtures_test.go`.
- When a file grows two responsibilities, split by responsibility and name each part. Do not add `_more` or `2` suffixes.
- Renames use `git mv` and stay separate from behavior changes. Update links in `docs/` and the persistence inventory (`internal/archtest/persistence_inventory.json` keys start with the file path) in the same change.

## Checklist

- [ ] The name predicts the contents without opening the file.
- [ ] A sibling already in the same family uses the same prefix.
- [ ] The test file's name points at the source file or states its topic.
- [ ] Docs and links that mention the old path were updated; `make docs-check` passes.
