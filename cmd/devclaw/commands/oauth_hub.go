package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	hubService      string
	hubConnectionID string
)

// newOAuthHubCommand creates the "devclaw oauth hub" command group.
func newOAuthHubCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Manage OAuth via OAuth Hub",
		Long: `Manage OAuth connections via a centralized OAuth Hub instance.

The OAuth Hub handles OAuth flows and token management centrally,
so you don't need to configure OAuth providers on each DevClaw instance.

Setup:
  devclaw oauth hub setup

Connect a service:
  devclaw oauth hub connect --service gmail

List connections:
  devclaw oauth hub connections

Disconnect:
  devclaw oauth hub disconnect --id <connection_id>

Status:
  devclaw oauth hub status
`,
	}

	cmd.AddCommand(newHubSetupCmd())
	cmd.AddCommand(newHubConnectCmd())
	cmd.AddCommand(newHubConnectionsCmd())
	cmd.AddCommand(newHubDisconnectCmd())
	cmd.AddCommand(newHubStatusCmd())

	return cmd
}

// --- Hub config ---

type hubConfig struct {
	HubURL string `json:"hub_url"`
	APIKey string `json:"api_key"`
}

func hubConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".devclaw", "oauth_hub.json")
}

func loadHubConfig() (*hubConfig, error) {
	data, err := os.ReadFile(hubConfigPath())
	if err != nil {
		return nil, fmt.Errorf("hub not configured. Run: devclaw oauth hub setup")
	}
	var cfg hubConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid hub config: %w", err)
	}
	return &cfg, nil
}

func saveHubConfig(cfg *hubConfig) error {
	path := hubConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// --- Commands ---

func newHubSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure OAuth Hub connection",
		Long: `Configure the OAuth Hub URL and API key for this DevClaw instance.

You'll need:
  - The Hub URL (e.g., http://localhost:8443 or https://oauth.example.com)
  - An API key from the Hub admin (dk_xxx)

Example:
  devclaw oauth hub setup
`,
		RunE: runHubSetup,
	}
}

func newHubConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect a service via OAuth Hub",
		Long: `Start an OAuth connection flow for a service via the Hub.

A browser window will open to complete the OAuth authorization.

Examples:
  devclaw oauth hub connect --service gmail
  devclaw oauth hub connect --service calendar
  devclaw oauth hub connect --service drive
`,
		RunE: runHubConnect,
	}

	cmd.Flags().StringVarP(&hubService, "service", "s", "", "Service to connect (gmail, calendar, drive, docs, sheets)")
	cmd.MarkFlagRequired("service")

	return cmd
}

func newHubConnectionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connections",
		Short: "List OAuth Hub connections",
		RunE:  runHubConnections,
	}
}

func newHubDisconnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disconnect",
		Short: "Disconnect a service from OAuth Hub",
		RunE:  runHubDisconnect,
	}

	cmd.Flags().StringVar(&hubConnectionID, "id", "", "Connection ID to disconnect")
	cmd.MarkFlagRequired("id")

	return cmd
}

func newHubStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show OAuth Hub connection status",
		RunE:  runHubStatus,
	}
}

// --- Implementations ---

func runHubSetup(cmd *cobra.Command, args []string) error {
	var cfg hubConfig

	// Check if already configured
	existing, _ := loadHubConfig()
	if existing != nil {
		fmt.Printf("Current Hub URL: %s\n", existing.HubURL)
		fmt.Print("Reconfigure? [y/N]: ")
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}

	fmt.Print("Hub URL (e.g., http://localhost:8443): ")
	fmt.Scanln(&cfg.HubURL)
	if cfg.HubURL == "" {
		return fmt.Errorf("hub URL is required")
	}
	cfg.HubURL = strings.TrimRight(cfg.HubURL, "/")

	fmt.Print("API Key (dk_xxx): ")
	fmt.Scanln(&cfg.APIKey)
	if cfg.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Verify connection
	fmt.Print("Verifying connection...")
	client := newHubClient(cfg.HubURL, cfg.APIKey)
	if err := client.healthCheck(context.Background()); err != nil {
		fmt.Printf(" FAILED\n")
		return fmt.Errorf("cannot connect to Hub: %w", err)
	}
	fmt.Printf(" OK\n")

	// Save config
	if err := saveHubConfig(&cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\nHub configured successfully!\n")
	fmt.Printf("  URL: %s\n", cfg.HubURL)
	fmt.Printf("  Key: %s...\n", cfg.APIKey[:min(12, len(cfg.APIKey))])
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  devclaw oauth hub connect --service gmail\n")

	return nil
}

func runHubConnect(cmd *cobra.Command, args []string) error {
	cfg, err := loadHubConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := newHubClient(cfg.HubURL, cfg.APIKey)

	service := strings.ToLower(hubService)

	// Start connection
	fmt.Printf("Starting OAuth connection for %s...\n", service)

	session, err := client.startConnection(ctx, "google", service, nil)
	if err != nil {
		return fmt.Errorf("failed to start connection: %w", err)
	}

	fmt.Printf("\nOpening browser for authentication...\n")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", session.ConnectURL)

	// Open browser
	if err := openHubBrowser(session.ConnectURL); err != nil {
		fmt.Printf("Please open this URL manually:\n%s\n\n", session.ConnectURL)
	}

	// Poll for completion
	fmt.Print("Waiting for authorization")
	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return fmt.Errorf("connection timed out")
		case <-ticker.C:
			status, err := client.checkStatus(ctx, session.SessionID)
			if err != nil {
				return fmt.Errorf("status check failed: %w", err)
			}

			switch status.Status {
			case "connected":
				fmt.Printf("\n\nConnected!\n")
				fmt.Printf("  Connection ID: %s\n", status.ConnectionID)
				if status.Email != "" {
					fmt.Printf("  Email: %s\n", status.Email)
				}
				return nil
			case "failed":
				return fmt.Errorf("connection failed: %s", status.Error)
			case "expired":
				return fmt.Errorf("connection session expired")
			default:
				fmt.Print(".")
			}
		}
	}
}

