import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Zap,
  Power,
  PowerOff,
  Trash2,
  CheckCircle2,
  XCircle,
  ChevronDown,
  ChevronRight,
  Info,
  Filter,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { HookInfo, HookEventInfo } from '@/lib/api'

/**
 * Lifecycle hooks management page.
 */
export function Hooks() {
  const { t } = useTranslation()
  const [hooks, setHooks] = useState<HookInfo[]>([])
  const [events, setEvents] = useState<HookEventInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [view, setView] = useState<'hooks' | 'events'>('hooks')
  const [filterEvent, setFilterEvent] = useState<string>('')

  const loadData = useCallback(async () => {
    try {
      const data = await api.hooks.list()
      setHooks(data.hooks || [])
      setEvents(data.events || [])
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    loadData()
  }, [loadData])

  /** Toggle hook enabled status */
  const handleToggle = async (name: string, enabled: boolean) => {
    setMessage(null)
    try {
      await api.hooks.toggle(name, enabled)
      await loadData()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Remove a hook */
  const handleDelete = async (name: string) => {
    setMessage(null)
    try {
      await api.hooks.unregister(name)
      setMessage({ type: 'success', text: t('common.success') })
      await loadData()
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    }
  }

  /** Hooks filtered by selected event */
  const filteredHooks = filterEvent
    ? hooks.filter((h) => h.events.includes(filterEvent))
    : hooks

  /** Total active hooks count */
  const activeCount = hooks.filter((h) => h.enabled).length

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
              {t('hooks.subtitle')}
            </p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">
              {t('hooks.title')}
            </h1>
            <p className="mt-2 text-base text-zinc-500">
              {hooks.length} · {activeCount} {t('common.enabled').toLowerCase()}
            </p>
          </div>

          {/* View toggle */}
          <div className="flex items-center gap-1 rounded-xl border border-zinc-700/50 bg-zinc-800/50 p-1">
            <button
              onClick={() => setView('hooks')}
              className={`cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-all ${
                view === 'hooks'
                  ? 'bg-blue-500/20 text-blue-300'
                  : 'text-zinc-400 hover:text-zinc-200'
              }`}
            >
              Hooks
            </button>
            <button
              onClick={() => setView('events')}
              className={`cursor-pointer rounded-lg px-3 py-1.5 text-xs font-medium transition-all ${
                view === 'events'
                  ? 'bg-blue-500/20 text-blue-300'
                  : 'text-zinc-400 hover:text-zinc-200'
              }`}
            >
              Eventos
            </button>
          </div>
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

        {view === 'hooks' ? (
          <>
            {/* Filtro por evento */}
            {hooks.length > 0 && (
              <div className="mt-6 flex items-center gap-3">
                <Filter className="h-4 w-4 text-zinc-500" />
                <select
                  value={filterEvent}
                  onChange={(e) => setFilterEvent(e.target.value)}
                  aria-label="Filtrar por evento"
                  className="h-9 cursor-pointer rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 text-xs text-zinc-300 outline-none transition-all focus:border-blue-500/50"
                >
                  <option value="">Todos os eventos</option>
                  {events
                    .filter((ev) => ev.hooks.length > 0)
                    .map((ev) => (
                      <option key={ev.event} value={ev.event}>
                        {ev.event} ({ev.hooks.length})
                      </option>
                    ))}
                </select>
                {filterEvent && (
                  <button
                    onClick={() => setFilterEvent('')}
                    className="cursor-pointer text-xs text-blue-400 hover:text-blue-300 transition-colors"
                  >
                    Limpar filtro
                  </button>
                )}
              </div>
            )}

            {/* Lista de hooks */}
            <div className="mt-6">
              <div className="mb-5 flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-blue-500/10">
                  <Zap className="h-4.5 w-4.5 text-blue-400" />
                </div>
                <div>
                  <h2 className="text-base font-bold text-white">Hooks registrados</h2>
                  <p className="text-xs text-zinc-500">
                    {filteredHooks.length === 0
                      ? 'Nenhum hook encontrado'
                      : `${filteredHooks.length} hook${filteredHooks.length > 1 ? 's' : ''}`}
                  </p>
                </div>
              </div>

              {filteredHooks.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-zinc-700/50 bg-dc-dark/40 px-8 py-14 text-center">
                  <Zap className="mx-auto h-10 w-10 text-zinc-700" />
                  <p className="mt-4 text-sm text-zinc-500">
                    {filterEvent
                      ? `Nenhum hook registrado para o evento "${filterEvent}"`
                      : 'Nenhum hook registrado. Hooks são adicionados por plugins, skills e pelo sistema.'}
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {filteredHooks.map((hook) => (
                    <HookCard
                      key={hook.name}
                      hook={hook}
                      onToggle={handleToggle}
                      onDelete={handleDelete}
                    />
                  ))}
                </div>
              )}
            </div>
          </>
        ) : (
          /* Vista de eventos */
          <div className="mt-6 space-y-3">
            {events.map((ev) => (
              <EventCard
                key={ev.event}
                event={ev}
                onFilterByEvent={(event) => {
                  setFilterEvent(event)
                  setView('hooks')
                }}
              />
            ))}
          </div>
        )}

        {/* Info card */}
        <div className="mt-10 mb-6 rounded-2xl border border-white/6 bg-dc-dark/60 p-6">
          <h3 className="text-sm font-bold text-zinc-300 mb-3">Sobre Hooks</h3>
          <p className="text-xs text-zinc-500 leading-relaxed">
            Hooks permitem que plugins, skills e o sistema observem e modifiquem o comportamento
            do agente em pontos específicos do ciclo de vida. Hooks com menor prioridade
            executam primeiro. Para eventos bloqueantes (<code className="text-zinc-400">pre_tool_use</code>,{' '}
            <code className="text-zinc-400">user_prompt_submit</code>), o primeiro hook que
            bloquear impede a operação.
          </p>
          <div className="mt-3 flex items-start gap-2 rounded-lg bg-blue-500/5 px-3 py-2 ring-1 ring-blue-500/10">
            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-400" />
            <p className="text-xs text-amber-400/80">
              Desativar hooks do sistema pode afetar funcionalidades essenciais. Use com cautela.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

/* ── Componente de Card de Hook ── */

function HookCard({
  hook,
  onToggle,
  onDelete,
}: {
  hook: HookInfo
  onToggle: (name: string, enabled: boolean) => void
  onDelete: (name: string) => void
}) {
  const [confirming, setConfirming] = useState(false)

  const sourceLabel = (source: string) => {
    if (!source || source === 'system') return 'Sistema'
    if (source.startsWith('plugin:')) return `Plugin: ${source.slice(7)}`
    if (source.startsWith('skill:')) return `Skill: ${source.slice(6)}`
    return source
  }

  const sourceColor = (source: string) => {
    if (!source || source === 'system') return 'text-blue-400 bg-blue-500/10 ring-blue-500/20'
    if (source.startsWith('plugin:')) return 'text-purple-400 bg-purple-500/10 ring-purple-500/20'
    if (source.startsWith('skill:')) return 'text-emerald-400 bg-emerald-500/10 ring-emerald-500/20'
    return 'text-zinc-400 bg-zinc-500/10 ring-zinc-500/20'
  }

  return (
    <div
      className={`rounded-2xl border bg-dc-dark/80 p-5 transition-all ${
        hook.enabled ? 'border-white/6' : 'border-zinc-800/50 opacity-60'
      }`}
    >
      {/* Linha superior */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2.5 mb-1.5">
            {hook.enabled ? (
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-emerald-400" />
            ) : (
              <XCircle className="h-3.5 w-3.5 shrink-0 text-zinc-600" />
            )}
            <span className="text-sm font-semibold text-white truncate">{hook.name}</span>
            <span
              className={`shrink-0 rounded-md px-2 py-0.5 text-[10px] font-medium ring-1 ${sourceColor(
                hook.source
              )}`}
            >
              {sourceLabel(hook.source)}
            </span>
          </div>

          {hook.description && (
            <p className="text-xs text-zinc-400 mt-1">{hook.description}</p>
          )}
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* Prioridade */}
          <span
            className="flex h-8 items-center rounded-lg px-2 text-[11px] font-mono text-zinc-500"
            title="Prioridade (menor = executa primeiro)"
          >
            P{hook.priority}
          </span>

          {/* Toggle */}
          <button
            onClick={() => onToggle(hook.name, !hook.enabled)}
            title={hook.enabled ? 'Desativar' : 'Ativar'}
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-300"
          >
            {hook.enabled ? (
              <PowerOff className="h-4 w-4" />
            ) : (
              <Power className="h-4 w-4" />
            )}
          </button>

          {/* Excluir */}
          {confirming ? (
            <button
              onClick={() => {
                onDelete(hook.name)
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
              title="Remover"
              className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-red-500/10 hover:text-red-400"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {/* Eventos */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {hook.events.map((event) => (
          <span
            key={event}
            className="rounded-md bg-zinc-800/80 px-2 py-0.5 text-[11px] font-mono text-zinc-400 ring-1 ring-zinc-700/30"
          >
            {event}
          </span>
        ))}
      </div>
    </div>
  )
}

/* ── Componente de Card de Evento ── */

function EventCard({
  event,
  onFilterByEvent,
}: {
  event: HookEventInfo
  onFilterByEvent: (event: string) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const hasHooks = event.hooks.length > 0

  return (
    <div className="rounded-2xl border border-white/6 bg-dc-dark/80 transition-all">
      <button
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
        className="flex w-full cursor-pointer items-center justify-between px-5 py-4"
      >
        <div className="flex items-center gap-3">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-zinc-500" />
          ) : (
            <ChevronRight className="h-4 w-4 text-zinc-500" />
          )}
          <div className="text-left">
            <code className="text-sm font-semibold text-white">{event.event}</code>
            <p className="text-xs text-zinc-500 mt-0.5">{event.description}</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {hasHooks ? (
            <span className="rounded-full bg-blue-500/10 px-2.5 py-0.5 text-[11px] font-medium text-blue-400">
              {event.hooks.length} hook{event.hooks.length > 1 ? 's' : ''}
            </span>
          ) : (
            <span className="text-[11px] text-zinc-600">sem hooks</span>
          )}
        </div>
      </button>

      {expanded && hasHooks && (
        <div className="border-t border-zinc-800/50 px-5 py-3">
          <div className="space-y-1.5">
            {event.hooks.map((hookName) => (
              <div
                key={hookName}
                className="flex items-center justify-between rounded-lg bg-zinc-800/30 px-3 py-2"
              >
                <span className="text-xs font-mono text-zinc-300">{hookName}</span>
                <button
                  onClick={() => onFilterByEvent(event.event)}
                  className="cursor-pointer text-[11px] text-blue-400/60 hover:text-blue-400 transition-colors"
                >
                  Ver na lista
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
