import { useEffect, useState, useRef, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Puzzle, Search, Package, Code, Zap, Globe, Database,
  Wrench, Sparkles, ChevronDown, ChevronRight, MessageSquare,
  Image, DollarSign, X,
} from 'lucide-react'
import type { SetupData } from './SetupWizard'
import {
  StepContainer, StepHeader, FieldGroup, Card, Checkbox, Button,
} from './SetupComponents'

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

  // Category meta with translations
  const CATEGORY_META: Record<string, { label: string; icon: React.FC<{ className?: string }>; color: string }> = useMemo(() => ({
    development:   { label: t('setupPage.catDevelopment'),   icon: Code,          color: 'text-violet-400' },
    data:          { label: t('setupPage.catData'),          icon: Globe,         color: 'text-cyan-400' },
    productivity:  { label: t('setupPage.catProductivity'),  icon: Zap,           color: 'text-amber-400' },
    infra:         { label: t('setupPage.catInfra'),         icon: Database,      color: 'text-teal-400' },
    media:         { label: t('setupPage.catMedia'),         icon: Image,         color: 'text-pink-400' },
    communication: { label: t('setupPage.catCommunication'), icon: MessageSquare, color: 'text-blue-400' },
    finance:       { label: t('setupPage.catFinance'),       icon: DollarSign,    color: 'text-green-400' },
    integration:   { label: t('setupPage.catIntegration'),   icon: Wrench,        color: 'text-indigo-400' },
    builtin:       { label: t('setupPage.catBuiltin'),       icon: Package,       color: 'text-emerald-400' },
  }), [t])

  function getCategoryMeta(cat?: string) {
    return CATEGORY_META[cat ?? ''] ?? { label: cat ?? t('setupPage.catOther'), icon: Puzzle, color: 'text-[#64748b]' }
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
    const next = data.enabledSkills.includes(name)
      ? data.enabledSkills.filter((s) => s !== name)
      : [...data.enabledSkills, name]
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
      updateData({ enabledSkills: [...new Set([...data.enabledSkills, ...starterNames])] })
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
      <div className="flex flex-col items-center justify-center gap-3 py-12">
        <div className="h-5 w-5 animate-spin rounded-full border-2 border-[#1e293b] border-t-[#3b82f6]" />
        <p className="text-sm text-[#64748b]">{t('setupPage.loadingCatalog')}</p>
      </div>
    )
  }

  if (skills.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 py-10">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-[#1e293b]">
          <Puzzle className="h-5 w-5 text-[#64748b]" />
        </div>
        <p className="text-sm text-[#94a3b8]">{t('setupPage.noSkills')}</p>
        <p className="text-xs text-[#64748b]">{t('setupPage.skillsLater')}</p>
      </div>
    )
  }

  return (
    <StepContainer>
      <StepHeader
        title={t('setupPage.skillsTitle')}
        description={t('setupPage.skillsDesc')}
      />

      <FieldGroup>
        {/* Starter Pack */}
        <Card highlight={allStarterSelected ? 'green' : undefined}>
          <Checkbox checked={allStarterSelected} onChange={toggleStarterPack}>
            <Sparkles className="h-4 w-4 text-[#22c55e]" />
            <span className="text-sm font-medium text-[#f8fafc]">{t('setupPage.starterPack')}</span>
            <span className="text-xs text-[#64748b]">({starterSkills.length})</span>
          </Checkbox>

          <div className="mt-2.5 flex flex-wrap gap-1.5">
            {starterSkills.map((skill) => {
              const isActive = data.enabledSkills.includes(skill.name)
              return (
                <button
                  key={skill.name}
                  onClick={() => toggleSkill(skill.name)}
                  className={`flex cursor-pointer items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs transition-all ${
                    isActive
                      ? 'border-[#22c55e]/30 bg-[#22c55e]/10 text-[#f8fafc]'
                      : 'border-white/[0.06] bg-[#0c1222]/50 text-[#94a3b8] hover:border-white/10 hover:text-[#f8fafc]'
                  }`}
                  title={skill.description}
                >
                  {isActive ? (
                    <svg className="h-3 w-3 text-[#22c55e]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  ) : (
                    <div className="h-3 w-3 rounded-sm border border-white/20" />
                  )}
                  <span className="font-medium">{skill.name}</span>
                </button>
              )
            })}
          </div>
        </Card>

        {/* Catalog */}
        <div>
          <div className="flex items-center justify-between">
            <p className="text-xs font-semibold uppercase tracking-wider text-[#64748b]">
              {t('setupPage.catalogAdd')}
            </p>
            <div className="flex gap-1.5">
              <Button onClick={selectAll} variant="ghost" size="sm">
                {t('setupPage.selectAll')}
              </Button>
              <Button onClick={deselectAll} variant="ghost" size="sm">
                {t('setupPage.clear')}
              </Button>
            </div>
          </div>

          {/* Category pills + Search */}
          <div className="mt-2.5 flex items-center gap-2">
            <div className="flex flex-1 gap-1 overflow-x-auto pb-0.5 scrollbar-none">
              <button
                onClick={() => setActiveCategory(null)}
                className={`shrink-0 cursor-pointer rounded-md px-2 py-1 text-[10px] font-medium transition-all ${
                  activeCategory === null
                    ? 'bg-[#3b82f6] text-white'
                    : 'text-[#64748b] hover:bg-[#1e293b] hover:text-[#f8fafc]'
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
                        ? 'bg-[#3b82f6] text-white'
                        : 'text-[#64748b] hover:bg-[#1e293b] hover:text-[#f8fafc]'
                    }`}
                  >
                    {meta.label}
                  </button>
                )
              })}
            </div>

            <div className="relative shrink-0">
              <Search className="absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-[#475569]" />
              <input
                type="text"
                placeholder={t('setupPage.search')}
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                className="w-28 rounded-md border border-white/[0.06] bg-[#0c1222] py-1 pl-7 pr-6 text-[11px] text-[#f8fafc] placeholder:text-[#475569] outline-none transition-all focus:w-36 focus:border-[#3b82f6]/50"
              />
              {filter && (
                <button
                  onClick={() => setFilter('')}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 cursor-pointer text-[#475569] hover:text-[#f8fafc]"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
          </div>

          {/* Skills by category */}
          <div className="mt-3 max-h-[200px] space-y-1 overflow-y-auto pr-1 scrollbar-thin scrollbar-track-transparent scrollbar-thumb-[#1e293b]">
            {sortedCategories.length === 0 && (
              <p className="py-6 text-center text-xs text-[#475569]">
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
                <div key={cat} className="rounded-lg border border-white/[0.04]">
                  <button
                    onClick={() => toggleCategory(cat)}
                    className="flex w-full cursor-pointer items-center gap-2 px-3 py-2"
                  >
                    {isCollapsed
                      ? <ChevronRight className="h-3 w-3 text-[#475569]" />
                      : <ChevronDown className="h-3 w-3 text-[#475569]" />
                    }
                    <CatIcon className={`h-3.5 w-3.5 ${meta.color}`} />
                    <span className={`text-xs font-semibold ${meta.color}`}>{meta.label}</span>
                    <span className="text-[10px] text-[#475569]">
                      {selectedInCat > 0 ? `${selectedInCat}/${skillsInCat.length}` : skillsInCat.length}
                    </span>
                    <div className="flex-1" />
                    <button
                      onClick={(e) => { e.stopPropagation(); toggleEntireCategory(cat, skillsInCat) }}
                      className={`cursor-pointer rounded px-1.5 py-0.5 text-[9px] font-medium transition-all ${
                        selectedInCat === skillsInCat.length
                          ? 'bg-[#22c55e]/10 text-[#22c55e]'
                          : 'text-[#475569] hover:bg-[#1e293b] hover:text-[#94a3b8]'
                      }`}
                    >
                      {selectedInCat === skillsInCat.length ? t('setupPage.deselect') : t('setupPage.selectSkills')}
                    </button>
                  </button>

                  {!isCollapsed && (
                    <div className="grid grid-cols-2 gap-1 px-2 pb-2">
                      {skillsInCat.map((skill) => {
                        const isActive = data.enabledSkills.includes(skill.name)
                        return (
                          <button
                            key={skill.name}
                            onClick={() => toggleSkill(skill.name)}
                            className={`flex w-full cursor-pointer items-center gap-2 rounded-md border px-2.5 py-2 text-left transition-all ${
                              isActive
                                ? 'border-[#3b82f6]/30 bg-[#3b82f6]/5'
                                : 'border-transparent bg-[#0c1222]/50 hover:bg-[#111827]'
                            }`}
                          >
                            <div className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-sm border transition-all ${
                              isActive
                                ? 'border-transparent bg-[#3b82f6] text-white'
                                : 'border-white/20 bg-[#1e293b]'
                            }`}>
                              {isActive && (
                                <svg className="h-2 w-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                                </svg>
                              )}
                            </div>
                            <div className="min-w-0 flex-1">
                              <span className="text-[11px] font-medium text-[#f8fafc]">{skill.name}</span>
                              <p className="truncate text-[9px] leading-tight text-[#64748b]">{skill.description}</p>
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

        {/* Summary */}
        <div className="flex items-center justify-between rounded-lg border border-white/[0.06] bg-[#0c1222]/50 px-3 py-2">
          <div className="flex items-center gap-1.5">
            <div className="h-2 w-2 rounded-full bg-[#3b82f6]" />
            <span className="text-[11px] text-[#94a3b8]">
              <strong className="text-[#f8fafc]">{data.enabledSkills.length}</strong> {t('setupPage.skillsSelected')}
            </span>
            {extraCount > 0 && (
              <span className="text-[10px] text-[#64748b]">
                ({starterSkills.filter((s) => data.enabledSkills.includes(s.name)).length} + {extraCount})
              </span>
            )}
          </div>
          {filtered.length !== catalogSkills.length && (
            <p className="text-[10px] text-[#475569]">
              {t('setupPage.showingOf', { shown: filtered.length, total: catalogSkills.length })}
            </p>
          )}
        </div>
      </FieldGroup>
    </StepContainer>
  )
}
