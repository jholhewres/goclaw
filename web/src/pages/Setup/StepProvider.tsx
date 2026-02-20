import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, XCircle, Loader2, Key, Cpu, ExternalLink, Link } from 'lucide-react'
import { api } from '@/lib/api'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

/* ── Provider SVG Icons ── */

const OpenAIIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.119 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135l-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5z"/>
  </svg>
)

const AnthropicIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M13.827 3.52h3.603L24 20.48h-3.603l-6.57-16.96zm-7.258 0h3.767L16.906 20.48h-3.674l-1.343-3.461H5.017l-1.344 3.46H.001L6.569 3.52zm2.327 5.364L6.723 14.98h4.404L8.896 8.884z"/>
  </svg>
)

const GoogleIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
    <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
    <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
    <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
  </svg>
)

const ZAiIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M3 4h18v2.5H8.5L21 17.5V20H3v-2.5h12.5L3 6.5V4z"/>
  </svg>
)

const XAiIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M3.005 3L10.065 12.53L3 21h1.586l6.18-7.41L16.044 21H21L13.58 10.98L20.2 3h-1.586l-5.735 6.886L7.961 3H3.005zM5.196 4.215h2.446l9.166 12.57h-2.446L5.196 4.215z"/>
  </svg>
)

const GroqIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 15v-4H7l6-8v4h4l-6 8z"/>
  </svg>
)

const OpenRouterIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <circle cx="12" cy="12" r="3"/>
    <path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 18a8 8 0 1 1 0-16 8 8 0 0 1 0 16z"/>
    <path d="M12 6v2M12 16v2M6 12h2M16 12h2"/>
  </svg>
)

const OllamaIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z"/>
    <circle cx="9" cy="10" r="1.5"/>
    <circle cx="15" cy="10" r="1.5"/>
    <path d="M12 17.5c2.33 0 4.31-1.46 5.11-3.5H6.89c.8 2.04 2.78 3.5 5.11 3.5z"/>
  </svg>
)

const MiniMaxIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
    <path d="M2 12l4-8h4l-4 8 4 8H6L2 12zm8 0l4-8h4l-4 8 4 8h-4l-4-8zm8 0l4-8h2l-4 8 4 8h-2l-4-8z"/>
  </svg>
)

const CustomIcon = () => (
  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="12" r="3"/>
    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
  </svg>
)

/* ── Provider definitions ── */

interface BaseUrlOption {
  value: string
  label: string
  extraModels?: string[]
}

interface ProviderDef {
  value: string
  label: string
  icon: React.FC
  models: string[]
  description: string
  keyPlaceholder: string
  noKey?: boolean
  baseUrls?: BaseUrlOption[]
  customBaseUrl?: boolean
}