func runHubConnections(cmd *cobra.Command, args []string) error {
	cfg, err := loadHubConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := newHubClient(cfg.HubURL, cfg.APIKey)

	connections, err := client.listConnections(ctx)
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}

	if len(connections) == 0 {
		fmt.Println("No active connections.")
		fmt.Println("\nTo connect a service:")
		fmt.Println("  devclaw oauth hub connect --service gmail")
		return nil
	}

	fmt.Println("OAuth Hub Connections:")
	fmt.Println(strings.Repeat("-", 70))

	for _, conn := range connections {
		var statusIcon string
		switch conn.Status {
		case "connected":
			statusIcon = "+"
		case "pending":
			statusIcon = "~"
		default:
			statusIcon = "-"
		}

		fmt.Printf("  %s %s/%s\n", statusIcon, conn.Provider, conn.Service)
		fmt.Printf("    ID:     %s\n", conn.ID)
		if conn.Email != "" {
			fmt.Printf("    Email:  %s\n", conn.Email)
		}
		fmt.Printf("    Status: %s\n", conn.Status)
	}

	return nil
}

func runHubDisconnect(cmd *cobra.Command, args []string) error {
	cfg, err := loadHubConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := newHubClient(cfg.HubURL, cfg.APIKey)

	if err := client.disconnect(ctx, hubConnectionID); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	fmt.Printf("Disconnected: %s\n", hubConnectionID)
	return nil
}

func runHubStatus(cmd *cobra.Command, args []string) error {
	cfg, err := loadHubConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := newHubClient(cfg.HubURL, cfg.APIKey)

	// Health check
	fmt.Printf("Hub: %s\n", cfg.HubURL)
	if err := client.healthCheck(ctx); err != nil {
		fmt.Printf("Status: OFFLINE (%s)\n", err)
		return nil
	}
	fmt.Printf("Status: ONLINE\n")

	// List connections
	connections, err := client.listConnections(ctx)
	if err != nil {
		fmt.Printf("Connections: error (%s)\n", err)
		return nil
	}

	activeCount := 0
	for _, c := range connections {
		if c.Status == "connected" {
			activeCount++
		}
	}
	fmt.Printf("Connections: %d active / %d total\n", activeCount, len(connections))

	return nil
}

// --- Hub client (self-contained, no external dependency) ---

type hubClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func newHubClient(baseURL, apiKey string) *hubClient {
	return &hubClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *hubClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var apiErr struct{ Error string `json:"error"` }
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("%s", apiErr.Error)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (h *hubClient) healthCheck(ctx context.Context) error {
	_, err := h.do(ctx, http.MethodGet, "/api/v1/health", nil)
	return err
}

type hubConnectSession struct {
	SessionID  string `json:"session_id"`
	ConnectURL string `json:"connect_url"`
	ExpiresIn  int    `json:"expires_in"`
}

func (h *hubClient) startConnection(ctx context.Context, provider, service string, scopes []string) (*hubConnectSession, error) {
	body := map[string]any{
		"provider": provider,
		"service":  service,
		"scopes":   scopes,
	}
	respBody, err := h.do(ctx, http.MethodPost, "/api/v1/connect/start", body)
	if err != nil {
		return nil, err
	}
	var session hubConnectSession
	return &session, json.Unmarshal(respBody, &session)
}

type hubConnectionStatus struct {
	Status       string `json:"status"`
	ConnectionID string `json:"connection_id,omitempty"`
	Email        string `json:"email,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (h *hubClient) checkStatus(ctx context.Context, sessionID string) (*hubConnectionStatus, error) {
	respBody, err := h.do(ctx, http.MethodGet, "/api/v1/connect/status/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	var status hubConnectionStatus
	return &status, json.Unmarshal(respBody, &status)
}

type hubConnectionInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Service  string `json:"service"`
	Email    string `json:"email,omitempty"`
	Status   string `json:"status"`
}

func (h *hubClient) listConnections(ctx context.Context) ([]hubConnectionInfo, error) {
	respBody, err := h.do(ctx, http.MethodGet, "/api/v1/connections", nil)
	if err != nil {
		return nil, err
	}
	var conns []hubConnectionInfo
	return conns, json.Unmarshal(respBody, &conns)
}

func (h *hubClient) disconnect(ctx context.Context, connectionID string) error {
	_, err := h.do(ctx, http.MethodDelete, "/api/v1/connections/"+connectionID, nil)
	return err
}

func openHubBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
