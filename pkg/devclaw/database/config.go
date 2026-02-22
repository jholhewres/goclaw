package database

import (
	"time"
)

// HubConfig represents the complete database hub configuration.
type HubConfig struct {
	// Backend is the primary database backend type (default: "sqlite")
	Backend BackendType `yaml:"backend"`

	// SQLite configuration
	SQLite SQLiteConfig `yaml:"sqlite"`

	// PostgreSQL configuration (includes Supabase)
	PostgreSQL PostgreSQLConfig `yaml:"postgresql"`

	// MySQL configuration
	MySQL MySQLConfig `yaml:"mysql"`

	// Additional named connections for multi-database setups
	Connections map[string]Config `yaml:"connections"`

	// Memory database configuration (can differ from primary)
	Memory MemoryDBConfig `yaml:"memory"`
}

// Config represents a generic database connection configuration.
type Config struct {
	// Type identifies the backend type
	Type BackendType `yaml:"type"`

	// Path is for SQLite databases
	Path string `yaml:"path"`

	// Host is for network databases (PostgreSQL, MySQL)
	Host string `yaml:"host"`

	// Port is for network databases
	Port int `yaml:"port"`

	// Database name
	Database string `yaml:"database"`

	// User for authentication
	User string `yaml:"user"`

	// Password for authentication (supports ${ENV_VAR} expansion)
	Password string `yaml:"password"`

	// SSLMode for PostgreSQL: disable, require, verify-full
	SSLMode string `yaml:"ssl_mode"`

	// Connection pooling
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`

	// Supabase-specific
	SupabaseURL    string `yaml:"supabase_url"`
	SupabaseAnonKey string `yaml:"supabase_anon_key"`

	// Vector search configuration
	Vector VectorConfig `yaml:"vector"`

	// Journal mode for SQLite (default: WAL)
	JournalMode string `yaml:"journal_mode"`

	// Busy timeout for SQLite in milliseconds (default: 5000)
	BusyTimeout int `yaml:"busy_timeout"`
}

// SQLiteConfig holds SQLite-specific configuration.
type SQLiteConfig struct {
	// Path to the database file (default: "./data/devclaw.db")
	Path string `yaml:"path"`

	// Journal mode (default: WAL)
	JournalMode string `yaml:"journal_mode"`

	// Busy timeout in milliseconds (default: 5000)
	BusyTimeout int `yaml:"busy_timeout"`

	// Enable foreign keys (default: true)
	ForeignKeys bool `yaml:"foreign_keys"`
}

// PostgreSQLConfig holds PostgreSQL and Supabase configuration.
type PostgreSQLConfig struct {
	// Host (default: "localhost")
	Host string `yaml:"host"`

	// Port (default: 5432)
	Port int `yaml:"port"`

	// Database name
	Database string `yaml:"database"`

	// User for authentication
	User string `yaml:"user"`

	// Password for authentication (supports ${ENV_VAR} expansion)
	Password string `yaml:"password"`

	// SSL mode: disable, require, verify-ca, verify-full
	SSLMode string `yaml:"ssl_mode"`

	// Connection pooling
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`

	// Supabase-specific (alternative to standard config)
	SupabaseURL     string `yaml:"supabase_url"`
	SupabaseAnonKey string `yaml:"supabase_anon_key"`

	// Vector search (pgvector)
	Vector VectorConfig `yaml:"vector"`
}

