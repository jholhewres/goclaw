// prompt_layers.go implementa o sistema de prompts em camadas (OpenClaw-style).
// Cada camada tem uma prioridade e budget de tokens, permitindo construção
// dinâmica do prompt final com otimização automática.
package copilot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot/memory"
)

// PromptLayer define a prioridade de uma camada de prompt.
// Valores menores = maior prioridade (nunca removidos primeiro em caso de corte).
type PromptLayer int

const (
	// LayerCore é a camada base do assistente (personalidade, regras fundamentais).
	LayerCore PromptLayer = 0

	// LayerIdentity define a identidade/personalidade customizada.
	LayerIdentity PromptLayer = 10

	// LayerBusiness contém contexto do usuário/empresa.
	LayerBusiness PromptLayer = 20

	// LayerObjective define o objetivo da conversa atual.
	LayerObjective PromptLayer = 30

	// LayerSkills injeta instruções e tools das skills ativas.
	LayerSkills PromptLayer = 40

	// LayerMemory injeta fatos relevantes da memória de longo prazo.
	LayerMemory PromptLayer = 50

	// LayerTemporal adiciona data/hora e contexto temporal.
	LayerTemporal PromptLayer = 60

	// LayerConversation contém o histórico recente da conversa.
	LayerConversation PromptLayer = 70

	// LayerBootstrap contains AGENTS.md, SOUL.md, etc. bootstrap files.
	LayerBootstrap PromptLayer = 15
)

// layerEntry representa uma entrada no compositor de prompts.
type layerEntry struct {
	layer   PromptLayer
	content string
}

// PromptComposer monta o prompt final combinando múltiplas camadas
// com otimização de tokens e priorização.
type PromptComposer struct {
	config      *Config
	memoryStore *memory.FileStore
	skillGetter func(name string) (interface{ SystemPrompt() string }, bool)
}

// NewPromptComposer cria um novo compositor de prompts.
func NewPromptComposer(config *Config) *PromptComposer {
	return &PromptComposer{config: config}
}

// SetMemoryStore configures the memory store for the prompt composer.
func (p *PromptComposer) SetMemoryStore(store *memory.FileStore) {
	p.memoryStore = store
}

// SetSkillGetter sets the function used to retrieve skill system prompts.
func (p *PromptComposer) SetSkillGetter(getter func(name string) (interface{ SystemPrompt() string }, bool)) {
	p.skillGetter = getter
}

// Compose monta o prompt final para uma sessão e input específicos.
// Aplica todas as camadas em ordem de prioridade e otimiza tokens.
func (p *PromptComposer) Compose(session *Session, input string) string {
	layers := make([]layerEntry, 0, 8)

	// Layer 0: Core - comportamento base.
	layers = append(layers, layerEntry{
		layer:   LayerCore,
		content: p.buildCoreLayer(),
	})

	// Layer 10: Identity - personalidade customizada.
	if p.config.Instructions != "" {
		layers = append(layers, layerEntry{
			layer:   LayerIdentity,
			content: p.config.Instructions,
		})
	}

	// Layer 15: Bootstrap - AGENTS.md, SOUL.md, etc.
	if bootstrapPrompt := p.buildBootstrapLayer(); bootstrapPrompt != "" {
		layers = append(layers, layerEntry{
			layer:   LayerBootstrap,
			content: bootstrapPrompt,
		})
	}

	// Layer 20: Business - contexto do usuário (via getter thread-safe).
	cfg := session.GetConfig()
	if cfg.BusinessContext != "" {
		layers = append(layers, layerEntry{
			layer:   LayerBusiness,
			content: cfg.BusinessContext,
		})
	}

	// Layer 40: Skills - instruções das skills ativas.
	if skillPrompt := p.buildSkillsLayer(session); skillPrompt != "" {
		layers = append(layers, layerEntry{
			layer:   LayerSkills,
			content: skillPrompt,
		})
	}

	// Layer 50: Memory - fatos relevantes.
	if memoryPrompt := p.buildMemoryLayer(session, input); memoryPrompt != "" {
		layers = append(layers, layerEntry{
			layer:   LayerMemory,
			content: memoryPrompt,
		})
	}

	// Layer 60: Temporal - data/hora atual.
	layers = append(layers, layerEntry{
		layer:   LayerTemporal,
		content: p.buildTemporalLayer(),
	})

	// Layer 70: Conversation - histórico recente.
	if historyPrompt := p.buildConversationLayer(session); historyPrompt != "" {
		layers = append(layers, layerEntry{
			layer:   LayerConversation,
			content: historyPrompt,
		})
	}

	return p.assembleLayers(layers)
}

