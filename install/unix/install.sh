#!/usr/bin/env bash
# DevClaw Installer — Linux + macOS
# Usage: curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/install/unix/install.sh | bash
#
# Options:
#   --url <base_url>    Base URL to download release (required)
#   --local <path>      Use local directory instead of downloading (for testing)
#   --version <tag>     Install specific version (default: latest)
#   --port <port>       Server port (default: 8090)
#   --no-prompt         Non-interactive mode
#   --dry-run           Show what would happen without making changes
#   --verbose           Show detailed output
#   --help              Show this help message
#
set -euo pipefail

# =============================================================================
# Configuration
# =============================================================================

BINARY="devclaw"
SCRIPT_VERSION="2.0.0"

# Installation directories
LINUX_INSTALL_DIR="/opt/devclaw"
MACOS_INSTALL_DIR="/usr/local/opt/devclaw"
BIN_SYMLINK="/usr/local/bin/devclaw"

# Colors
BOLD='\033[1m'
ACCENT='\033[38;2;0;229;204m'
SUCCESS='\033[38;2;0;229;204m'
WARN='\033[38;2;255;176;32m'
ERROR='\033[38;2;230;57;70m'
MUTED='\033[38;2;90;100;128m'
NC='\033[0m'

# Flags
BASE_URL=""
LOCAL_PATH=""
VERSION=""
PORT="8090"
NO_PROMPT=0
DRY_RUN=0
VERBOSE=0

# Detected platform
OS=""
ARCH=""
INSTALL_DIR=""

# Temp files for cleanup
TMPFILES=()

# Installation state
CONFIG_WAS_CREATED=0

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
ui_kv() { printf "${MUTED}%-18s${NC} %s\n" "$1:" "$2"; }

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
  curl -fsSL https://raw.githubusercontent.com/jholhewres/devclaw/master/install/unix/install.sh | bash -s -- --url <base_url>
  ./install.sh --local <path> [--version <tag>]

Options:
  --url <base_url>    Base URL to download release (required if not using --local)
                      Example: https://downloads.example.com/devclaw
  --local <path>      Use local directory instead of downloading (for testing)
                      Example: ./dist
  --version <tag>     Install specific version (default: latest, or from metadata)
  --port <port>       Server port (default: 8090)
  --no-prompt         Non-interactive mode (skip confirmations)
  --dry-run           Show what would happen without making changes
  --verbose           Show detailed output (set -x)
  --help, -h          Show this help message

Environment:
  DEVCLAW_VERSION     Version to install (same as --version)
  DEVCLAW_PORT        Server port (same as --port)
  DEVCLAW_NO_PROMPT   Set to 1 for non-interactive mode

Examples:
  # Install from remote URL (latest version)
  curl -fsSL ... | bash -s -- --url https://downloads.example.com/devclaw

  # Install from remote URL (specific version)
  curl -fsSL ... | bash -s -- --url https://downloads.example.com/devclaw --version v1.2.3 --port 3000

  # Install from local directory (for testing)
  ./install.sh --local ./dist --version v1.0.0

  # Non-interactive (for CI/CD)
  curl -fsSL ... | bash -s -- --url https://downloads.example.com/devclaw --no-prompt
EOF
}

