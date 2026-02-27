/** Typed HTTP client for DevClaw API */

const BASE = '/api'

/** Generic fetch wrapper with auth and error handling */
async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = localStorage.getItem('devclaw_token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers,
  })

  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new ApiError(res.status, body || res.statusText)
  }

  if (res.status === 204) return undefined as T

  return res.json()
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/* ── Types ── */

export interface SessionInfo {
  id: string
  channel: string
  chat_id: string
  message_count: number
  last_message_at: string
  created_at: string
}

export interface MessageInfo {
  role: 'user' | 'assistant' | 'tool'
  content: string
  timestamp: string
  tool_name?: string
  tool_input?: string
}

export interface UsageInfo {
  total_input_tokens: number
  total_output_tokens: number
  total_cost: number
  request_count: number
}

export interface ChannelHealth {
  name: string
  connected: boolean
  error_count: number
  last_msg_at: string
}

export interface JobInfo {
  id: string
  schedule: string
  type: string
  command: string
  enabled: boolean
  run_count: number
  last_run_at: string
  last_error: string
}

export interface SkillInfo {
  name: string
  description: string
  enabled: boolean
  tool_count: number
}

export interface WhatsAppStatus {
  connected: boolean
  needs_qr: boolean
  phone?: string
}

export interface DashboardData {
  sessions: SessionInfo[]
  usage: UsageInfo
  channels: ChannelHealth[]
  jobs: JobInfo[]
}

export interface SetupStatus {
  configured: boolean
  current_step: number
}

/* ── Security Types ── */

export interface AuditEntry {
  id: number
  tool: string
  caller: string
  level: string
  allowed: boolean
  args_summary: string
  result_summary: string
  created_at: string
}

export interface AuditResponse {
  entries: AuditEntry[]
  total: number
}

export interface ToolGuardStatus {
  enabled: boolean
  allow_destructive: boolean
  allow_sudo: boolean
  allow_reboot: boolean
  auto_approve: string[]
  require_confirmation: string[]
  protected_paths: string[]
  ssh_allowed_hosts: string[]
  dangerous_commands: string[]
  tool_permissions: Record<string, string>
}

export interface VaultStatus {
  exists: boolean
  unlocked: boolean
  keys: string[]
}

export interface SecurityStatus {
  gateway_auth_configured: boolean
  webui_auth_configured: boolean
  tool_guard_enabled: boolean
  vault_exists: boolean
  vault_unlocked: boolean
  audit_entry_count: number
}

/* ── Hook Types ── */

export interface HookInfo {
  name: string
  description: string
  source: string
  events: string[]
  priority: number
  enabled: boolean
}

export interface HookEventInfo {
  event: string
  description: string
  hooks: string[]
}

export interface HookListResponse {
  hooks: HookInfo[]
  events: HookEventInfo[]
}

/* ── Webhook Types ── */

export interface WebhookInfo {
  id: string
  url: string
  events: string[]
  active: boolean
  created_at: string
}

export interface WebhookListResponse {
  webhooks: WebhookInfo[]
  valid_events: string[]
}

/* ── Domain Types ── */

export interface DomainConfig {
  webui_address: string
  webui_auth_configured: boolean
  gateway_enabled: boolean
  gateway_address: string
  gateway_auth_configured: boolean
  cors_origins: string[]
  tailscale_enabled: boolean
  tailscale_serve: boolean
  tailscale_funnel: boolean
  tailscale_port: number
  tailscale_hostname: string
  tailscale_url: string
  webui_url: string
  gateway_url: string
  public_url: string
}

export interface DomainConfigUpdate {
  webui_address?: string
  webui_auth_token?: string
  gateway_enabled?: boolean
  gateway_address?: string
  gateway_auth_token?: string
  cors_origins?: string[]
  tailscale_enabled?: boolean
  tailscale_serve?: boolean
  tailscale_funnel?: boolean
  tailscale_port?: number
}

/* ── MCP Server Types ── */

export interface MCPServerInfo {
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  enabled: boolean
  status: string
  error?: string
}

export interface MCPServerListResponse {
  servers: MCPServerInfo[]
}

/* ── Database Types ── */

export interface DatabaseStatusInfo {
  name: string
  healthy: boolean
  latency: number
  version: string
  open_connections: number
  in_use: number
  idle: number
  wait_count: number
  wait_duration: number
  max_open_conns: number
  error?: string
}

/* Tool Profile Settings */
export interface ToolProfileInfo {
  name: string
  description: string
  allow: string[]
  deny: string[]
  builtin: boolean
}

/* ── OAuth Types ── */