// MySQLConfig holds MySQL configuration.
type MySQLConfig struct {
	// Host (default: "localhost")
	Host string `yaml:"host"`

	// Port (default: 3306)
	Port int `yaml:"port"`

	// Database name
	Database string `yaml:"database"`

	// User for authentication
	User string `yaml:"user"`

	// Password for authentication (supports ${ENV_VAR} expansion)
	Password string `yaml:"password"`

	// Connection pooling
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// MemoryDBConfig configures the memory database (can be separate from primary).
type MemoryDBConfig struct {
	// Backend type (default: same as primary)
	Backend BackendType `yaml:"backend"`

	// Path for SQLite memory database
	Path string `yaml:"path"`

	// UsePrimary indicates to use the primary database for memory storage
	UsePrimary bool `yaml:"use_primary"`
}

// VectorConfig configures vector search capabilities.
type VectorConfig struct {
	// Enabled activates vector search support
	Enabled bool `yaml:"enabled"`

	// Dimensions of the embedding vectors (default: 1536 for OpenAI)
	Dimensions int `yaml:"dimensions"`

	// Provider: "native" (pgvector), "memory" (in-memory for SQLite), "external"
	Provider string `yaml:"provider"`

	// Index type for pgvector: "hnsw" or "ivfflat"
	IndexType string `yaml:"index_type"`

	// Lists parameter for IVFFlat index (default: 100)
	IVFLists int `yaml:"ivf_lists"`

	// M parameter for HNSW index (default: 16)
	HNSWM int `yaml:"hnsw_m"`
}

// DefaultHubConfig returns the default hub configuration (SQLite).
func DefaultHubConfig() HubConfig {
	return HubConfig{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        "./data/devclaw.db",
			JournalMode: "WAL",
			BusyTimeout: 5000,
			ForeignKeys: true,
		},
		Memory: MemoryDBConfig{
			Backend: BackendSQLite,
			Path:    "./data/memory.db",
		},
	}
}

// DefaultSQLiteConfig returns default SQLite configuration.
func DefaultSQLiteConfig() SQLiteConfig {
	return SQLiteConfig{
		Path:        "./data/devclaw.db",
		JournalMode: "WAL",
		BusyTimeout: 5000,
		ForeignKeys: true,
	}
}

// DefaultPostgreSQLConfig returns default PostgreSQL configuration.
func DefaultPostgreSQLConfig() PostgreSQLConfig {
	return PostgreSQLConfig{
		Host:            "localhost",
		Port:            5432,
		SSLMode:         "require",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		Vector: VectorConfig{
			Enabled:    true,
			Dimensions: 1536,
			Provider:   "native",
			IndexType:  "hnsw",
		},
	}
}

// DefaultVectorConfig returns default vector configuration.
func DefaultVectorConfig() VectorConfig {
	return VectorConfig{
		Enabled:    false,
		Dimensions: 1536,
		Provider:   "memory",
		IndexType:  "hnsw",
	}
}

// ToConfig converts SQLiteConfig to generic Config.
func (s SQLiteConfig) ToConfig() Config {
	return Config{
		Type:        BackendSQLite,
		Path:        s.Path,
		JournalMode: s.JournalMode,
		BusyTimeout: s.BusyTimeout,
	}
}

// ToConfig converts PostgreSQLConfig to generic Config.
func (p PostgreSQLConfig) ToConfig() Config {
	return Config{
		Type:            BackendPostgreSQL,
		Host:            p.Host,
		Port:            p.Port,
		Database:        p.Database,
		User:            p.User,
		Password:        p.Password,
		SSLMode:         p.SSLMode,
		MaxOpenConns:    p.MaxOpenConns,
		MaxIdleConns:    p.MaxIdleConns,
		ConnMaxLifetime: p.ConnMaxLifetime,
		SupabaseURL:     p.SupabaseURL,
		SupabaseAnonKey: p.SupabaseAnonKey,
		Vector:          p.Vector,
	}
}

// ToConfig converts MySQLConfig to generic Config.
func (m MySQLConfig) ToConfig() Config {
	return Config{
		Type:            BackendMySQL,
		Host:            m.Host,
		Port:            m.Port,
		Database:        m.Database,
		User:            m.User,
		Password:        m.Password,
		MaxOpenConns:    m.MaxOpenConns,
		MaxIdleConns:    m.MaxIdleConns,
		ConnMaxLifetime: m.ConnMaxLifetime,
	}
}

// Effective returns a copy with default values filled in for zero fields.
func (c HubConfig) Effective() HubConfig {
	out := c

	if out.Backend == "" {
		out.Backend = BackendSQLite
	}

	if out.SQLite.Path == "" {
		out.SQLite.Path = "./data/devclaw.db"
	}
	if out.SQLite.JournalMode == "" {
		out.SQLite.JournalMode = "WAL"
	}
	if out.SQLite.BusyTimeout == 0 {
		out.SQLite.BusyTimeout = 5000
	}

	if out.Memory.Path == "" {
		out.Memory.Path = "./data/memory.db"
	}

	return out
}
