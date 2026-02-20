import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Webhook,
  Plus,
  Trash2,
  Loader2,
  CheckCircle2,
  XCircle,
  Power,
  PowerOff,
  Copy,
  Check,
  AlertTriangle,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { WebhookInfo } from '@/lib/api'

const inputClass =
  'flex h-11 w-full rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-4 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/10'

/**
 * Webhook management page.
 */
export function Webhooks() {
  const { t } = useTranslation()
  const [webhooks, setWebhooks] = useState<WebhookInfo[]>([])
  const [validEvents, setValidEvents] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  /* Create form */
  const [showForm, setShowForm] = useState(false)
  const [newUrl, setNewUrl] = useState('')
  const [selectedEvents, setSelectedEvents] = useState<string[]>([])
  const [creating, setCreating] = useState(false)

  /* Copied ID (for visual feedback) */
  const [copiedId, setCopiedId] = useState<string | null>(null)

  const loadWebhooks = useCallback(async () => {
    try {
      const data = await api.webhooks.list()
      setWebhooks(data.webhooks || [])
      setValidEvents(data.valid_events || [])
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    loadWebhooks()
  }, [loadWebhooks])

  /** Create a new webhook */
  const handleCreate = async () => {
    if (!newUrl.trim()) return
    setCreating(true)
    setMessage(null)
    try {
      await api.webhooks.create(newUrl.trim(), selectedEvents)
      setNewUrl('')
      setSelectedEvents([])
      setShowForm(false)
      setMessage({ type: 'success', text: t('common.success') })
      await loadWebhooks()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setCreating(false)
    }
  }

  /** Delete a webhook */
  const handleDelete = async (id: string) => {
    setMessage(null)
    try {
      await api.webhooks.delete(id)
      setMessage({ type: 'success', text: t('common.success') })
      await loadWebhooks()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Toggle webhook active status */
  const handleToggle = async (id: string, active: boolean) => {
    setMessage(null)
    try {
      await api.webhooks.toggle(id, active)
      await loadWebhooks()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Toggle event selection in form */
  const toggleEvent = (event: string) => {
    setSelectedEvents((prev) =>
      prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event]
    )
  }

  /** Copia ID do webhook para clipboard */
  const copyId = async (id: string) => {
    try {
      await navigator.clipboard.writeText(id)
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 2000)
    } catch {
      /* clipboard not available */
    }
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-10 w-10 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-dc-darker">
      <div className="mx-auto w-full max-w-3xl flex-1 overflow-y-auto px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">
              Integrações
            </p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Webhooks</h1>
            <p className="mt-2 text-base text-zinc-500">
              Receba notificações quando eventos acontecem no DevClaw
            </p>
          </div>
          <button
            onClick={() => setShowForm(!showForm)}
            className="flex cursor-pointer items-center gap-2 rounded-xl bg-linear-to-r from-blue-500 to-blue-500 px-5 py-3 text-sm font-bold text-white shadow-lg shadow-blue-500/20 transition-all hover:shadow-blue-500/30"
          >
            <Plus className="h-4 w-4" />
            Novo Webhook
          </button>
        </div>

        {/* Message */}
        {message && (
          <div
            className={`mt-6 rounded-2xl px-5 py-4 text-sm ring-1 ${
              message.type === 'success'
                ? 'bg-emerald-500/5 text-emerald-400 ring-emerald-500/20'
                : 'bg-red-500/5 text-red-400 ring-red-500/20'
            }`}
          >
            {message.text}
          </div>
        )}

        {/* Formulário de criação */}
        {showForm && (
          <div className="mt-6 rounded-2xl border border-white/6 bg-dc-dark/80 p-6">
            <h3 className="text-base font-bold text-white mb-5">Criar Webhook</h3>

            <div className="space-y-5">
              {/* URL */}
              <div>
                <label className="mb-2 block text-sm font-medium text-zinc-300">URL de destino</label>
                <input
                  value={newUrl}
                  onChange={(e) => setNewUrl(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
                  placeholder="https://example.com/webhooks/devclaw"
                  className={inputClass}
                />
                <p className="mt-1.5 text-xs text-zinc-500">
                  DevClaw will send a POST with the event's JSON payload
                </p>
              </div>

              {/* Eventos */}
              <div>
                <label className="mb-3 block text-sm font-medium text-zinc-300">Eventos</label>
                <div className="flex flex-wrap gap-2">
                  {validEvents.map((event) => {
                    const isSelected = selectedEvents.includes(event)
                    return (
                      <button
                        key={event}
                        onClick={() => toggleEvent(event)}
                        className={`cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-all ${
                          isSelected
                            ? 'bg-blue-500/20 text-blue-300 ring-1 ring-blue-500/30'
                            : 'bg-zinc-800/50 text-zinc-400 ring-1 ring-zinc-700/30 hover:ring-zinc-600 hover:text-zinc-300'
                        }`}
                      >
                        {event}
                      </button>
                    )
                  })}
                </div>
                {selectedEvents.length === 0 && (
                  <p className="mt-2 flex items-center gap-1.5 text-xs text-amber-400/70">
                    <AlertTriangle className="h-3 w-3" />
                    Nenhum evento selecionado — o webhook não receberá notificações
                  </p>
                )}
              </div>

              {/* Ações */}
              <div className="flex items-center justify-end gap-3 pt-2">
                <button
                  onClick={() => {
                    setShowForm(false)
                    setNewUrl('')
                    setSelectedEvents([])
                  }}
                  className="cursor-pointer rounded-xl px-4 py-2.5 text-sm font-medium text-zinc-400 transition-colors hover:text-white"
                >
                  Cancelar
                </button>
                <button
                  onClick={handleCreate}
                  disabled={creating || !newUrl.trim()}
                  className="flex cursor-pointer items-center gap-2 rounded-xl bg-blue-500 px-5 py-2.5 text-sm font-bold text-white transition-all hover:bg-blue-400 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {creating ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Webhook className="h-4 w-4" />
                  )}
                  {creating ? 'Criando...' : 'Criar'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Lista de webhooks */}
        <div className="mt-8">
          <div className="mb-5 flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-blue-500/10">
              <Webhook className="h-4.5 w-4.5 text-blue-400" />
            </div>
            <div>
              <h2 className="text-base font-bold text-white">Webhooks registrados</h2>
              <p className="text-xs text-zinc-500">
                {webhooks.length === 0
                  ? 'Nenhum webhook configurado'
                  : `${webhooks.length} webhook${webhooks.length > 1 ? 's' : ''}`}
              </p>
            </div>
          </div>

          {webhooks.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-zinc-700/50 bg-dc-dark/40 px-8 py-14 text-center">
              <Webhook className="mx-auto h-10 w-10 text-zinc-700" />
              <p className="mt-4 text-sm text-zinc-500">
                Webhooks permitem que sistemas externos sejam notificados sobre eventos do DevClaw.
              </p>
              <button
                onClick={() => setShowForm(true)}
                className="mt-4 cursor-pointer text-sm font-medium text-blue-400 hover:text-blue-300 transition-colors"
              >
                Criar primeiro webhook
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {webhooks.map((wh) => (
                <WebhookCard
                  key={wh.id}
                  webhook={wh}
                  copiedId={copiedId}
                  onToggle={handleToggle}
                  onDelete={handleDelete}
                  onCopyId={copyId}
                />
              ))}
            </div>
          )}
        </div>

        {/* Documentação rápida */}
        <div className="mt-10 mb-6 rounded-2xl border border-white/6 bg-dc-dark/60 p-6">
          <h3 className="text-sm font-bold text-zinc-300 mb-3">Eventos disponíveis</h3>
          <div className="grid grid-cols-2 gap-y-2 gap-x-4">
            {validEvents.map((event) => (
              <div key={event} className="flex items-center gap-2">
                <div className="h-1.5 w-1.5 rounded-full bg-blue-400/60" />
                <code className="text-xs font-mono text-zinc-400">{event}</code>
              </div>
            ))}
          </div>
          <p className="mt-4 text-xs text-zinc-500">
            Cada webhook recebe um POST com <code className="text-zinc-400">Content-Type: application/json</code>{' '}
            contendo <code className="text-zinc-400">{'{ "event": "...", "data": {...}, "timestamp": "..." }'}</code>
          </p>
        </div>
      </div>
    </div>
  )
}

/* ── Componente de Card de Webhook ── */

function WebhookCard({
  webhook,
  copiedId,
  onToggle,
  onDelete,
  onCopyId,
}: {
  webhook: WebhookInfo
  copiedId: string | null
  onToggle: (id: string, active: boolean) => void
  onDelete: (id: string) => void
  onCopyId: (id: string) => void
}) {
  const [confirming, setConfirming] = useState(false)

  return (
    <div
      className={`rounded-2xl border bg-dc-dark/80 p-5 transition-all ${
        webhook.active
          ? 'border-white/6'
          : 'border-zinc-800/50 opacity-60'
      }`}
    >
      {/* Linha superior: status + ações */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1.5">
            {webhook.active ? (
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-emerald-400" />
            ) : (
              <XCircle className="h-3.5 w-3.5 shrink-0 text-zinc-600" />
            )}
            <span className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">
              {webhook.active ? 'Ativo' : 'Inativo'}
            </span>
          </div>
          <p className="truncate font-mono text-sm text-zinc-200" title={webhook.url}>
            {webhook.url}
          </p>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* Toggle ativo/inativo */}
          <button
            onClick={() => onToggle(webhook.id, !webhook.active)}
            title={webhook.active ? 'Desativar' : 'Ativar'}
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-300"
          >
            {webhook.active ? (
              <PowerOff className="h-4 w-4" />
            ) : (
              <Power className="h-4 w-4" />
            )}
          </button>

          {/* Copiar ID */}
          <button
            onClick={() => onCopyId(webhook.id)}
            title="Copiar ID"
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-300"
          >
            {copiedId === webhook.id ? (
              <Check className="h-4 w-4 text-emerald-400" />
            ) : (
              <Copy className="h-4 w-4" />
            )}
          </button>

          {/* Excluir */}
          {confirming ? (
            <button
              onClick={() => {
                onDelete(webhook.id)
                setConfirming(false)
              }}
              className="flex h-8 cursor-pointer items-center gap-1 rounded-lg bg-red-500/10 px-2 text-xs font-medium text-red-400 transition-colors hover:bg-red-500/20"
            >
              Confirmar
            </button>
          ) : (
            <button
              onClick={() => {
                setConfirming(true)
                setTimeout(() => setConfirming(false), 3000)
              }}
              title="Excluir"
              className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-red-500/10 hover:text-red-400"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {/* Eventos */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {webhook.events && webhook.events.length > 0 ? (
          webhook.events.map((event) => (
            <span
              key={event}
              className="rounded-md bg-zinc-800/80 px-2 py-0.5 text-[11px] font-mono text-zinc-400 ring-1 ring-zinc-700/30"
            >
              {event}
            </span>
          ))
        ) : (
          <span className="text-[11px] text-zinc-600 italic">Sem eventos configurados</span>
        )}
      </div>

      {/* Meta: ID + data criação */}
      <div className="mt-3 flex items-center gap-4 text-[11px] text-zinc-600">
        <span>ID: {webhook.id}</span>
        <span>
          Created on{' '}
          {new Date(webhook.created_at).toLocaleDateString('en-US', {
            day: '2-digit',
            month: 'short',
            year: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
          })}
        </span>
      </div>
    </div>
  )
}
