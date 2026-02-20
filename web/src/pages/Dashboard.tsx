import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Cpu,
  Radio,
  Clock,
  ArrowRight,
  Puzzle,
  Shield,
  Settings,
  MessageSquare,
  Zap,
  Activity,
  DollarSign,
} from 'lucide-react'
import { api, type DashboardData } from '@/lib/api'
import { formatTokens, timeAgo } from '@/lib/utils'

function getGreeting(t: (key: string) => string): string {
  const h = new Date().getHours()
  if (h < 12) return t('greetings.morning')
  if (h < 18) return t('greetings.afternoon')
  return t('greetings.evening')
}

export function Dashboard() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.dashboard()
      .then(setData)
      .catch(() => {
        Promise.all([
          api.sessions.list().catch(() => []),
          api.usage().catch(() => ({ total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 })),
          api.channels.list().catch(() => []),
          api.jobs.list().catch(() => []),
        ]).then(([sessions, usage, channels, jobs]) => {
          setData({ sessions, usage, channels, jobs })
        })
      })
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-8 w-8 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  if (!data) return null

  const channels = data.channels ?? []
  const jobs = data.jobs ?? []
  const usage = data.usage ?? { total_input_tokens: 0, total_output_tokens: 0, total_cost: 0, request_count: 0 }
  const connectedChannels = channels.filter((c) => c.connected).length
  const activeJobs = jobs.filter((j) => j.enabled).length
  const totalTokens = usage.total_input_tokens + usage.total_output_tokens
  const sessionCount = (data.sessions ?? []).length
  const allChannelsOk = connectedChannels === channels.length && channels.length > 0

  return (
    <div className="flex-1 overflow-y-auto bg-dc-darker">
      <div className="mx-auto max-w-5xl px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-sm text-zinc-500">{getGreeting(t)}</p>
            <h1 className="mt-0.5 text-2xl font-black text-white tracking-tight">{t('dashboard.title')}</h1>
          </div>
          {/* Usage pill */}
          <div className="flex items-center gap-3 rounded-xl bg-zinc-800/40 px-4 py-2.5 ring-1 ring-zinc-700/20">
            <div className="flex items-center gap-1.5">
              <Activity className="h-3 w-3 text-blue-400" />
              <span className="text-xs font-semibold text-zinc-300">{formatTokens(totalTokens)}</span>
              <span className="text-[10px] text-zinc-500">{t('dashboard.tokens')}</span>
            </div>
            <span className="h-4 w-px bg-zinc-700/50" />
            <div className="flex items-center gap-1">
              <DollarSign className="h-3 w-3 text-emerald-400" />
              <span className="text-xs font-semibold text-zinc-300">{usage.total_cost?.toFixed(2) ?? '0.00'}</span>
            </div>
          </div>
        </div>

        {/* Stat cards */}
        <div className="mt-6 grid grid-cols-4 gap-2.5">
          <MetricCard
            label={t('dashboard.requests')}
            value={String(usage.request_count ?? 0)}
            icon={<Cpu className="h-3.5 w-3.5" />}
            onClick={() => navigate('/config')}
          />
          <MetricCard
            label={t('dashboard.channels')}
            value={`${connectedChannels}/${channels.length}`}
            icon={<Radio className="h-3.5 w-3.5" />}
            status={channels.length === 0 ? 'neutral' : allChannelsOk ? 'ok' : 'warn'}
            onClick={() => navigate('/channels')}
          />
          <MetricCard
            label={t('dashboard.jobs')}
            value={String(activeJobs)}
            icon={<Clock className="h-3.5 w-3.5" />}
            onClick={() => navigate('/jobs')}
          />
          <MetricCard
            label={t('dashboard.sessions')}
            value={String(sessionCount)}
            icon={<MessageSquare className="h-3.5 w-3.5" />}
            onClick={() => navigate('/sessions')}
          />
        </div>

        {/* Quick actions */}
        <div className="mt-8">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{t('dashboard.quickAccess')}</p>
          <div className="mt-2.5 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <QuickAction icon={Settings} label={t('dashboard.providers')} onClick={() => navigate('/config')} />
            <QuickAction icon={Puzzle} label={t('sidebar.skills')} onClick={() => navigate('/skills')} />
            <QuickAction icon={MessageSquare} label={t('sidebar.chat')} onClick={() => navigate('/')} />
            <QuickAction icon={Shield} label={t('sidebar.security')} onClick={() => navigate('/security')} />
          </div>
        </div>

        {/* Channels */}
        {channels.length > 0 && (
          <div className="mt-8">
            <div className="flex items-center justify-between">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{t('dashboard.channels')}</p>
              <button
                onClick={() => navigate('/channels')}
                className="flex cursor-pointer items-center gap-1 text-[11px] text-zinc-500 transition-colors hover:text-blue-400"
              >
                {t('common.viewAll')} <ArrowRight className="h-3 w-3" />
              </button>
            </div>
            <div className="mt-2.5 space-y-1.5">
              {channels.map((ch) => (
                <button
                  key={ch.name}
                  onClick={() => navigate('/channels')}
                  className={`flex w-full cursor-pointer items-center justify-between rounded-xl px-4 py-3 text-left ring-1 transition-colors ${
                    ch.connected
                      ? 'bg-emerald-500/3 ring-emerald-500/15 hover:bg-emerald-500/5'
                      : 'bg-zinc-800/30 ring-zinc-700/20 hover:ring-zinc-700/30'
                  }`}
                >
                  <div className="flex items-center gap-3">
                    <span className={`h-2 w-2 rounded-full ${ch.connected ? 'bg-emerald-400' : 'bg-zinc-600'}`} />
                    <span className="text-sm font-medium capitalize text-zinc-200">{ch.name}</span>
                  </div>
                  <span className={`text-[11px] font-medium ${ch.connected ? 'text-emerald-400' : 'text-zinc-500'}`}>
                    {ch.connected ? t('common.online') : t('common.offline')}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Recent sessions */}
        {sessionCount > 0 && (
          <div className="mt-8 mb-6">
            <div className="flex items-center justify-between">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{t('dashboard.recentSessions')}</p>
              <button
                onClick={() => navigate('/sessions')}
                className="flex cursor-pointer items-center gap-1 text-[11px] text-zinc-500 transition-colors hover:text-blue-400"
              >
                {t('common.viewAll')} <ArrowRight className="h-3 w-3" />
              </button>
            </div>
            <div className="mt-2.5 space-y-1.5">
              {(data.sessions ?? []).slice(0, 5).map((s) => (
                <button
                  key={s.id}
                  onClick={() => navigate(`/chat/${encodeURIComponent(s.id)}`)}
                  className="flex w-full cursor-pointer items-center justify-between rounded-xl bg-zinc-800/30 px-4 py-3 text-left ring-1 ring-zinc-700/20 transition-colors hover:ring-zinc-700/30"
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <Zap className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
                    <span className="truncate text-sm text-zinc-300">{s.id}</span>
                  </div>
                  <span className="shrink-0 text-[11px] text-zinc-600">{s.message_count} {t('common.msgs')} · {timeAgo(s.last_message_at)}</span>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

/* ── Metric Card ── */

function MetricCard({
  label,
  value,
  icon,
  status = 'neutral',
  onClick,
}: {
  label: string
  value: string
  icon: React.ReactNode
  status?: 'ok' | 'warn' | 'neutral'
  onClick?: () => void
}) {
  const borderColor = status === 'ok'
    ? 'ring-emerald-500/15 bg-emerald-500/3'
    : status === 'warn'
    ? 'ring-blue-500/15 bg-blue-500/3'
    : 'ring-zinc-700/20 bg-zinc-800/30'

  return (
    <button
      onClick={onClick}
      className={`group cursor-pointer rounded-xl px-4 py-3.5 text-left ring-1 transition-all hover:ring-zinc-700/40 hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/20 ${borderColor}`}
    >
      <div className="flex items-center gap-1.5 text-zinc-500">
        {icon}
        <span className="text-[11px] font-semibold uppercase tracking-wider">{label}</span>
      </div>
      <p className="mt-1.5 text-2xl font-black text-white tracking-tight">{value}</p>
    </button>
  )
}

/* ── Quick Action ── */

function QuickAction({ icon: Icon, label, onClick }: {
  icon: React.ElementType
  label: string
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className="group flex cursor-pointer items-center gap-2.5 rounded-xl bg-zinc-800/30 px-3.5 py-3 text-left ring-1 ring-zinc-700/20 transition-all hover:ring-blue-500/20 hover:bg-zinc-800/50"
    >
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-zinc-800 text-zinc-500 transition-colors group-hover:bg-blue-500/10 group-hover:text-blue-400">
        <Icon className="h-4 w-4" />
      </div>
      <span className="text-xs font-medium text-zinc-400 transition-colors group-hover:text-zinc-200">{label}</span>
    </button>
  )
}
