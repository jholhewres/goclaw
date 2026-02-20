import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, ArrowRight, ArrowLeft, Sparkles, Loader2 } from 'lucide-react'
import { StepIdentity } from './StepIdentity'
import { StepProvider } from './StepProvider'
import { StepSecurity } from './StepSecurity'

export interface SetupData {
  /* Step 1: Identity */
  name: string
  language: string
  timezone: string

  /* Step 2: Provider */
  provider: string
  apiKey: string
  model: string
  baseUrl: string

  /* Step 3: Security */
  ownerPhone: string
  webuiPassword: string
  vaultPassword: string
  accessMode: 'relaxed' | 'strict' | 'paranoid'
  channels: Record<string, boolean>
  enabledSkills: string[]
}

const INITIAL_DATA: SetupData = {
  name: 'DevClaw',
  language: 'pt-BR',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  provider: 'openai',
  apiKey: '',
  model: '',
  baseUrl: '',
  ownerPhone: '',
  webuiPassword: '',
  vaultPassword: '',
  accessMode: 'strict',
  channels: {},
  enabledSkills: [],
}

/**
 * 3-step setup wizard with refined visual stepper.
 */
export function SetupWizard() {
  const { t } = useTranslation()
  const [step, setStep] = useState(1)
  const [data, setData] = useState<SetupData>(INITIAL_DATA)
  const [submitting, setSubmitting] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  const STEPS = [
    { id: 1, label: t('setupPage.steps.identity') },
    { id: 2, label: t('setupPage.steps.provider') },
    { id: 3, label: t('setupPage.steps.security') },
  ]

  const updateData = (partial: Partial<SetupData>) => {
    setData((prev) => ({ ...prev, ...partial }))
  }

  const next = () => setStep((s) => Math.min(s + 1, 3))
  const prev = () => setStep((s) => Math.max(s - 1, 1))

  const handleFinalize = async () => {
    setSubmitting(true)
    setError('')
    try {
      const res = await fetch('/api/setup/finalize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      })
      if (res.ok) {
        setDone(true)
      } else {
        const body = await res.json().catch(() => ({}))
        setError(body.error || t('setupPage.errorFailed'))
      }
    } catch {
      setError(t('setupPage.errorConnection'))
    } finally {
      setSubmitting(false)
    }
  }

  /* Post-setup success screen — polling for auto-redirect */
  if (done) {
    return <SetupComplete hasPassword={!!data.webuiPassword} />
  }

  return (
    <div>
      {/* Stepper */}
      <div className="mb-6">
        <div className="flex items-center justify-center gap-1">
          {STEPS.map(({ id, label }, index) => (
            <div key={id} className="flex items-center">
              {/* Step pill */}
              <button
                onClick={() => id < step && setStep(id)}
                disabled={id > step}
                className={`group flex items-center gap-2 rounded-full px-3 py-1.5 transition-all duration-300 ${
                  id === step
                    ? 'bg-[#3b82f6] text-white shadow-lg shadow-blue-500/25'
                    : id < step
                      ? 'bg-[#22c55e]/10 text-[#22c55e] cursor-pointer hover:bg-[#22c55e]/20'
                      : 'bg-[#1e293b]/50 text-[#64748b]'
                }`}
              >
                <span className={`flex h-5 w-5 items-center justify-center rounded-full text-[11px] font-bold ${
                  id === step
                    ? 'bg-white/20'
                    : id < step
                      ? 'bg-[#22c55e]/20'
                      : 'bg-[#1e293b]'
                }`}>
                  {id < step ? (
                    <CheckCircle2 className="h-3 w-3" />
                  ) : (
                    id
                  )}
                </span>
                <span className="text-xs font-medium hidden sm:block">{label}</span>
              </button>

              {/* Connector */}
              {index < STEPS.length - 1 && (
                <div className="relative mx-1 h-0.5 w-4 sm:w-6 overflow-hidden rounded-full bg-[#1e293b]">
                  <div
                    className="absolute inset-y-0 left-0 bg-[#22c55e] transition-all duration-500 ease-out rounded-full"
                    style={{ width: id < step ? '100%' : '0%' }}
                  />
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Current step */}
      <div className="min-h-[280px]">
        {step === 1 && <StepIdentity data={data} updateData={updateData} />}
        {step === 2 && <StepProvider data={data} updateData={updateData} />}
        {step === 3 && <StepSecurity data={data} updateData={updateData} />}
      </div>

      {/* Error */}
      {error && (
        <div className="mt-4 rounded-xl border border-[#ef4444]/20 bg-[#ef4444]/10 px-4 py-3 text-sm text-[#f87171]">
          {error}
        </div>
      )}

      {/* Navigation */}
      <div className="mt-6 flex items-center justify-between">
        <button
          onClick={prev}
          disabled={step === 1}
          className="flex cursor-pointer items-center gap-1.5 text-sm text-[#64748b] transition-colors hover:text-[#f8fafc] disabled:pointer-events-none disabled:opacity-0"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          {t('setupPage.back')}
        </button>

        <div className="flex items-center gap-4">
          {/* Step indicator */}
          <div className="flex gap-1.5">
            {STEPS.map(({ id }) => (
              <div
                key={id}
                className={`h-1.5 rounded-full transition-all duration-300 ${
                  id === step
                    ? 'w-5 bg-[#3b82f6]'
                    : id < step
                      ? 'w-1.5 bg-[#22c55e]'
                      : 'w-1.5 bg-[#1e293b]'
                }`}
              />
            ))}
          </div>

          {step < 3 ? (
            <button
              onClick={next}
              className="group flex cursor-pointer items-center gap-2 rounded-xl bg-[#f8fafc] px-5 py-2.5 text-sm font-semibold text-[#0f1419] shadow-lg shadow-white/5 transition-all hover:bg-white"
            >
              {t('setupPage.next')}
              <ArrowRight className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5" />
            </button>
          ) : (
            <button
              onClick={handleFinalize}
              disabled={submitting}
              className="group flex cursor-pointer items-center gap-2 rounded-xl bg-[#22c55e] px-5 py-2.5 text-sm font-semibold text-white shadow-lg shadow-green-500/25 transition-all hover:bg-[#16a34a] disabled:cursor-wait disabled:opacity-50"
            >
              {submitting ? (
                <>
                  <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                  {t('setupPage.settingUp')}
                </>
              ) : (
                <>
                  <Sparkles className="h-3.5 w-3.5" />
                  {t('setupPage.finish')}
                </>
              )}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

/**
 * Post-setup screen: shows progress while the server restarts,
 * then auto-redirects to the dashboard.
 */
function SetupComplete({ hasPassword }: { hasPassword: boolean }) {
  const { t } = useTranslation()
  const [phase, setPhase] = useState<'restarting' | 'ready'>('restarting')

  useEffect(() => {
    let cancelled = false
    let attempts = 0

    const poll = async () => {
      while (!cancelled && attempts < 60) {
        attempts++
        await new Promise((r) => setTimeout(r, 2000))
        try {
          const res = await fetch('/api/auth/status')
          if (res.ok) {
            setPhase('ready')
            await new Promise((r) => setTimeout(r, 1500))
            if (!cancelled) {
              window.location.href = hasPassword ? '/login' : '/'
            }
            return
          }
        } catch {
          // Server still restarting — keep polling
        }
      }
    }

    poll()
    return () => { cancelled = true }
  }, [hasPassword])

  return (
    <div className="flex flex-col items-center gap-4 py-4 text-center">
      <div className={`flex h-14 w-14 items-center justify-center rounded-full transition-all duration-500 ${
        phase === 'restarting'
          ? 'bg-[#1e293b]'
          : 'bg-[#22c55e]/10 ring-1 ring-[#22c55e]/30'
      }`}>
        {phase === 'restarting' ? (
          <Loader2 className="h-7 w-7 animate-spin text-[#64748b]" />
        ) : (
          <CheckCircle2 className="h-7 w-7 text-[#22c55e]" />
        )}
      </div>
      <div>
        <h2 className="text-lg font-semibold text-[#f8fafc]">
          {phase === 'restarting' ? t('setupPage.startingUp') : t('setupPage.allSet')}
        </h2>
        <p className="mt-1.5 text-sm text-[#94a3b8] max-w-sm">
          {phase === 'restarting'
            ? t('setupPage.restartingDesc')
            : t('setupPage.redirecting')
          }
        </p>
      </div>

      {/* Progress bar */}
      <div className="w-40 h-1 rounded-full bg-[#1e293b] overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-1000 ${
          phase === 'restarting'
            ? 'w-2/3 bg-[#3b82f6] animate-pulse'
            : 'w-full bg-[#22c55e]'
        }`} />
      </div>

      {hasPassword && phase === 'ready' && (
        <p className="text-xs text-[#64748b]">
          {t('setupPage.usePasswordHint')}
        </p>
      )}
    </div>
  )
}
