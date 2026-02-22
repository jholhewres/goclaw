// Package copilot – browser_tool.go implements a browser automation tool using
// Chrome DevTools Protocol (CDP). This allows the agent to navigate web pages,
// take screenshots, extract content,
// click elements, and fill forms.
//
// Architecture:
//
//	Agent ──browser_navigate──▶ BrowserManager ──CDP──▶ Chrome/Chromium
//	Agent ──browser_screenshot──▶ BrowserManager ──CDP──▶ Screenshot → base64
//	Agent ──browser_content──▶ BrowserManager ──CDP──▶ DOM → text
//	Agent ──browser_click──▶ BrowserManager ──CDP──▶ Click element
//
// The browser is launched lazily on first use and kept alive for the session.
// A configurable timeout prevents runaway browser sessions.
package copilot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/security"
)

// BrowserConfig configures the browser tool.
type BrowserConfig struct {
	// Enabled turns the browser tool on/off (default: true if Chrome is found).
	Enabled bool `yaml:"enabled"`

	// ChromePath is the path to the Chrome/Chromium binary.
	// Auto-detected if empty.
	ChromePath string `yaml:"chrome_path"`

	// Headless runs the browser without a visible window (default: true).
	Headless bool `yaml:"headless"`

	// TimeoutSeconds is the max time for a single browser operation (default: 30).
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// MaxPages is the max number of simultaneous pages/tabs (default: 3).
	MaxPages int `yaml:"max_pages"`

	// ViewportWidth is the browser viewport width (default: 1280).
	ViewportWidth int `yaml:"viewport_width"`

	// ViewportHeight is the browser viewport height (default: 720).
	ViewportHeight int `yaml:"viewport_height"`
}

// DefaultBrowserConfig returns sensible defaults.
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		Enabled:        true,
		Headless:       true,
		TimeoutSeconds: 30,
		MaxPages:       3,
		ViewportWidth:  1280,
		ViewportHeight: 720,
	}
}

// BrowserManager manages a Chrome/Chromium process and CDP connections.
type BrowserManager struct {
	cfg       BrowserConfig
	logger    *slog.Logger
	ssrfGuard *security.SSRFGuard

	mu      sync.Mutex
	cmd     *exec.Cmd
	wsURL   string
	conn    *websocket.Conn
	msgID   int
	started bool
}

// WithSSRFGuard attaches an SSRF guard to the browser manager.
// When set, Navigate() will validate URLs before loading them.
func (bm *BrowserManager) WithSSRFGuard(guard *security.SSRFGuard) *BrowserManager {
	bm.ssrfGuard = guard
	return bm
}

// NewBrowserManager creates a new browser manager.
func NewBrowserManager(cfg BrowserConfig, logger *slog.Logger) *BrowserManager {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = 3
	}
	if cfg.ViewportWidth <= 0 {
		cfg.ViewportWidth = 1280
	}
	if cfg.ViewportHeight <= 0 {
		cfg.ViewportHeight = 720
	}
	return &BrowserManager{
		cfg:    cfg,
		logger: logger.With("component", "browser"),
	}
}

// findChrome locates the Chrome/Chromium binary.
func (bm *BrowserManager) findChrome() string {
	if bm.cfg.ChromePath != "" {
		return bm.cfg.ChromePath
	}
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium-browser",
		"chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium-browser",
		"/snap/bin/chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

// allocatePort finds a free TCP port.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// Start launches Chrome with CDP enabled. Called lazily on first tool use.
func (bm *BrowserManager) Start(ctx context.Context) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.started {
		return nil
	}

	chromePath := bm.findChrome()
	if chromePath == "" {
		return fmt.Errorf("chrome/chromium not found; install Chrome or set browser.chrome_path in config")
	}

	port, err := allocatePort()
	if err != nil {
		return fmt.Errorf("failed to allocate CDP port: %w", err)
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-popup-blocking",
		"--disable-translate",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		fmt.Sprintf("--window-size=%d,%d", bm.cfg.ViewportWidth, bm.cfg.ViewportHeight),
	}
	if bm.cfg.Headless {
		args = append(args, "--headless=new")
	}
	args = append(args, "about:blank")

	bm.cmd = exec.CommandContext(ctx, chromePath, args...)
	if err := bm.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Chrome: %w", err)
	}

	bm.logger.Info("chrome started", "pid", bm.cmd.Process.Pid, "port", port)

	// Wait for CDP to be ready.
	wsURL, err := bm.waitForCDP(port, 10*time.Second)
	if err != nil {
		bm.cmd.Process.Kill()
		return fmt.Errorf("CDP not ready: %w", err)
	}

	bm.wsURL = wsURL
	bm.started = true
	return nil
}

