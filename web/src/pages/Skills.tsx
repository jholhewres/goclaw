import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Search, ToggleLeft, ToggleRight, Zap, Package, Wrench, Plus, Download, X, Loader2, CheckCircle2 } from 'lucide-react'
import { api, type SkillInfo } from '@/lib/api'

interface AvailableSkill {
  name: string
  description: string
  category: string
  version?: string
  tags?: string[]
  installed: boolean
}

export function Skills() {
  const { t } = useTranslation()
  const [skills, setSkills] = useState<SkillInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [showInstall, setShowInstall] = useState(false)

  const [loadError, setLoadError] = useState(false)

  useEffect(() => {
    api.skills.list()
      .then(setSkills)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  const filtered = skills.filter(
    (s) =>
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.description.toLowerCase().includes(search.toLowerCase()),
  )

  const handleToggle = async (name: string, currentEnabled: boolean) => {
    try {
      await api.skills.toggle(name, !currentEnabled)
      setSkills((prev) =>
        prev.map((s) => (s.name === name ? { ...s, enabled: !currentEnabled } : s)),
      )
    } catch { /* ignore */ }
  }

  const handleInstalled = (name: string) => {
    if (!skills.find((s) => s.name === name)) {
      setSkills((prev) => [...prev, { name, description: t('skills.noSkills'), enabled: false, tool_count: 0 }])
    }
  }

  const enabledCount = skills.filter((s) => s.enabled).length

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-10 w-10 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center bg-dc-darker">
        <p className="text-sm text-red-400">{t('common.error')}</p>
        <button onClick={() => window.location.reload()} className="mt-3 text-xs text-blue-400 hover:text-blue-300 transition-colors">
          {t('common.loading')}
        </button>
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto bg-dc-darker">
      <div className="mx-auto max-w-5xl px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('skills.subtitle')}</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('skills.title')}</h1>
            <p className="mt-2 text-base text-zinc-500">
              {enabledCount} {t('skills.enabled').toLowerCase()} / {skills.length}
            </p>
          </div>
          <button
            onClick={() => setShowInstall(true)}
            className="flex cursor-pointer items-center gap-2 rounded-xl bg-blue-500 px-4 py-2.5 text-sm font-medium text-white shadow-lg shadow-blue-500/20 transition-all hover:bg-blue-400 hover:shadow-blue-500/30"
          >
            <Plus className="h-4 w-4" />
            Instalar Skill
          </button>
        </div>

        {/* Search */}
        <div className="relative mt-6">
          <Search className="absolute left-5 top-1/2 h-5 w-5 -translate-y-1/2 text-zinc-600" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Buscar skills..."
            className="w-full rounded-2xl border border-white/8 bg-dc-dark px-5 py-4 pl-14 text-base text-white outline-none placeholder:text-zinc-600 transition-all focus:border-blue-500/30 focus:ring-2 focus:ring-blue-500/10"
          />
        </div>

        {/* Grid */}
        <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((skill) => (
            <div
              key={skill.name}
              className={`group relative overflow-hidden rounded-2xl border p-6 transition-all ${
                skill.enabled
                  ? 'border-blue-500/25 bg-blue-500/4'
                  : 'border-white/6 bg-dc-dark hover:border-blue-500/15'
              }`}
            >
              {skill.enabled && (
                <div className="absolute right-4 top-4">
                  <span className="rounded-full bg-blue-500 px-2.5 py-0.5 text-[10px] font-bold text-white shadow-lg shadow-blue-500/30">ativa</span>
                </div>
              )}

                <div className={`flex h-14 w-14 items-center justify-center rounded-xl ${
                skill.enabled ? 'bg-blue-500/15 text-blue-400' : 'bg-white/5 text-zinc-500 group-hover:text-blue-400'
              } transition-colors`}>
                <Package className="h-7 w-7" />
              </div>

              <h3 className="mt-4 text-lg font-bold text-white">{skill.name}</h3>
              <p className="mt-2 text-sm leading-relaxed text-zinc-400 line-clamp-2">{skill.description}</p>

              <div className="mt-4 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="flex items-center gap-1.5 rounded-full bg-white/4 px-3 py-1 text-xs font-semibold text-zinc-500">
                    <Wrench className="h-3 w-3" />
                    {skill.tool_count} ferramentas
                  </span>
                </div>
                <button
                  onClick={() => handleToggle(skill.name, skill.enabled)}
                  aria-label={skill.enabled ? `Desativar ${skill.name}` : `Ativar ${skill.name}`}
                  className="cursor-pointer text-zinc-500 transition-colors hover:text-white"
                >
                  {skill.enabled ? (
                    <ToggleRight className="h-7 w-7 text-blue-400" />
                  ) : (
                    <ToggleLeft className="h-7 w-7" />
                  )}
                </button>
              </div>
            </div>
          ))}
        </div>

        {filtered.length === 0 && (
          <div className="mt-20 flex flex-col items-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/4">
              <Zap className="h-8 w-8 text-zinc-700" />
            </div>
            <p className="mt-4 text-lg font-semibold text-zinc-500">
              {search ? 'Nenhuma skill encontrada' : 'Nenhuma skill disponível'}
            </p>
          </div>
        )}
      </div>

      {showInstall && (
        <InstallModal
          onClose={() => setShowInstall(false)}
          onInstalled={handleInstalled}
        />
      )}
    </div>
  )
}

