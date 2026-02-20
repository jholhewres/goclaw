import { useEffect, useState, useRef, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Puzzle, Check, Search, Package, Code, Zap, Globe, Database,
  Wrench, Sparkles, ChevronDown, ChevronRight, MessageSquare,
  Image, DollarSign, X,
} from 'lucide-react'
import type { SetupData } from './SetupWizard'

interface CatalogSkill {
  name: string
  description: string
  category?: string
  version?: string
  tags?: string[]
  starter_pack?: boolean
  enabled: boolean
  tool_count: number
}

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

export function StepSkills({ data, updateData }: Props) {
  const { t } = useTranslation()
  const [skills, setSkills] = useState<CatalogSkill[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [collapsedCats, setCollapsedCats] = useState<Set<string>>(new Set())
  const [activeCategory, setActiveCategory] = useState<string | null>(null)
  const didInit = useRef(false)

  const CATEGORY_META: Record<string, { label: string; icon: React.FC<{ className?: string }>; color: string }> = {
    development:   { label: t('setupPage.catDevelopment'),     icon: Code,           color: 'text-violet-400' },
    data:          { label: t('setupPage.catData'),            icon: Globe,          color: 'text-cyan-400' },
    productivity:  { label: t('setupPage.catProductivity'),    icon: Zap,            color: 'text-amber-400' },
    infra:         { label: t('setupPage.catInfra'),           icon: Database,       color: 'text-teal-400' },
    media:         { label: t('setupPage.catMedia'),           icon: Image,          color: 'text-pink-400' },
    communication: { label: t('setupPage.catCommunication'),   icon: MessageSquare,  color: 'text-blue-400' },
    finance:       { label: t('setupPage.catFinance'),         icon: DollarSign,     color: 'text-green-400' },
    integration:   { label: t('setupPage.catIntegration'),     icon: Wrench,         color: 'text-blue-400' },
    builtin:       { label: t('setupPage.catBuiltin'),         icon: Package,        color: 'text-emerald-400' },
  }

  function getCategoryMeta(cat?: string) {
    return CATEGORY_META[cat ?? ''] ?? { label: cat ?? t('setupPage.catOther'), icon: Puzzle, color: 'text-zinc-400' }
  }

  useEffect(() => {
    fetch('/api/setup/skills')
      .then((r) => r.ok ? r.json() : [])
      .then((d: CatalogSkill[]) => {
        const list = Array.isArray(d) ? d : []
        setSkills(list)

        if (!didInit.current && data.enabledSkills.length === 0) {
          didInit.current = true
          const starterNames = list.filter((s) => s.starter_pack).map((s) => s.name)
          if (starterNames.length > 0) {
            updateData({ enabledSkills: starterNames })
          }
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const toggleSkill = (name: string) => {
    const current = data.enabledSkills
    const next = current.includes(name)
      ? current.filter((s) => s !== name)
      : [...current, name]
    updateData({ enabledSkills: next })
  }

  const starterSkills = useMemo(() => skills.filter((s) => s.starter_pack), [skills])
  const catalogSkills = useMemo(() => skills.filter((s) => !s.starter_pack), [skills])

  const allStarterSelected = starterSkills.length > 0 &&
    starterSkills.every((s) => data.enabledSkills.includes(s.name))

  const toggleStarterPack = () => {
    if (allStarterSelected) {
      const starterNames = new Set(starterSkills.map((s) => s.name))
      updateData({ enabledSkills: data.enabledSkills.filter((n) => !starterNames.has(n)) })
    } else {
      const starterNames = starterSkills.map((s) => s.name)
      const merged = [...new Set([...data.enabledSkills, ...starterNames])]
      updateData({ enabledSkills: merged })
    }
  }

  const selectAll = () => updateData({ enabledSkills: skills.map((s) => s.name) })
  const deselectAll = () => updateData({ enabledSkills: [] })

  const toggleCategory = (cat: string) => {
    setCollapsedCats((prev) => {
      const next = new Set(prev)
      if (next.has(cat)) next.delete(cat)
      else next.add(cat)
      return next
    })
  }

  const toggleEntireCategory = (_cat: string, skillsInCat: CatalogSkill[]) => {
    const names = skillsInCat.map((s) => s.name)
    const allSelected = names.every((n) => data.enabledSkills.includes(n))
    if (allSelected) {
      updateData({ enabledSkills: data.enabledSkills.filter((n) => !names.includes(n)) })
    } else {
      updateData({ enabledSkills: [...new Set([...data.enabledSkills, ...names])] })
    }
  }

  const filtered = useMemo(() => {
    let list = catalogSkills
    if (activeCategory) {
      list = list.filter((s) => (s.category ?? 'other') === activeCategory)
    }
    if (filter) {
      const q = filter.toLowerCase()
      list = list.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          s.description.toLowerCase().includes(q) ||
          (s.tags ?? []).some((t) => t.toLowerCase().includes(q)),
      )
    }
    return list
  }, [catalogSkills, filter, activeCategory])

  const grouped = useMemo(() => {
    return filtered.reduce<Record<string, CatalogSkill[]>>((acc, sk) => {
      const cat = sk.category ?? 'other'
      ;(acc[cat] ??= []).push(sk)
      return acc
    }, {})
  }, [filtered])

  const categoryOrder = ['development', 'data', 'productivity', 'infra', 'media', 'communication', 'finance', 'integration', 'builtin']
  const sortedCategories = Object.keys(grouped).sort(
    (a, b) => (categoryOrder.indexOf(a) === -1 ? 99 : categoryOrder.indexOf(a)) - (categoryOrder.indexOf(b) === -1 ? 99 : categoryOrder.indexOf(b)),
  )

  const availableCategories = useMemo(() => {
    const cats = new Set(catalogSkills.map((s) => s.category ?? 'other'))
    return categoryOrder.filter((c) => cats.has(c))
  }, [catalogSkills])

  const extraCount = data.enabledSkills.filter(
    (n) => !starterSkills.some((s) => s.name === n)
  ).length

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-16">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-700 border-t-zinc-400" />
        <p className="text-sm text-zinc-500">{t('setupPage.loadingCatalog')}</p>
      </div>
    )
  }

  if (skills.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 py-12">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-zinc-800/60 ring-1 ring-zinc-700/30">
          <Puzzle className="h-6 w-6 text-zinc-500" />
        </div>
        <p className="text-sm text-zinc-400">{t('setupPage.noSkills')}</p>
        <p className="text-xs text-zinc-500">{t('setupPage.skillsLater')}</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold text-white">{t('setupPage.skillsTitle')}</h2>
        <p className="mt-1 text-sm text-zinc-400">
          {t('setupPage.skillsDesc')}
        </p>
      </div>

      {/* ─── Starter Pack Section ─── */}
      <div className={`rounded-xl border p-3 transition-all ${
        allStarterSelected
          ? 'border-zinc-500 bg-zinc-700/50'
          : 'border-zinc-700/40 bg-zinc-800/20'
      }`}>
        <div className="flex items-center justify-between">
          <button
            onClick={toggleStarterPack}
            className="flex cursor-pointer items-center gap-2.5"
          >
            <div className={`flex h-5 w-5 shrink-0 items-center justify-center rounded border transition-all ${
              allStarterSelected
                ? 'border-transparent bg-zinc-500 text-white'
                : 'border-zinc-600 bg-zinc-800 hover:border-zinc-500'
            }`}>
              {allStarterSelected && <Check className="h-3 w-3" />}
            </div>
            <Sparkles className="h-4 w-4 text-zinc-300" />
            <span className="text-sm font-medium text-white">{t('setupPage.starterPack')}</span>
            <span className="text-xs text-zinc-500">({starterSkills.length} {t('setupPage.starterPackCount')})</span>
          </button>
        </div>

        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {starterSkills.map((skill) => {
            const isActive = data.enabledSkills.includes(skill.name)
            return (
              <button
                key={skill.name}
                onClick={() => toggleSkill(skill.name)}
                className={`flex cursor-pointer items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs transition-all ${
                  isActive
                    ? 'border-zinc-500 bg-zinc-700/50 text-zinc-200'
                    : 'border-zinc-700/40 bg-zinc-800/40 text-zinc-500 hover:border-zinc-600 hover:text-zinc-300'
                }`}
                title={skill.description}
              >
                {isActive ? (
                  <Check className="h-3 w-3 text-zinc-200" />
                ) : (
                  <div className="h-3 w-3 rounded-sm border border-zinc-600" />
                )}
                <span className="font-medium">{skill.name}</span>
              </button>
            )
          })}
        </div>
      </div>

      {/* ─── Catalog Section ─── */}
      <div>
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-wider text-zinc-500">
            {t('setupPage.catalogAdd')}
          </p>
          <div className="flex gap-1.5 text-[10px]">
            <button
              onClick={selectAll}
              className="cursor-pointer rounded-md border border-zinc-700/40 bg-zinc-800/30 px-2 py-1 text-zinc-500 transition-colors hover:bg-zinc-700/40 hover:text-zinc-300"
            >
              {t('setupPage.selectAll')}
            </button>
            <button
              onClick={deselectAll}
              className="cursor-pointer rounded-md border border-zinc-700/40 bg-zinc-800/30 px-2 py-1 text-zinc-500 transition-colors hover:bg-zinc-700/40 hover:text-zinc-300"
            >
              {t('setupPage.clear')}
            </button>
          </div>
        </div>

        {/* Category pills + Search */}
        <div className="mt-2.5 flex items-center gap-2">
          <div className="flex flex-1 gap-1 overflow-x-auto pb-0.5 scrollbar-none">
            <button
              onClick={() => setActiveCategory(null)}
              className={`shrink-0 cursor-pointer rounded-md px-2 py-1 text-[10px] font-medium transition-all ${
                activeCategory === null
                  ? 'bg-zinc-700/60 text-white'
                  : 'text-zinc-500 hover:bg-zinc-800/60 hover:text-zinc-300'
              }`}
            >
              {t('setupPage.all')}
            </button>
            {availableCategories.map((cat) => {
              const meta = getCategoryMeta(cat)
              return (
                <button
                  key={cat}
                  onClick={() => setActiveCategory(activeCategory === cat ? null : cat)}
                  className={`shrink-0 cursor-pointer rounded-md px-2 py-1 text-[10px] font-medium transition-all ${
                    activeCategory === cat
                      ? 'bg-zinc-700/60 text-white'
                      : 'text-zinc-500 hover:bg-zinc-800/60 hover:text-zinc-300'
                  }`}
                >
                  {meta.label}
                </button>
              )
            })}
          </div>

          <div className="relative shrink-0">
            <Search className="absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-zinc-600" />
            <input
              type="text"
              placeholder={t('setupPage.search')}
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="w-32 rounded-md border border-zinc-700/40 bg-zinc-800/40 py-1 pl-7 pr-6 text-[11px] text-white placeholder:text-zinc-600 outline-none transition-all focus:w-44 focus:border-zinc-600"
            />
            {filter && (
              <button
                onClick={() => setFilter('')}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 cursor-pointer text-zinc-600 hover:text-zinc-400"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
        </div>

        {/* Skills by category */}
        <div className="mt-3 max-h-[250px] space-y-1 overflow-y-auto pr-1 scrollbar-thin scrollbar-track-transparent scrollbar-thumb-zinc-700/50">
          {sortedCategories.length === 0 && (
            <p className="py-6 text-center text-xs text-zinc-600">
              {filter ? t('setupPage.noSkillsMatch', { filter }) : t('setupPage.noAdditional')}
            </p>
          )}

          {sortedCategories.map((cat) => {
            const meta = getCategoryMeta(cat)
            const CatIcon = meta.icon
            const skillsInCat = grouped[cat]
            const selectedInCat = skillsInCat.filter((s) => data.enabledSkills.includes(s.name)).length
            const isCollapsed = collapsedCats.has(cat)

            return (
              <div key={cat} className="rounded-lg border border-zinc-800/40">
                {/* Category header */}
                <button
                  onClick={() => toggleCategory(cat)}
                  className="flex w-full cursor-pointer items-center gap-2 px-3 py-2"
                >
                  {isCollapsed
                    ? <ChevronRight className="h-3 w-3 text-zinc-600" />
                    : <ChevronDown className="h-3 w-3 text-zinc-600" />
                  }
                  <CatIcon className={`h-3.5 w-3.5 ${meta.color}`} />
                  <span className={`text-xs font-semibold ${meta.color}`}>{meta.label}</span>
                  <span className="text-[10px] text-zinc-600">
                    {selectedInCat > 0
                      ? `${selectedInCat}/${skillsInCat.length} ${t('setupPage.selected')}`
                      : `${skillsInCat.length} ${t('setupPage.available')}`
                    }
                  </span>
                  <div className="flex-1" />
                  <button
                    onClick={(e) => { e.stopPropagation(); toggleEntireCategory(cat, skillsInCat) }}
                    className={`cursor-pointer rounded px-1.5 py-0.5 text-[9px] font-medium transition-all ${
                      selectedInCat === skillsInCat.length
                        ? 'bg-zinc-700 text-zinc-300 hover:bg-zinc-600'
                        : 'text-zinc-600 hover:bg-zinc-800 hover:text-zinc-400'
                    }`}
                  >
                    {selectedInCat === skillsInCat.length ? t('setupPage.deselect') : t('setupPage.selectSkills')}
                  </button>
                </button>

                {/* Skills grid (collapsible) */}
                {!isCollapsed && (
                  <div className="grid grid-cols-2 gap-1 px-2 pb-2">
                    {skillsInCat.map((skill) => {
                      const isActive = data.enabledSkills.includes(skill.name)
                      return (
                        <button
                          key={skill.name}
                          onClick={() => toggleSkill(skill.name)}
                          className={`flex w-full cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1.5 text-left transition-all ${
                            isActive
                              ? 'border-zinc-500 bg-zinc-700/50'
                              : 'border-transparent bg-zinc-800/20 hover:bg-zinc-800/50'
                          }`}
                        >
                          <div className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-sm border transition-all ${
                            isActive
                              ? 'border-transparent bg-zinc-500 text-white'
                              : 'border-zinc-600 bg-zinc-900/50'
                          }`}>
                            {isActive && <Check className="h-2 w-2" />}
                          </div>
                          <div className="min-w-0 flex-1">
                            <span className="text-[11px] font-medium text-white">{skill.name}</span>
                            <p className="truncate text-[9px] leading-tight text-zinc-600">{skill.description}</p>
                          </div>
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {/* ─── Summary bar ─── */}
      <div className="flex items-center justify-between rounded-lg bg-zinc-800/30 px-3 py-2">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5">
            <div className="h-2 w-2 rounded-full bg-zinc-400" />
            <span className="text-[11px] text-zinc-400">
              <strong className="text-white">{data.enabledSkills.length}</strong> {t('setupPage.skillsSelected')}
            </span>
          </div>
          {extraCount > 0 && (
            <span className="text-[10px] text-zinc-600">
              ({starterSkills.filter((s) => data.enabledSkills.includes(s.name)).length} {t('setupPage.packExtra', { count: extraCount })})
            </span>
          )}
        </div>
        {filtered.length !== catalogSkills.length && (
          <p className="text-[10px] text-zinc-600">
            {t('setupPage.showingOf', { shown: filtered.length, total: catalogSkills.length })}
          </p>
        )}
      </div>
    </div>
  )
}