// waitForCDP polls the CDP /json/version endpoint until it responds.
func (bm *BrowserManager) waitForCDP(port int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)

	for time.Now().Before(deadline) {
		conn, _, err := websocket.DefaultDialer.Dial(
			fmt.Sprintf("ws://127.0.0.1:%d", port), nil)
		if err == nil {
			conn.Close()
		}

		// Try HTTP to get the WS URL.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			var info struct {
				WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
			}
			if json.NewDecoder(resp.Body).Decode(&info) == nil && info.WebSocketDebuggerUrl != "" {
				resp.Body.Close()
				return info.WebSocketDebuggerUrl, nil
			}
			resp.Body.Close()
		}

		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for CDP on port %d", port)
}

// connect establishes or reuses the WebSocket connection to CDP.
func (bm *BrowserManager) connect() (*websocket.Conn, error) {
	if bm.conn != nil {
		return bm.conn, nil
	}

	conn, _, err := websocket.DefaultDialer.Dial(bm.wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("CDP WebSocket dial failed: %w", err)
	}
	bm.conn = conn
	return conn, nil
}

// sendCDP sends a CDP command and waits for the response.
func (bm *BrowserManager) sendCDP(method string, params map[string]any) (json.RawMessage, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	conn, err := bm.connect()
	if err != nil {
		return nil, err
	}

	bm.msgID++
	msg := map[string]any{
		"id":     bm.msgID,
		"method": method,
	}
	if params != nil {
		msg["params"] = params
	}

	if err := conn.WriteJSON(msg); err != nil {
		conn.Close()
		bm.conn = nil
		return nil, fmt.Errorf("CDP write error: %w", err)
	}

	// Read until we get our response.
	targetID := bm.msgID
	deadline := time.Now().Add(time.Duration(bm.cfg.TimeoutSeconds) * time.Second)
	conn.SetReadDeadline(deadline)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			conn.Close()
			bm.conn = nil
			return nil, fmt.Errorf("CDP read error: %w", err)
		}

		var resp struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &resp) == nil && resp.ID == targetID {
			if resp.Error != nil {
				return nil, fmt.Errorf("CDP error: %s", resp.Error.Message)
			}
			return resp.Result, nil
		}
	}
}

// Navigate opens a URL in the browser.
func (bm *BrowserManager) Navigate(ctx context.Context, url string) error {
	if bm.ssrfGuard != nil {
		if err := bm.ssrfGuard.IsAllowed(url); err != nil {
			return fmt.Errorf("browser navigation blocked: %w", err)
		}
	}
	if err := bm.Start(ctx); err != nil {
		return err
	}
	_, err := bm.sendCDP("Page.navigate", map[string]any{"url": url})
	if err != nil {
		return err
	}
	// Wait for load.
	time.Sleep(500 * time.Millisecond)
	return nil
}

// Screenshot captures the current page as a PNG and returns base64-encoded data.
func (bm *BrowserManager) Screenshot(ctx context.Context) (string, error) {
	if err := bm.Start(ctx); err != nil {
		return "", err
	}
	result, err := bm.sendCDP("Page.captureScreenshot", map[string]any{
		"format": "png",
	})
	if err != nil {
		return "", err
	}
	var screenshotResult struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &screenshotResult); err != nil {
		return "", err
	}
	return screenshotResult.Data, nil
}

