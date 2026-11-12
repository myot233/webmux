#!/usr/bin/env bash
set -euo pipefail

# The runner's Ubuntu mirror list can fail DNS resolution for every entry.
# Use an independent HTTPS mirror without changing the runner's apt sources.
# Ubuntu's archive key still authenticates the index and package checksums.
. /etc/os-release
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
chmod 755 "$work"
mkdir -p "$work/lists/partial"
cat > "$work/ubuntu.sources" <<EOF
Types: deb
URIs: https://mirrors.edge.kernel.org/ubuntu
Suites: $VERSION_CODENAME $VERSION_CODENAME-updates $VERSION_CODENAME-security
Components: main universe
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
EOF

apt_options=(
  -o "Dir::Etc::sourcelist=$work/ubuntu.sources"
  -o 'Dir::Etc::sourceparts=-'
  -o "Dir::State::lists=$work/lists"
)
# A fresh, isolated index must download successfully; never use stale indexes.
apt-get "${apt_options[@]}" update --error-on=any
apt-get "${apt_options[@]}" install -y --no-install-recommends nsis
makensis -VERSION
