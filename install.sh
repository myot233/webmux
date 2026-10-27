#!/bin/sh
# Install WebMux from GitHub Releases on Linux or macOS.
#
# Usage:
#   curl -fsSL <install.sh URL> | sh
#   curl -fsSL <install.sh URL> | WEBMUX_VERSION=0.2.0 sh
#
# Environment:
#   WEBMUX_VERSION      Release version without the leading "v" (default: latest)
#   WEBMUX_INSTALL_DIR  Directory for the webmux binary (default: ~/.local/bin)
#   WEBMUX_MIRRORS      Space-separated gh-proxy prefixes; empty disables fallback
#   WEBMUX_SHA256       Pin the binary digest instead of reading SHA256SUMS
#   WEBMUX_BASE_URL     GitHub repository URL (mainly for tests and mirrors)
#
# GitHub is tried first, then the mirrors. Every source must serve a binary
# whose SHA-256 matches SHA256SUMS from that same source before it is installed,
# so a mirror affects availability, not the expected content. The script never
# fetches from raw.githubusercontent.com; callers choose where to read this file.

set -eu

repo_base=${WEBMUX_BASE_URL:-https://github.com/mzdluo123/webmux}
mirrors=${WEBMUX_MIRRORS-https://ghfast.top https://gh-proxy.com}
install_dir=${WEBMUX_INSTALL_DIR:-"${HOME:-}/.local/bin"}

die() {
	printf 'webmux install: %s\n' "$*" >&2
	exit 1
}

note() {
	printf '%s\n' "$*"
}

[ -n "${HOME:-}" ] || [ -n "${WEBMUX_INSTALL_DIR:-}" ] || die "HOME is not set; set WEBMUX_INSTALL_DIR"
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v uname >/dev/null 2>&1 || die "uname is required"
command -v mktemp >/dev/null 2>&1 || die "mktemp is required"
command -v awk >/dev/null 2>&1 || die "awk is required"

os=$(uname -s)
case $os in
Linux) os=linux ;;
Darwin) os=darwin ;;
MINGW* | MSYS* | CYGWIN*)
	die "this script runs on Linux and macOS only; on Windows download webmux-windows-<arch>.exe or the tray executable from $repo_base/releases/latest"
	;;
*)
	die "unsupported operating system: $os (need Linux or macOS; on Windows use the exe or WSL)"
	;;
esac

machine=$(uname -m)
case $machine in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported architecture: $machine (need amd64 or arm64)" ;;
esac

# Rosetta reports x86_64 on Apple Silicon; prefer the native build.
if [ "$os" = darwin ] && [ "$arch" = amd64 ]; then
	translated=$(sysctl -in sysctl.proc_translated 2>/dev/null || true)
	if [ "$translated" = 1 ]; then
		arch=arm64
	fi
fi

if [ -n "${WEBMUX_VERSION:-}" ]; then
	version=${WEBMUX_VERSION#v}
	case $version in
	'' | *[!0-9A-Za-z._+-]*) die "invalid WEBMUX_VERSION: $WEBMUX_VERSION" ;;
	esac
	release_base="$repo_base/releases/download/v$version"
	release_label="v$version"
else
	release_base="$repo_base/releases/latest/download"
	release_label="latest"
fi

asset="webmux-$os-$arch"
pinned=""
if [ -n "${WEBMUX_SHA256:-}" ]; then
	pinned=$(printf '%s' "$WEBMUX_SHA256" | tr '[:upper:]' '[:lower:]')
	case $pinned in
	'' | *[!0-9a-f]*) die "WEBMUX_SHA256 must be a 64-character hex digest" ;;
	esac
	[ "${#pinned}" -eq 64 ] || die "WEBMUX_SHA256 must be a 64-character hex digest"
fi

digest() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "need sha256sum or shasum to verify the download"
	fi
}

sums_digest() {
	awk -v name="$2" '{ n=$2; sub(/^\*/, "", n); if (n == name) { print $1; exit } }' "$1"
}

download() {
	curl -fsSL --connect-timeout 10 --max-time "$3" --speed-limit 1024 --speed-time 20 -o "$2" "$1"
}

mkdir -p "$install_dir" 2>/dev/null || die "cannot create install directory: $install_dir"
[ -d "$install_dir" ] || die "install path is not a directory: $install_dir"
[ -w "$install_dir" ] || die "install directory is not writable: $install_dir (set WEBMUX_INSTALL_DIR)"

tmp_dir=$(mktemp -d "$install_dir/.webmux-install.XXXXXX") || die "cannot create a temporary directory in $install_dir"
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

target="$install_dir/webmux"
set -- "$release_base"
for mirror in $mirrors; do
	set -- "$@" "$mirror/$release_base"
done

failures=""
for source in "$@"; do
	asset_path="$tmp_dir/$asset"
	sums_path="$tmp_dir/SHA256SUMS"
	rm -f "$asset_path" "$sums_path"

	if ! download "$source/$asset" "$asset_path" 600; then
		failures="$failures
  $source: binary download failed"
		continue
	fi

	if [ -z "$pinned" ]; then
		if ! download "$source/SHA256SUMS" "$sums_path" 60; then
			failures="$failures
  $source: SHA256SUMS download failed"
			continue
		fi
		expected=$(sums_digest "$sums_path" "$asset")
		case $expected in
		'' | *[!0-9a-f]*)
			failures="$failures
  $source: SHA256SUMS has no valid digest for $asset"
			continue
			;;
		esac
		if [ "${#expected}" -ne 64 ]; then
			failures="$failures
  $source: SHA256SUMS has no valid digest for $asset"
			continue
		fi
	else
		expected=$pinned
	fi

	actual=$(digest "$asset_path")
	if [ "$actual" != "$expected" ]; then
		failures="$failures
  $source: sha256 mismatch (expected $expected, got $actual)"
		continue
	fi

	chmod 755 "$asset_path" || {
		failures="$failures
  $source: cannot make the download executable"
		continue
	}
	if ! mv -f "$asset_path" "$target"; then
		failures="$failures
  $source: cannot write $target"
		continue
	fi

	host=${source#*://}
	host=${host%%/*}
	note "Installed webmux ($release_label) to $target (from $host)."
	case ":${PATH:-}:" in
	*":$install_dir:"*) ;;
	*) note "Note: $install_dir is not on PATH; add it or run $target directly." ;;
	esac
	note ""
	note "Start WebMux:"
	note "  $target --listen 127.0.0.1:7681"
	note "Install it as a user service:"
	note "  $target service install"
	exit 0
done

printf 'webmux install: all download sources failed:%s\n' "$failures" >&2
exit 1
