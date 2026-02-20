import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, RotateCcw, Bot, Globe, ChevronDown } from 'lucide-react'
import { api } from '@/lib/api'

interface SystemConfig {
  name: string
  trigger: string
  language: string
  timezone: string
}

const LANGUAGES = [
  { value: 'pt-BR', label: 'Português (Brasil)' },
  { value: 'pt-PT', label: 'Português (Portugal)' },
  { value: 'en-US', label: 'English (US)' },
  { value: 'en-GB', label: 'English (UK)' },
  { value: 'es-ES', label: 'Español (España)' },
  { value: 'es-MX', label: 'Español (México)' },
  { value: 'fr-FR', label: 'Français' },
  { value: 'de-DE', label: 'Deutsch' },
  { value: 'it-IT', label: 'Italiano' },
  { value: 'ja-JP', label: '日本語' },
  { value: 'ko-KR', label: '한국어' },
  { value: 'zh-CN', label: '中文 (简体)' },
  { value: 'zh-TW', label: '中文 (繁體)' },
]

const TIMEZONES = [
  { value: 'America/Sao_Paulo', label: 'Brasília (GMT-3)' },
  { value: 'America/New_York', label: 'New York (GMT-5)' },
  { value: 'America/Los_Angeles', label: 'Los Angeles (GMT-8)' },
  { value: 'Europe/London', label: 'London (GMT+0)' },
  { value: 'Europe/Paris', label: 'Paris (GMT+1)' },
  { value: 'Europe/Berlin', label: 'Berlin (GMT+1)' },
  { value: 'Asia/Tokyo', label: 'Tokyo (GMT+9)' },
  { value: 'Asia/Shanghai', label: 'Shanghai (GMT+8)' },
  { value: 'Asia/Dubai', label: 'Dubai (GMT+4)' },
  { value: 'Australia/Sydney', label: 'Sydney (GMT+10)' },
  { value: 'UTC', label: 'UTC (GMT+0)' },
]

function Select({ value, onChange, options }: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="relative">
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full appearance-none rounded-xl border border-white/8 bg-dc-dark px-4 py-3 pr-10 text-sm text-zinc-200 outline-none transition-colors hover:border-white/15 focus:border-blue-500/50"
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
    </div>
  )
}

function Input({ value, onChange, placeholder }: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full rounded-xl border border-white/8 bg-dc-dark px-4 py-3 text-sm text-zinc-200 outline-none transition-colors placeholder:text-zinc-600 hover:border-white/15 focus:border-blue-500/50"
    />
  )
}

export function System() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<SystemConfig | null>(null)
  const [original, setOriginal] = useState<SystemConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    api.config.get()
      .then((data) => {
        const d = data as unknown as SystemConfig
        setConfig(d)
        setOriginal(JSON.parse(JSON.stringify(d)))
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original)

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      await api.config.update({
        name: config.name,
        trigger: config.trigger,
        language: config.language,
        timezone: config.timezone,
      })
      setOriginal(JSON.parse(JSON.stringify(config)))
      setMessage({ type: 'success', text: 'Configurações salvas com sucesso' })
    } catch {
      setMessage({ type: 'error', text: 'Erro ao salvar configurações' })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    if (original) {
      setConfig(JSON.parse(JSON.stringify(original)))
    }
    setMessage(null)
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-10 w-10 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  if (loadError || !config) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center bg-dc-darker">
        <p className="text-sm text-red-400">Erro ao carregar configurações</p>
        <button onClick={() => window.location.reload()} className="mt-3 text-xs text-blue-400 hover:text-blue-300 transition-colors cursor-pointer">
          Tentar novamente
        </button>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-dc-darker">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('system.subtitle')}</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('system.title')}</h1>
            <p className="mt-2 text-base text-zinc-500">
              {t('system.desc')}
            </p>
          </div>
          <div className="flex items-center gap-3">
            {hasChanges && (
              <button
                onClick={handleReset}
                className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/8 bg-dc-dark px-5 py-3 text-sm font-semibold text-zinc-400 transition-all hover:border-white/12 hover:text-white"
              >
                <RotateCcw className="h-4 w-4" />
                {t('common.reset')}
              </button>
            )}
            <button
              onClick={handleSave}
              disabled={!hasChanges || saving}
              className="flex cursor-pointer items-center gap-2 rounded-xl bg-blue-500 px-5 py-3 text-sm font-bold text-white shadow-lg shadow-blue-500/20 transition-all hover:shadow-blue-500/30 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Save className="h-4 w-4" />
              {saving ? t('common.saving') : t('common.save')}
            </button>
          </div>
        </div>

        {message && (
          <div className={`mt-6 rounded-2xl px-5 py-4 text-base ring-1 ${
            message.type === 'success'
              ? 'bg-emerald-500/5 text-emerald-400 ring-emerald-500/20'
              : 'bg-red-500/5 text-red-400 ring-red-500/20'
          }`}>
            {message.text}
          </div>
        )}

        {/* Identity Section */}
        <section className="mt-10">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-500/10 ring-1 ring-violet-500/20">
              <Bot className="h-5 w-5 text-violet-400" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white">{t('system.identity')}</h2>
              <p className="text-sm text-zinc-500">{t('system.identityDesc')}</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/6 bg-dc-dark p-6">
            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                {t('system.assistantName')}
              </label>
              <Input
                value={config.name}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, name: v } : prev)}
                placeholder="DevClaw"
              />
              <p className="mt-2 text-xs text-zinc-600">
                {t('system.assistantNameHint')}
              </p>
            </div>

            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                {t('system.trigger')}
              </label>
              <Input
                value={config.trigger}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, trigger: v } : prev)}
                placeholder="!devclaw"
              />
              <p className="mt-2 text-xs text-zinc-600">
                {t('system.triggerHint')}
              </p>
            </div>
          </div>
        </section>

        {/* Locale Section */}
        <section className="mt-10">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-500/10 ring-1 ring-blue-500/20">
              <Globe className="h-5 w-5 text-blue-400" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white">{t('system.localization')}</h2>
              <p className="text-sm text-zinc-500">{t('system.localizationDesc')}</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/6 bg-dc-dark p-6">
            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                {t('system.primaryLanguage')}
              </label>
              <Select
                value={config.language}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, language: v } : prev)}
                options={LANGUAGES}
              />
              <p className="mt-2 text-xs text-zinc-600">
                {t('system.primaryLanguageHint')}
              </p>
            </div>

            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                {t('system.timezone')}
              </label>
              <Select
                value={config.timezone}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, timezone: v } : prev)}
                options={TIMEZONES}
              />
              <p className="mt-2 text-xs text-zinc-600">
                {t('system.timezoneHint')}
              </p>
            </div>
          </div>
        </section>

        {/* Info */}
        <section className="mt-10 mb-10">
          <div className="rounded-2xl border border-white/4 bg-dc-dark/50 p-6">
            <h3 className="text-sm font-bold text-zinc-400 mb-3">{t('system.tips')}</h3>
            <ul className="space-y-2 text-xs text-zinc-500">
              <li>• {t('system.tip1')}</li>
              <li>• {t('system.tip2')}</li>
              <li>• {t('system.tip3')}</li>
              <li>• {t('system.tip4')}</li>
            </ul>
          </div>
        </section>
      </div>
    </div>
  )
}
