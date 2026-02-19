// Package skills – builtin_adapter.go provides a SkillLoader that
// creates lightweight built-in skills. Each built-in exposes one or more
// tools callable by the LLM agent.
//
// Built-in skills do not require external scripts or the sandbox.
// They run Go code directly inside the process.
package skills

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// BuiltinLoader creates and returns built-in skills based on the enabled list.
type BuiltinLoader struct {
	enabled  []string
	logger   *slog.Logger
	projProv ProjectProvider // optional, for coding skills (claude-code, project-manager)
	apiKey   string         // LLM API key (injected as ANTHROPIC_API_KEY for Claude Code)
	baseURL  string         // LLM API base URL (injected as ANTHROPIC_BASE_URL for Claude Code)
	model    string         // LLM model name (injected as ANTHROPIC_DEFAULT_*_MODEL)
}

// NewBuiltinLoader creates a loader for built-in skills.
func NewBuiltinLoader(enabled []string, logger *slog.Logger) *BuiltinLoader {
	if logger == nil {
		logger = slog.Default()
	}
	return &BuiltinLoader{enabled: enabled, logger: logger}
}

// SetProjectProvider injects the project provider for coding skills.
// Must be called before Load if "claude-code" or "project-manager" are enabled.
func (l *BuiltinLoader) SetProjectProvider(p ProjectProvider) {
	l.projProv = p
}

// SetAPIConfig injects the LLM API configuration for skills that need it (e.g. claude-code).
// The API key is injected as ANTHROPIC_API_KEY and base URL as ANTHROPIC_BASE_URL
// so that Claude Code CLI uses the same provider (e.g. Z.AI) as DevClaw.
// model is the default LLM model name (e.g. "glm-5") used to set ANTHROPIC_DEFAULT_*_MODEL.
func (l *BuiltinLoader) SetAPIConfig(apiKey, baseURL, model string) {
	l.apiKey = apiKey
	l.baseURL = baseURL
	l.model = model
}

