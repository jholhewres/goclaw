import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Users,
  MessageSquare,
  Bell,
  BellOff,
} from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigTextarea,
  ConfigToggle,
  ConfigActions,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface GroupsConfig {
  activation_mode: string
  intro_message: string
  max_participants: number
  quiet_hours_enabled: boolean
  quiet_hours_start: string
  quiet_hours_end: string
}

const ACTIVATION_MODES = [
  { value: 'always', label: 'Always (respond to all messages)' },
  { value: 'mention', label: 'Mention only (respond when mentioned)' },
  { value: 'reply', label: 'Reply only (respond when replied to)' },
]

export function Groups() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<GroupsConfig | null>(null)
  const [original, setOriginal] = useState<GroupsConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    api.config.get()
      .then((data) => {
        const groups = (data as unknown as { groups?: GroupsConfig }).groups || {
          activation_mode: 'mention',
          intro_message: "Hi! I'm {{name}}, your AI assistant. Mention me with {{trigger}} to get help!",
          max_participants: 100,
          quiet_hours_enabled: false,
          quiet_hours_start: '22:00',
          quiet_hours_end: '08:00',
        }
        setConfig(groups)
        setOriginal(JSON.parse(JSON.stringify(groups)))
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
      await api.config.update({ groups: config })
      setOriginal(JSON.parse(JSON.stringify(config)))
      setMessage({ type: 'success', text: t('common.success') })
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
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

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState onRetry={() => window.location.reload()} />

  return (
    <ConfigPage
      title={t('groups.title')}
      subtitle={t('groups.subtitle')}
      description={t('groups.description')}
      message={message}
      actions={
        <ConfigActions
          onSave={handleSave}
          onReset={handleReset}
          saving={saving}
          hasChanges={hasChanges}
          saveLabel={t('common.save')}
          savingLabel={t('common.saving')}
          resetLabel={t('common.reset')}
        />
      }
    >
      {/* Activation Mode */}
      <ConfigSection
        icon={MessageSquare}
        title={t('groups.activationSection')}
        description={t('groups.activationSectionDesc')}
      >
        <ConfigField label={t('groups.activationMode')} hint={t('groups.activationModeHint')}>
          <ConfigSelect
            value={config.activation_mode}
            onChange={(v) => setConfig(prev => prev ? { ...prev, activation_mode: v } : prev)}
            options={ACTIVATION_MODES}
          />
        </ConfigField>

        <ConfigField label={t('groups.maxParticipants')} hint={t('groups.maxParticipantsHint')}>
          <ConfigInput
            type="number"
            value={config.max_participants}
            onChange={(v) => setConfig(prev => prev ? { ...prev, max_participants: parseInt(v) || 100 } : prev)}
          />
        </ConfigField>
      </ConfigSection>

      {/* Intro Message */}
      <ConfigSection
        icon={Users}
        title={t('groups.introSection')}
        description={t('groups.introSectionDesc')}
      >
        <ConfigField label={t('groups.introMessage')} hint={t('groups.introMessageHint')}>
          <ConfigTextarea
            value={config.intro_message}
            onChange={(v) => setConfig(prev => prev ? { ...prev, intro_message: v } : prev)}
            placeholder="Hi! I'm {{name}}, your AI assistant. Mention me with {{trigger}} to get help!"
            rows={3}
          />
        </ConfigField>
      </ConfigSection>

      {/* Quiet Hours */}
      <ConfigSection
        icon={config.quiet_hours_enabled ? BellOff : Bell}
        title={t('groups.quietHoursSection')}
        description={t('groups.quietHoursSectionDesc')}
      >
        <ConfigToggle
          enabled={config.quiet_hours_enabled}
          onChange={(v) => setConfig(prev => prev ? { ...prev, quiet_hours_enabled: v } : prev)}
          label={t('groups.quietHoursEnabled')}
        />

        {config.quiet_hours_enabled && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
            <ConfigField label={t('groups.quietHoursStart')}>
              <ConfigInput
                type="time"
                value={config.quiet_hours_start}
                onChange={(v) => setConfig(prev => prev ? { ...prev, quiet_hours_start: v } : prev)}
              />
            </ConfigField>

            <ConfigField label={t('groups.quietHoursEnd')}>
              <ConfigInput
                type="time"
                value={config.quiet_hours_end}
                onChange={(v) => setConfig(prev => prev ? { ...prev, quiet_hours_end: v } : prev)}
              />
            </ConfigField>
          </div>
        )}

        {config.quiet_hours_enabled && (
          <p className="text-xs text-[#475569]">
            {t('groups.quietHoursHint')}
          </p>
        )}
      </ConfigSection>
    </ConfigPage>
  )
}
