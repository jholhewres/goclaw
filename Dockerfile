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

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /home/devclaw

COPY --from=builder /app/devclaw /usr/local/bin/devclaw

EXPOSE 8080 8090

ENTRYPOINT ["devclaw"]
CMD ["serve", "--config", "/etc/devclaw/config.yaml"]
