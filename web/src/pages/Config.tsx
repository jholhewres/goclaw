import { useEffect, useState, useCallback } from 'react'
import { Save, RotateCcw, Image, Mic, Eye, EyeOff, ChevronDown } from 'lucide-react'
import { api } from '@/lib/api'

interface MediaConfig {
  vision_enabled: boolean
  vision_model: string
  vision_detail: string
  transcription_enabled: boolean
  transcription_model: string
  transcription_base_url: string
  transcription_api_key: boolean | string
  transcription_language: string
}

interface ConfigData {
  name: string
  trigger: string
  model: string
  language: string
  timezone: string
  provider: string
  base_url: string
  media: MediaConfig
}

const VISION_MODELS: Record<string, { label: string; models: { value: string; label: string }[] }> = {
  'Z.AI': {
    label: 'Z.AI',
    models: [
      { value: 'glm-4.6v', label: 'GLM-4.6V (flagship, 128K, tool use)' },
      { value: 'glm-4.6v-flashx', label: 'GLM-4.6V-FlashX (lightweight)' },
      { value: 'glm-4.6v-flash', label: 'GLM-4.6V-Flash (free)' },
      { value: 'glm-4.5v', label: 'GLM-4.5V (106B MOE, thinking mode)' },
    ],
  },
  'OpenAI': {
    label: 'OpenAI',
    models: [
      { value: 'gpt-4o', label: 'GPT-4o (best quality)' },
      { value: 'gpt-4o-mini', label: 'GPT-4o Mini (fast, cheap)' },
      { value: 'gpt-4.1', label: 'GPT-4.1 (latest)' },
      { value: 'gpt-4.1-mini', label: 'GPT-4.1 Mini' },
    ],
  },
  'Anthropic': {
    label: 'Anthropic',
    models: [
      { value: 'claude-sonnet-4-20250514', label: 'Claude Sonnet 4' },
      { value: 'claude-opus-4-20250514', label: 'Claude Opus 4' },
      { value: 'claude-haiku-3-5-20241022', label: 'Claude Haiku 3.5' },
    ],
  },
  'Google': {
    label: 'Google',
    models: [
      { value: 'gemini-3-pro', label: 'Gemini 3 Pro' },
      { value: 'gemini-3-flash', label: 'Gemini 3 Flash' },
      { value: 'gemini-2.5-pro', label: 'Gemini 2.5 Pro' },
    ],
  },
}

const TRANSCRIPTION_PRESETS: { value: string; label: string; base_url: string }[] = [
  { value: 'glm-asr-2512', label: 'Z.AI — GLM-ASR-2512 (multilingual, CER 0.07)', base_url: 'https://api.z.ai/api/paas/v4' },
  { value: 'whisper-1', label: 'OpenAI — Whisper 1 (legacy)', base_url: 'https://api.openai.com/v1' },
  { value: 'gpt-4o-transcribe', label: 'OpenAI — GPT-4o Transcribe (best)', base_url: 'https://api.openai.com/v1' },
  { value: 'gpt-4o-mini-transcribe', label: 'OpenAI — GPT-4o Mini Transcribe', base_url: 'https://api.openai.com/v1' },
  { value: 'whisper-large-v3', label: 'Groq — Whisper Large V3 (fast, 189x)', base_url: 'https://api.groq.com/openai/v1' },
  { value: 'whisper-large-v3-turbo', label: 'Groq — Whisper Large V3 Turbo', base_url: 'https://api.groq.com/openai/v1' },
]

function Toggle({ enabled, onChange, label }: { enabled: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!enabled)}
      className="flex items-center gap-3 group cursor-pointer"
    >
      <div className={`relative h-6 w-11 rounded-full transition-colors ${enabled ? 'bg-orange-500' : 'bg-zinc-700'}`}>
        <div className={`absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform ${enabled ? 'translate-x-5' : ''}`} />
      </div>
      <span className="text-sm text-zinc-300 group-hover:text-white transition-colors">{label}</span>
    </button>
  )
}

function Select({ value, onChange, options, placeholder }: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string; group?: string }[]
  placeholder?: string
}) {
  return (
    <div className="relative">
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full appearance-none rounded-xl border border-white/8 bg-dc-dark px-4 py-3 pr-10 text-sm text-zinc-200 outline-none transition-colors hover:border-white/15 focus:border-orange-500/50"
      >
        {placeholder && <option value="">{placeholder}</option>}
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
    </div>
  )
}

