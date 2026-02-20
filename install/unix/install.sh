#!/usr/bin/env bash
# DevClaw Installer — macOS + Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/install/unix/install.sh | bash
#
# Options:
#   --version <tag>     Install specific version (default: latest)
#   --no-prompt         Non-interactive mode
#   --dry-run           Show what would happen without making changes
#   --verbose           Show detailed output
#   --help              Show this help message
#
set -euo pipefail

# =============================================================================
# Configuration
# =============================================================================

REPO="jholhewres/devclaw"
BINARY="devclaw"
INSTALL_DIR="/usr/local/bin"
FALLBACK_DIR="$HOME/.local/bin"
SCRIPT_VERSION="1.0.0"

# Colors
BOLD='\033[1m'
ACCENT='\033[38;2;0;229;204m'    # cyan #00e5cc
SUCCESS='\033[38;2;0;229;204m'   # cyan
WARN='\033[38;2;255;176;32m'     # amber
ERROR='\033[38;2;230;57;70m'     # coral
MUTED='\033[38;2;90;100;128m'    # gray
NC='\033[0m'

# Flags
VERSION=""
NO_PROMPT=0
DRY_RUN=0
VERBOSE=0

# Temp files for cleanup
TMPFILES=()

# =============================================================================
# Utility Functions
# =============================================================================

cleanup() {
  for f in "${TMPFILES[@]:-}"; do
    rm -rf "$f" 2>/dev/null || true
  done
}
trap cleanup EXIT

mktempdir() {
  local d
  d="$(mktemp -d)"
  TMPFILES+=("$d")
  echo "$d"
}

ui_info()  { printf "${MUTED}·${NC} %s\n" "$1"; }
ui_warn()  { printf "${WARN}!${NC} %s\n" "$1"; }
ui_error() { printf "${ERROR}✗${NC} %s\n" "$1" >&2; }
ui_success() { printf "${SUCCESS}✓${NC} %s\n" "$1"; }
ui_section() { printf "\n${ACCENT}${BOLD}%s${NC}\n" "$1"; }
ui_kv() { printf "${MUTED}%-16s${NC} %s\n" "$1:" "$2"; }

print_banner() {
  printf "\n"
  printf "${ACCENT}${BOLD}  ╔══════════════════════════════════════╗${NC}\n"
  printf "${ACCENT}${BOLD}  ║${NC}   ${BOLD}DevClaw${NC} — AI Agent for Tech Teams    ${ACCENT}${BOLD}║${NC}\n"
  printf "${ACCENT}${BOLD}  ╚══════════════════════════════════════╝${NC}\n"
  printf "\n"
}

print_usage() {
  cat <<EOF
DevClaw Installer v${SCRIPT_VERSION}

Usage:
  curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/install/unix/install.sh | bash

Options:
  --version <tag>     Install specific version (default: latest)
  --no-prompt         Non-interactive mode (skip confirmations)
  --dry-run           Show what would happen without making changes
  --verbose           Show detailed output (set -x)
  --help, -h          Show this help message

Environment:
  DEVCLAW_VERSION     Version to install (same as --version)
  DEVCLAW_NO_PROMPT   Set to 1 for non-interactive mode

Examples:
  # Install latest version
  curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/install/unix/install.sh | bash

  # Install specific version
  curl -fsSL ... | bash -s -- --version v1.2.3

  # Non-interactive (for CI/CD)
  curl -fsSL ... | bash -s -- --no-prompt
EOF
}

# =============================================================================
# Argument Parsing
# =============================================================================

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version) VERSION="$2"; shift 2 ;;
      --no-prompt) NO_PROMPT=1; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --verbose) VERBOSE=1; shift ;;
      --help|-h) print_usage; exit 0 ;;
      *) shift ;;
    esac
  done

  # Environment variable fallbacks
  VERSION="${VERSION:-${DEVCLAW_VERSION:-}}"
  NO_PROMPT="${NO_PROMPT:-${DEVCLAW_NO_PROMPT:-0}}"

  if [[ "$VERBOSE" == "1" ]]; then
    set -x
  fi
}

# =============================================================================
# Platform Detection
# =============================================================================

detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)
      ui_error "Unsupported OS: $OS"
      echo "This installer supports macOS and Linux."
      echo "For Windows, use: iwr -useb https://raw.githubusercontent.com/jholhewres/devclaw/master/install/windows/install.ps1 | iex"
      exit 1
      ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
      ui_error "Unsupported architecture: $ARCH"
      exit 1
      ;;
  esac

  ui_kv "Platform" "${OS}/${ARCH}"
}

# =============================================================================
# Version Detection
# =============================================================================

get_latest_version() {
  if [[ -n "$VERSION" ]]; then
    ui_kv "Version" "$VERSION (specified)"
    return
  fi

  ui_info "Fetching latest version..."

  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/' 2>/dev/null || true)

  if [[ -z "$VERSION" ]]; then
    VERSION="latest"
    ui_warn "Could not determine latest version, using 'latest'"
  else
    ui_kv "Version" "$VERSION"
  fi
}

# =============================================================================
# Installation Methods
# =============================================================================

install_binary() {
  local src="$1"
  local target

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would install binary to ${INSTALL_DIR} or ${FALLBACK_DIR}"
    return 0
  fi

  if [[ -w "$INSTALL_DIR" ]] || [[ "$NO_PROMPT" == "1" && "$(id -u)" == "0" ]]; then
    target="$INSTALL_DIR"
    install -m 755 "$src" "${target}/${BINARY}"
  elif command -v sudo &>/dev/null; then
    target="$INSTALL_DIR"
    ui_info "Installing to ${target} (requires sudo)..."
    sudo install -m 755 "$src" "${target}/${BINARY}"
  else
    target="$FALLBACK_DIR"
    mkdir -p "$target"
    install -m 755 "$src" "${target}/${BINARY}"
  fi

  ui_success "Installed to ${target}/${BINARY}"

  if ! command -v "$BINARY" &>/dev/null; then
    printf "\n"
    ui_warn "${target} is not in your PATH"
    echo "  Add to ~/.bashrc or ~/.zshrc:"
    echo "    export PATH=\"${target}:\$PATH\""
    printf "\n"
  fi
}

try_download_binary() {
  local archive="${BINARY}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"

  ui_info "Downloading binary from GitHub Releases..."

  local tmpdir
  tmpdir="$(mktempdir)"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would download: $url"
    return 1
  fi

  if ! curl -fsSL --progress-bar -o "${tmpdir}/${archive}" "$url" 2>/dev/null; then
    ui_info "Binary not available for this platform, trying source build..."
    return 1
  fi

  # Verify checksum
  local checksum_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
  if curl -fsSL -o "${tmpdir}/checksums.txt" "$checksum_url" 2>/dev/null; then
    local expected
    expected=$(grep "$archive" "${tmpdir}/checksums.txt" | awk '{print $1}')
    if [[ -n "$expected" ]]; then
      local actual
      actual=$(sha256sum "${tmpdir}/${archive}" 2>/dev/null || shasum -a 256 "${tmpdir}/${archive}" | awk '{print $1}')
      if [[ "$expected" != "$actual" ]]; then
        ui_warn "Checksum mismatch, falling back to source build"
        return 1
      fi
      ui_success "Checksum verified"
    fi
  fi

  ui_info "Extracting..."
  tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"

  install_binary "${tmpdir}/${BINARY}"
  return 0
}

try_go_install() {
  if ! command -v go &>/dev/null; then
    return 1
  fi

  local go_version
  go_version=$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//' || echo "0.0")
  local go_major go_minor
  go_major=$(echo "$go_version" | cut -d. -f1)
  go_minor=$(echo "$go_version" | cut -d. -f2)

  if [[ "$go_major" -lt 1 ]] || { [[ "$go_major" -eq 1 ]] && [[ "$go_minor" -lt 24 ]]; }; then
    ui_warn "Go ${go_version} found but 1.24+ required"
    return 1
  fi

  ui_info "Installing via go install (no WebUI)..."

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would run: CGO_ENABLED=1 go install -tags 'sqlite_fts5' github.com/${REPO}/cmd/devclaw@latest"
    return 1
  fi

  CGO_ENABLED=1 go install -tags 'sqlite_fts5' "github.com/${REPO}/cmd/devclaw@latest" 2>/dev/null || return 1

  local gobin
  gobin=$(go env GOPATH)/bin
  if [[ -f "${gobin}/${BINARY}" ]]; then
    ui_success "Installed to ${gobin}/${BINARY}"
    if ! command -v "$BINARY" &>/dev/null; then
      printf "\n"
      ui_info "Add Go bin to your PATH:"
      echo "  export PATH=\"${gobin}:\$PATH\""
      printf "\n"
    fi
    return 0
  fi

  return 1
}

