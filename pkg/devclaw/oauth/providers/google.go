package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
)

const (
	// Google OAuth endpoints
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v1/userinfo"

	// Default redirect port for Google OAuth
	googleDefaultPort = 8086
)

// GoogleScopes contains OAuth scopes for different Google services.
var GoogleScopes = struct {
	// Basic profile information
	OpenID  string
	Email   string
	Profile string

	// Gmail API scopes
	GmailReadonly string
	GmailModify   string
	GmailSend     string
	GmailFull     string

	// Calendar API scopes
	CalendarReadonly string
	Calendar         string
	CalendarEvents   string

	// Drive API scopes
	DriveReadonly string
	Drive         string
	DriveFile     string
	DriveAppdata  string

	// Sheets API scopes
	SheetsReadonly string
	Sheets         string

	// Docs API scopes
	DocsReadonly string
	Docs         string

	// Slides API scopes
	SlidesReadonly string
	Slides         string

	// Contacts API scopes
	ContactsReadonly string
	Contacts         string

	// People API scopes (newer API for contacts/profile)
	PeopleReadonly string
	People         string

	// Tasks API scopes
	TasksReadonly string
	Tasks         string

	// Cloud Platform (for Gemini/Code Assist)
	CloudPlatform string
}{
	OpenID:  "openid",
	Email:   "https://www.googleapis.com/auth/userinfo.email",
	Profile: "https://www.googleapis.com/auth/userinfo.profile",

	GmailReadonly: "https://www.googleapis.com/auth/gmail.readonly",
	GmailModify:   "https://www.googleapis.com/auth/gmail.modify",
	GmailSend:     "https://www.googleapis.com/auth/gmail.send",
	GmailFull:     "https://www.googleapis.com/auth/gmail.full_access",

	CalendarReadonly: "https://www.googleapis.com/auth/calendar.readonly",
	Calendar:         "https://www.googleapis.com/auth/calendar",
	CalendarEvents:   "https://www.googleapis.com/auth/calendar.events",

	DriveReadonly: "https://www.googleapis.com/auth/drive.readonly",
	Drive:         "https://www.googleapis.com/auth/drive",
	DriveFile:     "https://www.googleapis.com/auth/drive.file",
	DriveAppdata:  "https://www.googleapis.com/auth/drive.appdata",

	SheetsReadonly: "https://www.googleapis.com/auth/spreadsheets.readonly",
	Sheets:         "https://www.googleapis.com/auth/spreadsheets",

	DocsReadonly: "https://www.googleapis.com/auth/documents.readonly",
	Docs:         "https://www.googleapis.com/auth/documents",

	SlidesReadonly: "https://www.googleapis.com/auth/presentations.readonly",
	Slides:         "https://www.googleapis.com/auth/presentations",

	ContactsReadonly: "https://www.googleapis.com/auth/contacts.readonly",
	Contacts:         "https://www.googleapis.com/auth/contacts",

	PeopleReadonly: "https://www.googleapis.com/auth/userinfo.profile",
	People:         "https://www.googleapis.com/auth/userinfo.profile",

	TasksReadonly: "https://www.googleapis.com/auth/tasks.readonly",
	Tasks:         "https://www.googleapis.com/auth/tasks",

	CloudPlatform: "https://www.googleapis.com/auth/cloud-platform",
}

// GetGmailScopes returns the recommended Gmail scopes.
func GetGmailScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.GmailReadonly,
		GoogleScopes.GmailModify,
	}
}

// GetCalendarScopes returns the recommended Calendar scopes.
func GetCalendarScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.Calendar,
		GoogleScopes.CalendarEvents,
	}
}

// GetDriveScopes returns the recommended Drive scopes.
func GetDriveScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.DriveReadonly,
		GoogleScopes.DriveFile,
	}
}

// GetSheetsScopes returns the recommended Sheets scopes.
func GetSheetsScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.Sheets,
	}
}

// GetDocsScopes returns the recommended Docs scopes.
func GetDocsScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.Docs,
	}
}

// GetSlidesScopes returns the recommended Slides scopes.
func GetSlidesScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.Slides,
	}
}

// GetContactsScopes returns the recommended Contacts scopes.
func GetContactsScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.Contacts,
	}
}

// GetPeopleScopes returns the recommended People scopes.
func GetPeopleScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.People,
	}
}

// GetTasksScopes returns the recommended Tasks scopes.
func GetTasksScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.Tasks,
	}
}

// GetFullGoogleScopes returns all common Google service scopes.
func GetFullGoogleScopes() []string {
	return []string{
		GoogleScopes.OpenID,
		GoogleScopes.Email,
		GoogleScopes.Profile,
		GoogleScopes.GmailReadonly,
		GoogleScopes.GmailModify,
		GoogleScopes.Calendar,
		GoogleScopes.CalendarEvents,
		GoogleScopes.DriveReadonly,
		GoogleScopes.DriveFile,
		GoogleScopes.Sheets,
		GoogleScopes.Docs,
		GoogleScopes.Slides,
		GoogleScopes.Contacts,
		GoogleScopes.Tasks,
	}
}

// GoogleProvider implements OAuth for Google APIs (Gmail, Calendar, Drive).
type GoogleProvider struct {
	BaseProvider
	clientID     string
	clientSecret string
	redirectPort int
	scopes       []string
	httpClient   *http.Client
	logger       *slog.Logger
}

// GoogleOption configures the Google provider.
type GoogleOption func(*GoogleProvider)

// WithGoogleClientID sets a custom client ID.
func WithGoogleClientID(clientID string) GoogleOption {
	return func(p *GoogleProvider) {
		p.clientID = clientID
	}
}