# =============================================================================
# Argument Parsing
# =============================================================================

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --url) BASE_URL="$2"; shift 2 ;;
      --local) LOCAL_PATH="$2"; shift 2 ;;
      --version) VERSION="$2"; shift 2 ;;
      --port) PORT="$2"; shift 2 ;;
      --no-prompt) NO_PROMPT=1; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --verbose) VERBOSE=1; shift ;;
      --help|-h) print_usage; exit 0 ;;
      *) shift ;;
    esac
  done

  # Environment variable fallbacks
  VERSION="${VERSION:-${DEVCLAW_VERSION:-latest}}"
  PORT="${PORT:-${DEVCLAW_PORT:-8090}}"
  NO_PROMPT="${NO_PROMPT:-${DEVCLAW_NO_PROMPT:-0}}"

  if [[ "$VERBOSE" == "1" ]]; then
    set -x
  fi

  # Validate required arguments (either --url or --local)
  if [[ -z "$BASE_URL" && -z "$LOCAL_PATH" ]]; then
    ui_error "Missing required argument: --url or --local"
    echo ""
    echo "Usage: install.sh --url <base_url> [--version <tag>] [--port <port>]"
    echo "   or: install.sh --local <path> [--version <tag>] [--port <port>]"
    echo ""
    echo "Examples:"
    echo "  install.sh --url https://downloads.example.com/devclaw"
    echo "  install.sh --local ./dist --version v1.0.0"
    exit 1
  fi

  # Validate local path exists if specified
  if [[ -n "$LOCAL_PATH" && ! -d "$LOCAL_PATH" ]]; then
    ui_error "Local path does not exist: $LOCAL_PATH"
    exit 1
  fi

  # Remove trailing slash from URL
  if [[ -n "$BASE_URL" ]]; then
    BASE_URL="${BASE_URL%/}"
  fi
}

# =============================================================================
# Platform Detection
# =============================================================================

detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)
      OS="linux"
      INSTALL_DIR="$LINUX_INSTALL_DIR"
      ;;
    darwin)
      OS="darwin"
      INSTALL_DIR="$MACOS_INSTALL_DIR"
      ;;
    *)
      ui_error "Unsupported OS: $OS"
      echo "This installer supports Linux and macOS."
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
  ui_kv "Install dir" "$INSTALL_DIR"
}

# =============================================================================
# Dependency Installation
# =============================================================================

check_command() {
  command -v "$1" &>/dev/null
}

