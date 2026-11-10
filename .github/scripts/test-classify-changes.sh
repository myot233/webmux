#!/usr/bin/env bash
set -euo pipefail

root=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"
mkdir -p "$repo"
cd "$repo"
git init -q
git config user.name "Classifier Test"
git config user.email "classifier@example.invalid"
git commit --allow-empty -qm base
base=$(git rev-parse HEAD)

check() {
  local name=$1 path=$2 expected=$3
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$name" >> "$path"
  git add "$path"
  git commit -qm "$name"
  local head output
  head=$(git rev-parse HEAD)
  output="$tmp/output"
  : > "$output"
  BASE_SHA="$base" HEAD_SHA="$head" GITHUB_OUTPUT="$output" bash "$root/.github/scripts/classify-changes.sh" > /dev/null
  local actual
  actual=$(grep -E '^(go|web|docs_only|full)=' "$output" | tr '\n' ' ')
  if [[ "$actual" != "$expected" ]]; then
    printf 'classifier %s: got %s, want %s\n' "$name" "$actual" "$expected" >&2
    exit 1
  fi
  git reset -q --hard "$base"
  git clean -qfd
}

check docs-only docs/usage.md 'go=false web=false docs_only=true full=false '
check go-only internal/session/manager.go 'go=true web=false docs_only=false full=false '
check web-only web/src/api.ts 'go=false web=true docs_only=false full=false '
check api-wire internal/api/testdata/wire/example.json 'go=true web=true docs_only=false full=false '
check conversion-wire internal/wire/testdata/response-arrays.json 'go=true web=true docs_only=false full=false '
check unknown-path quality.toml 'go=true web=true docs_only=false full=true '
check tooling .github/scripts/classify-changes.sh 'go=true web=true docs_only=false full=true '
echo 'classifier tests passed'
