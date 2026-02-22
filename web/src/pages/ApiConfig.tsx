import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Eye,
  EyeOff,
  Key,
  CheckCircle2,
  XCircle,
  Loader2,
  Zap,
  Cpu,
  AlertTriangle,
  Bot,
  Sparkles,
  Cloud,
  Server,
  Globe,
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

// Provider configurations with default URLs
const PROVIDERS = [
  { id: 'openai', name: 'OpenAI', baseUrl: 'https://api.openai.com/v1', icon: Sparkles, color: '#10a37f' },
  { id: 'anthropic', name: 'Anthropic', baseUrl: 'https://api.anthropic.com/v1', icon: Bot, color: '#d97706' },
  { id: 'google', name: 'Google AI', baseUrl: 'https://generativelanguage.googleapis.com/v1beta', icon: Cloud, color: '#4285f4' },
  { id: 'zai', name: 'Z.AI', baseUrl: 'https://api.z.ai/v1', icon: Zap, color: '#8b5cf6' },
  { id: 'groq', name: 'Groq', baseUrl: 'https://api.groq.com/openai/v1', icon: Cpu, color: '#f55036' },
  { id: 'ollama', name: 'Ollama', baseUrl: 'http://localhost:11434/v1', icon: Server, color: '#6366f1' },
  { id: 'openrouter', name: 'OpenRouter', baseUrl: 'https://openrouter.ai/api/v1', icon: Globe, color: '#64748b' },
  { id: 'xai', name: 'xAI', baseUrl: 'https://api.x.ai/v1', icon: Zap, color: '#000000' },
  { id: 'custom', name: 'Custom', baseUrl: '', icon: Server, color: '#64748b' },
]

interface APIConfigData {
  provider: string
  base_url: string
  api_key_configured: boolean
  api_key_masked?: string
  model: string
  models?: string[]
}

interface ConnectionTestResult {
  success: boolean
  latency?: number
  error?: string
  models?: string[]
}

