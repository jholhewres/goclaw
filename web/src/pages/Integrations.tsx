import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Key, Check, X, ExternalLink, Plug, PlugZap } from 'lucide-react'

interface OAuthProvider {
  id: string
  name: string
  icon: string
  description: string
  connected: boolean
  scopes?: string[]
  connectedAt?: string
  account?: string
}

const MOCK_PROVIDERS: OAuthProvider[] = [
  {
    id: 'google',
    name: 'Google',
    icon: '🔵',
    description: 'Acesso ao Google Calendar, Gmail, Drive e outros serviços',
    connected: false,
    scopes: ['calendar', 'gmail', 'drive'],
  },
  {
    id: 'github',
    name: 'GitHub',
    icon: '⚫',
    description: 'Acesso a repositórios, issues e pull requests',
    connected: true,
    connectedAt: '2026-01-15T10:30:00Z',
    account: 'user@example.com',
    scopes: ['repo', 'read:org'],
  },
  {
    id: 'microsoft',
    name: 'Microsoft',
    icon: '🟦',
    description: 'Acesso ao Teams, Outlook, OneDrive e Office 365',
    connected: false,
    scopes: ['mail', 'calendar', 'files'],
  },
  {
    id: 'slack',
    name: 'Slack',
    icon: '🟣',
    description: 'Acesso a workspaces, canais e mensagens',
    connected: false,
    scopes: ['channels:read', 'chat:write'],
  },
  {
    id: 'discord',
    name: 'Discord',
    icon: '🟢',
    description: 'Acesso a servidores e canais do Discord',
    connected: false,
    scopes: ['guilds', 'messages'],
  },
  {
    id: 'notion',
    name: 'Notion',
    icon: '⬛',
    description: 'Acesso a páginas, databases e conteúdo do Notion',
    connected: false,
    scopes: ['read', 'write'],
  },
]

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  })
}

export function Integrations() {
  const { t } = useTranslation()
  const [providers, setProviders] = useState<OAuthProvider[]>(MOCK_PROVIDERS)
  const [connecting, setConnecting] = useState<string | null>(null)

  const handleConnect = async (providerId: string) => {
    setConnecting(providerId)
    // Simulate OAuth connection
    await new Promise((resolve) => setTimeout(resolve, 1500))
    setProviders((prev) =>
      prev.map((p) =>
        p.id === providerId
          ? {
              ...p,
              connected: true,
              connectedAt: new Date().toISOString(),
              account: 'user@example.com',
            }
          : p
      )
    )
    setConnecting(null)
  }

  const handleDisconnect = async (providerId: string) => {
    setProviders((prev) =>
      prev.map((p) =>
        p.id === providerId
          ? { ...p, connected: false, connectedAt: undefined, account: undefined }
          : p
      )
    )
  }

  const connectedCount = providers.filter((p) => p.connected).length

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-dc-darker">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('integrations.subtitle')}</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('integrations.title')}</h1>
            <p className="mt-2 text-base text-zinc-500">
              {connectedCount} {t('integrations.servicesConnected', { count: providers.length })}
            </p>
          </div>
          <div className="flex items-center gap-2 rounded-xl bg-zinc-800/40 px-4 py-2.5 ring-1 ring-zinc-700/20">
            <PlugZap className="h-4 w-4 text-blue-400" />
            <span className="text-xs font-semibold text-zinc-300">OAuth 2.0</span>
          </div>
        </div>

        {/* Connected Section */}
        {connectedCount > 0 && (
          <section className="mt-8">
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600 mb-4">{t('integrations.connected')}</p>
            <div className="space-y-3">
              {providers
                .filter((p) => p.connected)
                .map((provider) => (
                  <ProviderCard
                    key={provider.id}
                    provider={provider}
                    onDisconnect={() => handleDisconnect(provider.id)}
                  />
                ))}
            </div>
          </section>
        )}

        {/* Available Section */}
        <section className="mt-8">
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600 mb-4">
            {connectedCount > 0 ? t('integrations.available') : t('integrations.services')}
          </p>
          <div className="space-y-3">
            {providers
              .filter((p) => !p.connected)
              .map((provider) => (
                <ProviderCard
                  key={provider.id}
                  provider={provider}
                  connecting={connecting === provider.id}
                  onConnect={() => handleConnect(provider.id)}
                />
              ))}
          </div>
        </section>

        {/* Info */}
        <section className="mt-10 mb-10">
          <div className="rounded-2xl border border-white/4 bg-dc-dark/50 p-6">
            <h3 className="text-sm font-bold text-zinc-400 mb-3">{t('integrations.about')}</h3>
            <div className="space-y-3 text-xs text-zinc-500">
              <div className="flex items-start gap-2">
                <Key className="h-4 w-4 mt-0.5 text-zinc-600" />
                <p>{t('integrations.aboutText1')}</p>
              </div>
              <div className="flex items-start gap-2">
                <ExternalLink className="h-4 w-4 mt-0.5 text-zinc-600" />
                <p>{t('integrations.aboutText2')}</p>
              </div>
              <div className="flex items-start gap-2">
                <Plug className="h-4 w-4 mt-0.5 text-zinc-600" />
                <p>{t('integrations.aboutText3')}</p>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}

function ProviderCard({
  provider,
  connecting,
  onConnect,
  onDisconnect,
}: {
  provider: OAuthProvider
  connecting?: boolean
  onConnect?: () => void
  onDisconnect?: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center gap-4 rounded-2xl border border-white/6 bg-dc-dark p-5 transition-all hover:border-white/10">
      <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-white/5 text-2xl">
        {provider.icon}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <h3 className="text-base font-bold text-white">{provider.name}</h3>
          {provider.connected && (
            <span className="flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold text-emerald-400">
              <Check className="h-3 w-3" />
              {t('integrations.connected')}
            </span>
          )}
        </div>
        <p className="mt-1 text-sm text-zinc-500 truncate">{provider.description}</p>
        {provider.connected && provider.connectedAt && (
          <p className="mt-1 text-xs text-zinc-600">
            {t('integrations.connectedOn')} {formatDate(provider.connectedAt)} · {provider.account}
          </p>
        )}
      </div>
      <div className="shrink-0">
        {provider.connected ? (
          <button
            onClick={onDisconnect}
            className="flex cursor-pointer items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-4 py-2.5 text-sm font-semibold text-red-400 transition-all hover:bg-red-500/10"
          >
            <X className="h-4 w-4" />
            {t('common.disconnect')}
          </button>
        ) : (
          <button
            onClick={onConnect}
            disabled={connecting}
            className="flex cursor-pointer items-center gap-2 rounded-xl bg-blue-500 px-4 py-2.5 text-sm font-bold text-white shadow-lg shadow-blue-500/20 transition-all hover:shadow-blue-500/30 disabled:opacity-50"
          >
            {connecting ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                {t('common.connecting')}
              </>
            ) : (
              <>
                <Plug className="h-4 w-4" />
                {t('common.connect')}
              </>
            )}
          </button>
        )}
      </div>
    </div>
  )
}
