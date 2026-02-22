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
  ChevronLeft,
  ChevronRight,
  Cpu,
  Bot,
  ChevronDown,
  ChevronUp,
  BarChart3,
  Menu,
  X,
  Key,
  Users,
  DollarSign,
  Brain,
  Database,
  UsersRound,
  Cable,
} from 'lucide-react'
import { cn } from '@/lib/utils'

interface MenuItem {
  nameKey: string
  icon: React.ElementType
  route?: string
  sectionKey: string
  submenu?: { nameKey: string; icon: React.ElementType; route: string }[]
}

interface SidebarProps {
  compact: boolean
  setCompact: (value: boolean) => void
}

/** Navigation menu items organized by section */
const menuItems: MenuItem[] = [
  // Início
  {
    nameKey: 'sidebar.chat',
    icon: MessageSquare,
    route: '/',
    sectionKey: 'sidebarSections.start',
  },
  // Gerenciar
  {
    nameKey: 'sidebar.channels',
    icon: Radio,
    route: '/channels',
    sectionKey: 'sidebarSections.manage',
  },
  {
    nameKey: 'sidebar.jobs',
    icon: Clock,
    route: '/jobs',
    sectionKey: 'sidebarSections.manage',
  },
  {
    nameKey: 'sidebar.skills',
    icon: Puzzle,
    route: '/skills',
    sectionKey: 'sidebarSections.manage',
  },
  {
    nameKey: 'sidebar.statistics',
    icon: BarChart3,
    route: '/stats',
    sectionKey: 'sidebarSections.manage',
  },
  // Configurações
  {
    nameKey: 'sidebar.settings',
    icon: Settings,
    sectionKey: 'sidebarSections.settings',
    submenu: [
      { nameKey: 'sidebar.apiConfig', icon: Key, route: '/api-config' },
      { nameKey: 'sidebar.access', icon: Users, route: '/access' },
      { nameKey: 'sidebar.budget', icon: DollarSign, route: '/budget' },
      { nameKey: 'sidebar.memory', icon: Brain, route: '/memory' },
      { nameKey: 'sidebar.database', icon: Database, route: '/database' },
      { nameKey: 'sidebar.groups', icon: UsersRound, route: '/groups' },
      { nameKey: 'sidebar.mcp', icon: Cable, route: '/mcp' },
      { nameKey: 'sidebar.llmProviders', icon: Cpu, route: '/config' },
      { nameKey: 'sidebar.system', icon: Bot, route: '/system' },
      { nameKey: 'sidebar.domainNetwork', icon: Globe, route: '/domain' },
      { nameKey: 'sidebar.webhooks', icon: Webhook, route: '/webhooks' },
      { nameKey: 'sidebar.hooks', icon: Zap, route: '/hooks' },
    ],
  },
  // Segurança
  {
    nameKey: 'sidebar.security',
    icon: Shield,
    route: '/security',
    sectionKey: 'sidebarSections.security',
  },
]

function SidebarTooltip({ children, label, show }: { children: React.ReactNode; label: string; show: boolean }) {
  const [isHovered, setIsHovered] = useState(false)

  return (
    <div
      className="relative"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {children}
      {show && isHovered && (
        <div className="absolute left-full ml-2 top-1/2 -translate-y-1/2 z-50 pointer-events-none">
          <div className="bg-[#1e293b] text-[#f8fafc] text-xs font-medium px-3 py-1.5 rounded-lg whitespace-nowrap shadow-lg animate-fade-in border border-white/10">
            {label}
            <div className="absolute right-full top-1/2 -translate-y-1/2 border-4 border-transparent border-r-[#1e293b]" />
          </div>
        </div>
      )}
    </div>
  )
}

