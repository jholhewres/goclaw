import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Shield,
  UserPlus,
  X,
  UserCheck,
  UserX,
  AlertTriangle,
} from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigSelect,
  ConfigTextarea,
  ConfigActions,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface AccessConfig {
  default_policy: string
  owners: string[]
  admins: string[]
  allowed_users: string[]
  blocked_users: string[]
  pending_message: string
}

const POLICIES = [
  { value: 'deny', label: 'Deny (blocked by default)' },
  { value: 'allow', label: 'Allow (allowed by default)' },
  { value: 'ask', label: 'Ask (pending approval)' },
]

// Colored tag list for access control
function ColoredTagList({ tags, onRemove, color = 'blue' }: {
  tags: string[]
  onRemove: (tag: string) => void
  color?: 'blue' | 'green' | 'red' | 'yellow'
}) {
  const colorClasses = {
    blue: 'bg-[#3b82f6]/10 text-[#3b82f6] border-[#3b82f6]/20',
    green: 'bg-[#22c55e]/10 text-[#22c55e] border-[#22c55e]/20',
    red: 'bg-[#ef4444]/10 text-[#f87171] border-[#ef4444]/20',
    yellow: 'bg-[#f59e0b]/10 text-[#f59e0b] border-[#f59e0b]/20',
  }

  if (tags.length === 0) {
    return <p className="text-sm text-[#64748b]">None</p>
  }

  return (
    <div className="flex flex-wrap gap-2">
      {tags.map((tag) => (
        <span
          key={tag}
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-medium border ${colorClasses[color]}`}
        >
          {tag}
          <button
            onClick={() => onRemove(tag)}
            className="hover:opacity-70 cursor-pointer"
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
    </div>
  )
}

// Add tag input
function AddTagInput({ onAdd, placeholder }: {
  onAdd: (tag: string) => void
  placeholder: string
}) {
  const [value, setValue] = useState('')

  const handleAdd = () => {
    if (value.trim()) {
      onAdd(value.trim())
      setValue('')
    }
  }

  return (
    <div className="flex gap-2">
      <input
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), handleAdd())}
        placeholder={placeholder}
        className="h-10 flex-1 rounded-lg border border-white/10 bg-[#111827] px-3 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50"
      />
      <button
        onClick={handleAdd}
        className="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-[#1e293b] border border-white/10 text-sm text-[#94a3b8] hover:text-[#f8fafc] hover:border-white/20 transition-all cursor-pointer"
      >
        <UserPlus className="h-4 w-4" />
        Add
      </button>
    </div>
  )
}

export function Access() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<AccessConfig | null>(null)
  const [original, setOriginal] = useState<AccessConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    loadConfig()
  }, [])

  const loadConfig = async () => {
    try {
      const rawData = await api.config.get() as unknown as Partial<AccessConfig>
      // Ensure all arrays exist with defaults
      const data: AccessConfig = {
        default_policy: rawData.default_policy || 'deny',
        owners: rawData.owners || [],
        admins: rawData.admins || [],
        allowed_users: rawData.allowed_users || [],
        blocked_users: rawData.blocked_users || [],
        pending_message: rawData.pending_message || "You don't have permission to use this assistant. Please contact the administrator.",
      }
      setConfig(data)
      setOriginal(JSON.parse(JSON.stringify(data)))
    } catch {
      setLoadError(true)
    } finally {
      setLoading(false)
    }
  }

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original)

  const updateArray = (key: keyof AccessConfig, value: string) => {
    if (!config) return
    const arr = config[key] as string[]
    if (!arr.includes(value)) {
      setConfig({ ...config, [key]: [...arr, value] })
    }
  }

  const removeFromArray = (key: keyof AccessConfig, value: string) => {
    if (!config) return
    const arr = config[key] as string[]
    setConfig({ ...config, [key]: arr.filter(v => v !== value) })
  }

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      await api.config.update({
        access: {
          default_policy: config.default_policy,
          owners: config.owners,
          admins: config.admins,
          allowed_users: config.allowed_users,
          blocked_users: config.blocked_users,
          pending_message: config.pending_message,
        }
      })
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
      title={t('access.title')}
      subtitle={t('access.subtitle')}
      description={t('access.description')}
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
      {/* Default Policy */}
      <ConfigSection
        icon={Shield}
        title={t('access.policySection')}
        description={t('access.policySectionDesc')}
      >
        <ConfigField label={t('access.defaultPolicy')} hint={t('access.defaultPolicyHint')}>
          <ConfigSelect
            value={config.default_policy}
            onChange={(v) => setConfig(prev => prev ? { ...prev, default_policy: v } : prev)}
            options={POLICIES}
          />
        </ConfigField>

        <ConfigField label={t('access.pendingMessage')} hint={t('access.pendingMessageHint')}>
          <ConfigTextarea
            value={config.pending_message}
            onChange={(v) => setConfig(prev => prev ? { ...prev, pending_message: v } : prev)}
            placeholder="You don't have permission to use this assistant. Please contact the administrator."
            rows={3}
          />
        </ConfigField>
      </ConfigSection>

      {/* Owners */}
      <ConfigSection
        icon={UserCheck}
        title={t('access.owners')}
        description={t('access.ownersDesc')}
        iconColor="#22c55e"
      >
        <ColoredTagList tags={config.owners} onRemove={(v) => removeFromArray('owners', v)} color="green" />
        <AddTagInput onAdd={(v) => updateArray('owners', v)} placeholder={t('access.addOwnerPlaceholder')} />
      </ConfigSection>

      {/* Admins */}
      <ConfigSection
        icon={Shield}
        title={t('access.admins')}
        description={t('access.adminsDesc')}
        iconColor="#3b82f6"
      >
        <ColoredTagList tags={config.admins} onRemove={(v) => removeFromArray('admins', v)} color="blue" />
        <AddTagInput onAdd={(v) => updateArray('admins', v)} placeholder={t('access.addAdminPlaceholder')} />
      </ConfigSection>

      {/* Allowed Users */}
      <ConfigSection
        icon={UserPlus}
        title={t('access.allowedUsers')}
        description={t('access.allowedUsersDesc')}
        iconColor="#f59e0b"
      >
        <ColoredTagList tags={config.allowed_users} onRemove={(v) => removeFromArray('allowed_users', v)} color="yellow" />
        <AddTagInput onAdd={(v) => updateArray('allowed_users', v)} placeholder={t('access.addUserPlaceholder')} />
      </ConfigSection>

      {/* Blocked Users */}
      <ConfigSection
        icon={UserX}
        title={t('access.blockedUsers')}
        description={t('access.blockedUsersDesc')}
        iconColor="#f87171"
      >
        <ColoredTagList tags={config.blocked_users} onRemove={(v) => removeFromArray('blocked_users', v)} color="red" />
        <AddTagInput onAdd={(v) => updateArray('blocked_users', v)} placeholder={t('access.addBlockedPlaceholder')} />
      </ConfigSection>

      {/* Warning */}
      <div className="mb-10">
        <div className="flex items-start gap-3 rounded-xl border border-[#f59e0b]/20 bg-[#f59e0b]/5 p-4">
          <AlertTriangle className="h-5 w-5 text-[#f59e0b] flex-shrink-0 mt-0.5" />
          <div>
            <h3 className="text-sm font-semibold text-[#f59e0b]">{t('access.warning')}</h3>
            <p className="text-xs text-[#94a3b8] mt-1">{t('access.warningDesc')}</p>
          </div>
        </div>
      </div>
    </ConfigPage>
  )
}
