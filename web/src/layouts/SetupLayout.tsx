import { Outlet } from 'react-router-dom'
import { Terminal } from 'lucide-react'

/**
 * Setup wizard layout — clean dark design with subtle accent.
 */
export function SetupLayout() {
  return (
    <div className="relative flex min-h-full items-center justify-center overflow-hidden bg-dc-darker p-6">
      {/* Subtle gradient background */}
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute -left-40 -top-40 h-[500px] w-[500px] rounded-full bg-blue-600/5 blur-[120px]" />
        <div className="absolute -bottom-40 -right-40 h-[500px] w-[500px] rounded-full bg-blue-500/5 blur-[120px]" />
      </div>

      <div className="relative z-10 w-full max-w-xl">
        {/* Logo */}
        <div className="mb-8 text-center">
          <div className="inline-flex items-center justify-center">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-blue-500/15">
              <Terminal className="h-7 w-7 text-blue-400" />
            </div>
          </div>
          <h1 className="mt-4 text-2xl font-bold text-white">
            Dev<span className="text-blue-400">Claw</span>
          </h1>
          <p className="mt-1.5 text-sm text-zinc-500">Configure your AI agent</p>
        </div>

        {/* Card */}
        <div className="rounded-2xl border border-white/6 bg-dc-dark/90 p-8 shadow-2xl shadow-black/40 backdrop-blur-xl">
          <Outlet />
        </div>

        <p className="mt-6 text-center text-xs text-zinc-600">
          DevClaw &mdash; AI Agent for Tech Teams
        </p>
      </div>
    </div>
  )
}
