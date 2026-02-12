// config.go define as estruturas de configuração do AgentGo Copilot.
package copilot

// Config contém todas as configurações do assistente.
type Config struct {
	// Name é o nome do assistente exibido nas respostas.
	Name string `mapstructure:"name" yaml:"name"`

	// Trigger é a palavra-chave que ativa o copilot (ex: "@copilot").
	Trigger string `mapstructure:"trigger" yaml:"trigger"`

	// Model é o modelo LLM a ser utilizado (ex: "gpt-4o-mini", "claude-3-5-sonnet").
	Model string `mapstructure:"model" yaml:"model"`

	// Instructions são as instruções base do system prompt.
	Instructions string `mapstructure:"instructions" yaml:"instructions"`

	// Timezone é o fuso horário do usuário (ex: "America/Sao_Paulo").
	Timezone string `mapstructure:"timezone" yaml:"timezone"`

	// Language é o idioma preferido para respostas (ex: "pt-BR").
	Language string `mapstructure:"language" yaml:"language"`

	// Memory configura o sistema de memória.
	Memory MemoryConfig `mapstructure:"memory" yaml:"memory"`

	// Security configura os guardrails de segurança.
	Security SecurityConfig `mapstructure:"security" yaml:"security"`

	// TokenBudget configura os limites de tokens por camada de prompt.
	TokenBudget TokenBudgetConfig `mapstructure:"token_budget" yaml:"token_budget"`
}

// MemoryConfig configura o sistema de memória e persistência.
type MemoryConfig struct {
	// Type é o tipo de storage ("sqlite", "postgres", "memory").
	Type string `mapstructure:"type" yaml:"type"`

	// Path é o caminho do arquivo de banco (para sqlite).
	Path string `mapstructure:"path" yaml:"path"`

	// MaxMessages é o número máximo de mensagens mantidas por sessão.
	MaxMessages int `mapstructure:"max_messages" yaml:"max_messages"`

	// CompressionStrategy define a estratégia de compressão de memória
	// ("summarize", "truncate", "semantic").
	CompressionStrategy string `mapstructure:"compression_strategy" yaml:"compression_strategy"`
}

// SecurityConfig configura os guardrails de segurança.
type SecurityConfig struct {
	// MaxInputLength é o tamanho máximo de input aceito em caracteres.
	MaxInputLength int `mapstructure:"max_input_length" yaml:"max_input_length"`

	// RateLimit é o número máximo de mensagens por minuto por usuário.
	RateLimit int `mapstructure:"rate_limit" yaml:"rate_limit"`

	// EnablePIIDetection habilita detecção de dados pessoais.
	EnablePIIDetection bool `mapstructure:"enable_pii_detection" yaml:"enable_pii_detection"`

	// EnableURLValidation habilita validação de URLs nas respostas.
	EnableURLValidation bool `mapstructure:"enable_url_validation" yaml:"enable_url_validation"`
}

// TokenBudgetConfig configura a alocação de tokens por camada de prompt.
type TokenBudgetConfig struct {
	// Total é o tamanho total da janela de contexto do modelo.
	Total int `mapstructure:"total" yaml:"total"`

	// Reserved é o número de tokens reservados para a resposta.
	Reserved int `mapstructure:"reserved" yaml:"reserved"`

	// System é o budget para o system prompt base.
	System int `mapstructure:"system" yaml:"system"`

	// Skills é o budget para instruções de skills.
	Skills int `mapstructure:"skills" yaml:"skills"`

	// Memory é o budget para memórias relevantes.
	Memory int `mapstructure:"memory" yaml:"memory"`

	// History é o budget para histórico de conversa.
	History int `mapstructure:"history" yaml:"history"`

	// Tools é o budget para definições de ferramentas.
	Tools int `mapstructure:"tools" yaml:"tools"`
}

// DefaultConfig retorna a configuração padrão do assistente.
func DefaultConfig() *Config {
	return &Config{
		Name:         "Copilot",
		Trigger:      "@copilot",
		Model:        "gpt-4o-mini",
		Instructions: "You are a helpful personal assistant. Be concise and practical.",
		Timezone:     "America/Sao_Paulo",
		Language:     "pt-BR",
		Memory: MemoryConfig{
			Type:                "sqlite",
			Path:                "./data/memory.db",
			MaxMessages:         100,
			CompressionStrategy: "summarize",
		},
		Security: SecurityConfig{
			MaxInputLength:      4096,
			RateLimit:           30,
			EnablePIIDetection:  false,
			EnableURLValidation: true,
		},
		TokenBudget: TokenBudgetConfig{
			Total:    128000,
			Reserved: 4096,
			System:   500,
			Skills:   2000,
			Memory:   1000,
			History:  8000,
			Tools:    4000,
		},
	}
}
