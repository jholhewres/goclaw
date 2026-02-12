# Build stage
FROM golang:1.23-alpine AS builder

RUN apk --no-cache add ca-certificates git

WORKDIR /app

# Cache de dependências
COPY go.mod go.sum ./
RUN go mod download

# Build do binário
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o copilot ./cmd/copilot

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

# Cria usuário não-root para segurança
RUN addgroup -S copilot && adduser -S copilot -G copilot

USER copilot
WORKDIR /home/copilot

# Copia binário e config de exemplo
COPY --from=builder /app/copilot /usr/local/bin/copilot
COPY --from=builder /app/configs/copilot.example.yaml /etc/copilot/config.example.yaml

# Volumes para persistência de sessões e dados
VOLUME ["/home/copilot/sessions", "/home/copilot/data"]

# Health check via comando `copilot health`.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["copilot", "health"]

ENTRYPOINT ["copilot"]
CMD ["serve", "--config", "/etc/copilot/config.yaml"]