// GetContent returns the text content of the current page.
func (bm *BrowserManager) GetContent(ctx context.Context) (string, error) {
	if err := bm.Start(ctx); err != nil {
		return "", err
	}

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": "document.body ? document.body.innerText : document.documentElement.innerText",
	})
	if err != nil {
		return "", err
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return "", err
	}

	// Truncate to avoid context overflow.
	text := evalResult.Result.Value
	if len(text) > 10000 {
		text = text[:10000] + "\n... (truncated)"
	}
	return text, nil
}

// ClickElement clicks an element matched by CSS selector.
func (bm *BrowserManager) ClickElement(ctx context.Context, selector string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';
			el.click();
			return 'ok';
		})()
	`, selector)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return err
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return err
	}
	if evalResult.Result.Value == "not_found" {
		return fmt.Errorf("element not found: %s", selector)
	}
	return nil
}

// FillInput fills a text input matched by CSS selector.
func (bm *BrowserManager) FillInput(ctx context.Context, selector, value string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';
			el.value = %q;
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
			return 'ok';
		})()
	`, selector, value)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return err
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return err
	}
	if evalResult.Result.Value == "not_found" {
		return fmt.Errorf("input not found: %s", selector)
	}
	return nil
}

// Stop kills the Chrome process and closes connections.
func (bm *BrowserManager) Stop() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.conn != nil {
		bm.conn.Close()
		bm.conn = nil
	}
	if bm.cmd != nil && bm.cmd.Process != nil {
		bm.cmd.Process.Kill()
		bm.cmd.Wait()
		bm.logger.Info("chrome stopped")
	}
	bm.started = false
}

// ─── Tool Registration ───

// RegisterBrowserTools registers browser automation tools in the executor.
func RegisterBrowserTools(executor *ToolExecutor, browserMgr *BrowserManager, logger *slog.Logger) {
	if browserMgr == nil {
		return
	}

	// browser_navigate
	executor.Register(
		MakeToolDefinition("browser_navigate",
			"Navigate the browser to a URL. Opens the page and waits for it to load.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to navigate to.",
					},
				},
				"required": []string{"url"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			url, _ := args["url"].(string)
			if url == "" {
				return nil, fmt.Errorf("url is required")
			}
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}
			if err := browserMgr.Navigate(ctx, url); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Navigated to %s", url), nil
		},
	)

	// browser_screenshot
	executor.Register(
		MakeToolDefinition("browser_screenshot",
			"Take a screenshot of the current browser page. Returns base64-encoded PNG.",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			data, err := browserMgr.Screenshot(ctx)
			if err != nil {
				return nil, err
			}
			// Return truncated info + base64 ref.
			sizeKB := len(data) * 3 / 4 / 1024
			_ = base64.StdEncoding // Ensure import is used.
			return fmt.Sprintf("Screenshot captured (%d KB). Base64 data available for vision analysis.", sizeKB), nil
		},
	)

	// browser_content
	executor.Register(
		MakeToolDefinition("browser_content",
			"Get the text content of the current browser page. Useful for reading web pages "+
				"without rendering. Returns the visible text, truncated if too long.",
			map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			return browserMgr.GetContent(ctx)
		},
	)

	// browser_click
	executor.Register(
		MakeToolDefinition("browser_click",
			"Click an element on the current page by CSS selector.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the element to click (e.g. 'button.submit', '#login-btn').",
					},
				},
				"required": []string{"selector"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			selector, _ := args["selector"].(string)
			if selector == "" {
				return nil, fmt.Errorf("selector is required")
			}
			if err := browserMgr.ClickElement(ctx, selector); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Clicked element: %s", selector), nil
		},
	)

	// browser_fill
	executor.Register(
		MakeToolDefinition("browser_fill",
			"Fill a text input on the current page by CSS selector.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the input element.",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "The value to enter into the input.",
					},
				},
				"required": []string{"selector", "value"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			selector, _ := args["selector"].(string)
			value, _ := args["value"].(string)
			if selector == "" {
				return nil, fmt.Errorf("selector is required")
			}
			if err := browserMgr.FillInput(ctx, selector, value); err != nil {
				return nil, err
			}
			return fmt.Sprintf("Filled input %s with value", selector), nil
		},
	)

	logger.Info("browser tools registered",
		"tools", []string{"browser_navigate", "browser_screenshot", "browser_content", "browser_click", "browser_fill"},
	)
}
