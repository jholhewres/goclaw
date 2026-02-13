// Package skills – builtin_adapter.go provides a SkillLoader that
// creates lightweight built-in skills. Each built-in exposes one or more
// tools callable by the LLM agent.
//
// Built-in skills do not require external scripts or the sandbox.
// They run Go code directly inside the process.
package skills

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// BuiltinLoader creates and returns built-in skills based on the enabled list.
type BuiltinLoader struct {
	enabled []string
	logger  *slog.Logger
}

// NewBuiltinLoader creates a loader for built-in skills.
func NewBuiltinLoader(enabled []string, logger *slog.Logger) *BuiltinLoader {
	if logger == nil {
		logger = slog.Default()
	}
	return &BuiltinLoader{enabled: enabled, logger: logger}
}

// Load returns built-in skills matching the enabled list.
func (l *BuiltinLoader) Load(_ context.Context) ([]Skill, error) {
	all := map[string]func() Skill{
		"calculator": newCalculatorSkill,
		"web-fetch":  newWebFetchSkill,
		"datetime":   newDatetimeSkill,
	}

	var result []Skill
	for _, name := range l.enabled {
		factory, ok := all[name]
		if !ok {
			l.logger.Debug("builtin: unknown skill, skipping", "name", name)
			continue
		}
		result = append(result, factory())
		l.logger.Debug("builtin: loaded skill", "name", name)
	}

	l.logger.Info("builtin: loaded skills", "count", len(result))
	return result, nil
}

// Source returns the loader source identifier.
func (l *BuiltinLoader) Source() string {
	return "builtin"
}

// ============================================================
// Calculator Skill
// ============================================================

type calculatorSkill struct{}

func newCalculatorSkill() Skill { return &calculatorSkill{} }

func (s *calculatorSkill) Metadata() Metadata {
	return Metadata{
		Name:        "calculator",
		Version:     "1.0.0",
		Author:      "goclaw",
		Description: "Evaluates basic math expressions",
		Category:    "utility",
		Tags:        []string{"math", "calculate"},
	}
}

func (s *calculatorSkill) Tools() []Tool {
	return []Tool{
		{
			Name:        "calculate",
			Description: "Evaluate a mathematical expression. Supports +, -, *, /, ^, sqrt, abs. Example: '(2+3)*4'",
			Parameters: []ToolParameter{
				{Name: "expression", Type: "string", Description: "Math expression to evaluate", Required: true},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				expr, _ := args["expression"].(string)
				if expr == "" {
					return nil, fmt.Errorf("expression is required")
				}
				result, err := evalSimpleMath(expr)
				if err != nil {
					return nil, err
				}
				return fmt.Sprintf("%s = %g", expr, result), nil
			},
		},
	}
}

func (s *calculatorSkill) SystemPrompt() string {
	return "You have a calculator tool. Use it for any mathematical computation."
}

func (s *calculatorSkill) Triggers() []string {
	return []string{"calculate", "math", "calculator"}
}

func (s *calculatorSkill) Init(_ context.Context, _ map[string]any) error { return nil }

func (s *calculatorSkill) Execute(_ context.Context, input string) (string, error) {
	result, err := evalSimpleMath(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%g", result), nil
}

func (s *calculatorSkill) Shutdown() error { return nil }

// evalSimpleMath evaluates basic math expressions (recursive descent parser).
func evalSimpleMath(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Handle simple function calls.
	if strings.HasPrefix(expr, "sqrt(") && strings.HasSuffix(expr, ")") {
		inner := expr[5 : len(expr)-1]
		v, err := evalSimpleMath(inner)
		if err != nil {
			return 0, err
		}
		return math.Sqrt(v), nil
	}
	if strings.HasPrefix(expr, "abs(") && strings.HasSuffix(expr, ")") {
		inner := expr[4 : len(expr)-1]
		v, err := evalSimpleMath(inner)
		if err != nil {
			return 0, err
		}
		return math.Abs(v), nil
	}

	// Try direct parse first.
	if v, err := strconv.ParseFloat(expr, 64); err == nil {
		return v, nil
	}

	// Find the last + or - at the top level (lowest precedence).
	depth := 0
	lastAdd := -1
	for i := len(expr) - 1; i >= 0; i-- {
		switch expr[i] {
		case ')':
			depth++
		case '(':
			depth--
		case '+', '-':
			if depth == 0 && i > 0 {
				lastAdd = i
			}
		}
		if lastAdd >= 0 {
			break
		}
	}

	if lastAdd > 0 {
		left, err := evalSimpleMath(expr[:lastAdd])
		if err != nil {
			return 0, err
		}
		right, err := evalSimpleMath(expr[lastAdd+1:])
		if err != nil {
			return 0, err
		}
		if expr[lastAdd] == '+' {
			return left + right, nil
		}
		return left - right, nil
	}

	// Find last * or / at top level.
	lastMul := -1
	depth = 0
	for i := len(expr) - 1; i >= 0; i-- {
		switch expr[i] {
		case ')':
			depth++
		case '(':
			depth--
		case '*', '/':
			if depth == 0 {
				lastMul = i
			}
		}
		if lastMul >= 0 {
			break
		}
	}

	if lastMul > 0 {
		left, err := evalSimpleMath(expr[:lastMul])
		if err != nil {
			return 0, err
		}
		right, err := evalSimpleMath(expr[lastMul+1:])
		if err != nil {
			return 0, err
		}
		if expr[lastMul] == '*' {
			return left * right, nil
		}
		if right == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return left / right, nil
	}

	// Find ^ at top level.
	lastPow := -1
	depth = 0
	for i := len(expr) - 1; i >= 0; i-- {
		switch expr[i] {
		case ')':
			depth++
		case '(':
			depth--
		case '^':
			if depth == 0 {
				lastPow = i
			}
		}
		if lastPow >= 0 {
			break
		}
	}

	if lastPow > 0 {
		left, err := evalSimpleMath(expr[:lastPow])
		if err != nil {
			return 0, err
		}
		right, err := evalSimpleMath(expr[lastPow+1:])
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}

	// Parenthesized expression.
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evalSimpleMath(expr[1 : len(expr)-1])
	}

	return 0, fmt.Errorf("cannot evaluate: %s", expr)
}