function Input({ value, onChange, placeholder, type = 'text' }: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}) {
  const [show, setShow] = useState(false)

  if (type === 'password') {
    return (
      <div className="relative">
        <input
          type={show ? 'text' : 'password'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="w-full rounded-xl border border-white/8 bg-dc-dark px-4 py-3 pr-10 text-sm text-zinc-200 outline-none transition-colors placeholder:text-zinc-600 hover:border-white/15 focus:border-orange-500/50"
        />
        <button
          type="button"
          onClick={() => setShow(!show)}
          className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 cursor-pointer"
        >
          {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      </div>
    )
  }

  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full rounded-xl border border-white/8 bg-dc-dark px-4 py-3 text-sm text-zinc-200 outline-none transition-colors placeholder:text-zinc-600 hover:border-white/15 focus:border-orange-500/50"
    />
  )
}

export function Config() {
  const [config, setConfig] = useState<ConfigData | null>(null)
  const [original, setOriginal] = useState<ConfigData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [transcriptionApiKey, setTranscriptionApiKey] = useState('')

  useEffect(() => {
    api.config.get()
      .then((data) => {
        const d = data as unknown as ConfigData
        setConfig(d)
        setOriginal(JSON.parse(JSON.stringify(d)))
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original) || transcriptionApiKey !== ''

  const updateMedia = useCallback((key: keyof MediaConfig, value: unknown) => {
    setConfig((prev) => prev ? { ...prev, media: { ...prev.media, [key]: value } } : prev)
  }, [])

  const handleTranscriptionPreset = useCallback((model: string) => {
    const preset = TRANSCRIPTION_PRESETS.find((p) => p.value === model)
    if (preset) {
      setConfig((prev) => prev ? {
        ...prev,
        media: {
          ...prev.media,
          transcription_model: preset.value,
          transcription_base_url: preset.base_url,
        },
      } : prev)
    }
  }, [])

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      const payload: Record<string, unknown> = {
        media: {
          ...config.media,
          transcription_api_key: transcriptionApiKey || undefined,
        },
      }
      const result = await api.config.update(payload) as unknown as ConfigData
      setConfig(result)
      setOriginal(JSON.parse(JSON.stringify(result)))
      setTranscriptionApiKey('')
      setMessage({ type: 'success', text: 'Configuração salva com sucesso' })
    } catch {
      setMessage({ type: 'error', text: 'Erro ao salvar configuração' })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    if (original) {
      setConfig(JSON.parse(JSON.stringify(original)))
      setTranscriptionApiKey('')
    }
    setMessage(null)
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  if (loadError || !config) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center bg-dc-darker">
        <p className="text-sm text-red-400">Erro ao carregar configuração</p>
        <button onClick={() => window.location.reload()} className="mt-3 text-xs text-orange-400 hover:text-orange-300 transition-colors cursor-pointer">
          Tentar novamente
        </button>
      </div>
    )
  }

  const allVisionModels = Object.entries(VISION_MODELS).flatMap(([, group]) =>
    group.models.map((m) => ({ ...m, group: group.label }))
  )

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-dc-darker">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">Sistema</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Configuração</h1>
            <p className="mt-2 text-base text-zinc-500">
              {config.name} &middot; {config.provider} &middot; {config.model}
            </p>
          </div>
          <div className="flex items-center gap-3">
            {hasChanges && (
              <button
                onClick={handleReset}
                className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/8 bg-dc-dark px-5 py-3 text-sm font-semibold text-zinc-400 transition-all hover:border-white/12 hover:text-white"
              >
                <RotateCcw className="h-4 w-4" />
                Desfazer
              </button>
            )}
            <button
              onClick={handleSave}
              disabled={!hasChanges || saving}
              className="flex cursor-pointer items-center gap-2 rounded-xl bg-linear-to-r from-orange-500 to-amber-500 px-5 py-3 text-sm font-bold text-white shadow-lg shadow-orange-500/20 transition-all hover:shadow-orange-500/30 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Save className="h-4 w-4" />
              {saving ? 'Salvando...' : 'Salvar'}
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

        {/* Vision Section */}
        <section className="mt-10">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-500/10 ring-1 ring-violet-500/20">
              <Image className="h-5 w-5 text-violet-400" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white">Compreensão de Imagens</h2>
              <p className="text-sm text-zinc-500">Entender imagens e vídeos recebidos nos canais</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/6 bg-dc-dark p-6">
            <Toggle
              enabled={config.media.vision_enabled}
              onChange={(v) => updateMedia('vision_enabled', v)}
              label="Ativar compreensão visual"
            />

            {config.media.vision_enabled && (
              <>
                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    Modelo de Visão
                  </label>
                  <Select
                    value={config.media.vision_model}
                    onChange={(v) => updateMedia('vision_model', v)}
                    placeholder="Usar modelo principal do chat"
                    options={allVisionModels}
                  />
                  <p className="mt-2 text-xs text-zinc-600">
                    Se vazio, usa o modelo principal ({config.model})
                  </p>
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    Qualidade
                  </label>
                  <Select
                    value={config.media.vision_detail}
                    onChange={(v) => updateMedia('vision_detail', v)}
                    options={[
                      { value: 'auto', label: 'Auto (recomendado)' },
                      { value: 'low', label: 'Baixa (rápido, menos tokens)' },
                      { value: 'high', label: 'Alta (detalhado, mais tokens)' },
                    ]}
                  />
                </div>
              </>
            )}
          </div>
        </section>

        {/* Transcription Section */}
        <section className="mt-8">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-500/10 ring-1 ring-blue-500/20">
              <Mic className="h-5 w-5 text-blue-400" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white">Transcrição de Áudio</h2>
              <p className="text-sm text-zinc-500">Converter áudios e notas de voz em texto</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/6 bg-dc-dark p-6">
            <Toggle
              enabled={config.media.transcription_enabled}
              onChange={(v) => updateMedia('transcription_enabled', v)}
              label="Ativar transcrição de áudio"
            />

            {config.media.transcription_enabled && (
              <>
                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    Modelo de Transcrição
                  </label>
                  <Select
                    value={config.media.transcription_model}
                    onChange={(v) => handleTranscriptionPreset(v)}
                    options={TRANSCRIPTION_PRESETS.map((p) => ({ value: p.value, label: p.label }))}
                  />
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    Base URL da API
                  </label>
                  <Input
                    value={config.media.transcription_base_url}
                    onChange={(v) => updateMedia('transcription_base_url', v)}
                    placeholder="https://api.openai.com/v1"
                  />
                  <p className="mt-2 text-xs text-zinc-600">
                    Preenchida automaticamente ao selecionar o modelo
                  </p>
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    Idioma Principal
                  </label>
                  <Select
                    value={config.media.transcription_language}
                    onChange={(v) => updateMedia('transcription_language', v)}
                    placeholder="Auto-detectar"
                    options={[
                      { value: 'pt', label: 'Português' },
                      { value: 'en', label: 'English' },
                      { value: 'es', label: 'Español' },
                      { value: 'fr', label: 'Français' },
                      { value: 'de', label: 'Deutsch' },
                      { value: 'it', label: 'Italiano' },
                      { value: 'ja', label: '日本語' },
                      { value: 'ko', label: '한국어' },
                      { value: 'zh', label: '中文' },
                    ]}
                  />
                  <p className="mt-2 text-xs text-zinc-600">
                    Ajuda o modelo a reconhecer o idioma correto
                  </p>
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    API Key de Transcrição
                  </label>
                  <Input
                    value={transcriptionApiKey}
                    onChange={setTranscriptionApiKey}
                    placeholder={config.media.transcription_api_key ? '••••••• (configurada)' : 'Usa a API key principal se vazio'}
                    type="password"
                  />
                  <p className="mt-2 text-xs text-zinc-600">
                    Necessário quando o provedor de transcrição é diferente do principal
                  </p>
                </div>
              </>
            )}
          </div>
        </section>

        {/* Info Section */}
        <section className="mt-8 mb-10">
          <div className="rounded-2xl border border-white/4 bg-dc-dark/50 p-6">
            <h3 className="text-sm font-bold text-zinc-400 mb-3">Modelos por Provedor</h3>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <p className="text-xs font-bold text-violet-400 mb-1">Visão (Imagens/Vídeo)</p>
                <ul className="space-y-1 text-xs text-zinc-500">
                  <li><span className="text-zinc-400">Z.AI:</span> GLM-4.6V, GLM-4.5V</li>
                  <li><span className="text-zinc-400">OpenAI:</span> GPT-4o, GPT-4.1</li>
                  <li><span className="text-zinc-400">Anthropic:</span> Claude Opus/Sonnet 4</li>
                  <li><span className="text-zinc-400">Google:</span> Gemini 3 Pro/Flash</li>
                </ul>
              </div>
              <div>
                <p className="text-xs font-bold text-blue-400 mb-1">Transcrição (Áudio)</p>
                <ul className="space-y-1 text-xs text-zinc-500">
                  <li><span className="text-zinc-400">Z.AI:</span> GLM-ASR-2512</li>
                  <li><span className="text-zinc-400">OpenAI:</span> Whisper, GPT-4o Transcribe</li>
                  <li><span className="text-zinc-400">Groq:</span> Whisper Large V3 (189x speed)</li>
                </ul>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
