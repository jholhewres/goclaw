#!/usr/bin/env bash
# DevClaw installer — macOS + Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/scripts/install/install.sh | bash
set -euo pipefail

REPO="jholhewres/devclaw"
BINARY="devclaw"
INSTALL_DIR="/usr/local/bin"
FALLBACK_DIR="$HOME/.local/bin"

info()  { printf '\033[1;34m[info]\033[0m  %s\n' "$1"; }
ok()    { printf '\033[1;32m[ok]\033[0m    %s\n' "$1"; }
warn()  { printf '\033[1;33m[warn]\033[0m  %s\n' "$1"; }
err()   { printf '\033[1;31m[error]\033[0m %s\n' "$1" >&2; exit 1; }

detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      err "Unsupported OS: $OS" ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             err "Unsupported architecture: $ARCH" ;;
  esac

  info "Detected: ${OS}/${ARCH}"
}

get_latest_version() {
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/' 2>/dev/null || true)

  if [ -z "$VERSION" ]; then
    VERSION="latest"
    warn "Could not determine latest version tag"
  else
    info "Latest version: ${VERSION}"
  fi
}

try_download_binary() {
  ARCHIVE="${BINARY}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT

  info "Trying binary download..."
  if curl -fsSL -o "${TMPDIR}/${ARCHIVE}" "$URL" 2>/dev/null; then
    # Verify checksum if available
    CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
    if curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUM_URL" 2>/dev/null; then
      EXPECTED=$(grep "$ARCHIVE" "${TMPDIR}/checksums.txt" | awk '{print $1}')
      if [ -n "$EXPECTED" ]; then
        ACTUAL=$(sha256sum "${TMPDIR}/${ARCHIVE}" 2>/dev/null || shasum -a 256 "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
        ACTUAL=$(echo "$ACTUAL" | awk '{print $1}')
        if [ "$EXPECTED" != "$ACTUAL" ]; then
          warn "Checksum mismatch, falling back to source build"
          return 1
        fi
        ok "Checksum verified"
      fi
    fi

    info "Extracting..."
    tar -xzf "${TMPDIR}/${ARCHIVE}" -C "${TMPDIR}"
    install_binary "${TMPDIR}/${BINARY}"
    return 0
  fi

  return 1
}

try_go_install() {
  if ! command -v go &>/dev/null; then
    return 1
  fi

  GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
  GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
  GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)

  if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 24 ]; }; then
    warn "Go ${GO_VERSION} found but 1.24+ required"
    return 1
  fi

  info "Building from source with Go ${GO_VERSION}..."
  CGO_ENABLED=1 go install -tags 'sqlite_fts5' "github.com/${REPO}/cmd/devclaw@latest" || return 1

  GOBIN=$(go env GOPATH)/bin
  if [ -f "${GOBIN}/${BINARY}" ]; then
    ok "Installed to ${GOBIN}/${BINARY}"
    if ! command -v "$BINARY" &>/dev/null; then
      echo ""
      info "Add Go bin to your PATH:"
      echo "  export PATH=\"${GOBIN}:\$PATH\""
      echo ""
    fi
    return 0
  fi

  return 1
}

try_build_from_source() {
  if ! command -v go &>/dev/null; then
    return 1
  fi
  if ! command -v git &>/dev/null; then
    return 1
  fi

  info "Cloning and building from source..."
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT

  git clone --depth 1 "https://github.com/${REPO}.git" "${TMPDIR}/devclaw" || return 1
  cd "${TMPDIR}/devclaw"

  # Build frontend if node is available
  if command -v node &>/dev/null && command -v npm &>/dev/null; then
    info "Building frontend..."
    (cd web && npm ci --no-audit --no-fund && npm run build) || warn "Frontend build failed, continuing without WebUI"
    if [ -d web/dist ]; then
      rm -rf pkg/devclaw/webui/dist
      cp -r web/dist pkg/devclaw/webui/dist
    fi
  else
    warn "Node.js not found — skipping WebUI build"
  fi

  info "Building Go binary..."
  CGO_ENABLED=1 go build -tags 'sqlite_fts5' -ldflags="-s -w" -o "${TMPDIR}/${BINARY}" ./cmd/devclaw || return 1

  install_binary "${TMPDIR}/${BINARY}"
  return 0
}

install_binary() {
  local src="$1"

  if [ -w "$INSTALL_DIR" ]; then
    TARGET="$INSTALL_DIR"
  elif command -v sudo &>/dev/null; then
    TARGET="$INSTALL_DIR"
    info "Installing to ${TARGET} (requires sudo)..."
    sudo install -m 755 "$src" "${TARGET}/${BINARY}"
    ok "Installed to ${TARGET}/${BINARY}"
    return
  else
    TARGET="$FALLBACK_DIR"
    mkdir -p "$TARGET"
    info "No sudo available, installing to ${TARGET}"
  fi

  install -m 755 "$src" "${TARGET}/${BINARY}"
  ok "Installed to ${TARGET}/${BINARY}"

  if ! command -v "$BINARY" &>/dev/null; then
    echo ""
    info "Add ${TARGET} to your PATH:"
    echo "  export PATH=\"${TARGET}:\$PATH\""
    echo ""
  fi
}

main() {
  echo ""
  echo "  ╔══════════════════════════════════════╗"
  echo "  ║   DevClaw — AI Agent for Tech Teams  ║"
  echo "  ╚══════════════════════════════════════╝"
  echo ""

  detect_platform
  get_latest_version

  # Strategy: try binary download → go install → build from source
  if try_download_binary; then
    : # success
  elif try_go_install; then
    : # success
  elif try_build_from_source; then
    : # success
  else
    echo ""
    err "Installation failed. Try manually:

  # Option 1: Docker (no Go required)
  git clone https://github.com/${REPO}.git && cd devclaw
  docker compose up -d
  # Open http://localhost:8085

  # Option 2: From source (requires Go 1.24+ and Node 22+)
  git clone https://github.com/${REPO}.git && cd devclaw
  make build
  ./bin/devclaw serve"
  fi

  echo ""
  ok "DevClaw installed! Getting started:"
  echo ""
  echo "  devclaw serve        # start server + setup wizard"
  echo "  devclaw setup        # interactive CLI setup"
  echo "  devclaw --help       # all commands"
  echo ""
  echo "  Setup wizard: http://localhost:8085/setup"
  echo ""
}

main
