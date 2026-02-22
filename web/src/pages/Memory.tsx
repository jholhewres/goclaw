import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Brain,
  Database,
  Search,
} from 'lucide-react'
import { api } from '@/lib/api'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigToggle,
  ConfigActions,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

interface MemoryConfig {
  type: string
  max_messages: number
  compression_strategy: string
  embedding_enabled: boolean
  embedding_provider: string
  embedding_model: string
  embedding_dimensions: number
  search_hybrid_weight_vector: number
  search_hybrid_weight_bm25: number
  search_max_results: number
}

const MEMORY_TYPES = [
  { value: 'sqlite', label: 'SQLite (Recommended)' },
  { value: 'file', label: 'File-based' },
]

const COMPRESSION_STRATEGIES = [
  { value: 'summarize', label: 'Summarize old messages' },
  { value: 'truncate', label: 'Truncate old messages' },
  { value: 'semantic', label: 'Semantic pruning' },
]

const EMBEDDING_PROVIDERS = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'local', label: 'Local (Ollama)' },
]

// Slider component for hybrid search weights
function Slider({ label, value, onChange, hint }: {
  label: string
  value: number
  onChange: (value: number) => void
  hint?: string
}) {
  return (
    <div>
      <label className="mb-2 block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
        {label}
      </label>
      <div className="flex items-center gap-4">
        <input
          type="range"
          min="0"
          max="1"
          step="0.1"
          value={value}
          onChange={(e) => onChange(parseFloat(e.target.value))}
          className="flex-1 h-2 rounded-lg appearance-none cursor-pointer bg-[#1e293b] accent-[#3b82f6]"
        />
        <span className="text-sm text-[#f8fafc] w-12 text-right">
          {(value * 100).toFixed(0)}%
        </span>
      </div>
      {hint && <p className="mt-2 text-xs text-[#475569]">{hint}</p>}
    </div>
  )
}

export function Memory() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<MemoryConfig | null>(null)
  const [original, setOriginal] = useState<MemoryConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => {
    api.config.get()
      .then((data) => {
        const mem = (data as unknown as { memory?: MemoryConfig }).memory || {
          type: 'sqlite',
          max_messages: 100,
          compression_strategy: 'summarize',
          embedding_enabled: true,
          embedding_provider: 'openai',
          embedding_model: 'text-embedding-3-small',
          embedding_dimensions: 1536,
          search_hybrid_weight_vector: 0.7,
          search_hybrid_weight_bm25: 0.3,
          search_max_results: 10,
        }
        setConfig(mem)
        setOriginal(JSON.parse(JSON.stringify(mem)))
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
        memory: {
          type: config.type,
          max_messages: config.max_messages,
          compression_strategy: config.compression_strategy,
          embedding: {
            enabled: config.embedding_enabled,
            provider: config.embedding_provider,
            model: config.embedding_model,
            dimensions: config.embedding_dimensions,
          },
          search: {
            hybrid_weight_vector: config.search_hybrid_weight_vector,
            hybrid_weight_bm25: config.search_hybrid_weight_bm25,
            max_results: config.search_max_results,
          },
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
      title={t('memory.title')}
      subtitle={t('memory.subtitle')}
      description={t('memory.description')}
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
      {/* Storage Settings */}
      <ConfigSection
        icon={Database}
        title={t('memory.storageSection')}
        description={t('memory.storageSectionDesc')}
      >
        <ConfigField label={t('memory.storageType')} hint={t('memory.storageTypeHint')}>
          <ConfigSelect
            value={config.type}
            onChange={(v) => setConfig(prev => prev ? { ...prev, type: v } : prev)}
            options={MEMORY_TYPES}
          />
        </ConfigField>

        <ConfigField label={t('memory.maxMessages')} hint={t('memory.maxMessagesHint')}>
          <ConfigInput
            type="number"
            value={config.max_messages}
            onChange={(v) => setConfig(prev => prev ? { ...prev, max_messages: parseInt(v) || 100 } : prev)}
            placeholder="100"
          />
        </ConfigField>

        <ConfigField label={t('memory.compressionStrategy')} hint={t('memory.compressionStrategyHint')}>
          <ConfigSelect
            value={config.compression_strategy}
            onChange={(v) => setConfig(prev => prev ? { ...prev, compression_strategy: v } : prev)}
            options={COMPRESSION_STRATEGIES}
          />
        </ConfigField>
      </ConfigSection>

      {/* Embedding Settings */}
      <ConfigSection
        icon={Brain}
        title={t('memory.embeddingSection')}
        description={t('memory.embeddingSectionDesc')}
      >
        <ConfigToggle
          enabled={config.embedding_enabled}
          onChange={(v) => setConfig(prev => prev ? { ...prev, embedding_enabled: v } : prev)}
          label={t('memory.embeddingEnabled')}
        />

        {config.embedding_enabled && (
          <>
            <ConfigField label={t('memory.embeddingProvider')}>
              <ConfigSelect
                value={config.embedding_provider}
                onChange={(v) => setConfig(prev => prev ? { ...prev, embedding_provider: v } : prev)}
                options={EMBEDDING_PROVIDERS}
              />
            </ConfigField>

            <ConfigField label={t('memory.embeddingModel')} hint={t('memory.embeddingModelHint')}>
              <ConfigInput
                value={config.embedding_model}
                onChange={(v) => setConfig(prev => prev ? { ...prev, embedding_model: v } : prev)}
                placeholder="text-embedding-3-small"
              />
            </ConfigField>

            <ConfigField label={t('memory.embeddingDimensions')} hint={t('memory.embeddingDimensionsHint')}>
              <ConfigInput
                type="number"
                value={config.embedding_dimensions}
                onChange={(v) => setConfig(prev => prev ? { ...prev, embedding_dimensions: parseInt(v) || 1536 } : prev)}
                placeholder="1536"
              />
            </ConfigField>
          </>
        )}
      </ConfigSection>

      {/* Search Settings */}
      <ConfigSection
        icon={Search}
        title={t('memory.searchSection')}
        description={t('memory.searchSectionDesc')}
      >
        <Slider
          label={t('memory.vectorWeight')}
          value={config.search_hybrid_weight_vector}
          onChange={(val) => setConfig(prev => prev ? {
            ...prev,
            search_hybrid_weight_vector: val,
            search_hybrid_weight_bm25: 1 - val,
          } : prev)}
        />

        <Slider
          label={t('memory.bm25Weight')}
          value={config.search_hybrid_weight_bm25}
          onChange={(val) => setConfig(prev => prev ? {
            ...prev,
            search_hybrid_weight_bm25: val,
            search_hybrid_weight_vector: 1 - val,
          } : prev)}
          hint={t('memory.weightsHint')}
        />

        <ConfigField label={t('memory.maxResults')} hint={t('memory.maxResultsHint')}>
          <ConfigInput
            type="number"
            value={config.search_max_results}
            onChange={(v) => setConfig(prev => prev ? { ...prev, search_max_results: parseInt(v) || 10 } : prev)}
            placeholder="10"
          />
        </ConfigField>
      </ConfigSection>
    </ConfigPage>
  )
}