// buildCoreLayer monta a camada base do assistente.
func (p *PromptComposer) buildCoreLayer() string {
	return fmt.Sprintf(
		"You are %s, a helpful personal assistant.\n"+
			"Be concise, practical, and proactive.\n"+
			"Always verify information before acting.\n"+
			"Respond in %s when appropriate.",
		p.config.Name,
		p.config.Language,
	)
}

// buildSkillsLayer monta as instruções das skills ativas da sessão.
// Injeta o SystemPrompt() de cada skill ativa para dar contexto ao LLM.
func (p *PromptComposer) buildSkillsLayer(session *Session) string {
	activeSkills := session.GetActiveSkills()
	if len(activeSkills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Skills\n\n")

	for _, skillName := range activeSkills {
		b.WriteString(fmt.Sprintf("### %s\n", skillName))

		// Inject the skill's SystemPrompt if we can fetch it.
		if p.skillGetter != nil {
			if skill, ok := p.skillGetter(skillName); ok {
				sp := skill.SystemPrompt()
				if sp != "" {
					b.WriteString(sp)
					b.WriteString("\n")
				}
			}
		}

		b.WriteString("\n")
	}

	return b.String()
}

// buildMemoryLayer monta os fatos relevantes da memória de longo prazo.
// Pulls from persistent memory store if available, falls back to session facts.
func (p *PromptComposer) buildMemoryLayer(session *Session, input string) string {
	var parts []string

	// Pull from persistent memory store if available.
	if p.memoryStore != nil {
		facts := p.memoryStore.RecentFacts(15, input)
		if facts != "" {
			parts = append(parts, "## Long-Term Memory\n\n"+facts)
		}
	}

	// Also include session-level facts (backward compat).
	sessionFacts := session.GetFacts()
	if len(sessionFacts) > 0 {
		var b strings.Builder
		b.WriteString("## Session Facts\n\n")
		for _, fact := range sessionFacts {
			b.WriteString(fmt.Sprintf("- %s\n", fact))
		}
		parts = append(parts, b.String())
	}

	return strings.Join(parts, "\n")
}

// buildTemporalLayer adiciona contexto temporal ao prompt.
func (p *PromptComposer) buildTemporalLayer() string {
	loc, err := time.LoadLocation(p.config.Timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	return fmt.Sprintf(
		"Current date/time: %s (%s)\nTimezone: %s",
		now.Format("2006-01-02 15:04:05 (Mon)"),
		p.config.Timezone,
		loc.String(),
	)
}

// buildConversationLayer monta o histórico recente da conversa.
func (p *PromptComposer) buildConversationLayer(session *Session) string {
	history := session.RecentHistory(p.config.Memory.MaxMessages)
	if len(history) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Recent Conversation\n\n")

	for _, entry := range history {
		b.WriteString(fmt.Sprintf("User: %s\nAssistant: %s\n\n", entry.UserMessage, entry.AssistantResponse))
	}

	return b.String()
}

// buildBootstrapLayer loads optional bootstrap files (AGENTS.md, SOUL.md, IDENTITY.md, USER.md)
// from the workspace root if they exist. These provide persona, rules, and user context.
// Also loads HEARTBEAT.md and TOOLS.md for additional context.
func (p *PromptComposer) buildBootstrapLayer() string {
	bootstrapFiles := []struct {
		Path    string
		Section string
	}{
		{"SOUL.md", "Persona & Boundaries"},
		{"AGENTS.md", "Operational Instructions"},
		{"IDENTITY.md", "Identity"},
		{"USER.md", "User Profile"},
		{"TOOLS.md", "Local Tool Notes"},
	}

	// Determine workspace directory from config or use sensible defaults.
	searchDirs := []string{"."}
	if p.config.Heartbeat.WorkspaceDir != "" && p.config.Heartbeat.WorkspaceDir != "." {
		searchDirs = append([]string{p.config.Heartbeat.WorkspaceDir}, searchDirs...)
	}
	searchDirs = append(searchDirs, "configs")

	var parts []string
	for _, bf := range bootstrapFiles {
		var content []byte
		var err error

		for _, dir := range searchDirs {
			content, err = os.ReadFile(filepath.Join(dir, bf.Path))
			if err == nil {
				break
			}
		}
		if err != nil {
			continue
		}

		text := strings.TrimSpace(string(content))
		if text != "" {
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", bf.Section, text))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}

// assembleLayers combina todas as camadas em ordem de prioridade.
func (p *PromptComposer) assembleLayers(layers []layerEntry) string {
	// Ordena por prioridade (menor = mais importante).
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].layer < layers[j].layer
	})

	var parts []string
	for _, l := range layers {
		if l.content != "" {
			parts = append(parts, l.content)
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}
