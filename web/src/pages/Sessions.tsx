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
      <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
        <div className="h-10 w-10 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
      </div>
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{t('sidebar.sessions')}</p>
      <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">{t('sessions.title')}</h1>
      <p className="mt-2 text-base text-[#64748b]">{sessions.length} {t('sessions.subtitle')}</p>

      {loadError && (
        <div className="mt-4 rounded-xl px-4 py-3 text-sm text-[#f87171] bg-red-500/10 border border-red-500/20">
          {t('common.error')}
        </div>
      )}

      {/* Search */}
      <div className="relative mt-6">
        <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-[#64748b]" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('sessions.searchPlaceholder')}
          className="h-11 w-full rounded-xl border border-white/10 bg-[#111827] pl-11 pr-4 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20"
        />
      </div>

      {/* List */}
      <div className="mt-6 space-y-2">
          {filtered.map((session) => (
            <div
              key={session.id}
              className="group flex items-center rounded-xl border border-white/10 bg-[#111827] transition-all duration-200 hover:border-white/20"
            >
              <button
                onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                className="flex flex-1 cursor-pointer items-center gap-4 px-5 py-4 text-left"
              >
                <div className="flex h-11 w-11 items-center justify-center rounded-xl border border-white/10 bg-[#1e293b] text-[#64748b] transition-colors group-hover:border-[#3b82f6]/30 group-hover:text-[#3b82f6]">
                  <MessageSquare className="h-5 w-5" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-[#f8fafc]">
                    {session.chat_id || session.id}
                  </p>
                  <p className="mt-0.5 text-xs text-[#64748b]">
                    {session.message_count} {t('sessions.messages')} · {timeAgo(session.last_message_at)}
                  </p>
                </div>
                <span className="rounded-full border border-white/10 bg-[#1e293b] px-2.5 py-1 text-[10px] font-medium uppercase tracking-wider text-[#64748b]">
                  {session.channel}
                </span>
              </button>

              {confirmingDelete === session.id ? (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    handleDelete(session.id)
                  }}
                  className="mr-4 flex cursor-pointer items-center gap-1 rounded-lg bg-red-500/10 px-3 py-2 text-xs font-medium text-[#f87171] transition-all hover:bg-red-500/20"
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
                  className="mr-4 cursor-pointer rounded-lg p-2.5 text-[#64748b] opacity-0 transition-all hover:bg-red-500/10 hover:text-[#f87171] group-hover:opacity-100"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              )}
            </div>
          ))}

          {filtered.length === 0 && (
            <div className="mt-20 flex flex-col items-center">
              <div className="flex h-14 w-14 items-center justify-center rounded-xl border border-white/10 bg-[#1e293b]">
                <MessageCircle className="h-7 w-7 text-[#475569]" />
              </div>
              <p className="mt-4 text-base font-medium text-[#64748b]">
                {search ? t('sessions.noResults') : t('sessions.noSessions')}
              </p>
            </div>
          )}
        </div>
    </div>
  )
}