install_deps_linux() {
  ui_section "Installing Dependencies (Linux)"

  local pkg_manager=""
  local install_cmd=""

  if command -v apt-get &>/dev/null; then
    pkg_manager="apt"
    install_cmd="apt-get update && apt-get install -y"
  elif command -v yum &>/dev/null; then
    pkg_manager="yum"
    install_cmd="yum install -y"
  elif command -v dnf &>/dev/null; then
    pkg_manager="dnf"
    install_cmd="dnf install -y"
  else
    ui_error "No supported package manager found (apt, yum, dnf)"
    exit 1
  fi

  ui_info "Package manager: $pkg_manager"

  # Basic tools
  local basic_tools="wget curl git ca-certificates unzip"

  if [[ "$pkg_manager" == "apt" ]]; then
    basic_tools="$basic_tools build-essential"
  else
    basic_tools="$basic_tools gcc make"
  fi

  ui_info "Installing basic tools..."
  if [[ "$DRY_RUN" != "1" ]]; then
    if [[ "$(id -u)" == "0" ]]; then
      sh -c "$install_cmd $basic_tools" || ui_warn "Some basic tools may already be installed"
    else
      sudo sh -c "$install_cmd $basic_tools" || ui_warn "Some basic tools may already be installed"
    fi
  fi

  # Python 3
  ui_info "Checking Python 3..."
  if ! check_command python3; then
    ui_info "Installing Python 3..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$(id -u)" == "0" ]]; then
        sh -c "$install_cmd python3 python3-pip" || ui_warn "Python installation may need manual setup"
      else
        sudo sh -c "$install_cmd python3 python3-pip" || ui_warn "Python installation may need manual setup"
      fi
    fi
  else
    ui_success "Python 3 already installed: $(python3 --version 2>&1 || echo 'unknown')"
  fi

  # Node.js
  ui_info "Checking Node.js..."
  if ! check_command node; then
    ui_info "Installing Node.js 20.x..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$pkg_manager" == "apt" ]]; then
        curl -fsSL https://deb.nodesource.com/setup_20.x | sudo bash -
        sudo apt-get install -y nodejs
      else
        # For RHEL-based systems
        if [[ "$(id -u)" == "0" ]]; then
          curl -fsSL https://rpm.nodesource.com/setup_20.x | bash -
          $install_cmd nodejs
        else
          curl -fsSL https://rpm.nodesource.com/setup_20.x | sudo bash -
          sudo $install_cmd nodejs
        fi
      fi
    fi
  else
    local node_version
    node_version=$(node -v 2>/dev/null || echo "unknown")
    ui_success "Node.js already installed: $node_version"
  fi

  # PM2
  ui_info "Checking PM2..."
  if ! check_command pm2; then
    ui_info "Installing PM2..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$(id -u)" == "0" ]]; then
        npm install -g pm2
      else
        sudo npm install -g pm2
      fi
    fi
  else
    ui_success "PM2 already installed: $(pm2 -v 2>/dev/null || echo 'unknown')"
  fi

  # Chrome/Chromium (required for browser automation)
  ui_info "Checking Chrome/Chromium..."
  local chrome_found=""
  if command -v google-chrome &>/dev/null; then
    chrome_found="google-chrome"
  elif command -v google-chrome-stable &>/dev/null; then
    chrome_found="google-chrome-stable"
  elif command -v chromium-browser &>/dev/null; then
    chrome_found="chromium-browser"
  elif command -v chromium &>/dev/null; then
    chrome_found="chromium"
  fi

  if [[ -n "$chrome_found" ]]; then
    ui_success "Chrome/Chromium already installed: $chrome_found"
  else
    ui_info "Installing Chrome/Chromium (required for browser automation)..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$pkg_manager" == "apt" ]]; then
        # Try Chromium first (lighter, no external repo needed)
        if [[ "$(id -u)" == "0" ]]; then
          apt-get install -y chromium-browser 2>/dev/null || {
            # Fallback to Google Chrome
            ui_info "Chromium not available, installing Google Chrome..."
            wget -q -O /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
            apt-get install -y /tmp/google-chrome.deb || apt-get install -y -f /tmp/google-chrome.deb
            rm -f /tmp/google-chrome.deb
          }
        else
          sudo apt-get install -y chromium-browser 2>/dev/null || {
            ui_info "Chromium not available, installing Google Chrome..."
            wget -q -O /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
            sudo apt-get install -y /tmp/google-chrome.deb || sudo apt-get install -y -f /tmp/google-chrome.deb
            rm -f /tmp/google-chrome.deb
          }
        fi
      else
        # RHEL-based systems
        if [[ "$(id -u)" == "0" ]]; then
          $install_cmd chromium 2>/dev/null || {
            ui_info "Chromium not available, please install Chrome manually"
            ui_warn "Browser automation features will not work without Chrome/Chromium"
          }
        else
          sudo $install_cmd chromium 2>/dev/null || {
            ui_info "Chromium not available, please install Chrome manually"
            ui_warn "Browser automation features will not work without Chrome/Chromium"
          }
        fi
      fi
    fi
  fi
}