function InstallModal({ onClose, onInstalled }: { onClose: () => void; onInstalled: (name: string) => void }) {
  const [tab, setTab] = useState<'catalog' | 'manual'>('catalog')
  const [available, setAvailable] = useState<AvailableSkill[]>([])
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState(false)
  const [search, setSearch] = useState('')
  const [installing, setInstalling] = useState<string | null>(null)
  const [installed, setInstalled] = useState<Set<string>>(new Set())
  const [manualName, setManualName] = useState('')
  const [manualMsg, setManualMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const fetchCatalog = () => {
    setLoading(true)
    setFetchError(false)
    fetch('/api/skills/available', {
      headers: { Authorization: `Bearer ${localStorage.getItem('devclaw_token') || ''}` },
    })
      .then((r) => r.json())
      .then((data: AvailableSkill[]) => {
        setAvailable(Array.isArray(data) ? data : [])
        setInstalled(new Set((Array.isArray(data) ? data : []).filter((s) => s.installed).map((s) => s.name)))
        setFetchError(false)
      })
      .catch(() => setFetchError(true))
      .finally(() => setLoading(false))
  }

  useEffect(() => { fetchCatalog() }, [])

  const filtered = available.filter(
    (s) =>
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.description?.toLowerCase().includes(search.toLowerCase()) ||
      s.category?.toLowerCase().includes(search.toLowerCase()),
  )

  const handleInstall = async (name: string) => {
    setInstalling(name)
    try {
      await api.skills.install(name)
      setInstalled((prev) => new Set([...prev, name]))
      onInstalled(name)
    } catch { /* ignore */ }
    setInstalling(null)
  }

  const handleManualInstall = async () => {
    const name = manualName.trim()
    if (!name) return
    setManualMsg(null)
    setInstalling(name)
    try {
      await api.skills.install(name)
      setInstalled((prev) => new Set([...prev, name]))
      onInstalled(name)
      setManualMsg({ type: 'success', text: `"${name}" instalada com sucesso.` })
      setManualName('')
    } catch {
      setManualMsg({ type: 'error', text: `Falha ao instalar "${name}". Verifique o nome.` })
    }
    setInstalling(null)
  }

  const categories = [...new Set(available.map((s) => s.category).filter(Boolean))]
  const [activeCategory, setActiveCategory] = useState<string | null>(null)

  const categoryFiltered = activeCategory
    ? filtered.filter((s) => s.category === activeCategory)
    : filtered

  const displayList = [...categoryFiltered].sort((a, b) => {
    const aInst = installed.has(a.name) ? 1 : 0
    const bInst = installed.has(b.name) ? 1 : 0
    if (aInst !== bInst) return aInst - bInst
    return a.name.localeCompare(b.name)
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose} onKeyDown={(e) => e.key === 'Escape' && onClose()}>
      <div
        className="relative w-full max-w-2xl max-h-[85vh] overflow-hidden rounded-2xl border border-white/8 bg-dc-dark shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4">
          <div className="flex items-center gap-4">
            <h2 className="text-lg font-bold text-white">Instalar Skill</h2>
            <div className="flex rounded-lg bg-zinc-800/80 p-0.5">
              <button
                onClick={() => setTab('catalog')}
                className={`cursor-pointer rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                  tab === 'catalog' ? 'bg-blue-500 text-white' : 'text-zinc-400 hover:text-white'
                }`}
              >
                Catálogo
              </button>
              <button
                onClick={() => setTab('manual')}
                className={`cursor-pointer rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                  tab === 'manual' ? 'bg-blue-500 text-white' : 'text-zinc-400 hover:text-white'
                }`}
              >
                Manual
              </button>
            </div>
          </div>
          <button onClick={onClose} className="cursor-pointer rounded-lg p-1.5 text-zinc-500 hover:bg-white/5 hover:text-white transition-colors">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Catalog tab */}
        {tab === 'catalog' && (
          <>
            {/* Search + categories */}
            <div className="border-t border-white/6 px-6 py-3">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-600" />
                <input
                  value={search}
                  onChange={(e) => { setSearch(e.target.value); setActiveCategory(null) }}
                  placeholder="Buscar skills..."
                  autoFocus
                  className="w-full rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 py-2.5 pl-10 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-blue-500/50"
                />
              </div>
              {categories.length > 0 && !search && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {categories.map((cat) => (
                    <button
                      key={cat}
                      onClick={() => setActiveCategory(activeCategory === cat ? null : cat)}
                      className={`cursor-pointer rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors ${
                        activeCategory === cat
                          ? 'bg-blue-500/20 text-blue-400 ring-1 ring-blue-500/30'
                          : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700 hover:text-white'
                      }`}
                    >
                      {cat}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* List */}
            <div className="overflow-y-auto px-6 py-4" style={{ maxHeight: 'calc(85vh - 200px)' }}>
              {loading ? (
                <div className="flex flex-col items-center gap-3 py-16">
                  <Loader2 className="h-6 w-6 animate-spin text-blue-400" />
                  <p className="text-xs text-zinc-500">Carregando catálogo do GitHub...</p>
                </div>
              ) : fetchError ? (
                <div className="flex flex-col items-center py-12">
                  <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-red-500/10">
                    <X className="h-7 w-7 text-red-400" />
                  </div>
                  <p className="mt-4 text-sm font-medium text-zinc-300">Could not load the catalog</p>
                  <p className="mt-1 text-xs text-zinc-500 text-center max-w-xs">
                    Verifique a conexão com a internet. O catálogo é baixado de github.com/jholhewres/devclaw-skills.
                  </p>
                  <button
                    onClick={fetchCatalog}
                    className="mt-4 cursor-pointer rounded-lg bg-zinc-800 px-4 py-2 text-xs font-medium text-zinc-300 ring-1 ring-zinc-700/50 transition-colors hover:bg-zinc-700 hover:text-white"
                  >
                    Tentar novamente
                  </button>
                </div>
              ) : displayList.length === 0 ? (
                <div className="flex flex-col items-center py-12">
                  <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-zinc-800/50">
                    <Package className="h-7 w-7 text-zinc-600" />
                  </div>
                  {search || activeCategory ? (
                    <>
                      <p className="mt-4 text-sm font-medium text-zinc-400">Nenhuma skill encontrada</p>
                      <p className="mt-1 text-xs text-zinc-600">Tente outro termo ou use a aba Manual</p>
                    </>
                  ) : (
                    <>
                      <p className="mt-4 text-sm font-medium text-zinc-400">Catálogo vazio</p>
                      <p className="mt-1 text-xs text-zinc-600 text-center max-w-xs">
                        O catálogo remoto retornou vazio. Você pode instalar manualmente pela aba Manual.
                      </p>
                      <button
                        onClick={() => setTab('manual')}
                        className="mt-3 cursor-pointer text-xs font-medium text-blue-400 hover:text-blue-300 transition-colors"
                      >
                        Instalar manualmente →
                      </button>
                    </>
                  )}
                </div>
              ) : (
                <div className="space-y-1.5">
                  {displayList.map((skill) => {
                    const isInstalled = installed.has(skill.name)
                    const isInstalling = installing === skill.name

                    return (
                      <div
                        key={skill.name}
                        className={`flex items-center gap-4 rounded-xl px-4 py-3 transition-colors ${
                          isInstalled
                            ? 'bg-emerald-500/5 ring-1 ring-emerald-500/10'
                            : 'bg-zinc-800/30 ring-1 ring-zinc-700/20 hover:ring-zinc-700/40'
                        }`}
                      >
                        <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${
                          isInstalled ? 'bg-emerald-500/10' : 'bg-zinc-800'
                        }`}>
                          <Package className={`h-4 w-4 ${isInstalled ? 'text-emerald-400' : 'text-zinc-500'}`} />
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <h3 className="text-sm font-semibold text-white">{skill.name}</h3>
                            {skill.version && (
                              <span className="text-[10px] text-zinc-600">v{skill.version}</span>
                            )}
                            {skill.category && (
                              <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500">{skill.category}</span>
                            )}
                          </div>
                          {skill.description && (
                            <p className="mt-0.5 text-xs text-zinc-500 line-clamp-1">{skill.description}</p>
                          )}
                        </div>
                        <div className="shrink-0">
                          {isInstalled ? (
                            <span className="flex items-center gap-1 text-xs font-medium text-emerald-400">
                              <CheckCircle2 className="h-3.5 w-3.5" />
                              Instalada
                            </span>
                          ) : (
                            <button
                              onClick={() => handleInstall(skill.name)}
                              disabled={isInstalling}
                              className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-blue-500/10 px-3 py-1.5 text-xs font-medium text-blue-400 transition-colors hover:bg-blue-500/20 disabled:opacity-50"
                            >
                              {isInstalling ? (
                                <Loader2 className="h-3 w-3 animate-spin" />
                              ) : (
                                <Download className="h-3 w-3" />
                              )}
                              {isInstalling ? 'Instalando...' : 'Instalar'}
                            </button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </>
        )}

        {/* Manual tab */}
        {tab === 'manual' && (
          <div className="border-t border-white/6 px-6 py-6">
            <div className="space-y-5">
              <div>
                <p className="text-sm text-zinc-300">
                  Instale uma skill pelo nome exato do diretório no repositório{' '}
                  <a
                    href="https://github.com/jholhewres/devclaw-skills"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-400 hover:text-blue-300 transition-colors"
                  >
                    devclaw-skills
                  </a>.
                </p>
              </div>

              <div>
                <label className="mb-1.5 block text-[11px] font-semibold uppercase tracking-wider text-zinc-500">
                  Nome da Skill
                </label>
                <div className="flex gap-2">
                  <input
                    value={manualName}
                    onChange={(e) => setManualName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleManualInstall()}
                    placeholder="ex: docker-manager, api-tester, aws-tools"
                    className="flex-1 rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 py-2.5 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-blue-500/50"
                  />
                  <button
                    onClick={handleManualInstall}
                    disabled={!manualName.trim() || installing !== null}
                    className="flex cursor-pointer items-center gap-2 rounded-lg bg-blue-500 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-blue-400 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {installing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
                    Instalar
                  </button>
                </div>
              </div>

              {manualMsg && (
                <div className={`flex items-center gap-2 rounded-lg px-3 py-2.5 text-xs ring-1 ${
                  manualMsg.type === 'success'
                    ? 'bg-emerald-500/5 text-emerald-400 ring-emerald-500/20'
                    : 'bg-red-500/5 text-red-400 ring-red-500/20'
                }`}>
                  {manualMsg.type === 'success' ? <CheckCircle2 className="h-3.5 w-3.5 shrink-0" /> : <X className="h-3.5 w-3.5 shrink-0" />}
                  {manualMsg.text}
                </div>
              )}

              <div className="rounded-xl bg-zinc-800/30 px-4 py-3 ring-1 ring-zinc-700/20">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">Como funciona</p>
                <ul className="mt-2 space-y-1.5 text-xs text-zinc-400">
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500/60" />
                    O DevClaw baixa o <code className="text-blue-400/80">SKILL.md</code> do repositório no GitHub
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500/60" />
                    O arquivo é salvo em <code className="text-blue-400/80">./skills/{'{nome}'}/SKILL.md</code>
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500/60" />
                    Reinicie o servidor para que a skill fique disponível
                  </li>
                </ul>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
