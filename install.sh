#!/bin/sh
# Cabrero installer — downloads the latest release binary from GitHub.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/vladolaru/cabrero/main/install.sh | bash
#
# Requires: curl, tar, uname (all ship with macOS).

set -e

REPO="vladolaru/cabrero"
INSTALL_DIR="${HOME}/.cabrero/bin"

# --- helpers ---

info() {
  printf '  %s\n' "$1"
}

fail() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

# --- detect platform ---

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
if [ "$OS" != "darwin" ]; then
  fail "Cabrero currently supports macOS only (detected: ${OS})"
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  arm64)   ;;  # already correct
  aarch64) ARCH="arm64" ;;
  *)       fail "Unsupported architecture: ${ARCH}" ;;
esac

echo ""
echo "Cabrero Installer"
echo "═════════════════"
echo ""
info "Platform: ${OS}/${ARCH}"

# --- fetch latest release ---

info "Fetching latest release..."

RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest") \
  || fail "Cannot reach GitHub API. Check your connection."

TAG=$(printf '%s' "$RELEASE_JSON" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [ -z "$TAG" ]; then
  fail "Could not determine latest release tag. Is there a release published?"
fi

VERSION="${TAG#v}"
info "Latest version: ${VERSION} (${TAG})"

# --- download ---

ASSET_NAME="cabrero_${OS}_${ARCH}.tar.gz"
# Extract the browser_download_url for our asset.
DOWNLOAD_URL=$(printf '%s' "$RELEASE_JSON" | \
  sed -n "s|.*\"browser_download_url\"[[:space:]]*:[[:space:]]*\"\([^\"]*${ASSET_NAME}\)\".*|\1|p")

if [ -z "$DOWNLOAD_URL" ]; then
  fail "No release asset found for ${OS}/${ARCH} (looked for ${ASSET_NAME})"
fi

info "Downloading ${ASSET_NAME}..."

TMPDIR_PATH=$(mktemp -d)
trap 'rm -rf "$TMPDIR_PATH"' EXIT

curl -fsSL -o "${TMPDIR_PATH}/${ASSET_NAME}" "$DOWNLOAD_URL" \
  || fail "Download failed"

# --- extract ---

info "Extracting..."

tar xzf "${TMPDIR_PATH}/${ASSET_NAME}" -C "$TMPDIR_PATH" \
  || fail "Extraction failed"

if [ ! -f "${TMPDIR_PATH}/cabrero" ]; then
  fail "Binary not found in archive"
fi

chmod +x "${TMPDIR_PATH}/cabrero"

# --- install ---

mkdir -p "$INSTALL_DIR"
cp "${TMPDIR_PATH}/cabrero" "${INSTALL_DIR}/cabrero" \
  || fail "Cannot write to ${INSTALL_DIR}. Check permissions."

info "Installed to ${INSTALL_DIR}/cabrero"

# --- verify ---

if "${INSTALL_DIR}/cabrero" version >/dev/null 2>&1; then
  INSTALLED_VERSION=$("${INSTALL_DIR}/cabrero" version 2>&1)
  info "Verified: ${INSTALLED_VERSION}"
else
  fail "Installed binary failed to run"
fi

echo ""

# --- PATH check ---

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    info "~/.cabrero/bin is in PATH"
    echo ""
    echo "Run 'cabrero setup' to complete configuration."
    ;;
  *)
    SHELL_NAME="$(basename "$SHELL")"
    echo "  Add ~/.cabrero/bin to your PATH:"
    echo ""
    case "$SHELL_NAME" in
      zsh)
        echo "    echo 'export PATH=\"\$HOME/.cabrero/bin:\$PATH\"' >> ~/.zshrc"
        echo "    source ~/.zshrc"
        ;;
      bash)
        echo "    echo 'export PATH=\"\$HOME/.cabrero/bin:\$PATH\"' >> ~/.bashrc"
        echo "    source ~/.bashrc"
        ;;
      *)
        echo "    export PATH=\"\$HOME/.cabrero/bin:\$PATH\""
        ;;
    esac
    echo ""
    echo "  Then run 'cabrero setup' to complete configuration."
    ;;
esac

echo ""
