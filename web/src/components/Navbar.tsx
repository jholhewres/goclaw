import { useNavigate } from 'react-router-dom'
import { Settings, LogOut, ChevronDown } from 'lucide-react'
import { useState, useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface NavbarProps {
  sidebarCompact: boolean
}

export function Navbar({ sidebarCompact }: NavbarProps) {
  const navigate = useNavigate()
  const [isUserMenuOpen, setIsUserMenuOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsUserMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const handleLogout = () => {
    localStorage.removeItem('devclaw_token')
    window.location.href = '/login'
  }

  return (
    <header
      className={cn(
        'fixed right-0 top-0 z-30 h-16 bg-[#111827] border-b transition-all duration-300',
        sidebarCompact ? 'lg:left-20' : 'lg:left-64'
      )}
      style={{ borderColor: 'rgba(255, 255, 255, 0.08)' }}
    >
      <div className="h-full px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-end h-full gap-4">
          {/* User Dropdown */}
          <div className="relative" ref={dropdownRef}>
            <button
              onClick={() => setIsUserMenuOpen(!isUserMenuOpen)}
              className="flex items-center gap-2 hover:bg-white/5 rounded-lg px-2 py-1.5 transition-all"
            >
              <div className="w-9 h-9 rounded-lg bg-[#3b82f6] flex items-center justify-center text-white font-semibold text-sm">
                D
              </div>
              <div className="text-left hidden md:block">
                <div className="text-sm font-medium text-[#f8fafc]">DevClaw</div>
              </div>
              <ChevronDown
                className={cn('w-4 h-4 text-[#64748b] transition-transform duration-200', isUserMenuOpen && 'rotate-180')}
              />
            </button>

              {/* Dropdown Menu */}
              {isUserMenuOpen && (
                <div
                  className="absolute right-0 mt-2 w-56 rounded-xl shadow-xl overflow-hidden animate-fade-in border"
                  style={{
                    background: '#1e293b',
                    borderColor: 'rgba(255, 255, 255, 0.1)',
                  }}
                >
                  {/* User Info */}
                  <div className="px-4 py-3 border-b bg-[#111827]" style={{ borderColor: 'rgba(255, 255, 255, 0.08)' }}>
                    <p className="text-sm font-medium text-[#f8fafc]">DevClaw</p>
                    <p className="text-xs text-[#64748b]">v1.6.0</p>
                  </div>

                  {/* Menu Items */}
                  <div className="py-1">
                    <button
                      onClick={() => {
                        navigate('/system')
                        setIsUserMenuOpen(false)
                      }}
                      className="flex items-center gap-3 px-4 py-2.5 text-sm text-[#94a3b8] hover:bg-white/5 hover:text-[#f8fafc] transition-colors w-full"
                    >
                      <Settings className="w-5 h-5" />
                      <span>Sistema</span>
                    </button>
                  </div>

                  {/* Logout */}
                  <div style={{ borderTop: '1px solid rgba(255, 255, 255, 0.08)' }}>
                    <button
                      onClick={handleLogout}
                      className="flex items-center gap-3 px-4 py-2.5 text-sm text-[#94a3b8] hover:bg-white/5 hover:text-[#f8fafc] transition-colors w-full"
                    >
                      <LogOut className="w-5 h-5" />
                      <span>Sair</span>
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
    </header>
  )
}
