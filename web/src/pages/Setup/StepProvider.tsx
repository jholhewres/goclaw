import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, XCircle, Key, Cpu, ExternalLink, Link } from 'lucide-react'
import { api } from '@/lib/api'
import type { SetupData } from './SetupWizard'
import {
  StepContainer, StepHeader, FieldGroup, Field,
  Input, PasswordInput, Select, Button,
} from './SetupComponents'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

/* ── Provider SVG Icons ── */

const ProviderIcons = {
  openai: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.119 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135l-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5z"/>
    </svg>
  ),
  anthropic: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M13.827 3.52h3.603L24 20.48h-3.603l-6.57-16.96zm-7.258 0h3.767L16.906 20.48h-3.674l-1.343-3.461H5.017l-1.344 3.46H.001L6.569 3.52zm2.327 5.364L6.723 14.98h4.404L8.896 8.884z"/>
    </svg>
  ),
  google: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
    </svg>
  ),
  zai: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M3 4h18v2.5H8.5L21 17.5V20H3v-2.5h12.5L3 6.5V4z"/>
    </svg>
  ),
  xai: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M3.005 3L10.065 12.53L3 21h1.586l6.18-7.41L16.044 21H21L13.58 10.98L20.2 3h-1.586l-5.735 6.886L7.961 3H3.005zM5.196 4.215h2.446l9.166 12.57h-2.446L5.196 4.215z"/>
    </svg>
  ),
  groq: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 15v-4H7l6-8v4h4l-6 8z"/>
    </svg>
  ),
  cerebras: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M12 2L2 7v10l10 5 10-5V7L12 2zm0 2.5L19 8v8l-7 3.5L5 16V8l7-3.5z"/>
      <path d="M12 7l-4 2v6l4 2 4-2V9l-4-2zm0 2l2 1v4l-2 1-2-1v-4l2-1z"/>
    </svg>
  ),
  mistral: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <rect x="2" y="2" width="4" height="4" rx="0.5"/>
      <rect x="8" y="2" width="4" height="4" rx="0.5"/>
      <rect x="14" y="2" width="4" height="4" rx="0.5"/>
      <rect x="2" y="8" width="4" height="4" rx="0.5"/>
      <rect x="8" y="8" width="4" height="4" rx="0.5"/>
      <rect x="14" y="8" width="4" height="4" rx="0.5"/>
      <rect x="8" y="14" width="4" height="4" rx="0.5"/>
      <rect x="2" y="18" width="4" height="4" rx="0.5"/>
      <rect x="8" y="18" width="4" height="4" rx="0.5"/>
      <rect x="14" y="18" width="4" height="4" rx="0.5"/>
    </svg>
  ),
  openrouter: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <circle cx="12" cy="12" r="3"/>
      <path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 18a8 8 0 1 1 0-16 8 8 0 0 1 0 16z"/>
      <path d="M12 6v2M12 16v2M6 12h2M16 12h2"/>
    </svg>
  ),
  minimax: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M2 12l4-8h4l-4 8 4 8H6L2 12zm8 0l4-8h4l-4 8 4 8h-4l-4-8zm8 0l4-8h2l-4 8 4 8h-2l-4-8z"/>
    </svg>
  ),
  ollama: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z"/>
      <circle cx="9" cy="10" r="1.5"/>
      <circle cx="15" cy="10" r="1.5"/>
      <path d="M12 17.5c2.33 0 4.31-1.46 5.11-3.5H6.89c.8 2.04 2.78 3.5 5.11 3.5z"/>
    </svg>
  ),
  huggingface: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1.5 6c.83 0 1.5.67 1.5 1.5S11.33 11 10.5 11 9 10.33 9 9.5 9.67 8 10.5 8zm3 0c.83 0 1.5.67 1.5 1.5S14.33 11 13.5 11 12 10.33 12 9.5 12.67 8 13.5 8zM12 18c-2.33 0-4.31-1.46-5.11-3.5h10.22c-.8 2.04-2.78 3.5-5.11 3.5z"/>
    </svg>
  ),
  lmstudio: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-7 14l-5-5 1.41-1.41L12 14.17l4.59-4.58L18 11l-6 6z"/>
    </svg>
  ),
  vllm: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor">
      <path d="M12 2L2 19h20L12 2zm0 4l6.5 11h-13L12 6z"/>
    </svg>
  ),
  custom: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  ),
}

