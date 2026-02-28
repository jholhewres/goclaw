package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
	"github.com/jholhewres/devclaw/pkg/devclaw/oauth/providers"
)

var (
	oauthProvider string
	oauthRegion   string
)

// NewOAuthCommand creates the oauth command group.
func NewOAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oauth",
		Short: "OAuth authentication for LLM providers",
		Long: `Manage OAuth authentication for LLM providers.

Supported providers:
  - gemini   : Google Gemini/Code Assist (PKCE flow)
  - chatgpt  : ChatGPT/Codex (PKCE flow, EXPERIMENTAL)
  - qwen     : Qwen Portal (device code flow)
  - minimax  : MiniMax Portal (device code flow)

Examples:
  devclaw oauth login --provider gemini
  devclaw oauth status
  devclaw oauth logout --provider gemini
`,
	}

	cmd.AddCommand(newOAuthLoginCommand())
	cmd.AddCommand(newOAuthStatusCommand())
	cmd.AddCommand(newOAuthLogoutCommand())
	cmd.AddCommand(newOAuthHubCommand())

	return cmd
}

func newOAuthLoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login with OAuth provider",
		Long: `Login with an OAuth provider to use your subscription instead of API keys.

For PKCE providers (gemini, chatgpt), a browser window will open for authentication.
For device code providers (qwen, minimax), you'll be shown a URL and code to enter.

Examples:
  devclaw oauth login --provider gemini
  devclaw oauth login --provider chatgpt
  devclaw oauth login --provider qwen
  devclaw oauth login --provider minimax --region cn
`,
		RunE: runOAuthLogin,
	}

	cmd.Flags().StringVarP(&oauthProvider, "provider", "p", "", "OAuth provider (gemini, chatgpt, qwen, minimax)")
	cmd.Flags().StringVar(&oauthRegion, "region", "global", "Region for minimax (global, cn)")
	cmd.MarkFlagRequired("provider")

	return cmd
}

func newOAuthStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show OAuth authentication status",
		Long: `Show the authentication status for all OAuth providers.

Displays:
  - Provider name
  - Email (if available)
  - Token status (valid, expiring soon, expired)
  - Time until expiry
`,
		RunE: runOAuthStatus,
	}

	return cmd
}

func newOAuthLogoutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout from OAuth provider",
		Long: `Remove stored OAuth credentials for a provider.

Examples:
  devclaw oauth logout --provider gemini
  devclaw oauth logout --provider chatgpt
`,
		RunE: runOAuthLogout,
	}

	cmd.Flags().StringVarP(&oauthProvider, "provider", "p", "", "OAuth provider to logout from")
	cmd.MarkFlagRequired("provider")

	return cmd
}

func runOAuthLogin(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get data directory
	dataDir, err := getDataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}

	// Create token manager
	tm, err := oauth.NewTokenManager(dataDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create token manager: %w", err)
	}

	provider := strings.ToLower(oauthProvider)

	switch provider {
	case "gemini":
		return loginGemini(ctx, tm)
	case "chatgpt":
		return loginChatGPT(ctx, tm)
	case "qwen":
		return loginQwen(ctx, tm)
	case "minimax":
		return loginMiniMax(ctx, tm)
	default:
		return fmt.Errorf("unsupported OAuth provider: %s (supported: gemini, chatgpt, qwen, minimax)", provider)
	}
}

