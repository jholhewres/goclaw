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
      <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
        <div className="h-8 w-8 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
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
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <p className="text-sm text-[#64748b]">{getGreeting(t)}</p>
          <h1 className="mt-0.5 text-2xl font-bold text-[#f8fafc] tracking-tight">{t('dashboard.title')}</h1>
        </div>
        {/* Usage pill */}
        <div className="flex items-center gap-3 rounded-xl border border-white/10 bg-[#111827] px-4 py-2.5">
          <div className="flex items-center gap-1.5">
            <Activity className="h-3 w-3 text-[#3b82f6]" />
            <span className="text-xs font-semibold text-[#f8fafc]">{formatTokens(totalTokens)}</span>
            <span className="text-[10px] text-[#64748b]">{t('dashboard.tokens')}</span>
          </div>
          <span className="h-4 w-px bg-white/10" />
          <div className="flex items-center gap-1">
            <DollarSign className="h-3 w-3 text-[#3b82f6]" />
            <span className="text-xs font-semibold text-[#f8fafc]">{usage.total_cost?.toFixed(2) ?? '0.00'}</span>
          </div>
        </div>
      </div>

      {/* Stat cards */}
      <div className="mt-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
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
        <p className="text-[11px] font-semibold uppercase tracking-wider text-[#64748b]">{t('dashboard.quickAccess')}</p>
        <div className="mt-2.5 grid grid-cols-2 gap-3 sm:grid-cols-4">
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
            <p className="text-[11px] font-semibold uppercase tracking-wider text-[#64748b]">{t('dashboard.channels')}</p>
            <button
              onClick={() => navigate('/channels')}
              className="flex cursor-pointer items-center gap-1 text-[11px] text-[#64748b] hover:text-[#f8fafc] transition-colors"
            >
              {t('common.viewAll')} <ArrowRight className="h-3 w-3" />
            </button>
          </div>
          <div className="mt-2.5 space-y-2">
            {channels.map((ch) => (
              <button
                key={ch.name}
                onClick={() => navigate('/channels')}
                className="flex w-full cursor-pointer items-center justify-between rounded-xl border border-white/10 bg-[#111827] px-4 py-3.5 text-left transition-all duration-200 hover:border-white/20"
              >
                <div className="flex items-center gap-3">
                  <span className={`h-2 w-2 rounded-full ${ch.connected ? 'bg-[#22c55e]' : 'bg-[#64748b]'}`} />
                  <span className="text-sm font-medium capitalize text-[#f8fafc]">{ch.name}</span>
                </div>
                <span
                  className="text-[11px] font-medium"
                  style={{ color: ch.connected ? '#22c55e' : '#64748b' }}
                >
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
            <p className="text-[11px] font-semibold uppercase tracking-wider text-[#64748b]">{t('dashboard.recentSessions')}</p>
            <button
              onClick={() => navigate('/sessions')}
              className="flex cursor-pointer items-center gap-1 text-[11px] text-[#64748b] hover:text-[#f8fafc] transition-colors"
            >
              {t('common.viewAll')} <ArrowRight className="h-3 w-3" />
            </button>
          </div>
          <div className="mt-2.5 space-y-2">
            {(data.sessions ?? []).slice(0, 5).map((s) => (
              <button
                key={s.id}
                onClick={() => navigate(`/chat/${encodeURIComponent(s.id)}`)}
                className="flex w-full cursor-pointer items-center justify-between rounded-xl border border-white/10 bg-[#111827] px-4 py-3.5 text-left transition-all duration-200 hover:border-white/20"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <Zap className="h-3.5 w-3.5 shrink-0 text-[#3b82f6]" />
                  <span className="truncate text-sm text-[#f8fafc]">{s.id}</span>
                </div>
                <span className="shrink-0 text-[11px] text-[#64748b]">{s.message_count} {t('common.msgs')} · {timeAgo(s.last_message_at)}</span>
              </button>
            ))}
          </div>
        </div>
      )}
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
  const borderColor = status === 'ok' ? 'rgba(34, 197, 94, 0.3)' : status === 'warn' ? 'rgba(245, 158, 11, 0.3)' : 'rgba(255, 255, 255, 0.1)'

  return (
    <button
      onClick={onClick}
      className="group cursor-pointer rounded-2xl bg-[#111827] p-5 text-left transition-all duration-200 hover:-translate-y-0.5 hover:bg-[#1e293b]"
      style={{ border: `1px solid ${borderColor}` }}
    >
      <div className="flex items-center gap-1.5 text-[#64748b]">
        {icon}
        <span className="text-[11px] font-semibold uppercase tracking-wider">{label}</span>
      </div>
      <p className="mt-1.5 text-2xl font-bold text-[#f8fafc] tracking-tight">{value}</p>
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
      className="group flex cursor-pointer items-center gap-3 rounded-xl border border-white/10 bg-[#111827] p-4 text-left transition-all duration-200 hover:border-white/20 hover:bg-[#1e293b]"
    >
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[#1e293b] text-[#64748b] transition-colors group-hover:bg-[#3b82f6]/10 group-hover:text-[#3b82f6]">
        <Icon className="h-4 w-4" />
      </div>
      <span className="text-sm font-medium text-[#94a3b8] transition-colors group-hover:text-[#f8fafc]">{label}</span>
    </button>
  )
}