install_deps_macos() {
  ui_section "Installing Dependencies (macOS)"

  # Check for Homebrew
  if ! check_command brew; then
    ui_error "Homebrew is required for macOS installation"
    echo "Install Homebrew first: /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
    exit 1
  fi

  ui_info "Package manager: Homebrew"

  # Basic tools
  ui_info "Installing basic tools..."
  if [[ "$DRY_RUN" != "1" ]]; then
    brew install wget git curl || ui_warn "Some tools may already be installed"
  fi

  # Python 3
  ui_info "Checking Python 3..."
  if ! check_command python3; then
    ui_info "Installing Python 3..."
    if [[ "$DRY_RUN" != "1" ]]; then
      brew install python3 || ui_warn "Python installation may need manual setup"
    fi
  else
    ui_success "Python 3 already installed: $(python3 --version 2>&1 || echo 'unknown')"
  fi

  # Node.js
  ui_info "Checking Node.js..."
  if ! check_command node; then
    ui_info "Installing Node.js..."
    if [[ "$DRY_RUN" != "1" ]]; then
      brew install node@20 || brew install node || ui_warn "Node.js installation may need manual setup"
    fi
  else
    local node_version
    node_version=$(node -v 2>/dev/null || echo "unknown")
    ui_success "Node.js already installed: $node_version"
  fi

  # PM2
  ui_info "Checking PM2..."
  if ! check_command pm2; then
    ui_info "Installing PM2..."
    if [[ "$DRY_RUN" != "1" ]]; then
      npm install -g pm2
    fi
  else
    ui_success "PM2 already installed: $(pm2 -v 2>/dev/null || echo 'unknown')"
  fi

  # Chrome/Chromium (required for browser automation)
  ui_info "Checking Chrome/Chromium..."
  local chrome_path="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  if [[ -x "$chrome_path" ]]; then
    ui_success "Chrome already installed"
  elif command -v google-chrome-stable &>/dev/null; then
    ui_success "Chrome already installed: $(command -v google-chrome-stable)"
  else
    ui_info "Installing Google Chrome (required for browser automation)..."
    if [[ "$DRY_RUN" != "1" ]]; then
      brew install --cask google-chrome 2>/dev/null || {
        ui_warn "Could not install Chrome automatically"
        ui_info "Please install Chrome manually: https://www.google.com/chrome/"
        ui_warn "Browser automation features will not work without Chrome"
      }
    fi
  fi
}

install_dependencies() {
  case "$OS" in
    linux)  install_deps_linux ;;
    darwin) install_deps_macos ;;
  esac
}

# =============================================================================
# Download and Install
# =============================================================================

fetch_metadata() {
  # If using local path, read metadata directly from file
  if [[ -n "$LOCAL_PATH" ]]; then
    local metadata_file="${LOCAL_PATH}/metadata.json"

    if [[ -f "$metadata_file" ]]; then
      ui_info "Reading metadata from $metadata_file..."
      cat "$metadata_file"
      return 0
    else
      ui_warn "metadata.json not found in local path, using default values"
      echo "{\"version\": \"${VERSION:-local}\", \"binary\": \"devclaw\"}"
      return 0
    fi
  fi

  # Remote mode: download via curl
  local metadata_url="${BASE_URL}/metadata.json"

  ui_info "Fetching metadata from $metadata_url..."

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would fetch: $metadata_url"
    echo '{"version": "dry-run", "binary": "devclaw"}'
    return 0
  fi

  local metadata
  metadata=$(curl -fsSL "$metadata_url" 2>/dev/null) || {
    ui_warn "Could not fetch metadata.json, using default values"
    echo "{\"version\": \"${VERSION}\", \"binary\": \"devclaw\"}"
    return 0
  }

  echo "$metadata"
}