func loginGemini(ctx context.Context, tm *oauth.TokenManager) error {
	fmt.Println("🔐 Starting Google Gemini OAuth login...")

	// Create provider
	p := providers.NewGeminiProvider()
	tm.RegisterProvider(p)

	// Check if client ID is available
	if p.ClientID() == "" {
		fmt.Println("\n⚠️  No Gemini CLI found and no client ID configured.")
		fmt.Println("Please either:")
		fmt.Println("  1. Install Gemini CLI: npm install -g @google/gemini-cli")
		fmt.Println("  2. Set GEMINI_OAUTH_CLIENT_ID environment variable")
		return fmt.Errorf("no OAuth client ID available")
	}

	// Generate PKCE
	pkce, err := oauth.GeneratePKCE()
	if err != nil {
		return fmt.Errorf("failed to generate PKCE: %w", err)
	}

	// Generate state
	state := generateState()

	// Start callback server
	server := oauth.NewCallbackServer(p.RedirectPort(), state, 5*time.Minute, nil)
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	defer server.Close()

	// Build auth URL
	authURL := p.AuthURL(state, pkce.Challenge)

	fmt.Printf("\nOpening browser for authentication...\n")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	// Open browser
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Please open this URL manually:\n%s\n\n", authURL)
	}

	// Wait for callback
	fmt.Print("Waiting for authentication...")
	result, err := server.WaitForCallback()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("\n✓ Authorization code received")

	// Exchange code for tokens
	fmt.Print("Exchanging authorization code for tokens...")
	cred, err := p.ExchangeCode(ctx, result.Code, pkce.Verifier)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	// Save credential
	if err := tm.SaveCredential(cred); err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	fmt.Println("\n✓ Login successful!")
	fmt.Printf("  Provider: %s\n", cred.Provider)
	if cred.Email != "" {
		fmt.Printf("  Email: %s\n", cred.Email)
	}
	fmt.Printf("  Expires: %s\n", cred.ExpiresAt.Format(time.RFC3339))

	return nil
}

func loginChatGPT(ctx context.Context, tm *oauth.TokenManager) error {
	fmt.Println("🔐 Starting ChatGPT OAuth login...")
	fmt.Println(providers.ExperimentalWarning)

	// Create provider
	p := providers.NewChatGPTProvider()
	tm.RegisterProvider(p)

	// Generate PKCE
	pkce, err := oauth.GeneratePKCE()
	if err != nil {
		return fmt.Errorf("failed to generate PKCE: %w", err)
	}

	// Generate state
	state := generateState()

	// Start callback server
	server := oauth.NewCallbackServer(p.RedirectPort(), state, 5*time.Minute, nil)
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	defer server.Close()

	// Build auth URL
	authURL := p.AuthURL(state, pkce.Challenge)

	fmt.Printf("\nOpening browser for authentication...\n")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	// Open browser
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Please open this URL manually:\n%s\n\n", authURL)
	}

	// Wait for callback
	fmt.Print("Waiting for authentication...")
	result, err := server.WaitForCallback()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("\n✓ Authorization code received")

	// Exchange code for tokens
	fmt.Print("Exchanging authorization code for tokens...")
	cred, err := p.ExchangeCode(ctx, result.Code, pkce.Verifier)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	// Save credential
	if err := tm.SaveCredential(cred); err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	fmt.Println("\n✓ Login successful!")
	fmt.Printf("  Provider: %s (EXPERIMENTAL)\n", cred.Provider)
	if cred.Email != "" {
		fmt.Printf("  Email: %s\n", cred.Email)
	}
	fmt.Printf("  Expires: %s\n", cred.ExpiresAt.Format(time.RFC3339))

	return nil
}

func loginQwen(ctx context.Context, tm *oauth.TokenManager) error {
	fmt.Println("🔐 Starting Qwen Portal OAuth login...")

	// Create provider
	p := providers.NewQwenProvider()
	tm.RegisterProvider(p)

	// Start device code flow
	fmt.Print("Requesting device code...")
	deviceResp, err := p.StartDeviceFlow(ctx)
	if err != nil {
		return fmt.Errorf("failed to start device code flow: %w", err)
	}

	fmt.Println("\n\nPlease visit the following URL and enter the code:")
	fmt.Printf("\n  URL:  %s\n", deviceResp.VerificationURI)
	fmt.Printf("  Code: %s\n\n", deviceResp.UserCode)

	// Open browser
	if deviceResp.VerificationURIComplete != "" {
		openBrowser(deviceResp.VerificationURIComplete)
	} else {
		openBrowser(deviceResp.VerificationURI)
	}

	// Poll for token
	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	fmt.Print("Waiting for authorization...")
	cred, err := p.PollForToken(ctx, deviceResp.DeviceCode, interval)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credential
	if err := tm.SaveCredential(cred); err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	fmt.Println("\n✓ Login successful!")
	fmt.Printf("  Provider: %s\n", cred.Provider)
	fmt.Printf("  Expires: %s\n", cred.ExpiresAt.Format(time.RFC3339))

	return nil
}

