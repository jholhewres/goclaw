package webui

import (
	"context"
	"errors"
	"net/http"
)

// WhatsAppQREvent mirrors whatsapp.QREvent without importing the channel package.
type WhatsAppQREvent struct {
	Type        string `json:"type"`                   // "code", "success", "timeout", "error", "refresh"
	Code        string `json:"code,omitempty"`
	Message     string `json:"message"`
	ExpiresAt   string `json:"expires_at,omitempty"`   // ISO timestamp
	SecondsLeft int    `json:"seconds_left,omitempty"` // Seconds until QR expires
}

// WhatsAppStatus holds the current WhatsApp connection state for the UI.
type WhatsAppStatus struct {
	Connected         bool   `json:"connected"`
	State             string `json:"state"`              // "disconnected", "connecting", "connected", "waiting_qr", etc.
	NeedsQR           bool   `json:"needs_qr"`
	Phone             string `json:"phone,omitempty"`
	Platform          string `json:"platform,omitempty"`
	ErrorCount        int    `json:"error_count"`
	ReconnectAttempts int    `json:"reconnect_attempts"`
	Message           string `json:"message,omitempty"` // Human-readable status message
}

// AssistantAdapter wraps a generic set of callbacks to satisfy the AssistantAPI
// interface. This avoids a direct import cycle between copilot and webui.
type AssistantAdapter struct {
	GetConfigMapFn       func() map[string]any
	UpdateConfigMapFn    func(updates map[string]any) error
	ListSessionsFn       func() []SessionInfo
	GetSessionMessagesFn func(sessionID string) []MessageInfo
	GetUsageGlobalFn     func() UsageInfo
	GetChannelHealthFn   func() []ChannelHealthInfo
	GetSchedulerJobsFn   func() []JobInfo
	ListSkillsFn         func() []SkillInfo
	ToggleSkillFn        func(name string, enabled bool) error
	SendChatMessageFn    func(sessionID, content string) (string, error)
	StartChatStreamFn    func(ctx context.Context, sessionID, content string) (*RunHandle, error)
	AbortRunFn           func(sessionID string) bool
	DeleteSessionFn      func(sessionID string) error

	// WhatsApp QR support
	GetWhatsAppStatusFn   func() WhatsAppStatus
	SubscribeWhatsAppQRFn func() (chan WhatsAppQREvent, func())
	RequestWhatsAppQRFn   func() error

	// Security: Audit Log
	GetAuditLogFn   func(limit int) []AuditEntry
	GetAuditCountFn func() int

	// Security: Tool Guard
	GetToolGuardStatusFn func() ToolGuardStatus
	UpdateToolGuardFn    func(update ToolGuardStatus) error

	// Security: Vault (read-only, no values)
	GetVaultStatusFn func() VaultStatus

	// Security: Overview
	GetSecurityStatusFn func() SecurityStatus

	// Domain & Network
	GetDomainConfigFn    func() DomainConfigInfo
	UpdateDomainConfigFn func(update DomainConfigUpdate) error

	// Webhooks
	ListWebhooksFn          func() []WebhookInfo
	CreateWebhookFn         func(url string, events []string) (WebhookInfo, error)
	DeleteWebhookFn         func(id string) error
	ToggleWebhookFn         func(id string, active bool) error
	GetValidWebhookEventsFn func() []string

	// Hooks (lifecycle)
	ListHooksFn      func() []HookInfo
	ToggleHookFn     func(name string, enabled bool) error
	UnregisterHookFn func(name string) error
	GetHookEventsFn  func() []HookEventInfo

	// MCP Servers
	ListMCPServersFn    func() []MCPServerInfo
	CreateMCPServerFn   func(name, command string, args []string, env map[string]string) error
	UpdateMCPServerFn   func(name string, enabled bool) error
	DeleteMCPServerFn   func(name string) error
	StartMCPServerFn    func(name string) error
	StopMCPServerFn     func(name string) error

	// Database
	GetDatabaseStatusFn func() DatabaseStatusInfo

	// Settings: Tool Profiles
	ListToolProfilesFn   func() []ToolProfileInfo
	CreateToolProfileFn  func(profile ToolProfileDef) error
	UpdateToolProfileFn  func(name string, profile ToolProfileDef) error
	DeleteToolProfileFn  func(name string) error
}

// ToolProfileInfo contains profile info for API responses.
type ToolProfileInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny"`
	Builtin     bool     `json:"builtin"`
}