resolve_version() {
  local metadata="$1"

  if [[ "$VERSION" == "latest" ]]; then
    local detected_version
    detected_version=$(echo "$metadata" | grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
    if [[ -n "$detected_version" ]]; then
      VERSION="$detected_version"
      ui_kv "Version" "$VERSION (from metadata)"
    else
      VERSION="latest"
      ui_warn "Could not detect version from metadata, using 'latest'"
    fi
  else
    ui_kv "Version" "$VERSION (specified)"
  fi
}

download_and_install() {
  local tmpdir
  tmpdir="$(mktempdir)"

  local archive="devclaw-${VERSION}.zip"

  # Local mode: copy from local directory
  if [[ -n "$LOCAL_PATH" ]]; then
    ui_section "Installing from Local Directory"

    local local_archive="${LOCAL_PATH}/${archive}"

    # If specific version zip doesn't exist, try to find any zip file
    if [[ ! -f "$local_archive" ]]; then
      local found_zip
      found_zip=$(find "$LOCAL_PATH" -name "devclaw*.zip" -type f 2>/dev/null | head -1)
      if [[ -n "$found_zip" ]]; then
        local_archive="$found_zip"
        ui_info "Found archive: $local_archive"
      fi
    fi

    if [[ -f "$local_archive" ]]; then
      ui_info "Using local archive: $local_archive"

      if [[ "$DRY_RUN" == "1" ]]; then
        ui_info "[dry-run] Would extract to: $INSTALL_DIR"
        return 0
      fi

      # Extract
      ui_info "Extracting..."
      unzip -q -o "$local_archive" -d "${tmpdir}/extracted"
    else
      # No archive found, copy files directly from local path
      ui_info "No archive found, copying files from: $LOCAL_PATH"

      if [[ "$DRY_RUN" == "1" ]]; then
        ui_info "[dry-run] Would copy files to: $INSTALL_DIR"
        return 0
      fi

      # Create extraction directory and copy files
      mkdir -p "${tmpdir}/extracted"

      # Copy binary if it exists
      if [[ -f "${LOCAL_PATH}/devclaw" ]]; then
        cp "${LOCAL_PATH}/devclaw" "${tmpdir}/extracted/"
      elif [[ -f "${LOCAL_PATH}/devclaw-linux-amd64" ]]; then
        cp "${LOCAL_PATH}/devclaw-linux-amd64" "${tmpdir}/extracted/devclaw"
      fi

      # Copy ecosystem.config.js if it exists
      if [[ -f "${LOCAL_PATH}/ecosystem.config.js" ]]; then
        cp "${LOCAL_PATH}/ecosystem.config.js" "${tmpdir}/extracted/"
      elif [[ -f "${PWD}/install/unix/ecosystem.config.js" ]]; then
        cp "${PWD}/install/unix/ecosystem.config.js" "${tmpdir}/extracted/"
      fi
    fi
  else
    # Remote mode: download via curl
    local download_url="${BASE_URL}/${archive}"

    ui_section "Downloading DevClaw"

    ui_info "Downloading: $download_url"

    if [[ "$DRY_RUN" == "1" ]]; then
      ui_info "[dry-run] Would download to: ${tmpdir}/${archive}"
      ui_info "[dry-run] Would extract to: $INSTALL_DIR"
      return 0
    fi

    # Download
    if ! curl -fsSL --progress-bar -o "${tmpdir}/${archive}" "$download_url"; then
      ui_error "Failed to download: $download_url"
      echo ""
      echo "Make sure the release exists at:"
      echo "  ${BASE_URL}/${archive}"
      exit 1
    fi

    ui_success "Download complete"

    # Extract
    ui_info "Extracting..."
    unzip -q -o "${tmpdir}/${archive}" -d "${tmpdir}/extracted"
  fi

  # Verify we have something to install
  if [[ ! -f "${tmpdir}/extracted/devclaw" ]]; then
    ui_error "No devclaw binary found to install"
    exit 1
  fi

  # Create install directory
  ui_info "Installing to $INSTALL_DIR..."
  if [[ "$(id -u)" == "0" ]]; then
    mkdir -p "$INSTALL_DIR"
  else
    sudo mkdir -p "$INSTALL_DIR"
  fi

  # Copy files
  if [[ "$(id -u)" == "0" ]]; then
    cp -r "${tmpdir}/extracted/"* "$INSTALL_DIR/"
  else
    sudo cp -r "${tmpdir}/extracted/"* "$INSTALL_DIR/"
  fi

  # Make binary executable
  if [[ "$(id -u)" == "0" ]]; then
    chmod +x "${INSTALL_DIR}/devclaw"
  else
    sudo chmod +x "${INSTALL_DIR}/devclaw"
  fi

  ui_success "Binary installed"
}

setup_directories() {
  ui_section "Setting Up Directories"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would create directories in $INSTALL_DIR"
    return 0
  fi

  local dirs=("data" "sessions" "skills" "logs")

  for dir in "${dirs[@]}"; do
    local full_path="${INSTALL_DIR}/${dir}"
    if [[ ! -d "$full_path" ]]; then
      ui_info "Creating: $full_path"
      if [[ "$(id -u)" == "0" ]]; then
        mkdir -p "$full_path"
      else
        sudo mkdir -p "$full_path"
      fi
    fi
  done

  # Set permissions for sensitive directories
  local secure_dirs=("data" "sessions")
  for dir in "${secure_dirs[@]}"; do
    local full_path="${INSTALL_DIR}/${dir}"
    if [[ "$(id -u)" == "0" ]]; then
      chmod 700 "$full_path"
    else
      sudo chmod 700 "$full_path"
    fi
  done

  ui_success "Directories created"
}

setup_global_command() {
  ui_section "Setting Up Global Command"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would create symlink: $BIN_SYMLINK -> ${INSTALL_DIR}/devclaw"
    return 0
  fi

  # Remove existing symlink or file
  if [[ -L "$BIN_SYMLINK" ]] || [[ -f "$BIN_SYMLINK" ]]; then
    if [[ "$(id -u)" == "0" ]]; then
      rm -f "$BIN_SYMLINK"
    else
      sudo rm -f "$BIN_SYMLINK"
    fi
  fi

  # Create symlink
  if [[ "$(id -u)" == "0" ]]; then
    ln -sf "${INSTALL_DIR}/devclaw" "$BIN_SYMLINK"
  else
    sudo ln -sf "${INSTALL_DIR}/devclaw" "$BIN_SYMLINK"
  fi

  ui_success "Global command available: devclaw"
}

generate_config() {
  ui_section "Generating Configuration"

  local config_file="${INSTALL_DIR}/config.yaml"

  if [[ -f "$config_file" ]]; then
    ui_info "Config already exists: $config_file"
    return 0
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would skip config generation (DevClaw will create on first run)"
    return 0
  fi

  # Don't create config.yaml - let DevClaw create it on first run
  # This allows DevClaw to enter "First Run Setup" mode automatically
  ui_info "Skipping config generation (DevClaw will create on first run)"
  ui_info "DevClaw will start in 'First Run Setup' mode"

  CONFIG_WAS_CREATED=1
}

update_pm2_config() {
  ui_section "Configuring PM2"

  local ecosystem_file="${INSTALL_DIR}/ecosystem.config.js"

  if [[ ! -f "$ecosystem_file" ]]; then
    ui_warn "ecosystem.config.js not found, skipping PM2 setup"
    return 0
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would update ecosystem.config.js with:"
    ui_info "  PORT=$PORT"
    ui_info "  DEVCLAW_STATE_DIR=$INSTALL_DIR"
    return 0
  fi

  # Update paths in ecosystem.config.js
  if [[ "$(id -u)" == "0" ]]; then
    sed -i.bak "s|/opt/devclaw|${INSTALL_DIR}|g" "$ecosystem_file"
    rm -f "${ecosystem_file}.bak"
  else
    sudo sed -i.bak "s|/opt/devclaw|${INSTALL_DIR}|g" "$ecosystem_file"
    sudo rm -f "${ecosystem_file}.bak"
  fi

  ui_success "PM2 config updated"
}

start_with_pm2() {
  ui_section "Starting with PM2"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would run: pm2 start ${INSTALL_DIR}/ecosystem.config.js"
    ui_info "[dry-run] Would run: pm2 save"
    return 0
  fi

  cd "$INSTALL_DIR"

  # Stop existing process if running
  if pm2 describe devclaw &>/dev/null; then
    ui_info "Stopping existing devclaw process..."
    pm2 delete devclaw &>/dev/null || true
  fi

  # Start with PM2
  ui_info "Starting devclaw..."
  pm2 start ecosystem.config.js

  # Save PM2 configuration
  pm2 save

  if [[ "$CONFIG_WAS_CREATED" == "1" ]]; then
    ui_success "DevClaw started with PM2 (First Run Setup mode)"
    # Get server IP for display
    local server_ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "SERVER_IP")
    ui_info "Access http://${server_ip}:${PORT}/setup to complete configuration"
  else
    ui_success "DevClaw started with PM2"
  fi
}

