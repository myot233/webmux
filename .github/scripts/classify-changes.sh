#!/usr/bin/env bash
set -euo pipefail

# Classifies a pull request or push for job gating. Every path that is not
# clearly documentation, Go or frontend forces a full run. The workflow runs
# the base revision's copy of this script and also forces a full run whenever
# `.github/` changes, so a PR cannot weaken the rules that skip its own tests.
# The workflow always runs so the required `Test result` check still reports
# for skipped jobs.

base="${BASE_SHA:-}"
head="${HEAD_SHA:-${GITHUB_SHA:-HEAD}}"

full_run() {
  {
    echo "go=true"
    echo "web=true"
    echo "docs_only=false"
    echo "full=true"
  } >> "$GITHUB_OUTPUT"
  echo "classify: full run"
  exit 0
}

# Force a full run when the base is missing, zeroed (new branch) or not in the
# local history, because the changed set cannot be trusted in that case.
if [[ -z "$base" || "$base" =~ ^0+$ ]] || ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
  full_run
fi

# Three-dot compares the PR against its merge base, so base-branch drift
# cannot be mistaken for changes in this branch.
if ! changed=$(git diff --no-renames --name-only "$base...$head"); then
  full_run
fi
if [[ -z "$changed" ]]; then
  # Metadata-only pushes and merges with no tree difference still deserve the
  # full gate; there is no partial set to classify.
  full_run
fi

go=false
web=false
docs_only=true

while IFS= read -r file; do
  [[ -z "$file" ]] && continue

  case "$file" in
    # Tooling, dependencies and anything that changes how the workflow runs.
    .github/* | Makefile | go.mod | go.sum | go.work | go.work.sum | install.sh | \
      .nvmrc | .node-version | package.json | pnpm-lock.yaml | pnpm-workspace.yaml | \
      web/pnpm-lock.yaml | web/package.json | web/pnpm-workspace.yaml | web/.npmrc | \
      web/vite.config.* | web/playwright.config.* | web/vitest*.config.* | \
      web/svelte.config.* | web/tsconfig*.json | web/scripts/* | baseline/*)
      full_run
      ;;
  esac

  case "$file" in
    # Go sources, fixtures and the packages built into the server. Checked
    # before Markdown so future `.md` fixtures under internal/ count as Go.
    *.go | go.mod | go.sum | go.work | go.work.sum | cmd/* | internal/* | deploy/* | */testdata/*)
      docs_only=false
      go=true
      ;;
    # Everything else under web/ is frontend work, including its Markdown
    # fixtures; checked before the generic Markdown rule for the same reason.
    web/*)
      docs_only=false
      web=true
      ;;
    # Markdown and the documentation/spec trees only need the doc guards.
    *.md | docs/* | openspec/*)
      ;;
    # Unknown paths are treated as potentially affecting both stacks.
    *)
      full_run
      ;;
  esac
done <<< "$changed"

# Go wire fixtures are also inputs to the frontend Zod contract tests.
case "$changed" in
  *internal/api/testdata/wire/* | *internal/wire/testdata/*)
    docs_only=false
    web=true
    ;;
esac

{
  echo "go=$go"
  echo "web=$web"
  echo "docs_only=$docs_only"
  echo "full=false"
} >> "$GITHUB_OUTPUT"
echo "classify: go=$go web=$web docs_only=$docs_only"