// ToolProfileDef defines a tool profile for creation/update.
type ToolProfileDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny"`
}

func (a *AssistantAdapter) GetConfigMap() map[string]any {
	if a.GetConfigMapFn != nil {
		return a.GetConfigMapFn()
	}
	return nil
}

func (a *AssistantAdapter) UpdateConfigMap(updates map[string]any) error {
	if a.UpdateConfigMapFn != nil {
		return a.UpdateConfigMapFn(updates)
	}
	return errors.New("config update not available")
}

func (a *AssistantAdapter) ListSessions() []SessionInfo {
	if a.ListSessionsFn != nil {
		return a.ListSessionsFn()
	}
	return nil
}

func (a *AssistantAdapter) GetSessionMessages(sessionID string) []MessageInfo {
	if a.GetSessionMessagesFn != nil {
		return a.GetSessionMessagesFn(sessionID)
	}
	return nil
}

func (a *AssistantAdapter) GetUsageGlobal() UsageInfo {
	if a.GetUsageGlobalFn != nil {
		return a.GetUsageGlobalFn()
	}
	return UsageInfo{}
}

func (a *AssistantAdapter) GetChannelHealth() []ChannelHealthInfo {
	if a.GetChannelHealthFn != nil {
		return a.GetChannelHealthFn()
	}
	return nil
}

func (a *AssistantAdapter) GetSchedulerJobs() []JobInfo {
	if a.GetSchedulerJobsFn != nil {
		return a.GetSchedulerJobsFn()
	}
	return nil
}

func (a *AssistantAdapter) ListSkills() []SkillInfo {
	if a.ListSkillsFn != nil {
		return a.ListSkillsFn()
	}
	return nil
}

