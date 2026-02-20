import { useEffect, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Puzzle,
  Radio,
  Clock,
  Settings,
  Shield,
  Globe,
  Webhook,
  Zap,
  MessageSquare,
  Terminal,
  PanelLeftClose,
  BarChart3,
  ChevronDown,
  ChevronRight,
  Cpu,
  Bot,
  Link2,
  MessagesSquare,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app'
import { api, type SessionInfo } from '@/lib/api'
import { timeAgo } from '@/lib/utils'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'

/** Navigation icons and routes - labels come from i18n */
const mainLinkDefs = [
  { to: '/', icon: MessageSquare, labelKey: 'sidebar.chat' },
  { to: '/channels', icon: Radio, labelKey: 'sidebar.channels' },
  { to: '/jobs', icon: Clock, labelKey: 'sidebar.jobs' },
  { to: '/skills', icon: Puzzle, labelKey: 'sidebar.skills' },
  { to: '/stats', icon: BarChart3, labelKey: 'sidebar.statistics' },
] as const

/** Configuration submenu items */
const configLinkDefs = [
  { to: '/config', icon: Cpu, labelKey: 'sidebar.llmProviders' },
  { to: '/system', icon: Bot, labelKey: 'sidebar.system' },
  { to: '/domain', icon: Globe, labelKey: 'sidebar.domainNetwork' },
  { to: '/webhooks', icon: Webhook, labelKey: 'sidebar.webhooks' },
  { to: '/hooks', icon: Zap, labelKey: 'sidebar.hooks' },
  { to: '/integrations', icon: Link2, labelKey: 'sidebar.integrations' },
] as const

function NavButton({
  icon: Icon,
  label,
  active,
  onClick,
}: {
  icon: React.ElementType
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-all',
        active
          ? 'bg-blue-500/10 text-blue-400'
          : 'text-zinc-400 hover:bg-white/4 hover:text-zinc-200',
      )}
    >
      <Icon className="h-4.5 w-4.5" />
      {label}
    </button>
  )
}

function SessionItem({ session, onClick }: { session: SessionInfo; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left transition-all hover:bg-white/4"
    >
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-white/5 text-zinc-500">
        <MessagesSquare className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-medium text-zinc-300">{session.chat_id || session.id}</p>
        <p className="text-[10px] text-zinc-600">{timeAgo(session.last_message_at)}</p>
      </div>
      <span className="shrink-0 rounded-full bg-white/4 px-1.5 py-0.5 text-[9px] font-bold text-zinc-500">
        {session.message_count}
      </span>
    </button>
  )
}

