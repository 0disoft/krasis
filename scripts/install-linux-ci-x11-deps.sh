#!/usr/bin/env bash
set -euo pipefail

# This helper is for the Ubuntu amd64 CI runner, not installed Krasis clients.
. /etc/os-release
if [[ ${ID:-} != ubuntu || ! ${VERSION_CODENAME:-} =~ ^[a-z]+$ ]] || [[ $(dpkg --print-architecture) != amd64 ]]; then
  echo 'X11 CI dependency installation requires Ubuntu amd64.' >&2
  exit 2
fi
keyring=/usr/share/keyrings/ubuntu-archive-keyring.gpg
test -r "$keyring"
privilege=()
if (( EUID != 0 )); then privilege=(sudo -n); fi
work=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/krasis-ci-apt-XXXXXXXXXX")
cleanup() { "${privilege[@]}" rm -rf -- "$work"; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
chmod 755 "$work"
mkdir -m 755 "$work/lists"
cat > "$work/ubuntu.sources" <<EOF
Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: $VERSION_CODENAME $VERSION_CODENAME-updates $VERSION_CODENAME-backports
Components: main universe
Architectures: amd64
Signed-By: $keyring

Types: deb
URIs: https://security.ubuntu.com/ubuntu/
Suites: $VERSION_CODENAME-security
Components: main universe
Architectures: amd64
Signed-By: $keyring
EOF
chmod 644 "$work/ubuntu.sources"
# Both operations use the same fresh, signed indexes and canonical HTTPS URIs.
# Do not inherit the runner's mirror+file/Azure source or stale package indexes.
apt=("${privilege[@]}" apt-get
  -o "Dir::Etc::sourcelist=$work/ubuntu.sources" -o 'Dir::Etc::sourceparts=-'
  -o "Dir::State::lists=$work/lists" -o 'Acquire::Retries=1'
  -o 'Dir::Cache::pkgcache=' -o 'Dir::Cache::srcpkgcache='
  -o 'Acquire::Languages=none'
  -o 'Acquire::https::Timeout=20')
"${apt[@]}" update --error-on=any
"${apt[@]}" install -y --no-install-recommends xvfb xauth x11-utils
for tool in Xvfb xauth xdpyinfo; do command -v "$tool"; done