export function Sidebar({ compact, setCompact }: SidebarProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const [isMobileOpen, setIsMobileOpen] = useState(false)
  const [expandedMenus, setExpandedMenus] = useState<string[]>([])

  const isActive = (route?: string) => {
    if (!route) return false
    if (location.pathname === route) return true
    if (route !== '/' && location.pathname.startsWith(route)) return true
    return false
  }

  const isAnySubmenuActive = (item: MenuItem): boolean => {
    if (!item.submenu) return false
    return item.submenu.some(sub => isActive(sub.route))
  }

  // Auto-expand submenu if child route is active
  useEffect(() => {
    menuItems.forEach(item => {
      if (item.submenu && item.submenu.some(sub => isActive(sub.route))) {
        setExpandedMenus(prev => prev.includes(item.nameKey) ? prev : [...prev, item.nameKey])
      }
    })
  }, [location.pathname])

  useEffect(() => {
    const handleResize = () => {
      if (window.innerWidth >= 1024) {
        setIsMobileOpen(false)
      }
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  const showFullText = !compact || isMobileOpen

  // Group items by section
  const sections: { [key: string]: MenuItem[] } = {}
  menuItems.forEach(item => {
    if (!sections[item.sectionKey]) {
      sections[item.sectionKey] = []
    }
    sections[item.sectionKey].push(item)
  })

  return (
    <>
      {/* Mobile Overlay */}
      {isMobileOpen && (
        <div
          className="fixed inset-0 bg-black/60 z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={cn(
          'fixed top-0 left-0 z-50 h-screen bg-[#111827] border-r transition-all duration-300 flex flex-col',
          isMobileOpen ? 'w-64' : compact ? 'w-20' : 'w-64',
          isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'
        )}
        style={{ borderColor: 'rgba(255, 255, 255, 0.08)' }}
      >
        {/* Header com Logo */}
        <div className="flex items-center justify-between h-16 px-4" style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.08)' }}>
          {(isMobileOpen || !compact) ? (
            <button
              onClick={() => navigate('/')}
              className="flex items-center gap-2.5"
            >
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-[#3b82f6]">
                <Terminal className="h-4 w-4 text-white" />
              </div>
              <span className="text-sm font-semibold text-[#f8fafc]">
                Dev<span className="text-[#64748b]">Claw</span>
              </span>
            </button>
          ) : (
            <button
              onClick={() => navigate('/')}
              className="flex items-center justify-center w-full"
            >
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-[#3b82f6]">
                <Terminal className="h-4 w-4 text-white" />
              </div>
            </button>
          )}
          <div className="flex items-center gap-2">
            <button
              onClick={() => setCompact(!compact)}
              className="hidden lg:flex items-center justify-center w-9 h-9 rounded-lg hover:bg-[#1e293b] transition-all text-[#64748b] hover:text-[#f8fafc] group"
              title={compact ? t('sidebar.expandMenu') : t('sidebar.collapseMenu')}
            >
              {compact ? (
                <ChevronRight className="w-5 h-5 group-hover:translate-x-0.5 transition-transform" />
              ) : (
                <ChevronLeft className="w-5 h-5 group-hover:-translate-x-0.5 transition-transform" />
              )}
            </button>
            <button
              onClick={() => setIsMobileOpen(false)}
              className="lg:hidden flex items-center justify-center w-9 h-9 rounded-lg hover:bg-[#1e293b] transition-colors text-[#64748b]"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Menu */}
        <nav className="flex-1 overflow-y-auto p-4 space-y-6">
          {Object.entries(sections).map(([sectionKey, items]) => (
            <div key={sectionKey} className="space-y-1">
              {showFullText && (
                <div className="px-3 mb-2">
                  <span className="text-xs font-semibold text-[#475569] uppercase tracking-wider">
                    {t(sectionKey)}
                  </span>
                </div>
              )}
              {!showFullText && (
                <div className="flex justify-center mb-2">
                  <div className="w-8 h-px bg-[rgba(255,255,255,0.08)]" />
                </div>
              )}
              {items.map((item) => {
                const Icon = item.icon
                const hasSubmenu = item.submenu && item.submenu.length > 0
                const isExpanded = expandedMenus.includes(item.nameKey)
                const active = hasSubmenu ? isAnySubmenuActive(item) : isActive(item.route)
                const itemName = t(item.nameKey)

                // If has submenu and showing full text
                if (hasSubmenu && showFullText) {
                  return (
                    <div key={item.nameKey}>
                      <button
                        onClick={() => {
                          setExpandedMenus(prev =>
                            prev.includes(item.nameKey)
                              ? prev.filter(n => n !== item.nameKey)
                              : [...prev, item.nameKey]
                          )
                        }}
                        className={cn(
                          'relative w-full flex items-center justify-between gap-3 px-3 py-2.5 rounded-xl transition-all duration-200',
                          active
                            ? 'bg-white/5 text-[#f8fafc]'
                            : 'text-[#64748b] hover:bg-white/5 hover:text-[#f8fafc]'
                        )}
                      >
                        <div className="flex items-center gap-3">
                          {active && (
                            <span className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-[#3b82f6] rounded-full animate-fade-in" />
                          )}
                          <Icon className={cn('w-5 h-5 flex-shrink-0', active && 'text-[#3b82f6]')} />
                          <span className={cn('text-sm font-medium', active && 'text-[#f8fafc]')}>
                            {itemName}
                          </span>
                        </div>
                        {isExpanded ? (
                          <ChevronUp className="w-4 h-4" />
                        ) : (
                          <ChevronDown className="w-4 h-4" />
                        )}
                      </button>

                      {isExpanded && (
                        <div className="ml-4 mt-1 space-y-1">
                          {item.submenu?.map((subitem) => {
                            const SubIcon = subitem.icon
                            const subActive = isActive(subitem.route)
                            return (
                              <button
                                key={subitem.nameKey}
                                onClick={() => {
                                  navigate(subitem.route)
                                  setIsMobileOpen(false)
                                }}
                                className={cn(
                                  'relative flex items-center gap-3 px-3 py-2 rounded-lg transition-all duration-200 w-full',
                                  subActive
                                    ? 'bg-white/5 text-[#f8fafc]'
                                    : 'text-[#64748b] hover:bg-white/5 hover:text-[#f8fafc]'
                                )}
                              >
                                <SubIcon className={cn('w-4 h-4 flex-shrink-0', subActive && 'text-[#3b82f6]')} />
                                <span className={cn('text-sm', subActive && 'text-[#f8fafc] font-medium')}>
                                  {t(subitem.nameKey)}
                                </span>
                              </button>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  )
                }

                // Normal link
                const linkContent = (
                  <button
                    key={item.nameKey}
                    onClick={() => {
                      if (item.route) {
                        navigate(item.route)
                        setIsMobileOpen(false)
                      }
                    }}
                    className={cn(
                      'relative flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all duration-200 w-full',
                      active
                        ? 'bg-white/5 text-[#f8fafc]'
                        : 'text-[#64748b] hover:bg-white/5 hover:text-[#f8fafc]',
                      !showFullText && 'justify-center'
                    )}
                  >
                    {active && (
                      <span className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-[#3b82f6] rounded-full animate-fade-in" />
                    )}
                    <Icon className={cn('w-5 h-5 flex-shrink-0', active && 'text-[#3b82f6]')} />
                    {showFullText && (
                      <span className={cn('text-sm font-medium', active && 'text-[#f8fafc]')}>
                        {itemName}
                      </span>
                    )}
                  </button>
                )

                return showFullText ? (
                  <div key={item.nameKey}>{linkContent}</div>
                ) : (
                  <SidebarTooltip key={item.nameKey} label={itemName} show={!showFullText}>
                    {linkContent}
                  </SidebarTooltip>
                )
              })}
            </div>
          ))}
        </nav>
      </aside>

      {/* Mobile Menu Button */}
      <button
        onClick={() => setIsMobileOpen(true)}
        className={cn(
          'lg:hidden fixed top-4 left-4 z-50 flex items-center justify-center w-11 h-11 rounded-lg bg-[#111827] border text-[#f8fafc] shadow-lg',
          isMobileOpen ? 'opacity-0 pointer-events-none' : 'opacity-100'
        )}
        style={{ borderColor: 'rgba(255, 255, 255, 0.08)' }}
      >
        <Menu className="w-6 h-6" />
      </button>
    </>
  )
}
