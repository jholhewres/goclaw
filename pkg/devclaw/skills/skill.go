// Package skills define o sistema de extensibilidade do DevClaw.
// Skills são módulos que adicionam capacidades ao assistente, como integração
// com calendário, email, GitHub, etc. Podem ser built-in ou instaladas pela comunidade.
package skills

import (
	"context"
)

// Skill define a interface que toda skill deve implementar.
// Uma skill encapsula uma capacidade específica do assistente.
type Skill interface {
	// Metadata retorna os metadados da skill (nome, versão, autor, etc).
	Metadata() Metadata

	// Tools retorna as funções/ferramentas que a skill expõe ao agente LLM.
	Tools() []Tool

	// SystemPrompt retorna instruções adicionais a serem injetadas no prompt do sistema
	// quando a skill está ativa. Pode retornar string vazia se não necessário.
	SystemPrompt() string

	// Triggers retorna padrões de linguagem natural que ativam esta skill.
	// Usado para ativação automática baseada no conteúdo da mensagem.
	Triggers() []string

	// Init inicializa a skill com a configuração fornecida.
	// Chamado uma vez durante o startup.
	Init(ctx context.Context, config map[string]any) error

	// Execute executa a skill com o input fornecido e retorna o resultado.
	Execute(ctx context.Context, input string) (string, error)

	// Shutdown libera recursos da skill de forma graciosa.
	Shutdown() error
}

// ConfigRequirement descreve uma configuração/credencial obrigatória para uma skill.
// Skills podem declarar requisitos que serão verificados automaticamente antes da execução.
type ConfigRequirement struct {
	// Key é a chave usada para armazenar no vault (ex: "SLACK_BOT_TOKEN").
	Key string `yaml:"key"`

	// Name é o nome amigável para mostrar ao usuário (ex: "Slack Bot Token").
	Name string `yaml:"name"`

	// Description explica o que é e como obter essa configuração.
	Description string `yaml:"description"`

	// Pattern é um padrão opcional para validação (ex: "xoxb-*" para tokens Slack).
	Pattern string `yaml:"pattern,omitempty"`

	// Example mostra um exemplo de valor válido (ex: "xoxb-1234567890-1234567890-AbCdEf").
	Example string `yaml:"example,omitempty"`

	// Secret indica se o valor deve ser tratado como segredo (ocultar em logs).
	Secret bool `yaml:"secret"`

	// Required indica se é obrigatório. Se false, a skill pode funcionar sem ele.
	Required bool `yaml:"required"`

	// EnvVar é a variável de ambiente alternativa (ex: "SLACK_BOT_TOKEN").
	EnvVar string `yaml:"env_var,omitempty"`
}

// SetupStatus indica o estado de configuração de uma skill.
type SetupStatus struct {
	// IsComplete indica se todas as configurações obrigatórias estão presentes.
	IsComplete bool

	// MissingRequirements são as configurações que faltam.
	MissingRequirements []ConfigRequirement

	// OptionalMissing são configurações opcionais que não foram definidas.
	OptionalMissing []ConfigRequirement

	// Message é uma mensagem amigável sobre o estado do setup.
	Message string
}

// SkillSetupChecker é implementado por skills que precisam de configuração.
// O sistema verifica automaticamente antes de executar a skill.
type SkillSetupChecker interface {
	// RequiredConfig retorna as configurações obrigatórias e opcionais.
	RequiredConfig() []ConfigRequirement

	// CheckSetup verifica se a skill está corretamente configurada.
	// Recebe um VaultReader para verificar credenciais armazenadas.
	CheckSetup(vault VaultReader) SetupStatus
}

// VaultReader é uma interface minimalista para ler valores do vault.
// Skills usam isso para verificar se credenciais existem.
type VaultReader interface {
	// Get retorna o valor armazenado para a chave, ou erro se não existir.
	Get(key string) (string, error)
	// Has retorna true se a chave existe no vault.
	Has(key string) bool
}

// Metadata contém os metadados de uma skill.
type Metadata struct {
	// Name é o identificador único da skill (ex: "calendar", "github").
	Name string `yaml:"name"`

	// Version é a versão semântica da skill (ex: "1.0.0").
	Version string `yaml:"version"`

	// Author é o autor ou organização que criou a skill.
	Author string `yaml:"author"`

	// Description é uma breve descrição do que a skill faz.
	Description string `yaml:"description"`

	// Category é a categoria da skill (ex: "productivity", "development").
	Category string `yaml:"category"`

	// Tags são palavras-chave para busca e indexação.
	Tags []string `yaml:"tags"`
}

// Tool representa uma função/ferramenta exposta por uma skill ao agente LLM.
type Tool struct {
	// Name é o identificador da ferramenta (ex: "list_events").
	Name string `json:"name"`

	// Description descreve o que a ferramenta faz (usado no prompt do LLM).
	Description string `json:"description"`

	// Parameters define os parâmetros aceitos pela ferramenta.
	Parameters []ToolParameter `json:"parameters"`

	// Handler é a função que executa a ferramenta.
	Handler ToolHandler `json:"-"`
}

// ToolParameter define um parâmetro de uma ferramenta.
type ToolParameter struct {
	// Name é o nome do parâmetro.
	Name string `json:"name"`

	// Type é o tipo do parâmetro (string, integer, boolean, etc).
	Type string `json:"type"`

	// Description descreve o parâmetro.
	Description string `json:"description"`

	// Required indica se o parâmetro é obrigatório.
	Required bool `json:"required"`

	// Default é o valor padrão, se houver.
	Default any `json:"default,omitempty"`
}

// ToolHandler é a assinatura da função que processa a chamada de uma ferramenta.
type ToolHandler func(ctx context.Context, args map[string]any) (any, error)

// SkillLoader define a interface para carregar skills de diferentes fontes
// (embedded, filesystem, registry remoto, etc).
type SkillLoader interface {
	// Load carrega e retorna skills a partir da fonte configurada.
	Load(ctx context.Context) ([]Skill, error)

	// Source retorna o identificador da fonte (ex: "builtin", "filesystem", "registry").
	Source() string
}
