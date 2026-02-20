import { useTranslation } from 'react-i18next'
import { User, Globe, Clock } from 'lucide-react'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const LANGUAGES = [
  { value: 'pt-BR', label: 'Português (Brasil)', flag: '🇧🇷' },
  { value: 'en', label: 'English', flag: '🇺🇸' },
  { value: 'es', label: 'Español', flag: '🇪🇸' },
  { value: 'fr', label: 'Français', flag: '🇫🇷' },
  { value: 'de', label: 'Deutsch', flag: '🇩🇪' },
]

export function StepIdentity({ data, updateData }: Props) {
  const { t } = useTranslation()

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-white">{t('setupPage.identityTitle')}</h2>
        <p className="mt-1 text-sm text-zinc-400">
          {t('setupPage.identityDesc')}
        </p>
      </div>

      <div className="space-y-4">
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <User className="h-3.5 w-3.5 text-zinc-500" />
            {t('setupPage.assistantName')}
          </label>
          <input
            value={data.name}
            onChange={(e) => updateData({ name: e.target.value })}
            placeholder="DevClaw"
            className="h-11 w-full rounded-xl border border-zinc-700 bg-zinc-900 px-4 text-sm text-zinc-100 placeholder:text-zinc-500 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
          />
        </div>

        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Globe className="h-3.5 w-3.5 text-zinc-500" />
            {t('setupPage.language')}
          </label>
          <select
            value={data.language}
            onChange={(e) => updateData({ language: e.target.value })}
            className="h-11 w-full cursor-pointer rounded-xl border border-zinc-700 bg-zinc-900 px-4 text-sm text-zinc-100 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
          >
            {LANGUAGES.map((lang) => (
              <option key={lang.value} value={lang.value}>
                {lang.flag} {lang.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Clock className="h-3.5 w-3.5 text-zinc-500" />
            {t('setupPage.timezone')}
          </label>
          <input
            value={data.timezone}
            onChange={(e) => updateData({ timezone: e.target.value })}
            placeholder="America/Sao_Paulo"
            className="h-11 w-full rounded-xl border border-zinc-700 bg-zinc-900 px-4 text-sm text-zinc-100 placeholder:text-zinc-500 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
          />
          <p className="mt-1.5 text-xs text-zinc-500">{t('setupPage.timezoneHint')}</p>
        </div>
      </div>
    </div>
  )
}