// Password input with toggle
function PasswordInput({ value, onChange, placeholder }: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  const [show, setShow] = useState(false)

  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-11 w-full rounded-xl border border-white/10 bg-[#111827] px-4 pr-10 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20"
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-[#64748b] hover:text-[#f8fafc] cursor-pointer"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

// Provider card component
function ProviderCard({ provider, isSelected, onClick }: {
  provider: typeof PROVIDERS[0]
  isSelected: boolean
  onClick: () => void
}) {
  const Icon = provider.icon
  return (
    <button
      onClick={onClick}
      className={`relative flex flex-col items-center justify-center gap-2 p-4 rounded-xl border transition-all ${
        isSelected
          ? 'border-[#3b82f6] bg-[#3b82f6]/10'
          : 'border-white/10 bg-[#111827] hover:border-white/20 hover:bg-[#1e293b]'
      }`}
    >
      {isSelected && (
        <div className="absolute top-2 right-2">
          <CheckCircle2 className="h-4 w-4 text-[#3b82f6]" />
        </div>
      )}
      <div
        className="flex h-10 w-10 items-center justify-center rounded-lg"
        style={{ backgroundColor: `${provider.color}20` }}
      >
        <Icon className="h-5 w-5" style={{ color: provider.color }} />
      </div>
      <span className={`text-sm font-medium ${isSelected ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
        {provider.name}
      </span>
    </button>
  )
}

export function ApiConfig() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<APIConfigData | null>(null)
  const [original, setOriginal] = useState<APIConfigData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [testingConnection, setTestingConnection] = useState(false)
  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null)
  const [availableModels, setAvailableModels] = useState<string[]>([])

  useEffect(() => {
    loadConfig()
  }, [])

  const loadConfig = async () => {
    try {
      const data = await api.config.get() as unknown as APIConfigData
      setConfig(data)
      setOriginal(JSON.parse(JSON.stringify(data)))
      if (data.models && data.models.length > 0) {
        setAvailableModels(data.models)
      }
    } catch {
      setLoadError(true)
    } finally {
      setLoading(false)
    }
  }

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original) || apiKey !== ''

  const handleProviderChange = useCallback((providerId: string) => {
    const provider = PROVIDERS.find(p => p.id === providerId)
    setConfig(prev => prev ? {
      ...prev,
      provider: providerId,
      base_url: provider?.baseUrl || prev.base_url,
    } : prev)
    setTestResult(null)
    setAvailableModels([])
  }, [])

  const handleTestConnection = async () => {
    if (!config) return
    setTestingConnection(true)
    setTestResult(null)
    setMessage(null)

    try {
      const result = await api.setup.testProvider(
        config.provider,
        apiKey || 'test',
        config.model,
        config.base_url
      )
      setTestResult({
        success: result.success,
        error: result.error,
      })
    } catch (error) {
      setTestResult({
        success: false,
        error: error instanceof Error ? error.message : 'Connection test failed',
      })
    } finally {
      setTestingConnection(false)
    }
  }

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      const payload: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
        base_url: config.base_url,
      }
      if (apiKey) {
        payload.api_key = apiKey
      }
      const result = await api.config.update(payload) as unknown as APIConfigData
      setConfig(result)
      setOriginal(JSON.parse(JSON.stringify(result)))
      setApiKey('')
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
      setApiKey('')
    }
    setMessage(null)
    setTestResult(null)
  }

  if (loading) return <LoadingSpinner />
  if (loadError || !config) return <ErrorState onRetry={() => window.location.reload()} />

  const selectedProvider = PROVIDERS.find(p => p.id === config.provider)

  return (
    <ConfigPage
      title={t('apiConfig.title')}
      subtitle={t('apiConfig.subtitle')}
      description={t('apiConfig.description')}
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
      {/* Provider Selection */}
      <ConfigSection
        icon={Cpu}
        title={t('apiConfig.providerSection')}
        description={t('apiConfig.providerSectionDesc')}
      >
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3 -mt-2">
          {PROVIDERS.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              isSelected={config.provider === provider.id}
              onClick={() => handleProviderChange(provider.id)}
            />
          ))}
        </div>
      </ConfigSection>

      {/* API Configuration */}
      <ConfigSection
        icon={Key}
        title={t('apiConfig.connectionSection')}
        description={t('apiConfig.connectionSectionDesc')}
      >
        <ConfigField label={t('apiConfig.baseUrl')} hint={t('apiConfig.baseUrlHint')}>
          <ConfigInput
            value={config.base_url}
            onChange={(v) => setConfig(prev => prev ? { ...prev, base_url: v } : prev)}
            placeholder={selectedProvider?.baseUrl || 'https://api.example.com/v1'}
          />
        </ConfigField>

        <ConfigField label={t('apiConfig.apiKey')} hint={t('apiConfig.apiKeyHint')}>
          <PasswordInput
            value={apiKey}
            onChange={setApiKey}
            placeholder={config.api_key_configured ? (config.api_key_masked || '••••••• (configured)') : t('apiConfig.apiKeyPlaceholder')}
          />
        </ConfigField>

        <ConfigField label={t('apiConfig.model')} hint={t('apiConfig.modelHint')}>
          {availableModels.length > 0 ? (
            <ConfigSelect
              value={config.model}
              onChange={(v) => setConfig(prev => prev ? { ...prev, model: v } : prev)}
              options={availableModels.map(m => ({ value: m, label: m }))}
              placeholder={t('apiConfig.selectModel')}
            />
          ) : (
            <ConfigInput
              value={config.model}
              onChange={(v) => setConfig(prev => prev ? { ...prev, model: v } : prev)}
              placeholder="gpt-4o-mini, claude-sonnet-4-20250514, gemini-2.5-pro"
            />
          )}
        </ConfigField>

        {/* Connection Test */}
        <div className="pt-4 border-t border-white/5">
          <div className="flex items-center justify-between">
            <button
              onClick={handleTestConnection}
              disabled={testingConnection || !config.base_url}
              className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-[#1e293b] px-4 py-2.5 text-sm font-medium text-[#94a3b8] transition-all hover:border-white/20 hover:text-[#f8fafc] disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {testingConnection ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t('apiConfig.testing')}
                </>
              ) : (
                <>
                  <Zap className="h-4 w-4" />
                  {t('apiConfig.testConnection')}
                </>
              )}
            </button>

            {/* Test Result */}
            {testResult && (
              <div className={`flex items-center gap-2 px-3 py-1.5 rounded-lg ${
                testResult.success
                  ? 'bg-[#22c55e]/10 text-[#22c55e]'
                  : 'bg-[#ef4444]/10 text-[#f87171]'
              }`}>
                {testResult.success ? (
                  <>
                    <CheckCircle2 className="h-4 w-4" />
                    <span className="text-sm font-medium">
                      {testResult.latency && `${testResult.latency}ms`}
                    </span>
                  </>
                ) : (
                  <>
                    <XCircle className="h-4 w-4" />
                    <span className="text-sm">{t('apiConfig.connectionFailed')}</span>
                  </>
                )}
              </div>
            )}
          </div>

          {/* Error Message */}
          {testResult && !testResult.success && testResult.error && (
            <div className="mt-3 flex items-start gap-2 rounded-lg bg-[#ef4444]/5 border border-[#ef4444]/10 p-3">
              <AlertTriangle className="h-4 w-4 text-[#f87171] flex-shrink-0 mt-0.5" />
              <p className="text-xs text-[#f87171]">{testResult.error}</p>
            </div>
          )}
        </div>
      </ConfigSection>

      {/* Status Card */}
      <ConfigCard
        title={t('apiConfig.statusTitle')}
        icon={config.api_key_configured ? CheckCircle2 : AlertTriangle}
        status={config.api_key_configured ? 'success' : 'warning'}
        className="mb-10"
        actions={
          <div className="text-right">
            <p className="text-xs text-[#64748b]">{t('apiConfig.currentProvider')}</p>
            <p className="text-sm font-medium text-[#f8fafc] capitalize">{config.provider}</p>
          </div>
        }
      >
        <p className={`text-sm ${config.api_key_configured ? 'text-[#22c55e]' : 'text-[#f59e0b]'}`}>
          {config.api_key_configured
            ? t('apiConfig.statusConfigured')
            : t('apiConfig.statusNotConfigured')
          }
        </p>
      </ConfigCard>
    </ConfigPage>
  )
}