const PROVIDERS: ProviderDef[] = [
  {
    value: 'openai',
    label: 'OpenAI',
    icon: OpenAIIcon,
    models: ['gpt-5.3-codex', 'gpt-5.2-instant', 'gpt-5.2-thinking', 'o3', 'o4-mini', 'o3-pro', 'gpt-4.1', 'gpt-4.1-mini', 'gpt-4.1-nano'],
    description: 'GPT-5, o3, o4-mini',
    keyPlaceholder: 'sk-...',
  },
  {
    value: 'anthropic',
    label: 'Anthropic',
    icon: AnthropicIcon,
    models: ['claude-opus-4.6', 'claude-opus-4.5', 'claude-sonnet-4.5', 'claude-haiku-4.5', 'claude-sonnet-4-20250514'],
    description: 'Opus 4.6, Sonnet 4.5',
    keyPlaceholder: 'sk-ant-...',
    baseUrls: [
      { value: '', label: 'Anthropic (default)' },
      { value: 'https://api.z.ai/api/anthropic', label: 'Z.Ai Anthropic Proxy', extraModels: ['glm-5', 'glm-4.7', 'glm-4.7-flash', 'glm-4.7-flashx'] },
    ],
  },
  {
    value: 'google',
    label: 'Google',
    icon: GoogleIcon,
    models: ['gemini-3-pro', 'gemini-3-flash', 'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.0-flash'],
    description: 'Gemini 3, 2.5',
    keyPlaceholder: 'AIza...',
  },
  {
    value: 'zai',
    label: 'Z.Ai',
    icon: ZAiIcon,
    models: ['glm-5', 'glm-4.7', 'glm-4.7-flash', 'glm-4.7-flashx'],
    description: 'GLM-5, GLM-4.7',
    keyPlaceholder: 'Your Z.Ai API key',
    baseUrls: [
      { value: 'https://api.z.ai/api/paas/v4', label: 'Global' },
      { value: 'https://open.bigmodel.cn/api/paas/v4', label: 'China' },
      { value: 'https://api.z.ai/api/coding/paas/v4', label: 'Coding (Global)' },
      { value: 'https://open.bigmodel.cn/api/coding/paas/v4', label: 'Coding (China)' },
    ],
  },
  {
    value: 'xai',
    label: 'xAI',
    icon: XAiIcon,
    models: ['grok-4', 'grok-4.1-fast', 'grok-3', 'grok-3-mini'],
    description: 'Grok 4, 4.1 Fast',
    keyPlaceholder: 'xai-...',
  },
  {
    value: 'groq',
    label: 'Groq',
    icon: GroqIcon,
    models: ['llama-4-scout-17b-16e-instruct', 'llama-4-maverick-17b-128e-instruct', 'meta-llama/llama-4-scout-17b-16e-instruct', 'deepseek-r1-distill-llama-70b', 'qwen-qwq-32b'],
    description: 'Llama 4, DeepSeek R1',
    keyPlaceholder: 'gsk_...',
  },
  {
    value: 'openrouter',
    label: 'OpenRouter',
    icon: OpenRouterIcon,
    models: [],
    description: '400+ models',
    keyPlaceholder: 'sk-or-...',
  },
  {
    value: 'minimax',
    label: 'MiniMax',
    icon: MiniMaxIcon,
    models: ['MiniMax-M2.5', 'MiniMax-M2.5-Lightning', 'MiniMax-M2.1', 'MiniMax-VL-01'],
    description: 'M2.5, M2.1',
    keyPlaceholder: 'Your MiniMax API key',
  },
  {
    value: 'ollama',
    label: 'Ollama',
    icon: OllamaIcon,
    models: [],
    description: 'Local models',
    keyPlaceholder: '',
    noKey: true,
  },
  {
    value: 'custom',
    label: 'Custom',
    icon: CustomIcon,
    models: [],
    description: 'OpenAI-compatible',
    keyPlaceholder: 'Your API key',
    customBaseUrl: true,
  },
]