// ============================================================
// Web Fetch Skill
// ============================================================

type webFetchSkill struct {
	client *http.Client
}

func newWebFetchSkill() Skill {
	return &webFetchSkill{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *webFetchSkill) Metadata() Metadata {
	return Metadata{
		Name:        "web-fetch",
		Version:     "1.0.0",
		Author:      "goclaw",
		Description: "Fetches content from a URL and returns the text",
		Category:    "utility",
		Tags:        []string{"web", "fetch", "http"},
	}
}

func (s *webFetchSkill) Tools() []Tool {
	return []Tool{
		{
			Name:        "fetch_url",
			Description: "Fetch content from a URL. Returns the raw text content (first 10000 chars).",
			Parameters: []ToolParameter{
				{Name: "url", Type: "string", Description: "The URL to fetch", Required: true},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				url, _ := args["url"].(string)
				if url == "" {
					return nil, fmt.Errorf("url is required")
				}
				return s.fetch(ctx, url)
			},
		},
	}
}

func (s *webFetchSkill) SystemPrompt() string {
	return "You can fetch content from URLs using the web-fetch tool."
}

func (s *webFetchSkill) Triggers() []string {
	return []string{"fetch", "url", "web"}
}

func (s *webFetchSkill) Init(_ context.Context, _ map[string]any) error { return nil }

func (s *webFetchSkill) Execute(ctx context.Context, input string) (string, error) {
	return s.fetch(ctx, strings.TrimSpace(input))
}

func (s *webFetchSkill) Shutdown() error { return nil }

func (s *webFetchSkill) fetch(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "GoClaw/1.0")
	req.Header.Set("Accept", "text/html,text/plain,application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024)) // 50KB limit
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	content := string(body)
	// Truncate for LLM consumption.
	if len(content) > 10000 {
		content = content[:10000] + "\n... [truncated]"
	}

	return fmt.Sprintf("URL: %s\nStatus: %d\nContent-Type: %s\n\n%s",
		url, resp.StatusCode, resp.Header.Get("Content-Type"), content), nil
}

// ============================================================
// Datetime Skill
// ============================================================

type datetimeSkill struct{}

func newDatetimeSkill() Skill { return &datetimeSkill{} }

func (s *datetimeSkill) Metadata() Metadata {
	return Metadata{
		Name:        "datetime",
		Version:     "1.0.0",
		Author:      "goclaw",
		Description: "Provides current date, time, and timezone conversions",
		Category:    "utility",
		Tags:        []string{"date", "time", "timezone"},
	}
}

func (s *datetimeSkill) Tools() []Tool {
	return []Tool{
		{
			Name:        "current_time",
			Description: "Get the current date and time in a specific timezone. Defaults to UTC.",
			Parameters: []ToolParameter{
				{Name: "timezone", Type: "string", Description: "IANA timezone (e.g. 'America/Sao_Paulo', 'UTC')"},
			},
			Handler: func(_ context.Context, args map[string]any) (any, error) {
				tz, _ := args["timezone"].(string)
				if tz == "" {
					tz = "UTC"
				}
				loc, err := time.LoadLocation(tz)
				if err != nil {
					return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
				}
				now := time.Now().In(loc)
				return map[string]string{
					"datetime":  now.Format("2006-01-02 15:04:05"),
					"timezone":  tz,
					"day":       now.Format("Monday"),
					"unix":      fmt.Sprintf("%d", now.Unix()),
					"iso8601":   now.Format(time.RFC3339),
				}, nil
			},
		},
	}
}

func (s *datetimeSkill) SystemPrompt() string { return "" }

func (s *datetimeSkill) Triggers() []string {
	return []string{"time", "date", "datetime"}
}

func (s *datetimeSkill) Init(_ context.Context, _ map[string]any) error { return nil }

func (s *datetimeSkill) Execute(_ context.Context, input string) (string, error) {
	tz := strings.TrimSpace(input)
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("invalid timezone: %w", err)
	}
	now := time.Now().In(loc)
	return now.Format("2006-01-02 15:04:05 (Mon) MST"), nil
}

func (s *datetimeSkill) Shutdown() error { return nil }

