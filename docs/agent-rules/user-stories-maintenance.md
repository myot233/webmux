# User Stories Maintenance

Detail moved out of `AGENTS.md`, which has a 10 KiB budget. Apply it whenever a change adds, changes, or removes behavior.

## When stories must change

- Read `docs/user-stories.md` (the index) and the relevant group file under `docs/user-stories/` before changing a capability.
- Every implemented capability MUST have one or more stories explaining the user role, intended action, purpose, and observable acceptance criteria.
- In the same change, update the affected stories when adding, changing, or removing behavior.
  - This includes UI, REST/WebSocket APIs, CLI commands, configuration, security and session lifecycle, deployment, and contributor tooling.
  - Pure refactors, formatting, and dependency updates with no behavior change do not require catalog edits.

## How to edit

- Reuse existing stories when their purpose still fits. Add stories for new purposes.
- Keep IDs stable: do not renumber existing stories or reuse retired IDs.
- When removing a capability, mark its stories as retired and explain the replacement when applicable, rather than silently losing their history.
- Keep these consistent: acceptance criteria, per-group source references in `docs/user-stories/`, both feature indexes (`docs/user-stories.md` and `docs/user-stories/coverage.md`), and any stated counts.
- Use the existing Chinese story format and functional groups. Add a group only when existing groups do not fit.
- Record verification notes (scope, tests run, limits) in the newest `docs/user-stories/verification-log-*.md`, never in the `docs/user-stories.md` index, which stays an index. Split a log by date when it nears the size limits.

## Verification

- Check behavior against the implementation and relevant tests.
- Distinguish implemented capabilities from OpenSpec plans. Investigate disagreements before updating the catalog.
- Keep verification notes accurate. Never claim that tests ran or the whole catalog was reverified without that evidence.
- Before completing the change, verify that:
  - every added or changed capability maps to a story;
  - IDs are unique;
  - index references resolve;
  - linked source paths exist.
- Run the applicable Markdown formatting and whitespace checks.
- Run `go test -count=1 ./internal/docguard`: its story catalog guard rejects duplicate IDs, incomplete main-index ranges, unknown coverage references and active reuse of retired IDs. Retired stories remain in their historical ranges.
- In the delivery or PR summary, identify the affected story IDs, or briefly explain why the change has no user-story impact.