export function StepProvider({ data, updateData }: Props) {
  const { t } = useTranslation()
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; error?: string } | null>(null)

  const provider = PROVIDERS.find((p) => p.value === data.provider)

  const activeEndpoint = provider?.baseUrls?.find((ep) => ep.value === data.baseUrl)
  const visibleModels = [
    ...(provider?.models ?? []),
    ...(activeEndpoint?.extraModels ?? []),
  ]

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const result = await api.setup.testProvider(data.provider, data.apiKey, data.model, data.baseUrl)
      setTestResult(result)
    } catch (err) {
      setTestResult({ success: false, error: err instanceof Error ? err.message : t('setupPage.connectionFailed') })
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-white">{t('setupPage.providerTitle')}</h2>
        <p className="mt-1 text-sm text-zinc-400">
          {t('setupPage.providerDesc')}
        </p>
      </div>

      <div className="space-y-5">
        {/* Provider selector */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Cpu className="h-3.5 w-3.5 text-zinc-500" />
            {t('setupPage.provider')}
          </label>
          <div className="grid grid-cols-3 gap-2">
            {PROVIDERS.map((p) => {
              const Icon = p.icon
              const isActive = data.provider === p.value
              return (
                <button
                  key={p.value}
                  onClick={() => {
                    updateData({ provider: p.value, model: '', baseUrl: '' })
                    setTestResult(null)
                  }}
                  className={`flex cursor-pointer flex-col items-center gap-1.5 rounded-xl border px-2 py-3 text-center transition-all ${
                    isActive
                      ? 'border-zinc-500 bg-zinc-800 ring-1 ring-zinc-500/50'
                      : 'border-zinc-700/50 bg-zinc-900 hover:border-zinc-600 hover:bg-zinc-800'
                  }`}
                >
                  <div className={isActive ? 'text-zinc-200' : 'text-zinc-400'}>
                    <Icon />
                  </div>
                  <span className="text-[11px] font-medium text-zinc-300">{p.label}</span>
                </button>
              )
            })}
          </div>
        </div>

        {/* Endpoint selector */}
        {provider?.baseUrls && (
          <div>
            <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
              <Link className="h-3.5 w-3.5 text-zinc-500" />
              {t('setupPage.endpoint')}
            </label>
            <div className="grid grid-cols-2 gap-2">
              {provider.baseUrls.map((ep) => {
                const isActive = data.baseUrl === ep.value
                return (
                  <button
                    key={ep.value}
                    onClick={() => updateData({ baseUrl: ep.value })}
                    className={`cursor-pointer rounded-lg border px-3 py-2 text-left text-xs transition-all ${
                      isActive
                        ? 'border-zinc-500 bg-zinc-800 ring-1 ring-zinc-500/50'
                        : 'border-zinc-700/50 bg-zinc-900 hover:border-zinc-600 hover:bg-zinc-800'
                    }`}
                  >
                    <span className="font-medium text-zinc-200">{ep.label}</span>
                    {ep.value && (
                      <p className="mt-0.5 truncate text-[10px] text-zinc-500 font-mono">
                        {ep.value.replace('https://', '')}
                      </p>
                    )}
                  </button>
                )
              })}
            </div>
          </div>
        )}

        {/* Custom base URL */}
        {provider?.customBaseUrl && (
          <div>
            <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
              <Link className="h-3.5 w-3.5 text-zinc-500" />
              Base URL
            </label>
            <input
              value={data.baseUrl}
              onChange={(e) => updateData({ baseUrl: e.target.value })}
              placeholder="https://api.example.com/v1"
              className="h-11 w-full rounded-xl border border-zinc-700 bg-zinc-900 px-4 font-mono text-sm text-zinc-100 placeholder:text-zinc-500 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
            />
            <p className="mt-1.5 text-xs text-zinc-500">
              OpenAI-compatible endpoint (<code className="text-zinc-400">/v1/chat/completions</code>)
            </p>
          </div>
        )}

        {/* API Key */}
        {!provider?.noKey && (
          <div>
            <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
              <Key className="h-3.5 w-3.5 text-zinc-500" />
              {t('setupPage.apiKey')}
            </label>
            <input
              type="password"
              value={data.apiKey}
              onChange={(e) => {
                updateData({ apiKey: e.target.value })
                setTestResult(null)
              }}
              placeholder={provider?.keyPlaceholder || t('setupPage.apiKey')}
              className="h-11 w-full rounded-xl border border-zinc-700 bg-zinc-900 px-4 text-sm text-zinc-100 placeholder:text-zinc-500 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
            />
            <p className="mt-1.5 text-xs text-zinc-500">
              {t('setupPage.apiKeyHint')}
            </p>
          </div>
        )}

        {/* Model */}
        <div>
          <label className="mb-2 flex items-center gap-2 text-sm font-medium text-zinc-300">
            <Cpu className="h-3.5 w-3.5 text-zinc-500" />
            {t('setupPage.model')}
          </label>
          {visibleModels.length > 0 ? (
            <select
              value={data.model}
              onChange={(e) => updateData({ model: e.target.value })}
              className="h-11 w-full cursor-pointer rounded-xl border border-zinc-700 bg-zinc-900 px-4 text-sm text-zinc-100 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
            >
              <option value="">{t('setupPage.selectModel')}</option>
              {activeEndpoint?.extraModels && activeEndpoint.extraModels.length > 0 && (
                <optgroup label={activeEndpoint.label}>
                  {activeEndpoint.extraModels.map((m) => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </optgroup>
              )}
              {provider && (
                <optgroup label={provider.label}>
                  {provider.models.map((m) => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </optgroup>
              )}
            </select>
          ) : (
            <input
              value={data.model}
              onChange={(e) => updateData({ model: e.target.value })}
              placeholder={t('setupPage.modelName')}
              className="h-11 w-full rounded-xl border border-zinc-700 bg-zinc-900 px-4 text-sm text-zinc-100 placeholder:text-zinc-500 outline-none transition-all hover:border-zinc-600 focus:border-zinc-600 focus:ring-2 focus:ring-zinc-500/20"
            />
          )}
        </div>

        {/* Test connection */}
        <div className="flex items-center gap-3">
          <button
            onClick={handleTest}
            disabled={testing || (!data.apiKey && !provider?.noKey) || !data.model}
            className="flex cursor-pointer items-center gap-2 rounded-xl border border-zinc-700 bg-zinc-900 px-4 py-2.5 text-sm font-medium text-zinc-300 transition-all hover:border-zinc-600 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {testing ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <ExternalLink className="h-3.5 w-3.5" />
            )}
            {t('setupPage.testConnection')}
          </button>

          {testResult && (
            <div className="flex items-center gap-1.5 text-sm">
              {testResult.success ? (
                <span className="flex items-center gap-1.5 text-zinc-300">
                  <CheckCircle2 className="h-4 w-4" /> {t('setupPage.connected')}
                </span>
              ) : (
                <span className="flex items-center gap-1.5 text-red-400">
                  <XCircle className="h-4 w-4" /> {testResult.error}
                </span>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
