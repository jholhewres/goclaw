import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DollarSign,
  TrendingUp,
  AlertTriangle,
  CheckCircle2,
} from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigActions,
  ConfigCard,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface BudgetConfig {
  monthly_limit_usd: number
  warn_at_percent: number
  action_at_limit: string
}

interface UsageData {
  total_cost: number
  total_input_tokens: number
  total_output_tokens: number
  request_count: number
}

const ACTIONS = [
  { value: 'warn', label: 'Warn only (continue using)' },
  { value: 'block', label: 'Block requests' },
  { value: 'fallback_local', label: 'Fallback to local model' },
]

// Progress bar component
function ProgressBar({ percent, color }: { percent: number; color: string }) {
  return (
    <div className="h-3 rounded-full bg-[#1e293b] overflow-hidden">
      <div
        className="h-full rounded-full transition-all duration-500"
        style={{ width: `${percent}%`, backgroundColor: color }}
      />
    </div>
  )
}

// Stat card component
function StatCard({ label, value, prefix = '' }: { label: string; value: string | number; prefix?: string }) {
  return (
    <div className="p-4 rounded-xl bg-[#1e293b]/50">
      <p className="text-xs text-[#64748b] uppercase tracking-wide">{label}</p>
      <p className="text-xl font-semibold text-[#f8fafc] mt-1">
        {prefix}{typeof value === 'number' ? value.toLocaleString() : value}
      </p>
    </div>
  )
}

export function Budget() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<BudgetConfig | null>(null)
  const [original, setOriginal] = useState<BudgetConfig | null>(null)
  const [usage, setUsage] = useState<UsageData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    Promise.all([
      api.config.get(),
      api.usage(),
    ])
      .then(([configData, usageData]) => {
        const cfg = (configData as unknown as { budget?: BudgetConfig }).budget || {
          monthly_limit_usd: 100,
          warn_at_percent: 80,
          action_at_limit: 'warn',
        }
        setConfig(cfg)
        setOriginal(JSON.parse(JSON.stringify(cfg)))
        setUsage(usageData as unknown as UsageData)
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
        budget: {
          monthly_limit_usd: config.monthly_limit_usd,
          warn_at_percent: config.warn_at_percent,
          action_at_limit: config.action_at_limit,
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

  const getUsagePercent = () => {
    if (!config || !usage) return 0
    return Math.min(100, (usage.total_cost / config.monthly_limit_usd) * 100)
  }

  const getUsageColor = () => {
    const percent = getUsagePercent()
    if (percent >= 100) return '#ef4444'
    if (percent >= (config?.warn_at_percent || 80)) return '#f59e0b'
    return '#22c55e'
  }

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState onRetry={() => window.location.reload()} />

  const usagePercent = getUsagePercent()
  const usageColor = getUsageColor()
  const isOverBudget = usage && usage.total_cost >= config.monthly_limit_usd

  return (
    <ConfigPage
      title={t('budget.title')}
      subtitle={t('budget.subtitle')}
      description={t('budget.description')}
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
      {/* Usage Overview */}
      {usage && (
        <ConfigSection
          icon={TrendingUp}
          title={t('budget.usageSection')}
          description={t('budget.usageSectionDesc')}
        >
          {/* Progress Bar */}
          <div className="mb-6">
            <div className="flex justify-between items-center mb-2">
              <span className="text-sm text-[#94a3b8]">{t('budget.monthlyUsage')}</span>
              <span className="text-sm font-medium" style={{ color: usageColor }}>
                ${usage.total_cost.toFixed(2)} / ${config.monthly_limit_usd.toFixed(2)}
              </span>
            </div>
            <ProgressBar percent={usagePercent} color={usageColor} />
            <div className="flex justify-between items-center mt-2">
              <span className="text-xs text-[#64748b]">{usagePercent.toFixed(1)}%</span>
              {isOverBudget && (
                <span className="text-xs text-[#f87171] flex items-center gap-1">
                  <AlertTriangle className="h-3 w-3" />
                  {t('budget.overBudget')}
                </span>
              )}
            </div>
          </div>

          {/* Stats Grid */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <StatCard label={t('budget.totalCost')} value={usage.total_cost.toFixed(4)} prefix="$" />
            <StatCard label={t('budget.inputTokens')} value={usage.total_input_tokens} />
            <StatCard label={t('budget.outputTokens')} value={usage.total_output_tokens} />
            <StatCard label={t('budget.requests')} value={usage.request_count} />
          </div>
        </ConfigSection>
      )}

      {/* Budget Settings */}
      <ConfigSection
        icon={DollarSign}
        title={t('budget.settingsSection')}
        description={t('budget.settingsSectionDesc')}
      >
        <ConfigField label={t('budget.monthlyLimit')} hint={t('budget.monthlyLimitHint')}>
          <div className="relative">
            <span className="absolute left-4 top-1/2 -translate-y-1/2 text-sm text-[#64748b] z-10">$</span>
            <ConfigInput
              type="number"
              value={config.monthly_limit_usd}
              onChange={(v) => setConfig(prev => prev ? { ...prev, monthly_limit_usd: parseFloat(v) || 0 } : prev)}
              placeholder="100.00"
              className="pl-8"
            />
          </div>
        </ConfigField>

        <ConfigField label={t('budget.warnPercent')} hint={t('budget.warnPercentHint')}>
          <div className="relative">
            <ConfigInput
              type="number"
              value={config.warn_at_percent}
              onChange={(v) => setConfig(prev => prev ? { ...prev, warn_at_percent: parseInt(v) || 0 } : prev)}
              placeholder="80"
              className="pr-10"
            />
            <span className="absolute right-4 top-1/2 -translate-y-1/2 text-sm text-[#64748b]">%</span>
          </div>
        </ConfigField>

        <ConfigField label={t('budget.actionAtLimit')} hint={t('budget.actionAtLimitHint')}>
          <ConfigSelect
            value={config.action_at_limit}
            onChange={(v) => setConfig(prev => prev ? { ...prev, action_at_limit: v } : prev)}
            options={ACTIONS}
          />
        </ConfigField>
      </ConfigSection>

      {/* Status Card */}
      <ConfigCard
        title={t('budget.statusTitle')}
        icon={isOverBudget ? AlertTriangle : CheckCircle2}
        status={isOverBudget ? 'error' : usagePercent >= (config.warn_at_percent || 80) ? 'warning' : 'success'}
        className="mb-10"
        actions={
          <div className="text-right">
            <p className="text-xs text-[#64748b]">{t('budget.remainingBudget')}</p>
            <p className="text-sm font-medium text-[#f8fafc]">
              ${Math.max(0, config.monthly_limit_usd - (usage?.total_cost || 0)).toFixed(2)}
            </p>
          </div>
        }
      >
        <p className={`text-sm ${
          isOverBudget
            ? 'text-[#f87171]'
            : usagePercent >= (config.warn_at_percent || 80)
              ? 'text-[#f59e0b]'
              : 'text-[#22c55e]'
        }`}>
          {isOverBudget
            ? t('budget.statusOver')
            : usagePercent >= (config.warn_at_percent || 80)
              ? t('budget.statusWarning')
              : t('budget.statusOk')
          }
        </p>
      </ConfigCard>
    </ConfigPage>
  )
}
