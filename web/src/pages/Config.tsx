import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, RotateCcw, Image, Mic, Eye, EyeOff, Cpu } from 'lucide-react'
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
  api_key_configured: boolean
  media: MediaConfig
}

function Toggle({ enabled, onChange, label }: { enabled: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!enabled)}
      className="flex items-center gap-3 group cursor-pointer"
    >
      <div className={`relative h-6 w-11 rounded-full transition-colors ${enabled ? 'bg-[#3b82f6]' : 'bg-[#1e293b]'}`}>
        <div className={`absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform ${enabled ? 'translate-x-5' : ''}`} />
      </div>
      <span className="text-sm text-[#94a3b8] group-hover:text-[#f8fafc] transition-colors">{label}</span>
    </button>
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

  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="h-11 w-full rounded-xl border border-white/10 bg-[#111827] px-4 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20"
    />
  )
}

function Select({ value, onChange, options, placeholder }: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="h-11 w-full cursor-pointer appearance-none rounded-xl border border-white/10 bg-[#111827] px-4 pr-10 text-sm text-[#f8fafc] outline-none transition-all hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20"
    >
      {placeholder && <option value="">{placeholder}</option>}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>{opt.label}</option>
      ))}
    </select>
  )
}

export function Config() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ConfigData | null>(null)
  const [original, setOriginal] = useState<ConfigData | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [transcriptionApiKey, setTranscriptionApiKey] = useState('')
  const [mainApiKey, setMainApiKey] = useState('')

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

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original) || transcriptionApiKey !== '' || mainApiKey !== ''

  const updateMedia = useCallback((key: keyof MediaConfig, value: unknown) => {
    setConfig((prev) => prev ? { ...prev, media: { ...prev.media, [key]: value } } : prev)
  }, [])

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setMessage(null)
    try {
      const payload: Record<string, unknown> = {
        provider: config.provider,
        model: config.model,
        base_url: config.base_url,
        media: {
          ...config.media,
          transcription_api_key: transcriptionApiKey || undefined,
        },
      }
      if (mainApiKey) {
        payload.api_key = mainApiKey
      }
      const result = await api.config.update(payload) as unknown as ConfigData
      setConfig(result)
      setOriginal(JSON.parse(JSON.stringify(result)))
      setTranscriptionApiKey('')
      setMainApiKey('')
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
      setTranscriptionApiKey('')
      setMainApiKey('')
    }
    setMessage(null)
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
        <div className="h-10 w-10 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
      </div>
    )
  }

  if (loadError || !config) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center bg-[#0c1222]">
        <p className="text-sm text-[#f87171]">{t('common.error')}</p>
        <button onClick={() => window.location.reload()} className="mt-3 text-xs text-[#64748b] hover:text-[#f8fafc] transition-colors cursor-pointer">
          {t('common.loading')}
        </button>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-[#0c1222]">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{t('config.subtitle')}</p>
            <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">{t('config.title')}</h1>
            <p className="mt-2 text-base text-[#64748b]">
              {config.name}
            </p>
          </div>
          <div className="flex items-center gap-3">
            {hasChanges && (
              <button
                onClick={handleReset}
                className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-[#111827] px-5 py-3 text-sm font-medium text-[#94a3b8] transition-all hover:border-white/20 hover:text-[#f8fafc]"
              >
                <RotateCcw className="h-4 w-4" />
                Desfazer
              </button>
            )}
            <button
              onClick={handleSave}
              disabled={!hasChanges || saving}
              className="flex cursor-pointer items-center gap-2 rounded-xl bg-[#3b82f6] px-5 py-3 text-sm font-semibold text-white transition-all hover:bg-[#2563eb] disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Save className="h-4 w-4" />
              {saving ? 'Salvando...' : 'Salvar'}
            </button>
          </div>
        </div>

        {message && (
          <div className={`mt-6 rounded-xl px-5 py-4 text-sm border ${
            message.type === 'success'
              ? 'bg-[#22c55e]/10 text-[#22c55e] border-[#22c55e]/20'
              : 'bg-[#ef4444]/10 text-[#f87171] border-[#ef4444]/20'
          }`}>
            {message.text}
          </div>
        )}

        {/* Provider & Model Section */}
        <section className="mt-10">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-[#111827]">
              <Cpu className="h-5 w-5 text-[#64748b]" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-[#f8fafc]">Provider & Modelo LLM</h2>
              <p className="text-sm text-[#64748b]">Configuração do modelo de linguagem principal</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/10 bg-[#111827] p-6">
            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                Provider
              </label>
              <Input
                value={config.provider}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, provider: v } : prev)}
                placeholder="ex: openai, anthropic, google, zai, groq, ollama"
              />
            </div>

            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                Modelo
              </label>
              <Input
                value={config.model}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, model: v } : prev)}
                placeholder="ex: gpt-4o, claude-sonnet-4-20250514, gemini-2.5-pro"
              />
            </div>

            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                Base URL
              </label>
              <Input
                value={config.base_url}
                onChange={(v) => setConfig((prev) => prev ? { ...prev, base_url: v } : prev)}
                placeholder="https://api.example.com/v1 (opcional para alguns providers)"
              />
              <p className="mt-2 text-xs text-[#475569]">
                Deixe vazio para usar a URL padrão do provider
              </p>
            </div>

            <div>
              <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                API Key
              </label>
              <Input
                value={mainApiKey}
                onChange={setMainApiKey}
                placeholder={config.api_key_configured ? '••••••• (configurada)' : 'Sua API key'}
                type="password"
              />
              <p className="mt-2 text-xs text-[#475569]">
                Criptografada com AES-256-GCM e armazenada no vault local
              </p>
            </div>
          </div>
        </section>

        {/* Vision Section */}
        <section className="mt-10">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-[#111827]">
              <Image className="h-5 w-5 text-[#64748b]" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-[#f8fafc]">Compreensão de Imagens</h2>
              <p className="text-sm text-[#64748b]">Entender imagens e vídeos recebidos nos canais</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/10 bg-[#111827] p-6">
            <Toggle
              enabled={config.media.vision_enabled}
              onChange={(v) => updateMedia('vision_enabled', v)}
              label="Ativar compreensão visual"
            />

            {config.media.vision_enabled && (
              <>
                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                    Modelo de Visão
                  </label>
                  <Input
                    value={config.media.vision_model}
                    onChange={(v) => updateMedia('vision_model', v)}
                    placeholder="Deixe vazio para usar o modelo principal"
                  />
                  <p className="mt-2 text-xs text-[#475569]">
                    Se vazio, usa o modelo principal ({config.model})
                  </p>
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
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
        <section className="mt-8 mb-10">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-[#111827]">
              <Mic className="h-5 w-5 text-[#64748b]" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-[#f8fafc]">Transcrição de Áudio</h2>
              <p className="text-sm text-[#64748b]">Converter áudios e notas de voz em texto</p>
            </div>
          </div>

          <div className="space-y-5 rounded-2xl border border-white/10 bg-[#111827] p-6">
            <Toggle
              enabled={config.media.transcription_enabled}
              onChange={(v) => updateMedia('transcription_enabled', v)}
              label="Ativar transcrição de áudio"
            />

            {config.media.transcription_enabled && (
              <>
                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                    Modelo de Transcrição
                  </label>
                  <Input
                    value={config.media.transcription_model}
                    onChange={(v) => updateMedia('transcription_model', v)}
                    placeholder="ex: whisper-1, gpt-4o-transcribe"
                  />
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                    Base URL da API
                  </label>
                  <Input
                    value={config.media.transcription_base_url}
                    onChange={(v) => updateMedia('transcription_base_url', v)}
                    placeholder="https://api.openai.com/v1"
                  />
                  <p className="mt-2 text-xs text-[#475569]">
                    Deixe vazio para usar a URL do provider principal
                  </p>
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
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
                  <p className="mt-2 text-xs text-[#475569]">
                    Ajuda o modelo a reconhecer o idioma correto
                  </p>
                </div>

                <div>
                  <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
                    API Key de Transcrição
                  </label>
                  <Input
                    value={transcriptionApiKey}
                    onChange={setTranscriptionApiKey}
                    placeholder={config.media.transcription_api_key ? '••••••• (configurada)' : 'Usa a API key principal se vazio'}
                    type="password"
                  />
                  <p className="mt-2 text-xs text-[#475569]">
                    Necessário quando o provedor de transcrição é diferente do principal
                  </p>
                </div>
              </>
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