func (a *AssistantAdapter) ToggleSkill(name string, enabled bool) error {
	if a.ToggleSkillFn != nil {
		return a.ToggleSkillFn(name, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) SendChatMessage(sessionID, content string) (string, error) {
	if a.SendChatMessageFn != nil {
		return a.SendChatMessageFn(sessionID, content)
	}
	return "", nil
}

func (a *AssistantAdapter) StartChatStream(ctx context.Context, sessionID, content string) (*RunHandle, error) {
	if a.StartChatStreamFn != nil {
		return a.StartChatStreamFn(ctx, sessionID, content)
	}
	return nil, errors.New("streaming not available")
}

func (a *AssistantAdapter) AbortRun(sessionID string) bool {
	if a.AbortRunFn != nil {
		return a.AbortRunFn(sessionID)
	}
	return false
}

func (a *AssistantAdapter) DeleteSession(sessionID string) error {
	if a.DeleteSessionFn != nil {
		return a.DeleteSessionFn(sessionID)
	}
	return errors.New("not implemented")
}

// ── Security ──

func (a *AssistantAdapter) GetAuditLog(limit int) []AuditEntry {
	if a.GetAuditLogFn != nil {
		return a.GetAuditLogFn(limit)
	}
	return nil
}

func (a *AssistantAdapter) GetAuditCount() int {
	if a.GetAuditCountFn != nil {
		return a.GetAuditCountFn()
	}
	return 0
}

func (a *AssistantAdapter) GetToolGuardStatus() ToolGuardStatus {
	if a.GetToolGuardStatusFn != nil {
		return a.GetToolGuardStatusFn()
	}
	return ToolGuardStatus{}
}

func (a *AssistantAdapter) UpdateToolGuard(update ToolGuardStatus) error {
	if a.UpdateToolGuardFn != nil {
		return a.UpdateToolGuardFn(update)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetVaultStatus() VaultStatus {
	if a.GetVaultStatusFn != nil {
		return a.GetVaultStatusFn()
	}
	return VaultStatus{}
}

func (a *AssistantAdapter) GetSecurityStatus() SecurityStatus {
	if a.GetSecurityStatusFn != nil {
		return a.GetSecurityStatusFn()
	}
	return SecurityStatus{}
}

// ── Domain & Network ──

func (a *AssistantAdapter) GetDomainConfig() DomainConfigInfo {
	if a.GetDomainConfigFn != nil {
		return a.GetDomainConfigFn()
	}
	return DomainConfigInfo{}
}

func (a *AssistantAdapter) UpdateDomainConfig(update DomainConfigUpdate) error {
	if a.UpdateDomainConfigFn != nil {
		return a.UpdateDomainConfigFn(update)
	}
	return errors.New("not implemented")
}

// ── Webhooks ──

func (a *AssistantAdapter) ListWebhooks() []WebhookInfo {
	if a.ListWebhooksFn != nil {
		return a.ListWebhooksFn()
	}
	return nil
}

func (a *AssistantAdapter) CreateWebhook(url string, events []string) (WebhookInfo, error) {
	if a.CreateWebhookFn != nil {
		return a.CreateWebhookFn(url, events)
	}
	return WebhookInfo{}, errors.New("not implemented")
}

func (a *AssistantAdapter) DeleteWebhook(id string) error {
	if a.DeleteWebhookFn != nil {
		return a.DeleteWebhookFn(id)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) ToggleWebhook(id string, active bool) error {
	if a.ToggleWebhookFn != nil {
		return a.ToggleWebhookFn(id, active)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetValidWebhookEvents() []string {
	if a.GetValidWebhookEventsFn != nil {
		return a.GetValidWebhookEventsFn()
	}
	return nil
}

// ── Hooks (Lifecycle) ──

func (a *AssistantAdapter) ListHooks() []HookInfo {
	if a.ListHooksFn != nil {
		return a.ListHooksFn()
	}
	return nil
}

func (a *AssistantAdapter) ToggleHook(name string, enabled bool) error {
	if a.ToggleHookFn != nil {
		return a.ToggleHookFn(name, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UnregisterHook(name string) error {
	if a.UnregisterHookFn != nil {
		return a.UnregisterHookFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) GetHookEvents() []HookEventInfo {
	if a.GetHookEventsFn != nil {
		return a.GetHookEventsFn()
	}
	return nil
}

// ── MCP Servers ──

func (a *AssistantAdapter) ListMCPServers() []MCPServerInfo {
	if a.ListMCPServersFn != nil {
		return a.ListMCPServersFn()
	}
	return nil
}

func (a *AssistantAdapter) CreateMCPServer(name, command string, args []string, env map[string]string) error {
	if a.CreateMCPServerFn != nil {
		return a.CreateMCPServerFn(name, command, args, env)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) UpdateMCPServer(name string, enabled bool) error {
	if a.UpdateMCPServerFn != nil {
		return a.UpdateMCPServerFn(name, enabled)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) DeleteMCPServer(name string) error {
	if a.DeleteMCPServerFn != nil {
		return a.DeleteMCPServerFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) StartMCPServer(name string) error {
	if a.StartMCPServerFn != nil {
		return a.StartMCPServerFn(name)
	}
	return errors.New("not implemented")
}

func (a *AssistantAdapter) StopMCPServer(name string) error {
	if a.StopMCPServerFn != nil {
		return a.StopMCPServerFn(name)
	}
	return errors.New("not implemented")
}

// ── Database ──

func (a *AssistantAdapter) GetDatabaseStatus() DatabaseStatusInfo {
	if a.GetDatabaseStatusFn != nil {
		return a.GetDatabaseStatusFn()
	}
	return DatabaseStatusInfo{}
}

// ── Media API Adapter ──

// MediaAdapter wraps a MediaService to implement the MediaAPI interface.
// This avoids importing the media package directly in webui.
type MediaAdapter struct {
	UploadFn func(r *http.Request, sessionID string) (mediaID string, mediaType string, filename string, size int64, err error)
	GetFn    func(mediaID string) ([]byte, string, string, error)
	ListFn   func(sessionID string, mediaType string, limit int) ([]MediaInfo, error)
	DeleteFn func(mediaID string) error
}

// Upload implements MediaAPI.Upload.
func (m *MediaAdapter) Upload(r *http.Request, sessionID string) (string, string, string, int64, error) {
	if m.UploadFn != nil {
		return m.UploadFn(r, sessionID)
	}
	return "", "", "", 0, errors.New("media upload not available")
}

// Get implements MediaAPI.Get.
func (m *MediaAdapter) Get(mediaID string) ([]byte, string, string, error) {
	if m.GetFn != nil {
		return m.GetFn(mediaID)
	}
	return nil, "", "", errors.New("media get not available")
}

// List implements MediaAPI.List.
func (m *MediaAdapter) List(sessionID string, mediaType string, limit int) ([]MediaInfo, error) {
	if m.ListFn != nil {
		return m.ListFn(sessionID, mediaType, limit)
	}
	return nil, errors.New("media list not available")
}

// Delete implements MediaAPI.Delete.
func (m *MediaAdapter) Delete(mediaID string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(mediaID)
	}
	return errors.New("media delete not available")
}