/* ── Provider definitions ── */

interface BaseUrlOption {
  value: string
  label: string
  extraModels?: string[]
}

interface ProviderDef {
  value: string
  label: string
  models: string[]
  description: string
  keyPlaceholder: string
  noKey?: boolean
  baseUrls?: BaseUrlOption[]
  customBaseUrl?: boolean
  freeUrl?: string
  freeNote?: string
  isFree?: boolean
  isLocal?: boolean
}

const PROVIDERS: ProviderDef[] = [
  // ── Free Online Providers ──
  {
    value: 'google',
    label: 'Google',
    models: [
      'gemini-2.5-pro-preview', 'gemini-2.5-flash', 'gemini-2.5-flash-lite',
      'gemini-2.0-flash', 'gemini-2.0-flash-lite', 'gemini-2.0-flash-thinking',
      'gemini-1.5-pro', 'gemini-1.5-flash',
    ],
    description: '1M tokens/min',
    keyPlaceholder: 'AIza...',
    isFree: true,
    freeUrl: 'https://aistudio.google.com/apikey',
    freeNote: '1M tokens/min, 1.5K req/day',
  },
  {
    value: 'groq',
    label: 'Groq',
    models: [
      'llama-3.3-70b-versatile', 'llama-3.3-70b-specdec',
      'llama-3.1-8b-instant', 'llama-3.1-70b-versatile',
      'llama-3.2-1b-preview', 'llama-3.2-3b-preview', 'llama-3.2-11b-vision-preview', 'llama-3.2-90b-vision-preview',
      'mixtral-8x7b-32768', 'gemma2-9b-it',
      'deepseek-r1-distill-llama-70b',
    ],
    description: 'Fastest inference',
    keyPlaceholder: 'gsk_...',
    isFree: true,
    freeUrl: 'https://console.groq.com/keys',
    freeNote: '6K tokens/min, ultra fast',
  },
  {
    value: 'cerebras',
    label: 'Cerebras',
    models: [
      'llama-4-maverick-17b-128e-instruct', 'llama-4-scout-17b-16e-instruct',
      'llama-3.3-70b', 'llama-3.1-8b',
      'deepseek-r1-distill-llama-70b',
    ],
    description: '1M tokens/day',
    keyPlaceholder: 'csk-...',
    isFree: true,
    freeUrl: 'https://cloud.cerebras.ai',
    freeNote: '1M tokens/day, 30 req/min',
  },
  {
    value: 'mistral',
    label: 'Mistral',
    models: [
      'mistral-large-latest', 'mistral-medium-latest',
      'codestral-latest', 'codestral-mamba',
      'ministral-8b-latest', 'ministral-3b-latest',
      'open-mistral-7b', 'open-mixtral-8x7b', 'open-mixtral-8x22b',
    ],
    description: '1M tokens/month',
    keyPlaceholder: 'API key',
    isFree: true,
    freeUrl: 'https://console.mistral.ai/api-keys',
    freeNote: '1M tokens/month',
  },
  {
    value: 'openrouter',
    label: 'OpenRouter',
    models: [
      'openrouter/free', 'openrouter/auto',
      'meta-llama/llama-3.3-70b-instruct:free',
      'deepseek/deepseek-r1:free',
      'google/gemma-3-27b-it:free',
      'qwen/qwen-2.5-72b-instruct:free',
    ],
    description: '50 req/day, 400+ models',
    keyPlaceholder: 'sk-or-...',
    isFree: true,
    freeUrl: 'https://openrouter.ai/keys',
    freeNote: '50 req/day free tier',
  },
  // ── Paid Providers ──
  {
    value: 'openai',
    label: 'OpenAI',
    models: ['gpt-5', 'gpt-5-mini', 'gpt-5-nano', 'gpt-5.2', 'gpt-5.2-instant', 'gpt-5.2-thinking', 'o3', 'o3-pro', 'o4-mini', 'gpt-4.1', 'gpt-4.1-mini', 'gpt-4.1-nano'],
    description: 'GPT-5, o3',
    keyPlaceholder: 'sk-...',
    freeUrl: 'https://platform.openai.com/api-keys',
  },
  {
    value: 'anthropic',
    label: 'Anthropic',
    models: ['claude-opus-4.6', 'claude-opus-4.5', 'claude-opus-4.1-20250805', 'claude-sonnet-4.5-20250929', 'claude-haiku-4.5-20251001', 'claude-sonnet-4-20250514'],
    description: 'Opus 4.6, Sonnet 4.5',
    keyPlaceholder: 'sk-ant-...',
    baseUrls: [
      { value: '', label: 'Anthropic (default)' },
      { value: 'https://api.z.ai/api/anthropic', label: 'Z.Ai Proxy', extraModels: ['glm-5', 'glm-4.7', 'glm-4.7-flash', 'glm-4.7-flashx'] },
    ],
    freeUrl: 'https://console.anthropic.com/settings/keys',
  },
  {
    value: 'zai',
    label: 'Z.Ai',
    models: ['glm-5', 'glm-4.7', 'glm-4.7-flash', 'glm-4.7-flashx'],
    description: 'GLM-5, GLM-4.7',
    keyPlaceholder: 'API key',
    baseUrls: [
      { value: 'https://api.z.ai/api/paas/v4', label: 'Global' },
      { value: 'https://open.bigmodel.cn/api/paas/v4', label: 'China' },
    ],
    freeUrl: 'https://open.bigmodel.cn',
  },
  {
    value: 'xai',
    label: 'xAI',
    models: ['grok-4-0709', 'grok-4.1-fast', 'grok-3', 'grok-3-mini'],
    description: 'Grok 4, 4.1',
    keyPlaceholder: 'xai-...',
    freeUrl: 'https://console.x.ai',
  },
  {
    value: 'minimax',
    label: 'MiniMax',
    models: ['MiniMax-Text-01', 'MiniMax-M2.5', 'MiniMax-M2.5-Lightning', 'MiniMax-VL-01'],
    description: 'Text-01, M2.5',
    keyPlaceholder: 'API key',
    freeUrl: 'https://www.minimaxi.com',
  },
  // ── Local / Self-Hosted ──
  {
    value: 'ollama',
    label: 'Ollama',
    models: [],
    description: 'Run models locally',
    keyPlaceholder: '',
    noKey: true,
    isLocal: true,
    freeUrl: 'https://ollama.com/download',
    freeNote: 'No API key needed, runs on your machine',
  },
  {
    value: 'huggingface',
    label: 'HuggingFace',
    models: [],
    description: 'Inference API (use org/model format)',
    keyPlaceholder: 'hf_...',
    isLocal: true,
    freeUrl: 'https://huggingface.co/settings/tokens',
    freeNote: 'Use model format: organization/model-name',
  },
  {
    value: 'lmstudio',
    label: 'LM Studio',
    models: [],
    description: 'Local GUI for LLMs',
    keyPlaceholder: '',
    noKey: true,
    isLocal: true,
    customBaseUrl: true,
    freeUrl: 'https://lmstudio.ai',
    freeNote: 'Runs locally, any model from HuggingFace',
  },
  {
    value: 'vllm',
    label: 'vLLM',
    models: [],
    description: 'High-performance serving',
    keyPlaceholder: '',
    noKey: true,
    isLocal: true,
    customBaseUrl: true,
    freeUrl: 'https://github.com/vllm-project/vllm',
    freeNote: 'Self-hosted, GPU required',
  },
  {
    value: 'custom',
    label: 'Custom',
    models: [],
    description: 'OpenAI-compatible API',
    keyPlaceholder: 'API key (optional)',
    isLocal: true,
    customBaseUrl: true,
  },
]