func loginMiniMax(ctx context.Context, tm *oauth.TokenManager) error {
	region := strings.ToLower(oauthRegion)
	if region != "global" && region != "cn" {
		return fmt.Errorf("invalid region: %s (must be 'global' or 'cn')", region)
	}

	fmt.Printf("🔐 Starting MiniMax Portal OAuth login (region: %s)...\n", region)

	// Create provider
	p := providers.NewMiniMaxProvider(providers.WithMiniMaxRegion(region))
	tm.RegisterProvider(p)

	// Start device code flow
	fmt.Print("Requesting device code...")
	deviceResp, err := p.StartDeviceFlow(ctx)
	if err != nil {
		return fmt.Errorf("failed to start device code flow: %w", err)
	}

	fmt.Println("\n\nPlease visit the following URL and enter the code:")
	fmt.Printf("\n  URL:  %s\n", deviceResp.VerificationURI)
	fmt.Printf("  Code: %s\n\n", deviceResp.UserCode)

	// Open browser
	openBrowser(deviceResp.VerificationURI)

	// Poll for token
	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	fmt.Print("Waiting for authorization...")
	cred, err := p.PollForToken(ctx, deviceResp.DeviceCode, interval)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credential
	if err := tm.SaveCredential(cred); err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	fmt.Println("\n✓ Login successful!")
	fmt.Printf("  Provider: %s\n", cred.Provider)
	fmt.Printf("  Region: %s\n", region)
	fmt.Printf("  Expires: %s\n", cred.ExpiresAt.Format(time.RFC3339))

	return nil
}

func runOAuthStatus(cmd *cobra.Command, args []string) error {
	// Get data directory
	dataDir, err := getDataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}

	// Create token manager
	tm, err := oauth.NewTokenManager(dataDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create token manager: %w", err)
	}

	status := tm.GetStatus()

	if len(status) == 0 {
		fmt.Println("No OAuth providers configured.")
		fmt.Println("\nTo login, run:")
		fmt.Println("  devclaw oauth login --provider <provider>")
		return nil
	}

	fmt.Println("OAuth Provider Status:")
	fmt.Println(strings.Repeat("-", 60))

	for provider, s := range status {
		var statusIcon string
		switch s.Status {
		case "valid":
			statusIcon = "✓"
		case "expiring_soon":
			statusIcon = "⚠"
		case "expired":
			statusIcon = "✗"
		default:
			statusIcon = "?"
		}

		fmt.Printf("  %s %s\n", statusIcon, provider)
		if s.Email != "" {
			fmt.Printf("    Email: %s\n", s.Email)
		}
		fmt.Printf("    Status: %s", s.Status)
		if s.ExpiresIn > 0 {
			fmt.Printf(" (expires in %s)", s.ExpiresIn.Round(time.Minute))
		}
		fmt.Println()
	}

	return nil
}

func runOAuthLogout(cmd *cobra.Command, args []string) error {
	provider := strings.ToLower(oauthProvider)

	// Get data directory
	dataDir, err := getDataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}

	// Create token manager
	tm, err := oauth.NewTokenManager(dataDir, nil)
	if err != nil {
		return fmt.Errorf("failed to create token manager: %w", err)
	}

	if err := tm.DeleteCredential(provider); err != nil {
		return fmt.Errorf("failed to logout: %w", err)
	}

	fmt.Printf("✓ Logged out from %s\n", provider)
	return nil
}

// Helper functions

func getDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".devclaw"), nil
}

func generateState() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default: // linux, etc.
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}