try_build_from_source() {
  if ! command -v go &>/dev/null; then
    ui_warn "Go not found, cannot build from source"
    return 1
  fi
  if ! command -v git &>/dev/null; then
    ui_warn "Git not found, cannot build from source"
    return 1
  fi

  ui_section "Building from Source"

  local tmpdir
  tmpdir="$(mktempdir)"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would clone and build from source"
    return 1
  fi

  ui_info "Cloning repository..."
  git clone --depth 1 "https://github.com/${REPO}.git" "${tmpdir}/devclaw" 2>/dev/null || return 1

  cd "${tmpdir}/devclaw"

  # Build frontend if node is available
  if command -v node &>/dev/null && command -v npm &>/dev/null; then
    local node_version
    node_version=$(node -v 2>/dev/null | cut -d'v' -f2 | cut -d'.' -f1 || echo "0")
    if [[ "$node_version" -ge 22 ]]; then
      ui_info "Building frontend (Node.js $(node -v))..."
      (cd web && npm ci --no-audit --no-fund 2>/dev/null && npm run build 2>/dev/null) || {
        ui_warn "Frontend build failed, continuing without WebUI"
      }
      if [[ -d web/dist ]]; then
        rm -rf pkg/devclaw/webui/dist
        cp -r web/dist pkg/devclaw/webui/dist
      fi
    else
      ui_warn "Node.js $(node -v 2>/dev/null || echo 'unknown') found but 22+ required, skipping WebUI"
    fi
  else
    ui_warn "Node.js not found — skipping WebUI build"
  fi

  ui_info "Building Go binary..."
  CGO_ENABLED=1 go build -tags 'sqlite_fts5' -ldflags="-s -w" -o "${tmpdir}/${BINARY}" ./cmd/devclaw 2>/dev/null || return 1

  install_binary "${tmpdir}/${BINARY}"
  return 0
}

# =============================================================================
# Main
# =============================================================================

print_install_plan() {
  ui_section "Install Plan"
  ui_kv "Repository" "github.com/${REPO}"
  ui_kv "Binary" "$BINARY"
  ui_kv "Install dir" "${INSTALL_DIR} (or ${FALLBACK_DIR})"
  if [[ "$DRY_RUN" == "1" ]]; then
    ui_kv "Mode" "dry-run (no changes)"
  fi
}

print_success() {
  printf "\n"
  printf "${SUCCESS}${BOLD}  ✓ DevClaw installed successfully!${NC}\n"
  printf "\n"
  echo "  Getting started:"
  echo ""
  echo "    devclaw serve        # start server + setup wizard"
  echo "    devclaw setup        # interactive CLI setup"
  echo "    devclaw --help       # all commands"
  echo ""
  echo "    Setup wizard: http://localhost:8085/setup"
  printf "\n"
}

print_fallback_help() {
  printf "\n"
  ui_error "Automatic installation failed"
  printf "\n"
  echo "Try one of these alternatives:"
  printf "\n"
  echo "  ${BOLD}Option 1: Docker${NC} (recommended)"
  echo "    git clone https://github.com/${REPO}.git"
  echo "    cd devclaw && docker compose -f install/docker/docker-compose.yml up -d"
  echo ""
  echo "  ${BOLD}Option 2: Build from source${NC}"
  echo "    # Requires: Go 1.24+ and Node 22+"
  echo "    git clone https://github.com/${REPO}.git"
  echo "    cd devclaw && make build && ./bin/devclaw serve"
  printf "\n"
}

main() {
  parse_args "$@"
  print_banner
  detect_platform
  get_latest_version
  print_install_plan

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "Dry run complete (no changes made)"
    exit 0
  fi

  # Strategy: binary download → go install → build from source
  if try_download_binary; then
    print_success
  elif try_go_install; then
    print_success
  elif try_build_from_source; then
    print_success
  else
    print_fallback_help
    exit 1
  fi
}

main "$@"