print_pm2_startup() {
  if [[ "$DRY_RUN" == "1" ]]; then
    return 0
  fi

  # Skip if config was just created (service not started yet)
  if [[ "$CONFIG_WAS_CREATED" == "1" ]]; then
    return 0
  fi

  echo ""
  ui_info "To enable auto-start on boot, run:"
  echo ""
  echo "    pm2 startup"
  echo ""
  echo "  Then run the command it prints."
}

print_success() {
  printf "\n"
  printf "${SUCCESS}${BOLD}  ✓ DevClaw installed successfully!${NC}\n"
  printf "\n"
  printf "  Installation:\n"
  printf "    Binary:     %s\n" "${INSTALL_DIR}/devclaw"
  printf "    Config:     %s\n" "${INSTALL_DIR}/config.yaml"
  printf "    Data:       %s\n" "${INSTALL_DIR}/data/"
  printf "    Logs:       %s\n" "${INSTALL_DIR}/logs/"
  printf "\n"

  if [[ "$CONFIG_WAS_CREATED" == "1" ]]; then
    # Get server IP for display
    local server_ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "SERVER_IP")
    printf "  ${ACCENT}${BOLD}→ First Run Setup${NC}\n"
    printf "\n"
    printf "  DevClaw is running in setup mode.\n"
    printf "  Complete the configuration by accessing:\n"
    printf "\n"
    printf "    http://%s:%s/setup\n" "$server_ip" "${PORT}"
    printf "\n"
    printf "  Or run manually:\n"
    printf "    devclaw setup\n"
    printf "\n"
  fi

  # Get server IP for display
  local display_ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")

  printf "  Commands:\n"
  printf "    devclaw --version    # Check version\n"
  printf "    devclaw serve        # Start server (foreground)\n"
  printf "    pm2 status           # Check PM2 status\n"
  printf "    pm2 logs devclaw     # View logs\n"
  printf "    pm2 restart devclaw  # Restart service\n"
  printf "    pm2 stop devclaw     # Stop service\n"
  printf "\n"
  printf "  Web UI:\n"
  printf "    http://%s:%s\n" "$display_ip" "${PORT}"
  printf "    http://%s:%s/setup\n" "$display_ip" "${PORT}"
  printf "\n"
}