export function StepProvider({ data, updateData }: Props) {
  const { t } = useTranslation()
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; error?: string } | null>(null)

  const provider = PROVIDERS.find((p) => p.value === data.provider)
  const activeEndpoint = provider?.baseUrls?.find((ep) => ep.value === data.baseUrl)
  const visibleModels = [...(provider?.models ?? []), ...(activeEndpoint?.extraModels ?? [])]

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

  const handleProviderChange = (value: string) => {
    updateData({ provider: value, model: '', baseUrl: '' })
    setTestResult(null)
  }

  const handleApiKeyChange = (value: string) => {
    updateData({ apiKey: value })
    setTestResult(null)
  }

  // Separate free, paid, and local providers
  const freeProviders = PROVIDERS.filter(p => p.isFree && !p.isLocal)
  const paidProviders = PROVIDERS.filter(p => !p.isFree && !p.isLocal)
  const localProviders = PROVIDERS.filter(p => p.isLocal)

  return (
    <StepContainer>
      <StepHeader
        title={t('setupPage.providerTitle')}
        description={t('setupPage.providerDesc')}
      />

      <FieldGroup>
        {/* Free Providers */}
        <Field label={t('setupPage.freeProviders')} icon={Cpu}>
          <div className="grid grid-cols-5 gap-1.5">
            {freeProviders.map((p) => (
              <button
                key={p.value}
                onClick={() => handleProviderChange(p.value)}
                className={`flex cursor-pointer flex-col items-center gap-1 rounded-xl border px-2 py-2.5 text-center transition-all ${
                  data.provider === p.value
                    ? 'border-[#22c55e]/50 bg-[#22c55e]/10'
                    : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
                }`}
                title={p.description}
              >
                <div className={data.provider === p.value ? 'text-[#22c55e]' : 'text-[#64748b]'}>
                  {ProviderIcons[p.value as keyof typeof ProviderIcons]}
                </div>
                <span className={`text-[10px] font-medium ${data.provider === p.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
                  {p.label}
                </span>
              </button>
            ))}
          </div>
        </Field>

        {/* Paid Providers */}
        <Field label={t('setupPage.paidProviders')} icon={Cpu}>
          <div className="grid grid-cols-5 gap-1.5">
            {paidProviders.map((p) => (
              <button
                key={p.value}
                onClick={() => handleProviderChange(p.value)}
                className={`flex cursor-pointer flex-col items-center gap-1 rounded-xl border px-2 py-2.5 text-center transition-all ${
                  data.provider === p.value
                    ? 'border-[#3b82f6]/50 bg-[#3b82f6]/10'
                    : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
                }`}
                title={p.description}
              >
                <div className={data.provider === p.value ? 'text-[#3b82f6]' : 'text-[#64748b]'}>
                  {ProviderIcons[p.value as keyof typeof ProviderIcons]}
                </div>
                <span className={`text-[10px] font-medium ${data.provider === p.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
                  {p.label}
                </span>
              </button>
            ))}
          </div>
        </Field>

        {/* Local / Self-Hosted Providers */}
        <Field label={t('setupPage.localProviders')} icon={Cpu}>
          <div className="grid grid-cols-5 gap-1.5">
            {localProviders.map((p) => (
              <button
                key={p.value}
                onClick={() => handleProviderChange(p.value)}
                className={`flex cursor-pointer flex-col items-center gap-1 rounded-xl border px-2 py-2.5 text-center transition-all ${
                  data.provider === p.value
                    ? 'border-[#a855f7]/50 bg-[#a855f7]/10'
                    : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
                }`}
                title={p.description}
              >
                <div className={data.provider === p.value ? 'text-[#a855f7]' : 'text-[#64748b]'}>
                  {ProviderIcons[p.value as keyof typeof ProviderIcons]}
                </div>
                <span className={`text-[10px] font-medium ${data.provider === p.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
                  {p.label}
                </span>
              </button>
            ))}
          </div>
        </Field>

        {/* Provider info with link */}
        {provider && provider.freeUrl && (
          <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0c1222] px-3 py-2">
            <div className="flex-1">
              <p className="text-xs text-[#94a3b8]">
                {provider.freeNote || provider.description}
              </p>
            </div>
            <a
              href={provider.freeUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 text-xs text-[#3b82f6] hover:text-[#60a5fa] transition-colors"
            >
              {t('setupPage.getApiKey')}
              <ExternalLink className="h-3 w-3" />
            </a>
          </div>
        )}

        {/* Endpoint selector */}
        {provider?.baseUrls && (
          <Field label={t('setupPage.endpoint')} icon={Link}>
            <div className="grid grid-cols-2 gap-2">
              {provider.baseUrls.map((ep) => (
                <button
                  key={ep.value}
                  onClick={() => updateData({ baseUrl: ep.value })}
                  className={`cursor-pointer rounded-xl border px-3 py-2.5 text-left transition-all ${
                    data.baseUrl === ep.value
                      ? 'border-[#3b82f6]/50 bg-[#3b82f6]/10'
                      : 'border-white/10 bg-[#0c1222] hover:border-white/20 hover:bg-[#111827]'
                  }`}
                >
                  <span className={`text-xs font-medium ${data.baseUrl === ep.value ? 'text-[#f8fafc]' : 'text-[#94a3b8]'}`}>
                    {ep.label}
                  </span>
                  {ep.value && (
                    <p className="mt-0.5 truncate text-[10px] text-[#64748b] font-mono">
                      {ep.value.replace('https://', '')}
                    </p>
                  )}
                </button>
              ))}
            </div>
          </Field>
        )}

        {/* Custom base URL */}
        {provider?.customBaseUrl && (
          <Field label="Base URL" icon={Link} hint="OpenAI-compatible endpoint (/v1/chat/completions)">
            <Input
              value={data.baseUrl}
              onChange={(val) => updateData({ baseUrl: val })}
              placeholder="https://api.example.com/v1"
              mono
            />
          </Field>
        )}

        {/* API Key */}
        {!provider?.noKey && (
          <Field label={t('setupPage.apiKey')} icon={Key} hint={t('setupPage.apiKeyHint')}>
            <PasswordInput
              value={data.apiKey}
              onChange={handleApiKeyChange}
              placeholder={provider?.keyPlaceholder || t('setupPage.apiKey')}
            />
          </Field>
        )}

        {/* Model */}
        <Field label={t('setupPage.model')} icon={Cpu}>
          {visibleModels.length > 0 ? (
            <Select
              value={data.model}
              onChange={(val) => updateData({ model: val })}
              placeholder={t('setupPage.selectModel')}
              groups={[
                ...(activeEndpoint?.extraModels ? [{
                  label: activeEndpoint.label,
                  options: activeEndpoint.extraModels.map(m => ({ value: m, label: m })),
                }] : []),
                ...(provider ? [{
                  label: provider.label,
                  options: provider.models.map(m => ({ value: m, label: m })),
                }] : []),
              ]}
            />
          ) : (
            <Input
              value={data.model}
              onChange={(val) => updateData({ model: val })}
              placeholder={t('setupPage.modelName')}
            />
          )}
        </Field>

        {/* Test connection */}
        <div className="flex items-center gap-3 pt-1">
          <Button
            onClick={handleTest}
            disabled={testing || (!data.apiKey && !provider?.noKey) || !data.model}
            loading={testing}
            icon={ExternalLink}
          >
            {t('setupPage.testConnection')}
          </Button>

          {testResult && (
            <div className="flex items-center gap-1.5 text-sm">
              {testResult.success ? (
                <span className="flex items-center gap-1.5 text-[#22c55e]">
                  <CheckCircle2 className="h-4 w-4" /> {t('setupPage.connected')}
                </span>
              ) : (
                <span className="flex items-center gap-1.5 text-[#f87171]">
                  <XCircle className="h-4 w-4" /> {testResult.error}
                </span>
              )}
            </div>
          )}
        </div>
      </FieldGroup>
    </StepContainer>
  )
}
