# Build from Source

This guide explains how to build DevClaw from source.

## Prerequisites

- **Go 1.24+** (with CGO enabled)
- **Node.js 22+** and npm (for WebUI)
- **Git**
- **Make** (optional, for convenience)
- **GCC/Clang** (for SQLite CGO bindings)

### Installing Prerequisites

**Linux (Debian/Ubuntu):**
```bash
sudo apt update
sudo apt install -y build-essential git
# Install Go from https://go.dev/doc/install
# Install Node.js from https://nodejs.org/
```

**Linux (Fedora/RHEL):**
```bash
sudo dnf install -y gcc gcc-c++ make git
# Install Go from https://go.dev/doc/install
# Install Node.js from https://nodejs.org/
```

**macOS:**
```bash
xcode-select --install
brew install go node git
```

## Quick Build

```bash
# Clone the repository
git clone https://github.com/jholhewres/devclaw.git
cd devclaw

# Build (includes frontend)
make build

# Run
./bin/devclaw serve
```

## Manual Build

### 1. Clone Repository

```bash
git clone https://github.com/jholhewres/devclaw.git
cd devclaw
```

### 2. Build Frontend

```bash
cd web
npm ci --no-audit --no-fund
npm run build
cd ..
```

### 3. Embed Frontend

```bash
rm -rf pkg/devclaw/webui/dist
cp -r web/dist pkg/devclaw/webui/dist
```

### 4. Build Binary

```bash
CGO_ENABLED=1 go build \
    -tags 'sqlite_fts5' \
    -ldflags="-s -w" \
    -o bin/devclaw \
    ./cmd/devclaw
```

### 5. Install (Optional)

```bash
# Install to /usr/local/bin (requires sudo)
sudo install -m 755 bin/devclaw /usr/local/bin/devclaw

# Or install to user directory
mkdir -p ~/.local/bin
install -m 755 bin/devclaw ~/.local/bin/devclaw
```

## Build Options

### Without WebUI

If you don't need the web interface:

```bash
CGO_ENABLED=1 go build \
    -tags 'sqlite_fts5' \
    -ldflags="-s -w" \
    -o bin/devclaw \
    ./cmd/devclaw
```

### With Version Info

```bash
VERSION=$(git describe --tags --always --dirty)

CGO_ENABLED=1 go build \
    -tags 'sqlite_fts5' \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o bin/devclaw \
    ./cmd/devclaw
```

### Cross-Compilation

Due to CGO/SQLite, cross-compilation requires the target platform's C compiler.

**For Linux ARM64 on Linux AMD64:**
```bash
CC=aarch64-linux-gnu-gcc CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
    go build -tags 'sqlite_fts5' -o bin/devclaw-linux-arm64 ./cmd/devclaw
```

## Development

### Run in Development Mode

```bash
# Terminal 1: Backend with hot reload
make dev

# Terminal 2: Frontend with HMR
cd web && npm run dev
```

### Run Tests

```bash
make test
```

### Lint

```bash
make lint
```

## Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build binary with frontend |
| `make build-run` | Build and run |
| `make dev` | Run in development mode |
| `make test` | Run tests |
| `make lint` | Run linters |
| `make clean` | Clean build artifacts |

## Troubleshooting

### CGO Errors

If you see CGO-related errors:
- Ensure GCC/Clang is installed
- On macOS, run `xcode-select --install`
- On Linux, install `build-essential` or `gcc`

### SQLite Errors

If you see SQLite compilation errors:
- Ensure CGO is enabled: `CGO_ENABLED=1`
- Install SQLite development headers: `apt install libsqlite3-dev`

### Node/npm Not Found

If frontend build fails:
- Install Node.js 22+ from https://nodejs.org/
- Verify: `node -v` and `npm -v`

## Next Steps

After building, run the setup wizard:

```bash
./bin/devclaw serve
# Open http://localhost:8085/setup
```