// WithGoogleClientSecret sets a custom client secret.
func WithGoogleClientSecret(secret string) GoogleOption {
	return func(p *GoogleProvider) {
		p.clientSecret = secret
	}
}

// WithGoogleRedirectPort sets the redirect port.
func WithGoogleRedirectPort(port int) GoogleOption {
	return func(p *GoogleProvider) {
		p.redirectPort = port
	}
}

// WithGoogleScopes sets custom OAuth scopes.
func WithGoogleScopes(scopes []string) GoogleOption {
	return func(p *GoogleProvider) {
		p.scopes = scopes
	}
}

// WithGoogleLogger sets the logger.
func WithGoogleLogger(logger *slog.Logger) GoogleOption {
	return func(p *GoogleProvider) {
		p.logger = logger
	}
}

// WithGoogleName sets the provider name (e.g., "google-gmail", "google-calendar").
func WithGoogleName(name string) GoogleOption {
	return func(p *GoogleProvider) {
		p.name = name
	}
}

// WithGoogleService configures the provider for a specific Google service.
func WithGoogleService(service string) GoogleOption {
	return func(p *GoogleProvider) {
		switch service {
		case "gmail":
			p.scopes = GetGmailScopes()
		case "calendar":
			p.scopes = GetCalendarScopes()
		case "drive":
			p.scopes = GetDriveScopes()
		case "full":
			p.scopes = GetFullGoogleScopes()
		default:
			// Keep default scopes
		}
	}
}

// NewGoogleProvider creates a new Google OAuth provider.
func NewGoogleProvider(opts ...GoogleOption) *GoogleProvider {
	p := &GoogleProvider{
		BaseProvider: BaseProvider{
			name:               "google",
			label:              "Google",
			authURL:            googleAuthURL,
			tokenURL:           googleTokenURL,
			scopes:             GetFullGoogleScopes(),
			supportsPKCE:       true,
			supportsDeviceCode: false,
		},
		redirectPort: googleDefaultPort,
		scopes:       GetFullGoogleScopes(),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		logger:       slog.Default().With("provider", "google"),
	}

	for _, opt := range opts {
		opt(p)
	}

	// Update base provider scopes
	p.BaseProvider.scopes = p.scopes

	return p
}

// NewGmailProvider creates a Google provider configured for Gmail access.
func NewGmailProvider(opts ...GoogleOption) *GoogleProvider {
	allOpts := append([]GoogleOption{WithGoogleService("gmail")}, opts...)
	p := NewGoogleProvider(allOpts...)
	p.name = "google-gmail"
	p.label = "Gmail"
	return p
}

// NewCalendarProvider creates a Google provider configured for Calendar access.
func NewCalendarProvider(opts ...GoogleOption) *GoogleProvider {
	allOpts := append([]GoogleOption{WithGoogleService("calendar")}, opts...)
	p := NewGoogleProvider(allOpts...)
	p.name = "google-calendar"
	p.label = "Google Calendar"
	return p
}

// NewDriveProvider creates a Google provider configured for Drive access.
func NewDriveProvider(opts ...GoogleOption) *GoogleProvider {
	allOpts := append([]GoogleOption{WithGoogleService("drive")}, opts...)
	p := NewGoogleProvider(allOpts...)
	p.name = "google-drive"
	p.label = "Google Drive"
	return p
}

// AuthURL returns the authorization URL for the OAuth flow.
func (p *GoogleProvider) AuthURL(state, challenge string) string {
	params := url.Values{
		"client_id":             {p.clientID},
		"response_type":         {"code"},
		"redirect_uri":          {p.redirectURI()},
		"scope":                 {strings.Join(p.scopes, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}

	return googleAuthURL + "?" + params.Encode()
}

// redirectURI returns the OAuth redirect URI.
func (p *GoogleProvider) redirectURI() string {
	return fmt.Sprintf("http://localhost:%d/oauth/callback", p.redirectPort)
}

// RedirectPort returns the configured redirect port.
func (p *GoogleProvider) RedirectPort() int {
	return p.redirectPort
}

// ClientID returns the configured client ID.
func (p *GoogleProvider) ClientID() string {
	return p.clientID
}

// Scopes returns the configured OAuth scopes.
func (p *GoogleProvider) Scopes() []string {
	return p.scopes
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *GoogleProvider) ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {p.redirectURI()},
	}

	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauth.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Get user email
	email, _ := p.getUserEmail(ctx, tokenResp.AccessToken)

	cred := &oauth.OAuthCredential{
		Provider:     p.name,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Email:        email,
		ClientID:     p.clientID,
		Metadata: map[string]string{
			"scopes": strings.Join(p.scopes, ","),
		},
	}

	return cred, nil
}

// RefreshToken refreshes an access token.
func (p *GoogleProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauth.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &oauth.OAuthCredential{
		Provider:     p.name,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		ClientID:     p.clientID,
		Metadata: map[string]string{
			"scopes": strings.Join(p.scopes, ","),
		},
	}, nil
}

// getUserEmail fetches the user's email from the userinfo endpoint.
func (p *GoogleProvider) getUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return "", err
	}

	return userInfo.Email, nil
}

// ValidateToken validates an access token and returns token info.
func (p *GoogleProvider) ValidateToken(ctx context.Context, accessToken string) (*TokenInfo, error) {
	// Google's tokeninfo endpoint
	tokenInfoURL := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?access_token=%s", accessToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenInfoURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token validation failed: %s", string(body))
	}

	var info TokenInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// TokenInfo contains information about an OAuth token.
type TokenInfo struct {
	IssuedTo      string `json:"issued_to"`
	Audience      string `json:"audience"`
	UserID        string `json:"user_id"`
	Scope         string `json:"scope"`
	ExpiresIn     int    `json:"expires_in"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	AccessType    string `json:"access_type"`
}
