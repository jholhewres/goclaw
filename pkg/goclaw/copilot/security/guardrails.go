// Package security implementa os guardrails de segurança do AgentGo Copilot.
// Inclui validação de input (injection, rate limit, PII), validação de output
// (URLs, fatos, PII) e políticas de segurança para execução de tools.
package security

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// --- Input Guardrails ---

// InputGuardrail valida mensagens de entrada antes do processamento pelo LLM.
type InputGuardrail struct {
	// maxLength é o tamanho máximo de input aceito.
	maxLength int

	// rateLimit é o número máximo de mensagens por minuto por usuário.
	rateLimit int

	// rateLimiter controla a frequência de mensagens por usuário.
	rateLimiter *RateLimiter
}

// NewInputGuardrail cria um novo guardrail de input.
func NewInputGuardrail(maxLength, rateLimit int) *InputGuardrail {
	if maxLength <= 0 {
		maxLength = 4096
	}
	if rateLimit <= 0 {
		rateLimit = 30
	}

	return &InputGuardrail{
		maxLength:   maxLength,
		rateLimit:   rateLimit,
		rateLimiter: NewRateLimiter(rateLimit, time.Minute),
	}
}

// Validate executa todas as validações no input.
func (g *InputGuardrail) Validate(userID, input string) error {
	// 1. Verifica tamanho máximo.
	if len(input) > g.maxLength {
		return ErrInputTooLong
	}

	// 2. Verifica rate limit.
	if !g.rateLimiter.Allow(userID) {
		return ErrRateLimited
	}

	// 3. Verifica padrões de prompt injection.
	if detectPromptInjection(input) {
		return ErrPromptInjection
	}

	return nil
}

// detectPromptInjection verifica padrões comuns de prompt injection.
func detectPromptInjection(input string) bool {
	lower := strings.ToLower(input)

	// Padrões conhecidos de prompt injection.
	patterns := []string{
		"ignore previous instructions",
		"ignore all previous",
		"disregard your instructions",
		"you are now",
		"new instructions:",
		"system prompt:",
		"forget your rules",
		"override your programming",
	}

	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// --- Output Guardrails ---

// OutputGuardrail valida respostas geradas pelo LLM antes do envio.
type OutputGuardrail struct{}

// NewOutputGuardrail cria um novo guardrail de output.
func NewOutputGuardrail() *OutputGuardrail {
	return &OutputGuardrail{}
}

// Validate executa todas as validações no output do LLM.
func (g *OutputGuardrail) Validate(output string) error {
	// 1. Verifica se o output não está vazio.
	if strings.TrimSpace(output) == "" {
		return ErrEmptyOutput
	}

	// 2. Verifica padrões de leak de system prompt.
	if detectSystemPromptLeak(output) {
		return ErrSystemPromptLeak
	}

	// TODO: Implementar validação de URLs contra resultados de tools.
	// TODO: Implementar detecção de PII no output.

	return nil
}

// detectSystemPromptLeak verifica se o output contém trechos do system prompt.
func detectSystemPromptLeak(output string) bool {
	lower := strings.ToLower(output)

	// Indicadores de que o modelo está vazando instruções internas.
	indicators := []string{
		"my instructions are",
		"my system prompt is",
		"i was instructed to",
		"my programming says",
	}

	for _, indicator := range indicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}

	return false
}

// --- Rate Limiter ---

// RateLimiter implementa rate limiting por usuário usando sliding window.
type RateLimiter struct {
	// maxRequests é o número máximo de requisições no intervalo.
	maxRequests int

	// window é o intervalo de tempo da janela deslizante.
	window time.Duration

	// requests armazena os timestamps das requisições por usuário.
	requests map[string][]time.Time

	mu sync.Mutex
}

// NewRateLimiter cria um novo rate limiter.
func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		maxRequests: maxRequests,
		window:      window,
		requests:    make(map[string][]time.Time),
	}
}

// Allow verifica se o usuário pode fazer uma nova requisição.
// Retorna true se permitido, false se excedeu o limite.
func (rl *RateLimiter) Allow(userID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove requisições fora da janela.
	timestamps := rl.requests[userID]
	valid := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Remove usuários sem requisições válidas para evitar leak de memória.
	if len(valid) == 0 {
		delete(rl.requests, userID)
		// Registra nova requisição.
		rl.requests[userID] = []time.Time{now}
		return true
	}

	// Verifica se excedeu o limite.
	if len(valid) >= rl.maxRequests {
		rl.requests[userID] = valid
		return false
	}

	// Registra nova requisição.
	rl.requests[userID] = append(valid, now)
	return true
}

// --- Tool Security ---

// ToolSecurityPolicy define políticas de segurança para execução de tools.
type ToolSecurityPolicy struct {
	// AllowedTools lista as tools permitidas por skill (chave = skill name, valor = tool names).
	AllowedTools map[string][]string

	// RequiresConfirmation lista tools que precisam de confirmação do usuário.
	RequiresConfirmation []string

	// ToolRateLimits define rate limits específicos por tool.
	ToolRateLimits map[string]int
}

// BeforeToolCall valida se uma tool pode ser executada para uma skill específica.
func (p *ToolSecurityPolicy) BeforeToolCall(skillName, tool string) error {
	// 1. Verifica whitelist se configurada.
	if len(p.AllowedTools) > 0 {
		allowed, ok := p.AllowedTools[skillName]
		if !ok {
			return fmt.Errorf("%w: skill %q não possui tools permitidas", ErrToolNotAllowed, skillName)
		}
		if !containsString(allowed, tool) {
			return fmt.Errorf("%w: tool %q não permitida para skill %q", ErrToolNotAllowed, tool, skillName)
		}
	}

	// 2. Verifica se a tool requer confirmação.
	if containsString(p.RequiresConfirmation, tool) {
		return ErrConfirmationRequired
	}

	return nil
}

// containsString verifica se um slice contém uma string.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// --- Errors ---

var (
	ErrInputTooLong         = fmt.Errorf("mensagem excede o tamanho máximo permitido")
	ErrRateLimited          = fmt.Errorf("limite de mensagens por minuto excedido, aguarde um momento")
	ErrPromptInjection      = fmt.Errorf("conteúdo potencialmente malicioso detectado")
	ErrEmptyOutput          = fmt.Errorf("resposta vazia gerada pelo modelo")
	ErrSystemPromptLeak     = fmt.Errorf("possível vazamento de instruções internas")
	ErrHallucinatedURL      = fmt.Errorf("URL na resposta não corresponde aos resultados")
	ErrConfirmationRequired = fmt.Errorf("esta ação requer confirmação do usuário")
	ErrToolNotAllowed       = fmt.Errorf("tool não permitida pela política de segurança")
)
