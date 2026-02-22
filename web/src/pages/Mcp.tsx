import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Plus,
  Trash2,
  Play,
  Square,
  RefreshCw,
  Server,
  Cable,
  AlertTriangle,
} from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigToggle,
  ConfigCard,
  ConfigEmptyState,
  ConfigInfoBox,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface MCPServer {
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  enabled: boolean
  status?: string
  error?: string
}

export function Mcp() {
  const { t } = useTranslation()
  const [servers, setServers] = useState<MCPServer[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [showAddForm, setShowAddForm] = useState(false)
  const [newServer, setNewServer] = useState<Partial<MCPServer>>({
    name: '',
    command: '',
    args: [],
    env: {},
    enabled: true,
  })

  useEffect(() => {
    loadServers()
  }, [])

  const loadServers = async () => {
    setLoading(true)
    setLoadError(null)
    try {
      const data = await api.mcp.list()
      setServers(data.servers || [])
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load MCP servers')
    } finally {
      setLoading(false)
    }
  }

  const handleToggleServer = async (name: string, enabled: boolean) => {
    try {
      await api.mcp.update(name, enabled)
      setServers(prev => prev.map(s => s.name === name ? { ...s, enabled } : s))
      setMessage({ type: 'success', text: enabled ? t('mcp.serverEnabled') : t('mcp.serverDisabled') })
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') })
    }
  }

  const handleStartServer = async (name: string) => {
    try {
      await api.mcp.start(name)
      setServers(prev => prev.map(s => s.name === name ? { ...s, status: 'running', error: undefined } : s))
      setMessage({ type: 'success', text: t('mcp.serverStarted') })
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') })
    }
  }

  const handleStopServer = async (name: string) => {
    try {
      await api.mcp.stop(name)
      setServers(prev => prev.map(s => s.name === name ? { ...s, status: 'stopped' } : s))
      setMessage({ type: 'success', text: t('mcp.serverStopped') })
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') })
    }
  }

  const handleDeleteServer = async (name: string) => {
    try {
      await api.mcp.delete(name)
      setServers(prev => prev.filter(s => s.name !== name))
      setMessage({ type: 'success', text: t('mcp.serverDeleted') })
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') })
    }
  }

  const handleAddServer = async () => {
    if (!newServer.name || !newServer.command) {
      setMessage({ type: 'error', text: t('mcp.nameRequired') })
      return
    }

    try {
      await api.mcp.create(
        newServer.name,
        newServer.command,
        newServer.args || [],
        newServer.env || {}
      )
      const server: MCPServer = {
        name: newServer.name,
        command: newServer.command,
        args: newServer.args || [],
        env: newServer.env || {},
        enabled: newServer.enabled ?? true,
        status: 'stopped',
      }
      setServers(prev => [...prev, server])
      setNewServer({ name: '', command: '', args: [], env: {}, enabled: true })
      setShowAddForm(false)
      setMessage({ type: 'success', text: t('mcp.serverAdded') })
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') })
    }
  }

  if (loading) return <LoadingSpinner />
  if (loadError) return <ErrorState message={loadError} onRetry={loadServers} />

  return (
    <ConfigPage
      title={t('mcp.title')}
      subtitle={t('mcp.subtitle')}
      description={t('mcp.description')}
      message={message}
      actions={
        <div className="flex items-center gap-3">
          <button
            onClick={() => loadServers()}
            className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-[#111827] px-4 py-3 text-sm font-medium text-[#94a3b8] transition-all hover:border-white/20 hover:text-[#f8fafc]"
          >
            <RefreshCw className="h-4 w-4" />
            {t('mcp.refresh')}
          </button>
          <button
            onClick={() => setShowAddForm(!showAddForm)}
            className="flex cursor-pointer items-center gap-2 rounded-xl bg-[#3b82f6] px-5 py-3 text-sm font-semibold text-white transition-all hover:bg-[#2563eb]"
          >
            <Plus className="h-4 w-4" />
            {t('mcp.addServer')}
          </button>
        </div>
      }
    >
      {/* Add Server Form */}
      {showAddForm && (
        <ConfigSection title={t('mcp.newServer')}>
          <ConfigField label={t('mcp.serverName')}>
            <ConfigInput
              value={newServer.name || ''}
              onChange={(v) => setNewServer(prev => ({ ...prev, name: v }))}
              placeholder="my-server"
            />
          </ConfigField>

          <ConfigField label={t('mcp.command')} hint={t('mcp.commandHint')}>
            <ConfigInput
              value={newServer.command || ''}
              onChange={(v) => setNewServer(prev => ({ ...prev, command: v }))}
              placeholder="mcp-server-command"
            />
          </ConfigField>

          <div className="flex gap-3 pt-2">
            <button
              onClick={handleAddServer}
              className="flex cursor-pointer items-center gap-2 rounded-xl bg-[#22c55e] px-4 py-2.5 text-sm font-semibold text-white transition-all hover:bg-[#16a34a]"
            >
              <Plus className="h-4 w-4" />
              {t('mcp.createServer')}
            </button>
            <button
              onClick={() => setShowAddForm(false)}
              className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-[#111827] px-4 py-2.5 text-sm font-medium text-[#94a3b8] transition-all hover:border-white/20 hover:text-[#f8fafc]"
            >
              {t('common.cancel')}
            </button>
          </div>
        </ConfigSection>
      )}

      {/* Servers List */}
      <ConfigSection
        icon={Cable}
        title={t('mcp.serversSection')}
        description={t('mcp.serversSectionDesc')}
      >
        {servers.length === 0 ? (
          <ConfigEmptyState
            icon={Server}
            title={t('mcp.noServers')}
            description={t('mcp.noServersHint')}
          />
        ) : (
          <div className="space-y-4">
            {servers.map((server) => (
              <ConfigCard
                key={server.name}
                title={server.name}
                subtitle={server.command}
                icon={Server}
                status={
                  server.status === 'running'
                    ? 'success'
                    : server.status === 'error'
                    ? 'error'
                    : 'neutral'
                }
                actions={
                  <div className="flex items-center gap-2">
                    {server.status === 'running' ? (
                      <button
                        onClick={() => handleStopServer(server.name)}
                        className="flex cursor-pointer items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#ef4444]/10 text-[#f87171] text-xs font-medium hover:bg-[#ef4444]/20 transition-all"
                      >
                        <Square className="h-3.5 w-3.5" />
                        {t('mcp.stop')}
                      </button>
                    ) : (
                      <button
                        onClick={() => handleStartServer(server.name)}
                        className="flex cursor-pointer items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#22c55e]/10 text-[#22c55e] text-xs font-medium hover:bg-[#22c55e]/20 transition-all"
                      >
                        <Play className="h-3.5 w-3.5" />
                        {t('mcp.start')}
                      </button>
                    )}
                    <button
                      onClick={() => handleDeleteServer(server.name)}
                      className="flex cursor-pointer items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#1e293b] text-[#94a3b8] text-xs font-medium hover:text-[#f87171] transition-all"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t('common.delete')}
                    </button>
                  </div>
                }
              >
                {server.error && (
                  <div className="mb-4 flex items-start gap-2 rounded-lg bg-[#ef4444]/5 border border-[#ef4444]/10 p-3 -mt-2">
                    <AlertTriangle className="h-4 w-4 text-[#f87171] flex-shrink-0 mt-0.5" />
                    <p className="text-xs text-[#f87171]">{server.error}</p>
                  </div>
                )}
                <ConfigToggle
                  enabled={server.enabled}
                  onChange={(v) => handleToggleServer(server.name, v)}
                  label={t('mcp.enabled')}
                />
              </ConfigCard>
            ))}
          </div>
        )}
      </ConfigSection>

      {/* Info */}
      <ConfigInfoBox
        title={t('mcp.info')}
        items={[
          t('mcp.info1'),
          t('mcp.info2'),
          t('mcp.info3'),
        ]}
      />
    </ConfigPage>
  )
}
