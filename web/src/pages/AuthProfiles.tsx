import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Key,
  Plus,
  Trash2,
  RefreshCw,
  CheckCircle2,
  XCircle,
  AlertCircle,
  ExternalLink,
  ChevronDown,
  ChevronUp,
  Save,
  X,
} from 'lucide-react'
import { api, type AuthProfileInfo, type AuthProviderInfo } from '@/lib/api'

/**
 * Auth Profiles management page — manage OAuth/API keys for Google, OpenAI, etc.
 */
export function AuthProfiles() {
  const { t } = useTranslation()
  const [profiles, setProfiles] = useState<AuthProfileInfo[]>([])
  const [providers, setProviders] = useState<AuthProviderInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [expandedProfile, setExpandedProfile] = useState<string | null>(null)
  const [isCreating, setIsCreating] = useState(false)

  // Form state
  const [formData, setFormData] = useState({
    provider: '',
    name: '',
    mode: 'api_key' as 'api_key' | 'oauth' | 'token',
    api_key: '',
    token: '',
    priority: 0,
  })

  const fetchData = async () => {
    try {
      setLoading(true)
      setError(null)
      const [profilesRes, providersRes] = await Promise.all([
        api.authProfiles.list(),
        api.authProfiles.providers(),
      ])
      setProfiles(profilesRes.profiles)
      setProviders(providersRes.providers)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load profiles')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
  }, [])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await api.authProfiles.create({
        provider: formData.provider,
        name: formData.name,
        mode: formData.mode,
        api_key: formData.api_key || undefined,
        token: formData.token || undefined,
        priority: formData.priority || undefined,
      })
      setIsCreating(false)
      setFormData({
        provider: '',
        name: '',
        mode: 'api_key',
        api_key: '',
        token: '',
        priority: 0,
      })
      fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create profile')
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm(t('authProfiles.confirmDelete'))) return
    try {
      await api.authProfiles.delete(id)
      fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete profile')
    }
  }

  const handleToggleEnabled = async (profile: AuthProfileInfo) => {
    try {
      await api.authProfiles.update(profile.id, {
        enabled: !profile.enabled,
      })
      fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update profile')
    }
  }

  const handleTest = async (id: string) => {
    try {
      const result = await api.authProfiles.test(id)
      if (result.valid) {
        alert(t('authProfiles.testSuccess'))
      } else {
        alert(t('authProfiles.testFailed') + (result.error ? `: ${result.error}` : ''))
      }
      fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to test profile')
    }
  }

  const getProviderLabel = (providerName: string) => {
    const provider = providers.find((p) => p.name === providerName)
    return provider?.label || providerName
  }

  const getProviderIcon = (providerName: string) => {
    // Return appropriate icon based on provider
    if (providerName.startsWith('google')) {
      return (
        <svg className="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
          <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
          <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
          <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
          <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
        </svg>
      )
    }
    return <Key className="h-5 w-5" />
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
        <div className="h-8 w-8 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
      </div>
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">
            {t('authProfiles.subtitle')}
          </p>
          <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">
            {t('authProfiles.title')}
          </h1>
        </div>
        <button
          onClick={() => setIsCreating(true)}
          className="flex items-center gap-2 px-4 py-2 bg-[#3b82f6] hover:bg-[#2563eb] text-white rounded-lg transition-colors"
        >
          <Plus className="h-4 w-4" />
          {t('authProfiles.addProfile')}
        </button>
      </div>

      {error && (
        <div className="mt-4 p-3 bg-[#dc2626]/10 border border-[#dc2626]/30 rounded-lg flex items-center gap-2 text-[#f87171]">
          <AlertCircle className="h-4 w-4" />
          <span className="text-sm">{error}</span>
          <button
            onClick={() => setError(null)}
            className="ml-auto text-[#f87171] hover:text-white"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Create Profile Form */}
      {isCreating && (
        <div className="mt-6 p-6 bg-[#1e293b] rounded-xl border border-[#334155]">
          <h3 className="text-lg font-semibold text-[#f8fafc] mb-4">
            {t('authProfiles.createProfile')}
          </h3>
          <form onSubmit={handleCreate} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-[#94a3b8] mb-1">
                  {t('authProfiles.provider')}
                </label>
                <select
                  value={formData.provider}
                  onChange={(e) =>
                    setFormData({ ...formData, provider: e.target.value })
                  }
                  className="w-full px-3 py-2 bg-[#0f172a] border border-[#334155] rounded-lg text-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-[#3b82f6]"
                  required
                >
                  <option value="">{t('authProfiles.selectProvider')}</option>
                  {providers.map((provider) => (
                    <option key={provider.name} value={provider.name}>
                      {provider.label}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-[#94a3b8] mb-1">
                  {t('authProfiles.profileName')}
                </label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) =>
                    setFormData({ ...formData, name: e.target.value })
                  }
                  placeholder={t('authProfiles.namePlaceholder')}
                  className="w-full px-3 py-2 bg-[#0f172a] border border-[#334155] rounded-lg text-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-[#3b82f6]"
                  required
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-[#94a3b8] mb-1">
                  {t('authProfiles.authMode')}
                </label>
                <select
                  value={formData.mode}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      mode: e.target.value as 'api_key' | 'oauth' | 'token',
                    })
                  }
                  className="w-full px-3 py-2 bg-[#0f172a] border border-[#334155] rounded-lg text-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-[#3b82f6]"
                >
                  <option value="api_key">{t('authProfiles.modeApiKey')}</option>
                  <option value="oauth">{t('authProfiles.modeOAuth')}</option>
                  <option value="token">{t('authProfiles.modeToken')}</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-[#94a3b8] mb-1">
                  {t('authProfiles.priority')}
                </label>
                <input
                  type="number"
                  value={formData.priority}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      priority: parseInt(e.target.value) || 0,
                    })
                  }
                  className="w-full px-3 py-2 bg-[#0f172a] border border-[#334155] rounded-lg text-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-[#3b82f6]"
                />
              </div>
            </div>

            {formData.mode === 'api_key' && (
              <div>
                <label className="block text-sm font-medium text-[#94a3b8] mb-1">
                  {t('authProfiles.apiKey')}
                </label>
                <input
                  type="password"
                  value={formData.api_key}
                  onChange={(e) =>
                    setFormData({ ...formData, api_key: e.target.value })
                  }
                  placeholder={t('authProfiles.apiKeyPlaceholder')}
                  className="w-full px-3 py-2 bg-[#0f172a] border border-[#334155] rounded-lg text-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-[#3b82f6]"
                />
              </div>
            )}

            {formData.mode === 'token' && (
              <div>
                <label className="block text-sm font-medium text-[#94a3b8] mb-1">
                  {t('authProfiles.token')}
                </label>
                <input
                  type="password"
                  value={formData.token}
                  onChange={(e) =>
                    setFormData({ ...formData, token: e.target.value })
                  }
                  placeholder={t('authProfiles.tokenPlaceholder')}
                  className="w-full px-3 py-2 bg-[#0f172a] border border-[#334155] rounded-lg text-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-[#3b82f6]"
                />
              </div>
            )}

            {formData.mode === 'oauth' && (
              <div className="p-3 bg-[#0f172a] rounded-lg border border-[#334155]">
                <p className="text-sm text-[#94a3b8]">
                  {t('authProfiles.oauthNote')}
                </p>
              </div>
            )}

            <div className="flex gap-3 pt-2">
              <button
                type="submit"
                className="flex items-center gap-2 px-4 py-2 bg-[#3b82f6] hover:bg-[#2563eb] text-white rounded-lg transition-colors"
              >
                <Save className="h-4 w-4" />
                {t('common.save')}
              </button>
              <button
                type="button"
                onClick={() => setIsCreating(false)}
                className="px-4 py-2 bg-[#334155] hover:bg-[#475569] text-[#f8fafc] rounded-lg transition-colors"
              >
                {t('common.cancel')}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Profiles List */}
      <div className="mt-6 space-y-3">
        {profiles.length === 0 ? (
          <div className="p-8 text-center bg-[#1e293b] rounded-xl border border-[#334155]">
            <Key className="h-12 w-12 mx-auto text-[#475569] mb-3" />
            <p className="text-[#94a3b8]">{t('authProfiles.noProfiles')}</p>
            <p className="mt-1 text-sm text-[#64748b]">
              {t('authProfiles.noProfilesHint')}
            </p>
          </div>
        ) : (
          profiles.map((profile) => (
            <div
              key={profile.id}
              className="bg-[#1e293b] rounded-xl border border-[#334155] overflow-hidden"
            >
              <div
                className="p-4 flex items-center gap-4 cursor-pointer hover:bg-[#252f47] transition-colors"
                onClick={() =>
                  setExpandedProfile(
                    expandedProfile === profile.id ? null : profile.id
                  )
                }
              >
                <div className="text-[#60a5fa]">{getProviderIcon(profile.provider)}</div>

                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-[#f8fafc]">
                      {getProviderLabel(profile.provider)}
                    </span>
                    <span className="text-[#64748b]">/</span>
                    <span className="text-[#94a3b8]">{profile.name}</span>
                    {profile.email && (
                      <span className="text-xs text-[#64748b]">({profile.email})</span>
                    )}
                  </div>
                  <div className="flex items-center gap-3 mt-1">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full ${
                        profile.enabled
                          ? 'bg-[#22c55e]/20 text-[#4ade80]'
                          : 'bg-[#ef4444]/20 text-[#f87171]'
                      }`}
                    >
                      {profile.enabled ? t('common.enabled') : t('common.disabled')}
                    </span>
                    <span className="text-xs text-[#64748b] uppercase">
                      {profile.mode}
                    </span>
                    {profile.valid ? (
                      <CheckCircle2 className="h-3.5 w-3.5 text-[#22c55e]" />
                    ) : (
                      <XCircle className="h-3.5 w-3.5 text-[#ef4444]" />
                    )}
                    {profile.expired && (
                      <span className="text-xs text-[#f87171]">
                        {t('authProfiles.expired')}
                      </span>
                    )}
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      handleToggleEnabled(profile)
                    }}
                    className={`p-2 rounded-lg transition-colors ${
                      profile.enabled
                        ? 'text-[#22c55e] hover:bg-[#22c55e]/10'
                        : 'text-[#64748b] hover:text-[#f8fafc] hover:bg-[#334155]'
                    }`}
                    title={profile.enabled ? t('common.disable') : t('common.enable')}
                  >
                    {profile.enabled ? (
                      <CheckCircle2 className="h-4 w-4" />
                    ) : (
                      <XCircle className="h-4 w-4" />
                    )}
                  </button>

                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      handleTest(profile.id)
                    }}
                    className="p-2 text-[#3b82f6] hover:bg-[#3b82f6]/10 rounded-lg transition-colors"
                    title={t('authProfiles.test')}
                  >
                    <RefreshCw className="h-4 w-4" />
                  </button>

                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      handleDelete(profile.id)
                    }}
                    className="p-2 text-[#ef4444] hover:bg-[#ef4444]/10 rounded-lg transition-colors"
                    title={t('common.delete')}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>

                  {expandedProfile === profile.id ? (
                    <ChevronUp className="h-5 w-5 text-[#64748b]" />
                  ) : (
                    <ChevronDown className="h-5 w-5 text-[#64748b]" />
                  )}
                </div>
              </div>

              {expandedProfile === profile.id && (
                <div className="px-4 pb-4 border-t border-[#334155] bg-[#0f172a]/50">
                  <div className="pt-4 space-y-3">
                    <div className="grid grid-cols-2 gap-4 text-sm">
                      <div>
                        <span className="text-[#64748b]">{t('authProfiles.id')}:</span>
                        <span className="ml-2 text-[#94a3b8] font-mono">{profile.id}</span>
                      </div>
                      <div>
                        <span className="text-[#64748b]">{t('authProfiles.priority')}:</span>
                        <span className="ml-2 text-[#94a3b8]">{profile.priority}</span>
                      </div>
                      <div>
                        <span className="text-[#64748b]">{t('authProfiles.created')}:</span>
                        <span className="ml-2 text-[#94a3b8]">
                          {new Date(profile.created_at).toLocaleDateString()}
                        </span>
                      </div>
                      <div>
                        <span className="text-[#64748b]">{t('authProfiles.updated')}:</span>
                        <span className="ml-2 text-[#94a3b8]">
                          {new Date(profile.updated_at).toLocaleDateString()}
                        </span>
                      </div>
                      {profile.last_used_at && (
                        <div>
                          <span className="text-[#64748b]">{t('authProfiles.lastUsed')}:</span>
                          <span className="ml-2 text-[#94a3b8]">
                            {new Date(profile.last_used_at).toLocaleDateString()}
                          </span>
                        </div>
                      )}
                    </div>

                    {profile.last_error && (
                      <div className="p-3 bg-[#dc2626]/10 border border-[#dc2626]/30 rounded-lg">
                        <p className="text-sm text-[#f87171]">
                          <span className="font-medium">{t('authProfiles.lastError')}:</span>{' '}
                          {profile.last_error}
                        </p>
                      </div>
                    )}

                    {profile.mode === 'oauth' && (
                      <div className="flex gap-2">
                        <button
                          onClick={async () => {
                            try {
                              const response = await fetch(`/api/oauth/start/${profile.provider}`, {
                                method: 'POST',
                                headers: {
                                  'Content-Type': 'application/json',
                                  'Authorization': `Bearer ${localStorage.getItem('devclaw_token')}`,
                                },
                              })
                              const data = await response.json()

                              if (data.flow_type === 'manual') {
                                // Redirect to manual setup page
                                window.location.href = '/oauth/google/setup'
                              } else if (data.flow_type === 'pkce' && data.auth_url) {
                                // Open popup for PKCE flow
                                window.open(data.auth_url, 'oauth', 'width=600,height=800,scrollbars=yes')
                              } else if (data.flow_type === 'device_code') {
                                // Show device code instructions
                                alert(`Visit: ${data.verify_url}\nCode: ${data.user_code}`)
                              }
                            } catch (err) {
                              setError(err instanceof Error ? err.message : 'Failed to start OAuth')
                            }
                          }}
                          className="flex items-center gap-2 px-3 py-2 bg-[#3b82f6] hover:bg-[#2563eb] text-white text-sm rounded-lg transition-colors"
                        >
                          <ExternalLink className="h-4 w-4" />
                          {t('authProfiles.connectOAuth')}
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
