// Package copilot – testing_tools.go implements a testing engine that wraps
// common test runners and provides tools for running tests, API testing,
// and generating test reports.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---------- Data Types ----------

type testRunResult struct {
	Framework string `json:"framework"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Duration  string `json:"duration"`
}

type apiTestResult struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	StatusCode int               `json:"status_code"`
	Duration   string            `json:"duration"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Pass       bool              `json:"pass"`
}

// ---------- Tool Registration ----------

// RegisterTestingTools registers testing engine tools.
func RegisterTestingTools(executor *ToolExecutor) {
	// test_run
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "test_run",
			Description: "Run tests using the appropriate test runner. Auto-detects framework (Go, Jest, Pytest, PHPUnit, etc.) or accepts explicit command.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":   map[string]any{"type": "string", "description": "Explicit test command (overrides auto-detect)"},
					"path":      map[string]any{"type": "string", "description": "Specific file or directory to test"},
					"framework": map[string]any{"type": "string", "enum": []string{"go", "jest", "pytest", "phpunit", "rspec", "cargo", "dotnet"}, "description": "Force a specific framework"},
					"verbose":   map[string]any{"type": "boolean", "description": "Enable verbose output"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		cmdStr, _ := args["command"].(string)
		path, _ := args["path"].(string)
		framework, _ := args["framework"].(string)
		verbose, _ := args["verbose"].(bool)

		if cmdStr == "" {
			if framework == "" {
				framework = detectTestFramework()
			}
			cmdStr = buildTestCommand(framework, path, verbose)
		}

		start := time.Now()
		parts := strings.Fields(cmdStr)
		cmd := exec.Command(parts[0], parts[1:]...)
		out, err := cmd.CombinedOutput()
		duration := time.Since(start)

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				return nil, fmt.Errorf("running tests: %w", err)
			}
		}

		result := testRunResult{
			Framework: framework,
			Command:   cmdStr,
			ExitCode:  exitCode,
			Output:    truncateOutput(string(out), 6000),
			Duration:  duration.Truncate(time.Millisecond).String(),
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})

	// api_test
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "api_test",
			Description: "Test an HTTP API endpoint: send request and validate response status, headers, and body.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":             map[string]any{"type": "string", "description": "URL to test"},
					"method":          map[string]any{"type": "string", "description": "HTTP method (default: GET)"},
					"headers":         map[string]any{"type": "object", "description": "Request headers"},
					"body":            map[string]any{"type": "string", "description": "Request body (JSON)"},
					"expected_status": map[string]any{"type": "integer", "description": "Expected status code (default: 200)"},
				},
				"required": []string{"url"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		url, _ := args["url"].(string)
		method, _ := args["method"].(string)
		if method == "" {
			method = "GET"
		}
		body, _ := args["body"].(string)
		expectedStatus := 200
		if v, ok := args["expected_status"].(float64); ok {
			expectedStatus = int(v)
		}

		var bodyReader *strings.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		if headers, ok := args["headers"].(map[string]any); ok {
			for k, v := range headers {
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}

		client := &http.Client{Timeout: 30 * time.Second}
		start := time.Now()
		resp, err := client.Do(req)
		duration := time.Since(start)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		respBody := make([]byte, 0, 4096)
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		respBody = append(respBody, buf[:n]...)

		headers := map[string]string{}
		for k, v := range resp.Header {
			headers[k] = strings.Join(v, ", ")
		}

		result := apiTestResult{
			URL:        url,
			Method:     method,
			StatusCode: resp.StatusCode,
			Duration:   duration.Truncate(time.Millisecond).String(),
			Headers:    headers,
			Body:       truncateOutput(string(respBody), 3000),
			Pass:       resp.StatusCode == expectedStatus,
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})

	// test_coverage
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "test_coverage",
			Description: "Run tests with coverage reporting. Auto-detects the test framework.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Package or directory to test"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		path, _ := args["path"].(string)
		framework := detectTestFramework()

		var cmdStr string
		switch framework {
		case "go":
			pkg := "./..."
			if path != "" {
				pkg = path
			}
			cmdStr = fmt.Sprintf("go test -coverprofile=coverage.out -covermode=atomic %s", pkg)
		case "jest":
			cmdStr = "npx jest --coverage"
			if path != "" {
				cmdStr += " " + path
			}
		case "pytest":
			cmdStr = "python -m pytest --cov --cov-report=term-missing"
			if path != "" {
				cmdStr += " " + path
			}
		default:
			return nil, fmt.Errorf("coverage not supported for framework: %s", framework)
		}

		parts := strings.Fields(cmdStr)
		cmd := exec.Command(parts[0], parts[1:]...)
		out, err := cmd.CombinedOutput()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}

		result := map[string]any{
			"framework": framework,
			"command":   cmdStr,
			"exit_code": exitCode,
			"output":    truncateOutput(string(out), 6000),
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	})
}

func detectTestFramework() string {
	detectors := map[string][]string{
		"go":      {"go.mod"},
		"jest":    {"jest.config.js", "jest.config.ts", "jest.config.cjs"},
		"pytest":  {"pytest.ini", "pyproject.toml", "setup.cfg"},
		"phpunit": {"phpunit.xml", "phpunit.xml.dist"},
		"rspec":   {"Gemfile", ".rspec"},
		"cargo":   {"Cargo.toml"},
		"dotnet":  {"*.csproj", "*.sln"},
	}

	for framework, files := range detectors {
		for _, pattern := range files {
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				return framework
			}
		}
	}

	// Check package.json for test scripts
	if _, err := os.Stat("package.json"); err == nil {
		content, _ := os.ReadFile("package.json")
		if strings.Contains(string(content), "jest") || strings.Contains(string(content), "vitest") {
			return "jest"
		}
		if strings.Contains(string(content), "mocha") {
			return "jest" // Close enough for the command builder
		}
	}

	return "go" // Default
}

func buildTestCommand(framework, path string, verbose bool) string {
	switch framework {
	case "go":
		cmd := "go test"
		if verbose {
			cmd += " -v"
		}
		if path != "" {
			cmd += " " + path
		} else {
			cmd += " ./..."
		}
		return cmd

	case "jest":
		cmd := "npx jest"
		if verbose {
			cmd += " --verbose"
		}
		if path != "" {
			cmd += " " + path
		}
		return cmd

	case "pytest":
		cmd := "python -m pytest"
		if verbose {
			cmd += " -v"
		}
		if path != "" {
			cmd += " " + path
		}
		return cmd

	case "phpunit":
		cmd := "vendor/bin/phpunit"
		if verbose {
			cmd += " --verbose"
		}
		if path != "" {
			cmd += " " + path
		}
		return cmd

	case "rspec":
		cmd := "bundle exec rspec"
		if path != "" {
			cmd += " " + path
		}
		return cmd

	case "cargo":
		cmd := "cargo test"
		if verbose {
			cmd += " -- --nocapture"
		}
		return cmd

	case "dotnet":
		cmd := "dotnet test"
		if verbose {
			cmd += " --verbosity normal"
		}
		return cmd

	default:
		return "go test ./..."
	}
}

func truncateOutput(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n... (truncated)"
	}
	return s
}
