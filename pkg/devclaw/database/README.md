# Database Hub

O Database Hub fornece uma camada de abstracao para multiplas conexoes de banco de dados no DevClaw.

## Visao Geral

O Hub permite:
- Usar SQLite como backend padrao (zero configuracao)
- Adicionar PostgreSQL/Supabase para producao
- Extensibilidade para MySQL, CockroachDB, etc.
- Busca vetorial nativa com pgvector
- Gerenciamento via tools do agente
- **Rate limiting** para prevenir abuso
- **Metricas do pool de conexoes** para monitoramento

## Arquitetura

```
┌─────────────────────────────────────────────────────────────┐
│                    DATABASE HUB                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   Backend   │  │   Vector    │  │  Migration  │         │
│  │   Factory   │  │   Store     │  │   Manager   │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
├─────────────────────────────────────────────────────────────┤
│                    BACKENDS                                 │
│  ┌────────┐  ┌────────────┐  ┌───────┐                     │
│  │ SQLite │  │ PostgreSQL │  │ MySQL │                     │
│  │(padrao)│  │ + pgvector │  │(futuro)│                    │
│  └────────┘  └────────────┘  └───────┘                     │
└─────────────────────────────────────────────────────────────┘
```

## Estrutura de Arquivos

```
pkg/devclaw/database/
├── interfaces.go        # Interfaces core (Backend, VectorStore, Migrator, HealthChecker)
├── config.go            # Estruturas de configuracao
├── hub.go               # Hub central, gerencia conexoes
├── factory.go           # Factory pattern para backends
├── backends/
│   ├── types.go         # Tipos compartilhados (VectorConfig, SearchResult)
│   ├── sqlite.go        # Backend SQLite + InMemoryVectorStore
│   └── postgresql.go    # Backend PostgreSQL + PgVectorStore
└── README.md            # Esta documentacao
```

## Interfaces

### Backend

```go
type Backend struct {
    Name     string
    Type     BackendType
    DB       *sql.DB
    Config   Config
    Migrator Migrator
    Vector   VectorStore
    Health   HealthChecker
}
```

### VectorStore

```go
type VectorStore interface {
    Insert(ctx context.Context, collection string, id string, vector []float32, metadata map[string]any) error
    Search(ctx context.Context, collection string, vector []float32, k int, filter map[string]any) ([]SearchResult, error)
    Delete(ctx context.Context, collection string, id string) error
    SupportsVector() bool
}
```

### Migrator

```go
type Migrator interface {
    CurrentVersion(ctx context.Context) (int, error)
    Migrate(ctx context.Context, target int) error
    NeedsMigration(ctx context.Context) (bool, error)
}
```

### HealthChecker com Metricas do Pool

```go
type HealthStatus struct {
    Healthy bool          `json:"healthy"`
    Latency time.Duration `json:"latency"`
    Version string        `json:"version"`
    Error   string        `json:"error,omitempty"`

    // Metricas do pool de conexoes
    OpenConnections   int           `json:"open_connections"`
    InUse             int           `json:"in_use"`
    Idle              int           `json:"idle"`
    WaitCount         int64         `json:"wait_count"`
    WaitDuration      time.Duration `json:"wait_duration"`
    MaxOpenConns      int           `json:"max_open_conns"`
    MaxIdleClosed     int64         `json:"max_idle_closed"`
    MaxLifetimeClosed int64         `json:"max_lifetime_closed"`
}
```

## Configuracao

### SQLite (Padrao)

```yaml
database:
  hub:
    backend: "sqlite"
    sqlite:
      path: "./data/devclaw.db"
      journal_mode: "WAL"
      busy_timeout: 5000
      foreign_keys: true
```

### PostgreSQL

```yaml
database:
  hub:
    backend: "postgresql"
    postgresql:
      host: "localhost"
      port: 5432
      database: "devclaw"
      user: "devclaw"
      password: "${POSTGRES_PASSWORD}"
      ssl_mode: "require"

      # Connection pooling
      max_open_conns: 25
      max_idle_conns: 10
      conn_max_lifetime: "30m"

      # Vector search (pgvector)
      vector:
        enabled: true
        dimensions: 1536
        index_type: "hnsw"  # hnsw | ivfflat
```

### Supabase

```yaml
database:
  hub:
    backend: "postgresql"
    postgresql:
      supabase_url: "${SUPABASE_URL}"
      password: "${SUPABASE_DB_PASSWORD}"
      vector:
        enabled: true
```

## Uso Programatico

### Inicializando o Hub

```go
import "github.com/jholhewres/devclaw/pkg/devclaw/database"

hub, err := database.NewHub(config.Hub, logger)
if err != nil {
    log.Fatal(err)
}
defer hub.Close()
```

### Obtendo Backend

```go
// Backend primario
backend := hub.Primary()

// Backend especifico
backend, err := hub.GetBackend("memory")
```

### Executando Queries

