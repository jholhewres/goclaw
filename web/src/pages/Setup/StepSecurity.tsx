import { useState } from 'react'
import { Shield, ShieldCheck, ShieldAlert, Lock, KeyRound, Eye, EyeOff, Info } from 'lucide-react'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const MODES = [
  {
    value: 'relaxed' as const,
    label: 'Relaxed',
    description: 'Tools run without asking permission. Ideal for personal use.',
    icon: Shield,
    color: 'emerald',
  },
  {
    value: 'strict' as const,
    label: 'Strict',
    description: 'Potentially dangerous commands require approval before running.',
    icon: ShieldCheck,
    color: 'blue',
  },
  {
    value: 'paranoid' as const,
    label: 'Paranoid',
    description: 'All external actions require approval. Maximum security.',
    icon: ShieldAlert,
    color: 'amber',
  },
]

const COLOR_MAP = {
  emerald: {
    active: 'border-emerald-500/50 bg-emerald-500/10 ring-1 ring-emerald-500/20',
    icon: 'text-emerald-400',
    dot: 'bg-emerald-400',
  },
  blue: {
    active: 'border-orange-500/50 bg-orange-500/10 ring-1 ring-orange-500/20',
    icon: 'text-orange-400',
    dot: 'bg-orange-400',
  },
  amber: {
    active: 'border-amber-500/50 bg-amber-500/10 ring-1 ring-amber-500/20',
    icon: 'text-amber-400',
    dot: 'bg-amber-400',
  },
}

export function StepSecurity({ data, updateData }: Props) {
  const [showPassword, setShowPassword] = useState(false)
  const [showVault, setShowVault] = useState(false)

  const useCustomVault = data.vaultPassword !== '' && data.vaultPassword !== data.webuiPassword

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-white">Security</h2>
        <p className="mt-1 text-sm text-zinc-400">
          Protect access and set the tool permission level
        </p>
      </div>

      <div className="space-y-5">
        {/* Web UI Password */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Lock className="h-3.5 w-3.5 text-zinc-500" />
            Web UI Password
          </label>
          <div className="relative">
            <input
              type={showPassword ? 'text' : 'password'}
              value={data.webuiPassword}
              onChange={(e) => {
                updateData({ webuiPassword: e.target.value })
                if (!useCustomVault) {
                  updateData({ vaultPassword: e.target.value })
                }
              }}
              placeholder="Set a password for the dashboard"
              className="flex h-11 w-full rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-4 pr-10 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
            />
            <button
              type="button"
              onClick={() => setShowPassword(!showPassword)}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 transition-colors"
            >
              {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
          <p className="mt-1.5 text-xs text-zinc-500">
            Optional for local access. Recommended if exposed to the internet.
          </p>
        </div>

        {/* Vault Password */}
        <div className="rounded-xl border border-zinc-700/30 bg-zinc-800/20 p-4">
          <div className="flex items-start gap-3">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-orange-500/10">
              <KeyRound className="h-4 w-4 text-orange-400" />
            </div>
            <div className="flex-1">
              <h3 className="text-sm font-medium text-white">Vault</h3>
              <p className="mt-1 text-xs text-zinc-400">
                Your credentials are encrypted (AES-256). Uses the same password as the Web UI by default.
              </p>

              <div className="mt-3 flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => {
                    if (useCustomVault) {
                      updateData({ vaultPassword: data.webuiPassword })
                    } else {
                      updateData({ vaultPassword: '' })
                    }
                  }}
                  className={`relative h-5 w-9 rounded-full transition-colors ${
                    useCustomVault ? 'bg-orange-500' : 'bg-zinc-700'
                  }`}
                >
                  <span className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${
                    useCustomVault ? 'translate-x-4' : 'translate-x-0.5'
                  }`} />
                </button>
                <span className="text-xs text-zinc-400">Use a different password for the vault</span>
              </div>

              {useCustomVault && (
                <div className="mt-3">
                  <div className="relative">
                    <input
                      type={showVault ? 'text' : 'password'}
                      value={data.vaultPassword}
                      onChange={(e) => updateData({ vaultPassword: e.target.value })}
                      placeholder="Vault password (AES-256 + Argon2id)"
                      className="flex h-10 w-full rounded-lg border border-zinc-700/50 bg-zinc-900/50 px-3 pr-10 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-orange-500/50 focus:ring-2 focus:ring-orange-500/10"
                    />
                    <button
                      type="button"
                      onClick={() => setShowVault(!showVault)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 transition-colors"
                    >
                      {showVault ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                    </button>
                  </div>
                </div>
              )}

              {data.webuiPassword && (
                <div className="mt-2 flex items-start gap-1.5">
                  <Info className="mt-0.5 h-3 w-3 shrink-0 text-emerald-400/60" />
                  <p className="text-[11px] text-emerald-400/60">
                    API key will be saved to the vault automatically.
                  </p>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Access mode */}
        <div>
          <label className="mb-3 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Shield className="h-3.5 w-3.5 text-zinc-500" />
            Access mode
          </label>
          <div className="space-y-2.5">
            {MODES.map((mode) => {
              const colors = COLOR_MAP[mode.color as keyof typeof COLOR_MAP]
              const isActive = data.accessMode === mode.value
              const Icon = mode.icon

              return (
                <button
                  key={mode.value}
                  onClick={() => updateData({ accessMode: mode.value })}
                  className={`flex w-full items-start gap-4 rounded-xl border px-4 py-3.5 text-left transition-all ${
                    isActive
                      ? colors.active
                      : 'border-zinc-700/50 bg-zinc-800/30 hover:border-zinc-600 hover:bg-zinc-800/60'
                  }`}
                >
                  <div className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${
                    isActive ? 'bg-white/5' : 'bg-zinc-800'
                  }`}>
                    <Icon className={`h-4 w-4 ${isActive ? colors.icon : 'text-zinc-500'}`} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-white">{mode.label}</span>
                      {isActive && (
                        <div className={`h-1.5 w-1.5 rounded-full ${colors.dot}`} />
                      )}
                    </div>
                    <p className="mt-0.5 text-xs text-zinc-400">{mode.description}</p>
                  </div>
                </button>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}
