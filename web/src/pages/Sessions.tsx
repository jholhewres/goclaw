import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Search, MessageSquare, Trash2, MessageCircle } from 'lucide-react'
import { api, type SessionInfo } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

export function Sessions() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState<string | null>(null)

  useEffect(() => {
    api.sessions.list()
      .then(setSessions)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const filtered = sessions.filter(
    (s) =>
      s.id.toLowerCase().includes(search.toLowerCase()) ||
      s.channel.toLowerCase().includes(search.toLowerCase()) ||
      (s.chat_id && s.chat_id.toLowerCase().includes(search.toLowerCase())),
  )

  const handleDelete = async (id: string) => {
    try {
      await api.sessions.delete(id)
      setSessions((prev) => prev.filter((s) => s.id !== id))
    } catch {
      /* silent — session may already be gone */
    } finally {
      setConfirmingDelete(null)
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
    <div className="flex-1 overflow-y-auto bg-dc-darker">
      <div className="mx-auto max-w-5xl px-8 py-10">
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('sidebar.sessions')}</p>
        <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('sessions.title')}</h1>
        <p className="mt-2 text-base text-zinc-500">{sessions.length} {t('sessions.subtitle')}</p>

        {loadError && (
          <div className="mt-4 rounded-xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-400">
            {t('common.error')}
          </div>
        )}

        {/* Search */}
        <div className="relative mt-6">
          <Search className="absolute left-5 top-1/2 h-5 w-5 -translate-y-1/2 text-zinc-600" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('sessions.searchPlaceholder')}
            className="w-full rounded-2xl border border-white/8 bg-dc-dark px-5 py-4 pl-14 text-base text-white outline-none placeholder:text-zinc-600 transition-all focus:border-blue-500/30 focus:ring-2 focus:ring-blue-500/10"
          />
        </div>

        {/* List */}
        <div className="mt-6 space-y-3">
          {filtered.map((session) => (
            <div
              key={session.id}
              className="group flex items-center rounded-2xl border border-white/6 bg-dc-dark transition-all hover:border-blue-500/20"
            >
              <button
                onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                className="flex flex-1 cursor-pointer items-center gap-5 px-6 py-5 text-left"
              >
                <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-white/5 text-zinc-500 group-hover:bg-blue-500/15 group-hover:text-blue-400 transition-colors">
                  <MessageSquare className="h-6 w-6" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-base font-bold text-white">
                    {session.chat_id || session.id}
                  </p>
                  <p className="mt-1 text-sm text-zinc-500">
                    {session.message_count} {t('sessions.messages')} · {timeAgo(session.last_message_at)}
                  </p>
                </div>
                <span className="rounded-full bg-white/4 px-3 py-1 text-xs font-bold uppercase tracking-wider text-zinc-500">
                  {session.channel}
                </span>
              </button>

              {confirmingDelete === session.id ? (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    handleDelete(session.id)
                  }}
                  className="mr-4 flex cursor-pointer items-center gap-1 rounded-xl bg-red-500/10 px-3 py-2 text-xs font-medium text-red-400 transition-all hover:bg-red-500/20"
                >
                  {t('common.confirm')}
                </button>
              ) : (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    setConfirmingDelete(session.id)
                    setTimeout(() => setConfirmingDelete(null), 3000)
                  }}
                  aria-label="Delete session"
                  className="mr-4 cursor-pointer rounded-xl p-3 text-zinc-600 opacity-0 transition-all hover:bg-red-500/10 hover:text-red-400 group-hover:opacity-100"
                >
                  <Trash2 className="h-5 w-5" />
                </button>
              )}
            </div>
          ))}

          {filtered.length === 0 && (
            <div className="mt-20 flex flex-col items-center">
              <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/4">
                <MessageCircle className="h-8 w-8 text-zinc-700" />
              </div>
              <p className="mt-4 text-lg font-semibold text-zinc-500">
                {search ? t('sessions.noResults') : t('sessions.noSessions')}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