export function Sidebar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const { sidebarOpen, toggleSidebar } = useAppStore()
  const [sessionsOpen, setSessionsOpen] = useState(false)
  const [configOpen, setConfigOpen] = useState(false)
  const [sessions, setSessions] = useState<SessionInfo[]>([])
  const [sessionsLoading, setSessionsLoading] = useState(false)

  // Build links with translations
  const mainLinks = mainLinkDefs.map((l) => ({ ...l, label: t(l.labelKey) }))
  const configLinks = configLinkDefs.map((l) => ({ ...l, label: t(l.labelKey) }))

  // Load sessions when dropdown opens
  useEffect(() => {
    if (sessionsOpen && sessions.length === 0) {
      setSessionsLoading(true)
      api.sessions.list()
        .then(setSessions)
        .catch(() => {})
        .finally(() => setSessionsLoading(false))
    }
  }, [sessionsOpen, sessions.length])

  if (!sidebarOpen) return null

  const isConfigActive = configLinkDefs.some((l) => location.pathname === l.to || location.pathname.startsWith(l.to + '/'))

  return (
    <aside className="flex h-full w-64 flex-col border-r border-white/6 bg-dc-dark">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-4">
        <button
          onClick={() => navigate('/')}
          className="flex cursor-pointer items-center gap-2.5 transition-opacity hover:opacity-80"
        >
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-blue-500/15">
            <Terminal className="h-5 w-5 text-blue-400" />
          </div>
          <span className="text-base font-bold text-white">
            Dev<span className="text-blue-400">Claw</span>
          </span>
        </button>
        <button
          onClick={toggleSidebar}
          aria-label="Close sidebar"
          className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-white/6 hover:text-zinc-300"
        >
          <PanelLeftClose className="h-4.5 w-4.5" />
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto px-3 pb-3">
        {/* Main Links */}
        <p className="mb-1.5 px-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-600">
          {t('sidebar.menu')}
        </p>
        {mainLinks.map(({ to, icon, label }) => (
          <NavButton
            key={to}
            icon={icon}
            label={label}
            active={location.pathname === to || (to !== '/' && location.pathname.startsWith(to))}
            onClick={() => navigate(to)}
          />
        ))}

        {/* Sessions Dropdown */}
        <div className="mt-6">
          <button
            onClick={() => setSessionsOpen(!sessionsOpen)}
            className={cn(
              'flex w-full cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-sm font-medium transition-all',
              sessionsOpen || location.pathname.startsWith('/sessions') || location.pathname.startsWith('/chat/')
                ? 'bg-blue-500/10 text-blue-400'
                : 'text-zinc-400 hover:bg-white/4 hover:text-zinc-200',
            )}
          >
            <div className="flex items-center gap-2.5">
              <MessageSquare className="h-4.5 w-4.5" />
              {t('sidebar.sessions')}
            </div>
            {sessionsOpen ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
          </button>
          {sessionsOpen && (
            <div className="mt-1 ml-2 border-l border-white/6 pl-2">
              {sessionsLoading ? (
                <div className="flex items-center justify-center py-4">
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-blue-500/30 border-t-blue-500" />
                </div>
              ) : sessions.length > 0 ? (
                <>
                  {sessions.slice(0, 5).map((session) => (
                    <SessionItem
                      key={session.id}
                      session={session}
                      onClick={() => navigate(`/chat/${encodeURIComponent(session.id)}`)}
                    />
                  ))}
                  {sessions.length > 5 && (
                    <button
                      onClick={() => navigate('/sessions')}
                      className="mt-1 w-full px-3 py-2 text-left text-xs text-zinc-500 hover:text-zinc-300 transition-colors"
                    >
                      {t('common.viewAll')} ({sessions.length})
                    </button>
                  )}
                </>
              ) : (
                <p className="px-3 py-3 text-xs text-zinc-600">{t('sidebar.noActiveSessions')}</p>
              )}
            </div>
          )}
        </div>

        {/* Config Dropdown */}
        <div className="mt-6">
          <button
            onClick={() => setConfigOpen(!configOpen)}
            className={cn(
              'flex w-full cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-sm font-medium transition-all',
              configOpen || isConfigActive
                ? 'bg-blue-500/10 text-blue-400'
                : 'text-zinc-400 hover:bg-white/4 hover:text-zinc-200',
            )}
          >
            <div className="flex items-center gap-2.5">
              <Settings className="h-4.5 w-4.5" />
              {t('sidebar.settings')}
            </div>
            {configOpen ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
          </button>
          {configOpen && (
            <div className="mt-1 ml-2 border-l border-white/6 pl-2">
              {configLinks.map(({ to, icon, label }) => (
                <NavButton
                  key={to}
                  icon={icon}
                  label={label}
                  active={location.pathname === to}
                  onClick={() => navigate(to)}
                />
              ))}
            </div>
          )}
        </div>

        {/* Security (separate) */}
        <div className="mt-6">
          <p className="mb-1.5 px-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-600">
            {t('sidebar.security')}
          </p>
          <NavButton
            icon={Shield}
            label={t('sidebar.security')}
            active={location.pathname === '/security'}
            onClick={() => navigate('/security')}
          />
        </div>
      </nav>

      {/* Footer */}
      <div className="border-t border-white/6 px-4 py-3">
        <div className="flex items-center justify-between">
          <p className="text-xs text-zinc-600">DevClaw v1.6.0</p>
          <div className="flex items-center gap-2">
            <LanguageSwitcher />
            <div className="flex items-center gap-1.5">
              <div className="h-2 w-2 rounded-full bg-emerald-400" />
              <span className="text-[11px] text-zinc-500">{t('common.online')}</span>
            </div>
          </div>
        </div>
      </div>
    </aside>
  )
}