export interface OAuthProvider {
  id: string
  label: string
  flow_type: 'pkce' | 'device_code' | 'manual'
  experimental?: boolean
}

export interface OAuthStatus {
  provider: string
  status: 'valid' | 'expiring_soon' | 'expired' | 'unknown'
  email?: string
  expires_in?: number
  has_token: boolean
}

export interface OAuthStartResponse {
  flow_type: 'pkce' | 'device_code' | 'manual'
  auth_url?: string
  provider: string
  user_code?: string
  verify_url?: string
  expires_in?: number
  experimental?: boolean
}

/* ── API Methods ── */

export const api = {
  /* Dashboard */
  dashboard: () => request<DashboardData>('/dashboard'),

  /* Sessions */
  sessions: {
    list: () => request<SessionInfo[]>('/sessions'),
    messages: (id: string) => request<MessageInfo[]>(`/sessions/${id}/messages`),
    delete: (id: string) => request<void>(`/sessions/${id}`, { method: 'DELETE' }),
  },

  /* Chat */
  chat: {
    send: (sessionId: string, content: string) =>
      request<{ run_id: string }>(`/chat/${sessionId}/send`, {
        method: 'POST',
        body: JSON.stringify({ content }),
      }),
    abort: (sessionId: string) =>
      request<void>(`/chat/${sessionId}/abort`, { method: 'POST' }),
    history: (sessionId: string) =>
      request<MessageInfo[]>(`/chat/${sessionId}/history`),
  },

  /* Skills */
  skills: {
    list: () => request<SkillInfo[]>('/skills'),
    toggle: (name: string, enabled: boolean) =>
      request<void>(`/skills/${name}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    install: (name: string) =>
      request<void>(`/skills/install`, {
        method: 'POST',
        body: JSON.stringify({ name }),
      }),
  },

  /* Channels */
  channels: {
    list: () => request<ChannelHealth[]>('/channels'),
    whatsapp: {
      status: () => request<WhatsAppStatus>('/channels/whatsapp/status'),
      requestQR: () => request<{ status: string; message: string }>('/channels/whatsapp/qr', { method: 'POST' }),
    },
  },

  /* Config */
  config: {
    get: () => request<Record<string, unknown>>('/config'),
    update: (data: Record<string, unknown>) =>
      request<Record<string, unknown>>('/config', {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  /* Hooks (lifecycle) */
  hooks: {
    list: () => request<HookListResponse>('/hooks'),
    toggle: (name: string, enabled: boolean) =>
      request<void>(`/hooks/${name}`, {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      }),
    unregister: (name: string) =>
      request<void>(`/hooks/${name}`, { method: 'DELETE' }),
  },

  /* Webhooks */
  webhooks: {
    list: () => request<WebhookListResponse>('/webhooks'),
    create: (url: string, events: string[]) =>
      request<WebhookInfo>('/webhooks', {
        method: 'POST',
        body: JSON.stringify({ url, events }),
      }),
    delete: (id: string) =>
      request<void>(`/webhooks/${id}`, { method: 'DELETE' }),
    toggle: (id: string, active: boolean) =>
      request<void>(`/webhooks/${id}`, {
        method: 'PATCH',
        body: JSON.stringify({ active }),
      }),
  },

  /* Domain & Network */
  domain: {
    get: () => request<DomainConfig>('/domain'),
    update: (data: DomainConfigUpdate) =>
      request<{ status: string; message: string }>('/domain', {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  /* Usage */
  usage: () => request<UsageInfo>('/usage'),

  /* Jobs */
  jobs: {
    list: () => request<JobInfo[]>('/jobs'),
  },

  /* Setup */
  setup: {
    status: () => request<SetupStatus>('/setup/status'),
    step: (step: number, data: Record<string, unknown>) =>
      request<{ next_step: number; done: boolean }>(`/setup/step/${step}`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    testProvider: (provider: string, apiKey: string, model: string, baseUrl?: string) =>
      request<{ success: boolean; error?: string }>('/setup/test-provider', {
        method: 'POST',
        body: JSON.stringify({ provider, api_key: apiKey, model, base_url: baseUrl || '' }),
      }),
    finalize: (data: Record<string, unknown>) =>
      request<{ status: string; message: string }>('/setup/finalize', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },

  /* Security */
  security: {
    overview: () => request<SecurityStatus>('/security'),
    audit: (limit = 50) => request<AuditResponse>(`/security/audit?limit=${limit}`),
    toolGuard: {
      get: () => request<ToolGuardStatus>('/security/tool-guard'),
      update: (data: Partial<ToolGuardStatus>) =>
        request<void>('/security/tool-guard', {
          method: 'PUT',
          body: JSON.stringify(data),
        }),
    },
    vault: () => request<VaultStatus>('/security/vault'),
  },

  /* Auth */
  auth: {
    login: (password: string) =>
      request<{ token: string }>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ password }),
      }),
  },

  /* MCP Servers */
  mcp: {
    list: () => request<MCPServerListResponse>('/mcp/servers'),
    create: (name: string, command: string, args: string[] = [], env: Record<string, string> = {}) =>
      request<{ status: string }>('/mcp/servers', {
        method: 'POST',
        body: JSON.stringify({ name, command, args, env }),
      }),
    get: (name: string) => request<MCPServerInfo>(`/mcp/servers/${name}`),
    update: (name: string, enabled: boolean) =>
      request<{ status: string }>(`/mcp/servers/${name}`, {
        method: 'PUT',
        body: JSON.stringify({ enabled }),
      }),
    delete: (name: string) =>
      request<{ status: string }>(`/mcp/servers/${name}`, { method: 'DELETE' }),
    start: (name: string) =>
      request<{ status: string }>(`/mcp/servers/${name}/start`, { method: 'POST' }),
    stop: (name: string) =>
      request<{ status: string }>(`/mcp/servers/${name}/stop`, { method: 'POST' }),
  },

  /* Database */
  database: {
    status: () => request<DatabaseStatusInfo>('/database/status'),
  },

  /* Settings / Tool Profiles */
  settings: {
    toolProfiles: {
      list: () =>
        request<{
          profiles: ToolProfileInfo[]
          groups: Record<string, string[]>
        }>('/settings/tool-profiles'),
      create: (profile: ToolProfileInfo) =>
        request<ToolProfileInfo>('/settings/tool-profiles', {
          method: 'POST',
          body: JSON.stringify(profile),
        }),
      update: (name: string, profile: ToolProfileInfo) =>
        request<ToolProfileInfo>(`/settings/tool-profiles/${name}`, {
          method: 'PUT',
          body: JSON.stringify(profile),
        }),
      delete: (name: string) =>
        request<void>(`/settings/tool-profiles/${name}`, { method: 'DELETE' }),
    },
  },

  /* OAuth */
  oauth: {
    providers: () => request<OAuthProvider[]>('/oauth/providers'),
    status: () => request<Record<string, OAuthStatus>>('/oauth/status'),
    start: (provider: string) =>
      request<OAuthStartResponse>(`/oauth/start/${provider}`, { method: 'POST' }),
    callback: (provider: string, code: string, state: string) =>
      request<{ status: string }>(`/oauth/callback/${provider}?code=${code}&state=${state}`),
    refresh: (provider: string) =>
      request<{ status: string }>(`/oauth/refresh/${provider}`, { method: 'POST' }),
    logout: (provider: string) =>
      request<void>(`/oauth/logout/${provider}`, { method: 'DELETE' }),
    poll: (provider: string) =>
      request<{ status: string; message?: string }>(`/oauth/poll/${provider}`, { method: 'POST' }),
  },

  /* Auth Profiles */
  authProfiles: {
    providers: () => request<{ providers: AuthProviderInfo[]; count: number }>('/auth/providers'),
    list: () => request<{ profiles: AuthProfileInfo[]; count: number }>('/profiles'),
    get: (id: string) => request<AuthProfileInfo>(`/profiles/${id}`),
    create: (data: CreateAuthProfileRequest) =>
      request<{ id: string; success: boolean; message: string }>('/profiles', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<AuthProfileInfo>) =>
      request<{ id: string; success: boolean }>(`/profiles/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<{ id: string; success: boolean }>(`/profiles/${id}`, { method: 'DELETE' }),
    test: (id: string) =>
      request<{ id: string; valid: boolean; expired: boolean; error?: string }>(`/profiles/${id}/test`, {
        method: 'POST',
      }),
  },
}

/* ── Auth Profile Types ── */

export interface AuthProviderInfo {
  name: string
  label: string
  description: string
  modes: string[]
  website?: string
  env_key?: string
  parent_provider?: string
}

export interface AuthProfileInfo {
  id: string
  provider: string
  name: string
  mode: 'api_key' | 'oauth' | 'token'
  enabled: boolean
  priority: number
  valid: boolean
  expired: boolean
  email?: string
  last_error?: string
  created_at: string
  updated_at: string
  last_used_at?: string
}

export interface CreateAuthProfileRequest {
  provider: string
  name: string
  mode: 'api_key' | 'oauth' | 'token'
  api_key?: string
  token?: string
  priority?: number
}
