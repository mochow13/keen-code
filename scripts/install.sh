#!/usr/bin/env bash
set -euo pipefail

REPO="mochow13/keen-code"
BINARY="keen"
INSTALL_DIR=""

# --- helpers ---

die() { echo "error: $*" >&2; exit 1; }

detect_platform() {
  local os arch

  case "$(uname -s)" in
    Darwin) os="darwin" ;;
    Linux)  os="linux"  ;;
    *) die "unsupported OS: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64)          arch="amd64" ;;
    arm64|aarch64)   arch="arm64" ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac

  echo "${os}_${arch}"
}

resolve_version() {
  local version="$1"
  if [ -z "$version" ]; then
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    [ -n "$version" ] || die "could not determine latest version"
  fi
  echo "$version"
}

pick_install_dir() {
  if [ -w "/usr/local/bin" ]; then
    echo "/usr/local/bin"
  else
    local dir="${HOME}/.local/bin"
    mkdir -p "$dir"
    echo "$dir"
  fi
}

verify_checksum() {
  local file="$1" expected="$2"
  local actual
  if command -v sha256sum &>/dev/null; then
    actual=$(sha256sum "$file" | awk '{print $1}')
  elif command -v shasum &>/dev/null; then
    actual=$(shasum -a 256 "$file" | awk '{print $1}')
  else
    die "no sha256 tool found (sha256sum or shasum)"
  fi
  [ "$actual" = "$expected" ] || die "checksum mismatch for $file"
}

# --- main ---

VERSION=""

while [ $# -gt 0 ]; do
  case "$1" in
    -v|--version) VERSION="$2"; shift 2 ;;
    -d|--dir)     INSTALL_DIR="$2"; shift 2 ;;
    *) die "unknown option: $1" ;;
  esac
done

PLATFORM=$(detect_platform)
VERSION=$(resolve_version "$VERSION")
TAG="${VERSION}"
VERSION_BARE="${VERSION#v}"

ARCHIVE="keen_${VERSION_BARE}_${PLATFORM}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

[ -z "$INSTALL_DIR" ] && INSTALL_DIR=$(pick_install_dir)

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "downloading keen ${VERSION} (${PLATFORM})..."
curl -fsSL "${BASE_URL}/${ARCHIVE}"       -o "${TMPDIR}/${ARCHIVE}"
curl -fsSL "${BASE_URL}/checksums.txt"    -o "${TMPDIR}/checksums.txt"

EXPECTED=$(grep " ${ARCHIVE}$" "${TMPDIR}/checksums.txt" | awk '{print $1}')
[ -n "$EXPECTED" ] || die "no checksum entry found for ${ARCHIVE}"

verify_checksum "${TMPDIR}/${ARCHIVE}" "$EXPECTED"

tar -xzf "${TMPDIR}/${ARCHIVE}" -C "${TMPDIR}" "${BINARY}"
chmod +x "${TMPDIR}/${BINARY}"
mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"

if ! echo ":${PATH}:" | grep -q ":${INSTALL_DIR}:"; then
  echo ""
  echo "note: ${INSTALL_DIR} is not in your PATH."

  # Detect shell config file
  SHELL_RC=""
  case "${SHELL:-}" in
    */zsh)  SHELL_RC="${ZDOTDIR:-$HOME}/.zshrc" ;;
    */bash) SHELL_RC="$HOME/.bashrc" ;;
  esac

  if [ -n "$SHELL_RC" ] && [ -t 0 ]; then
    printf "add %s to PATH in %s? [y/N] " "${INSTALL_DIR}" "${SHELL_RC}"
    read -r reply
    if [ "${reply}" = "y" ] || [ "${reply}" = "Y" ]; then
      printf '\nexport PATH="%s:$PATH"\n' "${INSTALL_DIR}" >> "${SHELL_RC}"
      echo "added to ${SHELL_RC} — restart your shell or run: source ${SHELL_RC}"
    else
      echo "skipped. add this line manually:"
      echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    fi
  else
    echo "add this line to your shell profile:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi
fi
