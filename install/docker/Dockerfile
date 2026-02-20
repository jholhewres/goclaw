# ── Frontend build stage ──
FROM node:22-alpine AS frontend

WORKDIR /app/web

COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund

COPY web/ .
RUN npm run build

# ── Go build stage ──
FROM golang:1.24-alpine AS builder

RUN apk --no-cache add ca-certificates git gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/web/dist ./pkg/devclaw/webui/dist/

ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -tags 'sqlite_fts5' \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o devclaw ./cmd/devclaw

# ── Runtime stage ──
FROM alpine:3.21

# Install runtime dependencies and build tools
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    bash \
    curl \
    wget \
    git \
    jq \
    openssh-client \
    python3 \
    py3-pip \
    nodejs \
    npm \
    sqlite \
    make \
    gcc \
    g++ \
    zip \
    unzip

# Create non-root user (optional, can run as root for flexibility)
# RUN addgroup -S devclaw && adduser -S devclaw -G devclaw

WORKDIR /home/devclaw

COPY --from=builder /app/devclaw /usr/local/bin/devclaw

# Healthcheck
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8085/health || exit 1

EXPOSE 8085 8090

ENTRYPOINT ["devclaw"]
CMD ["serve"]