// Load returns built-in skills matching the enabled list.
func (l *BuiltinLoader) Load(_ context.Context) ([]Skill, error) {
	all := map[string]func() Skill{
		"calculator":      newCalculatorSkill,
		"web-fetch":       newWebFetchSkill,
		"datetime":        newDatetimeSkill,
		"image-gen":       newImageGenSkill,
		"claude-code":     func() Skill { return NewClaudeCodeSkill(l.projProv, l.apiKey, l.baseURL, l.model) },
		"project-manager": func() Skill { return NewProjectManagerSkill(l.projProv) },
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
		Author:      "devclaw",
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
		Author:      "devclaw",
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
	req.Header.Set("User-Agent", "DevClaw/1.0")
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
		Author:      "devclaw",
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

// ============================================================
// Image Generation Skill (DALL-E 3 / gpt-image-1)
// ============================================================

type imageGenSkill struct {
	client *http.Client
	apiKey string
	model  string
}

func newImageGenSkill() Skill {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("DEVCLAW_API_KEY")
	}
	model := os.Getenv("DEVCLAW_IMAGE_MODEL")
	if model == "" {
		model = "dall-e-3"
	}
	return &imageGenSkill{
		client: &http.Client{Timeout: 120 * time.Second},
		apiKey: apiKey,
		model:  model,
	}
}

func (s *imageGenSkill) Metadata() Metadata {
	return Metadata{
		Name:        "image-gen",
		Version:     "1.0.0",
		Author:      "devclaw",
		Description: "Generate images from text descriptions using DALL-E or GPT-image models",
		Category:    "creative",
		Tags:        []string{"image", "generate", "dalle", "art", "picture"},
	}
}

func (s *imageGenSkill) Tools() []Tool {
	return []Tool{
		{
			Name:        "generate_image",
			Description: "Generate an image from a text prompt using DALL-E 3 or gpt-image-1. Returns a base64-encoded PNG. Supported sizes: 1024x1024, 1024x1792, 1792x1024. Supported qualities: standard, hd.",
			Parameters: []ToolParameter{
				{Name: "prompt", Type: "string", Description: "A detailed description of the image to generate", Required: true},
				{Name: "size", Type: "string", Description: "Image size: 1024x1024 (default), 1024x1792, 1792x1024", Default: "1024x1024"},
				{Name: "quality", Type: "string", Description: "Image quality: standard (default) or hd", Default: "standard"},
				{Name: "style", Type: "string", Description: "Image style: vivid (default) or natural", Default: "vivid"},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				prompt, _ := args["prompt"].(string)
				if prompt == "" {
					return nil, fmt.Errorf("prompt is required")
				}
				size, _ := args["size"].(string)
				if size == "" {
					size = "1024x1024"
				}
				quality, _ := args["quality"].(string)
				if quality == "" {
					quality = "standard"
				}
				style, _ := args["style"].(string)
				if style == "" {
					style = "vivid"
				}
				return s.generateImage(ctx, prompt, size, quality, style)
			},
		},
	}
}

func (s *imageGenSkill) SystemPrompt() string {
	return `You can generate images using the generate_image tool. When the user asks for an image, picture, illustration, art, drawing, or photo:
1. Create a detailed, descriptive prompt in English (even if the user speaks another language)
2. Include style details (photorealistic, illustration, painting, etc.)
3. Include lighting, mood, composition details for better results
4. Use "hd" quality for important or detailed images
5. Choose the appropriate size based on the image content`
}

func (s *imageGenSkill) Triggers() []string {
	return []string{"image", "picture", "generate", "draw", "create image", "dalle", "art", "illustration", "photo"}
}

func (s *imageGenSkill) Init(_ context.Context, cfg map[string]any) error {
	if key, ok := cfg["api_key"].(string); ok && key != "" {
		s.apiKey = key
	}
	if model, ok := cfg["model"].(string); ok && model != "" {
		s.model = model
	}
	return nil
}

func (s *imageGenSkill) Execute(ctx context.Context, input string) (string, error) {
	result, err := s.generateImage(ctx, input, "1024x1024", "standard", "vivid")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", result), nil
}

func (s *imageGenSkill) Shutdown() error { return nil }

// imageGenResponse represents the OpenAI images API response.
type imageGenResponse struct {
	Data []struct {
		B64JSON       string `json:"b64_json"`
		URL           string `json:"url"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (s *imageGenSkill) generateImage(ctx context.Context, prompt, size, quality, style string) (any, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("image generation requires OPENAI_API_KEY or DEVCLAW_API_KEY to be set")
	}

	// Validate size.
	validSizes := map[string]bool{
		"1024x1024": true,
		"1024x1792": true,
		"1792x1024": true,
	}
	if !validSizes[size] {
		size = "1024x1024"
	}

	// Validate quality.
	if quality != "standard" && quality != "hd" {
		quality = "standard"
	}

	// Build request body.
	reqBody := map[string]any{
		"model":           s.model,
		"prompt":          prompt,
		"n":               1,
		"size":            size,
		"quality":         quality,
		"response_format": "b64_json",
	}
	// Style is only supported for dall-e-3.
	if strings.HasPrefix(s.model, "dall-e") {
		if style != "vivid" && style != "natural" {
			style = "vivid"
		}
		reqBody["style"] = style
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var result imageGenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no image generated")
	}

	img := result.Data[0]

	// Save the image to a temp file for media channels.
	// Use os.CreateTemp for a random name (avoids predictable path injection)
	// and write with owner-only permissions (0o600) so other local users
	// cannot read potentially sensitive generated images.
	imgData, err := base64.StdEncoding.DecodeString(img.B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decoding image data: %w", err)
	}
	tmpImgFile, err := os.CreateTemp("", "devclaw-img-*.png")
	if err != nil {
		return nil, fmt.Errorf("creating image temp file: %w", err)
	}
	imgPath := tmpImgFile.Name()
	// Restrict before writing data — prevents a race where another process
	// could read a world-readable file between creation and chmod.
	if err := os.Chmod(imgPath, 0o600); err != nil {
		tmpImgFile.Close()
		os.Remove(imgPath)
		return nil, fmt.Errorf("setting image temp file permissions: %w", err)
	}
	if _, err := tmpImgFile.Write(imgData); err != nil {
		tmpImgFile.Close()
		os.Remove(imgPath)
		return nil, fmt.Errorf("saving image: %w", err)
	}
	if err := tmpImgFile.Close(); err != nil {
		os.Remove(imgPath)
		return nil, fmt.Errorf("closing image temp file: %w", err)
	}

	response := map[string]any{
		"image_path":     imgPath,
		"revised_prompt": img.RevisedPrompt,
		"size":           size,
		"quality":        quality,
		"model":          s.model,
		"message":        fmt.Sprintf("Image generated and saved to %s", imgPath),
	}

	return response, nil
}