```go
ctx := context.Background()

// Query
rows, err := hub.Query(ctx, "", "SELECT * FROM users WHERE active = ?", true)
defer rows.Close()

// Exec
result, err := hub.Exec(ctx, "", "INSERT INTO users (name) VALUES (?)", "John")
```

### Busca Vetorial

```go
// Inserir embedding
vector := []float32{0.1, 0.2, 0.3, ...} // 1536 dimensoes
err := backend.Vector.Insert(ctx, "memory", "doc-123", vector, map[string]any{
    "source": "chat",
    "user_id": "user-456",
})

// Buscar similares
results, err := backend.Vector.Search(ctx, "memory", queryVector, 10, nil)
for _, r := range results {
    fmt.Printf("ID: %s, Score: %.2f\n", r.ID, r.Score)
}
```

### Health Check

```go
// Status de todos backends
status := hub.Status(ctx)
for name, info := range status {
    fmt.Printf("%s: %v\n", name, info["healthy"])
}

// Ping individual
if err := backend.Health.Ping(ctx); err != nil {
    log.Printf("backend unhealthy: %v", err)
}
```

## Tools do Agente

O Database Hub fornece tools nativas para o agente:

| Tool | Descricao | Rate Limit |
|------|-----------|------------|
| `db_hub_status` | Status de saude de todos backends com metricas do pool | Nao |
| `db_hub_query` | Executar SELECT queries | Nao |
| `db_hub_execute` | Executar INSERT/UPDATE/DELETE | Nao |
| `db_hub_schema` | Ver schema/tabelas do banco (validado contra SQL injection) | Nao |
| `db_hub_migrate` | Executar migrations | Nao |
| `db_hub_backup` | Criar backup do banco (SQLite apenas) | Nao |
| `db_hub_backends` | Listar backends disponiveis | Nao |
| `db_hub_raw` | Executar SQL raw | **10 ops/seg** |

### Seguranca

- **db_hub_schema**: Valida nomes de tabelas (apenas alphanumericos e underscore)
- **db_hub_backup**: Valida caminhos contra path traversal
- **db_hub_query**: Apenas SELECT, PRAGMA, SHOW, EXPLAIN permitidos
- **db_hub_raw**: Rate limiting de 10 operacoes/segundo por sessao

### Exemplos de Uso

```
Usuario: Qual o status do banco de dados?
Agente: [usa db_hub_status] O banco SQLite esta saudavel, versao do schema: 5.

Usuario: Mostre todas as tabelas
Agente: [usa db_hub_schema] Tabelas: jobs, session_entries, audit_log...

Usuario: Quantas sessoes temos?
Agente: [usa db_hub_query] SELECT COUNT(*) FROM session_entries → 127 sessoes.
```

## Migracao SQLite → PostgreSQL

1. Configure o PostgreSQL no YAML
2. Execute o backup: `[usa db_hub_backup]`
3. Altere o backend no YAML
4. Reinicie o DevClaw
5. O Hub criara as tabelas automaticamente
6. Restaure os dados (futuro: tool de migracao automatica)

## Comparativo

| Aspecto | SQLite | PostgreSQL + pgvector |
|---------|--------|----------------------|
| **Busca vetorial** | In-memory O(n) | Index HNSW O(log n) |
| **Concorrencia** | WAL, 1 writer | MVCC, multi writers |
| **Full-text search** | FTS5 basico | tsvector avancado |
| **Escalabilidade** | ~10k chunks | Milhoes de chunks |
| **Backup** | Copiar arquivo | PITR, replication |
| **Setup** | Zero config | Requer servidor |

## Testes

### Testes Unitarios (SQLite)

```bash
# Rodar testes
go test ./pkg/devclaw/database/... -v -cover

# Coverage report
go test ./pkg/devclaw/database/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Testes de Integracao (PostgreSQL)

Os testes de integracao requerem um PostgreSQL com pgvector:

```bash
# 1. Iniciar container PostgreSQL com pgvector
docker run -d --name devclaw-test-pg \
  -e POSTGRES_USER=test \
  -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=devclaw_test \
  -p 5432:5432 \
  pgvector/pgvector:pg16

# 2. Executar testes de integracao
go test -tags=integration ./pkg/devclaw/database/backends/... -v

# 3. Parar container
docker stop devclaw-test-pg && docker rm devclaw-test-pg
```

**Variaveis de ambiente para configuracao:**
- `PGHOST` - Host PostgreSQL (default: localhost)
- `PGPORT` - Porta PostgreSQL (default: 5432)
- `PGUSER` - Usuario (default: test)
- `PGPASSWORD` - Senha (default: test)
- `PGDATABASE` - Banco de dados (default: devclaw_test)

### Cobertura Atual

| Pacote | Cobertura |
|--------|-----------|
| `database/` | ~62% |
| `database/backends/` | ~32% |

## Extensibilidade

Para adicionar um novo backend (ex: CockroachDB):

1. Implemente as interfaces em `backends/cockroachdb.go`
2. Adicione a config em `config.go`
3. Registre o factory em `factory.go`
4. Adicione testes em `backends/cockroachdb_test.go`
