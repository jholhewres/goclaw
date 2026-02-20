import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, ArrowRight, ArrowLeft, Sparkles, Loader2 } from 'lucide-react'
import { StepIdentity } from './StepIdentity'
import { StepProvider } from './StepProvider'
import { StepSecurity } from './StepSecurity'
import { StepChannels } from './StepChannels'
import { StepSkills } from './StepSkills'

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

  /* Step 4: Channels */
  channels: Record<string, boolean>

  /* Step 5: Skills */
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
 * 5-step setup wizard with modern visual stepper.
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
    { id: 4, label: t('setupPage.steps.channels') },
    { id: 5, label: t('setupPage.steps.skills') },
  ]

  const updateData = (partial: Partial<SetupData>) => {
    setData((prev) => ({ ...prev, ...partial }))
  }

  const next = () => setStep((s) => Math.min(s + 1, 5))
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
      <div className="mb-8">
        <div className="flex items-center justify-between">
          {STEPS.map(({ id, label }, index) => (
            <div key={id} className="flex items-center">
              {/* Step circle + label */}
              <div className="flex flex-col items-center gap-1.5">
                <button
                  onClick={() => id < step && setStep(id)}
                  disabled={id > step}
                  className={`flex h-9 w-9 items-center justify-center rounded-full text-xs font-semibold transition-all duration-300 ${
                    id === step
                      ? 'bg-zinc-500 text-white shadow-lg shadow-zinc-900/10 ring-4 ring-zinc-500/20'
                      : id < step
                        ? 'bg-zinc-700 text-zinc-300 ring-1 ring-zinc-600 cursor-pointer hover:bg-zinc-600'
                        : 'bg-zinc-800/60 text-zinc-600 ring-1 ring-zinc-700/50'
                  }`}
                >
                  {id < step ? (
                    <CheckCircle2 className="h-4 w-4" />
                  ) : (
                    id
                  )}
                </button>
                <span className={`text-[11px] font-medium transition-colors ${
                  id === step
                    ? 'text-zinc-300'
                    : id < step
                      ? 'text-zinc-400'
                      : 'text-zinc-600'
                }`}>
                  {label}
                </span>
              </div>

              {/* Connector line */}
              {index < STEPS.length - 1 && (
                <div className="relative mx-2 mt-[-18px] h-0.5 w-8 sm:w-12 lg:w-16 overflow-hidden rounded-full bg-zinc-800">
                  <div
                    className="absolute inset-y-0 left-0 bg-zinc-500/60 transition-all duration-500 ease-out rounded-full"
                    style={{ width: id < step ? '100%' : '0%' }}
                  />
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Separator */}
      <div className="mb-6 h-px bg-gradient-to-r from-transparent via-zinc-700/50 to-transparent" />

      {/* Current step */}
      <div className="min-h-[300px]">
        {step === 1 && <StepIdentity data={data} updateData={updateData} />}
        {step === 2 && <StepProvider data={data} updateData={updateData} />}
        {step === 3 && <StepSecurity data={data} updateData={updateData} />}
        {step === 4 && <StepChannels data={data} updateData={updateData} />}
        {step === 5 && <StepSkills data={data} updateData={updateData} />}
      </div>

      {/* Error */}
      {error && (
        <div className="mt-3 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-2.5 text-sm text-red-400">
          {error}
        </div>
      )}

      {/* Navigation */}
      <div className="mt-8 flex items-center justify-between">
        <button
          onClick={prev}
          disabled={step === 1}
          className="flex cursor-pointer items-center gap-1.5 text-sm text-zinc-500 transition-colors hover:text-zinc-300 disabled:pointer-events-none disabled:opacity-0"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          {t('setupPage.back')}
        </button>

        <div className="flex items-center gap-4">
          {/* Dot indicators */}
          <div className="flex gap-1.5">
            {STEPS.map(({ id }) => (
              <div
                key={id}
                className={`h-1.5 rounded-full transition-all duration-300 ${
                  id === step
                    ? 'w-6 bg-zinc-400'
                    : id < step
                      ? 'w-1.5 bg-zinc-500'
                      : 'w-1.5 bg-zinc-700'
                }`}
              />
            ))}
          </div>

          {step < 5 ? (
            <button
              onClick={next}
              className="group flex cursor-pointer items-center gap-2 rounded-xl bg-zinc-100 px-5 py-2.5 text-sm font-medium text-zinc-900 shadow-lg shadow-zinc-900/10 transition-all hover:bg-zinc-200"
            >
              {t('setupPage.next')}
              <ArrowRight className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5" />
            </button>
          ) : (
            <button
              onClick={handleFinalize}
              disabled={submitting}
              className="group flex cursor-pointer items-center gap-2 rounded-xl bg-zinc-100 px-5 py-2.5 text-sm font-medium text-zinc-900 shadow-lg shadow-zinc-900/10 transition-all hover:bg-zinc-200 disabled:cursor-wait disabled:opacity-50"
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
        // Wait for server restart (pm2 auto-restarts)
        await new Promise((r) => setTimeout(r, 2000))
        try {
          const res = await fetch('/api/auth/status')
          if (res.ok) {
            setPhase('ready')
            // Short delay so the user sees the success message
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
    <div className="flex flex-col items-center gap-5 py-6 text-center">
      <div className="flex h-16 w-16 items-center justify-center rounded-full bg-zinc-800 ring-1 ring-zinc-700">
        {phase === 'restarting' ? (
          <Loader2 className="h-8 w-8 animate-spin text-zinc-400" />
        ) : (
          <CheckCircle2 className="h-8 w-8 text-zinc-300" />
        )}
      </div>
      <div>
        <h2 className="text-xl font-semibold text-white">
          {phase === 'restarting' ? t('setupPage.startingUp') : t('setupPage.allSet')}
        </h2>
        <p className="mt-2 text-sm text-zinc-400 max-w-sm">
          {phase === 'restarting'
            ? t('setupPage.restartingDesc')
            : t('setupPage.redirecting')
          }
        </p>
      </div>

      {/* Progress bar */}
      <div className="w-48 h-1 rounded-full bg-zinc-800 overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-1000 ${
          phase === 'restarting'
            ? 'w-2/3 bg-zinc-500 animate-pulse'
            : 'w-full bg-zinc-400'
        }`} />
      </div>

      {hasPassword && phase === 'ready' && (
        <p className="text-xs text-zinc-500">
          {t('setupPage.usePasswordHint')}
        </p>
      )}
    </div>
  )
}