# =============================================================================
# Main
# =============================================================================

print_install_plan() {
  ui_section "Install Plan"

  if [[ -n "$LOCAL_PATH" ]]; then
    ui_kv "Source" "local directory"
    ui_kv "Local path" "$LOCAL_PATH"
  else
    ui_kv "Source" "remote URL"
    ui_kv "Base URL" "$BASE_URL"
  fi

  ui_kv "Version" "$VERSION"
  ui_kv "Port" "$PORT"
  ui_kv "Platform" "${OS}/${ARCH}"
  ui_kv "Install dir" "$INSTALL_DIR"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_kv "Mode" "dry-run (no changes)"
  fi
}

confirm_install() {
  if [[ "$NO_PROMPT" == "1" ]] || [[ "$DRY_RUN" == "1" ]]; then
    return 0
  fi

  echo ""
  read -p "Continue with installation? [y/N] " -n 1 -r
  echo ""

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    ui_info "Installation cancelled"
    exit 0
  fi
}

main() {
  parse_args "$@"
  print_banner
  detect_platform

  local metadata
  metadata=$(fetch_metadata)
  resolve_version "$metadata"

  print_install_plan
  confirm_install

  install_dependencies
  download_and_install
  setup_directories
  setup_global_command
  generate_config
  update_pm2_config
  start_with_pm2

  print_pm2_startup
  print_success
}

main "$@"
