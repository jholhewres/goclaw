import { Outlet } from 'react-router-dom'
import { Bot } from 'lucide-react'

/**
 * Setup wizard layout — refined dark design.
 */
export function SetupLayout() {
  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[#0a0f1a] p-6">
      {/* Background effects */}
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-[#1e293b]/20 via-transparent to-transparent" />
        <div className="absolute -left-40 -top-40 h-[600px] w-[600px] rounded-full bg-[#3b82f6]/5 blur-[120px]" />
        <div className="absolute -bottom-40 -right-40 h-[600px] w-[600px] rounded-full bg-[#3b82f6]/3 blur-[120px]" />
        {/* Grid pattern */}
        <div
          className="absolute inset-0 opacity-[0.02]"
          style={{
            backgroundImage: `linear-gradient(rgba(255,255,255,0.1) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.1) 1px, transparent 1px)`,
            backgroundSize: '60px 60px',
          }}
        />
      </div>

      <div className="relative z-10 w-full max-w-lg">
        {/* Logo */}
        <div className="mb-6 text-center">
          <div className="inline-flex items-center justify-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-[#3b82f6] to-[#2563eb] shadow-lg shadow-blue-500/25">
              <Bot className="h-6 w-6 text-white" />
            </div>
          </div>
          <h1 className="mt-3 text-xl font-bold text-[#f8fafc] tracking-tight">
            Dev<span className="text-[#64748b]">Claw</span>
          </h1>
          <p className="mt-1 text-sm text-[#64748b]">Configure seu agente de IA</p>
        </div>

        {/* Card */}
        <div className="rounded-2xl border border-white/[0.08] bg-[#0f1419]/90 backdrop-blur-xl p-6 shadow-2xl shadow-black/50">
          <Outlet />
        </div>

        <p className="mt-5 text-center text-xs text-[#475569]">
          DevClaw &mdash; AI Agent for Tech Teams
        </p>
      </div>
    </div>
  )
}
