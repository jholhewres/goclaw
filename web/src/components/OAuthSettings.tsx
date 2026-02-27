import { useState, useEffect, useCallback } from 'react'
import { api, type OAuthProvider, type OAuthStatus, type OAuthStartResponse } from '../lib/api'

export function OAuthSettings() {
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [statuses, setStatuses] = useState<Record<string, OAuthStatus>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activeFlow, setActiveFlow] = useState<OAuthStartResponse | null>(null)
  const [pollingProvider, setPollingProvider] = useState<string | null>(null)

  const loadProviders = useCallback(async () => {
    try {
      const data = await api.oauth.providers()
      setProviders(data)
    } catch (err) {
      console.error('Failed to load providers:', err)
    }
  }, [])

  const loadStatus = useCallback(async () => {
    try {
      setLoading(true)
      const data = await api.oauth.status()
      setStatuses(data)
      setError(null)
    } catch (err) {
      setError('Failed to load OAuth status')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadProviders()
    loadStatus()
  }, [loadProviders, loadStatus])

  // Handle PKCE flow popup
  useEffect(() => {
    if (activeFlow?.flow_type === 'pkce' && activeFlow.auth_url) {
      const popup = window.open(
        activeFlow.auth_url,
        'oauth',
        'width=600,height=800,scrollbars=yes'
      )

      const handleMessage = (event: MessageEvent) => {
        if (event.data?.type === 'oauth-success') {
          setActiveFlow(null)
          loadStatus()
        }
      }

      window.addEventListener('message', handleMessage)

      const pollClosed = setInterval(() => {
        if (popup?.closed) {
          clearInterval(pollClosed)
          window.removeEventListener('message', handleMessage)
          setActiveFlow(null)
          loadStatus()
        }
      }, 500)

      return () => {
        clearInterval(pollClosed)
        window.removeEventListener('message', handleMessage)
      }
    }
  }, [activeFlow, loadStatus])

  // Handle device code polling
  useEffect(() => {
    if (!pollingProvider) return

    const pollInterval = setInterval(async () => {
      try {
        const result = await api.oauth.poll(pollingProvider)
        if (result.status === 'completed') {
          setPollingProvider(null)
          setActiveFlow(null)
          loadStatus()
        } else if (result.status === 'error') {
          setError(result.message || 'Authentication failed')
          setPollingProvider(null)
          setActiveFlow(null)
        }
      } catch (err) {
        console.error('Poll error:', err)
      }
    }, 5000)

    return () => clearInterval(pollInterval)
  }, [pollingProvider, loadStatus])

  const startLogin = async (providerId: string) => {
    try {
      setError(null)
      const data = await api.oauth.start(providerId)
      setActiveFlow(data)

      if (data.experimental) {
        alert(
          'EXPERIMENTAL FEATURE\n\n' +
          'This OAuth provider uses unofficial endpoints and may stop working at any time.\n\n' +
          'Use at your own risk.'
        )
      }

      if (data.flow_type === 'device_code') {
        setPollingProvider(providerId)
      } else if (data.flow_type === 'manual') {
        // Redirect to manual setup page
        window.location.href = '/oauth/google/setup'
      }
    } catch (err) {
      setError(`Failed to start OAuth login for ${providerId}`)
      console.error(err)
    }
  }

  const logout = async (providerId: string) => {
    if (!confirm(`Logout from ${providerId}?`)) return

    try {
      await api.oauth.logout(providerId)
      loadStatus()
    } catch (err) {
      setError(`Failed to logout from ${providerId}`)
      console.error(err)
    }
  }

  const refresh = async (providerId: string) => {
    try {
      await api.oauth.refresh(providerId)
      loadStatus()
    } catch (err) {
      setError(`Failed to refresh token for ${providerId}`)
      console.error(err)
    }
  }

  const formatExpiresIn = (seconds?: number): string => {
    if (!seconds) return ''
    const hours = Math.floor(seconds / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    if (hours > 0) {
      return `${hours}h ${minutes}m`
    }
    return `${minutes}m`
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'valid':
        return <span className="px-2 py-1 text-xs rounded bg-green-100 text-green-800">Valid</span>
      case 'expiring_soon':
        return <span className="px-2 py-1 text-xs rounded bg-yellow-100 text-yellow-800">Expiring Soon</span>
      case 'expired':
        return <span className="px-2 py-1 text-xs rounded bg-red-100 text-red-800">Expired</span>
      default:
        return <span className="px-2 py-1 text-xs rounded bg-gray-100 text-gray-800">Unknown</span>
    }
  }

  if (loading) {
    return <div className="p-4">Loading OAuth settings...</div>
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold mb-4">OAuth Providers</h2>
        <p className="text-gray-600 mb-4">
          Connect your LLM subscriptions to use them instead of API keys.
        </p>
      </div>

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 rounded text-red-700">
          {error}
        </div>
      )}

      {/* Active Device Code Flow */}
      {activeFlow?.flow_type === 'device_code' && (
        <div className="p-4 bg-blue-50 border border-blue-200 rounded">
          <h3 className="font-medium mb-2">Complete Authentication</h3>
          <p className="text-sm text-gray-600 mb-3">
            Visit the URL below and enter the code to authenticate.
          </p>
          <div className="bg-white p-3 rounded border">
            <p className="text-sm text-gray-500">URL:</p>
            <a
              href={activeFlow.verify_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-600 hover:underline break-all"
            >
              {activeFlow.verify_url}
            </a>
            <p className="text-sm text-gray-500 mt-2">Code:</p>
            <p className="font-mono text-lg font-bold">{activeFlow.user_code}</p>
          </div>
          <button
            onClick={() => {
              setActiveFlow(null)
              setPollingProvider(null)
            }}
            className="mt-3 px-4 py-2 text-sm text-gray-600 hover:text-gray-800"
          >
            Cancel
          </button>
        </div>
      )}

      {/* Providers List */}
      <div className="space-y-3">
        {providers.map((provider) => {
          const status = statuses[provider.id]
          const isActive = activeFlow?.provider === provider.id

          return (
            <div
              key={provider.id}
              className="p-4 border rounded-lg flex items-center justify-between"
            >
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="font-medium">{provider.label}</h3>
                  {provider.experimental && (
                    <span className="px-2 py-0.5 text-xs rounded bg-orange-100 text-orange-800">
                      Experimental
                    </span>
                  )}
                </div>
                {status && (
                  <div className="flex items-center gap-2 mt-1 text-sm text-gray-500">
                    {getStatusBadge(status.status)}
                    {status.email && <span>- {status.email}</span>}
                    {status.expires_in && status.expires_in > 0 && (
                      <span>- Expires in {formatExpiresIn(status.expires_in)}</span>
                    )}
                  </div>
                )}
              </div>

              <div className="flex items-center gap-2">
                {status?.has_token ? (
                  <>
                    <button
                      onClick={() => refresh(provider.id)}
                      className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
                    >
                      Refresh
                    </button>
                    <button
                      onClick={() => logout(provider.id)}
                      className="px-3 py-1.5 text-sm text-red-600 border border-red-200 rounded hover:bg-red-50"
                    >
                      Logout
                    </button>
                  </>
                ) : (
                  <button
                    onClick={() => startLogin(provider.id)}
                    disabled={isActive}
                    className="px-4 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
                  >
                    {isActive ? 'Waiting...' : 'Login'}
                  </button>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* Warning for experimental providers */}
      <div className="p-4 bg-yellow-50 border border-yellow-200 rounded">
        <h4 className="font-medium text-yellow-800">Experimental Providers</h4>
        <p className="text-sm text-yellow-700 mt-1">
          ChatGPT OAuth uses unofficial endpoints and may stop working at any time.
          OpenAI may block this approach like Anthropic did with Claude.
          Consider using Gemini as a more stable alternative.
        </p>
      </div>
    </div>
  )
}

export default OAuthSettings
