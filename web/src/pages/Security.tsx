import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  Shield,
  Key,
  Activity,
  Lock,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ChevronDown,
  X,
  ExternalLink,
} from 'lucide-react'
import { api, type SecurityStatus, type AuditEntry, type ToolGuardStatus, type VaultStatus } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

/**
 * Security panel — vault, tool guard, audit log, API keys.
 */
export function Security() {
  const { t } = useTranslation()
  const [overview, setOverview] = useState<SecurityStatus | null>(null)
  const [loading, setLoading] = useState(true)

  const [loadError, setLoadError] = useState(false)

  useEffect(() => {
    api.security.overview()
      .then(setOverview)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-8 w-8 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
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

  const vaultOk = overview?.vault_exists && overview?.vault_unlocked
  const guardOk = overview?.tool_guard_enabled
  const authOk = overview?.webui_auth_configured

  return (
    <div className="flex-1 overflow-y-auto bg-dc-darker">
      <div className="mx-auto max-w-3xl px-8 py-10">
        {/* Header */}
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('security.subtitle')}</p>
          <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('security.title')}</h1>
        </div>

        {/* Quick status */}
        <div className="mt-6 grid grid-cols-3 gap-2.5">
          <StatusPill label={t('security.vault')} ok={!!vaultOk} text={vaultOk ? t('common.enabled') : t('common.disabled')} />
          <StatusPill label={t('security.toolGuard')} ok={!!guardOk} text={guardOk ? t('common.enabled') : t('common.disabled')} />
          <StatusPill label={t('security.auth')} ok={!!authOk} text={authOk ? t('common.enabled') : t('common.disabled')} />
        </div>

        <div className="mt-6 space-y-3">
          <VaultSection exists={overview?.vault_exists ?? false} unlocked={overview?.vault_unlocked ?? false} />
          <ToolGuardSection enabled={overview?.tool_guard_enabled ?? false} />
          <APIKeysSection
            gatewayConfigured={overview?.gateway_auth_configured ?? false}
            webuiConfigured={overview?.webui_auth_configured ?? false}
          />
          <AuditLogSection entryCount={overview?.audit_entry_count ?? 0} />
        </div>
      </div>
    </div>
  )
}

/* ── Status Pill ── */

function StatusPill({ label, ok, text }: { label: string; ok: boolean; text: string }) {
  return (
    <div className={`rounded-xl px-3.5 py-2.5 ring-1 ${
      ok ? 'bg-emerald-500/3 ring-emerald-500/15' : 'bg-zinc-800/30 ring-zinc-700/20'
    }`}>
      <span className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{label}</span>
      <div className="mt-0.5 flex items-center gap-1.5">
        <span className={`h-1.5 w-1.5 rounded-full ${ok ? 'bg-emerald-400' : 'bg-zinc-600'}`} />
        <span className={`text-xs font-medium ${ok ? 'text-emerald-400' : 'text-zinc-500'}`}>{text}</span>
      </div>
    </div>
  )
}

/* ── Accordion wrapper ── */

function Accordion({
  icon,
  iconColor,
  title,
  subtitle,
  badge,
  defaultOpen = false,
  onOpen,
  children,
}: {
  icon: React.ReactNode
  iconColor: string
  title: string
  subtitle: string
  badge?: React.ReactNode
  defaultOpen?: boolean
  onOpen?: () => void
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)

  const toggle = () => {
    const next = !open
    setOpen(next)
    if (next && onOpen) onOpen()
  }

  return (
    <section className="overflow-hidden rounded-2xl border border-white/6 bg-(--color-dc-dark)/80">
      <button
        onClick={toggle}
        aria-expanded={open}
        className="flex w-full cursor-pointer items-center gap-4 px-5 py-4 text-left transition-colors hover:bg-white/2"
      >
        <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${iconColor}`}>
          {icon}
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-bold text-white">{title}</h3>
          <p className="text-[11px] text-zinc-500">{subtitle}</p>
        </div>
        {badge}
        <ChevronDown className={`h-4 w-4 shrink-0 text-zinc-600 transition-transform ${open ? '' : '-rotate-90'}`} />
      </button>
      {open && <div className="border-t border-white/4 px-5 py-5">{children}</div>}
    </section>
  )
}

/* ── Vault ── */

function VaultSection({ exists, unlocked }: { exists: boolean; unlocked: boolean }) {
  const [vault, setVault] = useState<VaultStatus | null>(null)
  const [loading, setLoading] = useState(false)

  const load = () => {
    if (vault) return
    setLoading(true)
    api.security.vault()
      .then(setVault)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  const statusBadge = (
    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ring-1 ${
      !exists
        ? 'bg-zinc-800/50 text-zinc-500 ring-zinc-700/30'
        : unlocked
        ? 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'
        : 'bg-blue-500/10 text-amber-400 ring-blue-500/20'
    }`}>
      {!exists ? 'Não configurado' : unlocked ? 'Protegido' : 'Inacessível'}
    </span>
  )

  return (
    <Accordion
      icon={<Lock className="h-4 w-4 text-violet-400" />}
      iconColor="bg-violet-500/10"
      title="Vault"
      subtitle="Cofre criptografado (AES-256-GCM + Argon2id)"
      badge={statusBadge}
      onOpen={load}
    >
      {loading ? (
        <Spinner />
      ) : !vault || !vault.exists ? (
        <EmptyState
          icon={<Lock className="h-8 w-8 text-zinc-700" />}
          title="Vault não configurado"
          description={<>Execute <Code>devclaw config vault-init</Code> ou complete o setup wizard</>}
        />
      ) : !vault.unlocked ? (
        <EmptyState
          icon={<Lock className="h-8 w-8 text-amber-400/40" />}
          title="Vault inacessível"
          description="Defina DEVCLAW_VAULT_PASSWORD no ambiente para liberar o acesso"
        />
      ) : (
        <div>
          {vault.keys.length === 0 ? (
            <EmptyState
              icon={<Key className="h-8 w-8 text-zinc-700" />}
              title="Nenhum secret armazenado"
              description="Adicione secrets via CLI ou chat"
            />
          ) : (
            <div className="space-y-1.5">
              {vault.keys.map((key) => (
                <div
                  key={key}
                  className="flex items-center gap-3 rounded-xl bg-zinc-800/30 px-4 py-3 ring-1 ring-zinc-700/20"
                >
                  <Key className="h-3.5 w-3.5 shrink-0 text-violet-400" />
                  <span className="min-w-0 flex-1 truncate font-mono text-sm text-zinc-200">{key}</span>
                  <span className="text-xs tracking-widest text-zinc-600">••••••••</span>
                </div>
              ))}
              <p className="pt-2 text-[11px] text-zinc-600">
                {vault.keys.length} secret{vault.keys.length !== 1 ? 's' : ''} armazenado{vault.keys.length !== 1 ? 's' : ''}. Valores nunca são exibidos.
              </p>
            </div>
          )}
        </div>
      )}
    </Accordion>
  )
}

/* ── Tool Guard ── */

function ToolGuardSection({ enabled }: { enabled: boolean }) {
  const [guard, setGuard] = useState<ToolGuardStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [newConfirmTool, setNewConfirmTool] = useState('')
  const [newAutoTool, setNewAutoTool] = useState('')

  const load = () => {
    if (guard) return
    setLoading(true)
    api.security.toolGuard.get()
      .then(setGuard)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  const save = async (partial: Partial<ToolGuardStatus>) => {
    if (!guard) return
    setSaving(true)
    try {
      const updated = { ...guard, ...partial }
      await api.security.toolGuard.update(updated)
      setGuard(updated)
    } catch { /* ignore */ }
    setSaving(false)
  }

  const addToList = (field: 'require_confirmation' | 'auto_approve', value: string) => {
    if (!guard || !value.trim()) return
    const current = guard[field] ?? []
    if (current.includes(value.trim())) return
    save({ [field]: [...current, value.trim()] })
    if (field === 'require_confirmation') setNewConfirmTool('')
    else setNewAutoTool('')
  }

  const removeFromList = (field: 'require_confirmation' | 'auto_approve', value: string) => {
    if (!guard) return
    save({ [field]: (guard[field] ?? []).filter((v) => v !== value) })
  }

  const statusBadge = (
    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ring-1 ${
      enabled
        ? 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'
        : 'bg-zinc-800/50 text-zinc-500 ring-zinc-700/30'
    }`}>
      {enabled ? 'Ativo' : 'Desativado'}
    </span>
  )

  return (
    <Accordion
      icon={<Shield className="h-4 w-4 text-amber-400" />}
      iconColor="bg-blue-500/10"
      title="Tool Guard"
      subtitle="Controle de permissões de ferramentas"
      badge={statusBadge}
      onOpen={load}
    >
      {loading || !guard ? (
        <Spinner />
      ) : !enabled ? (
        <EmptyState
          icon={<Shield className="h-8 w-8 text-zinc-700" />}
          title="Tool Guard desativado"
          description={<>Ative no <Code>config.yaml</Code> → <Code>security.tool_guard.enabled: true</Code></>}
        />
      ) : (
        <div className="space-y-5">
          {/* Permission toggles */}
          <div>
            <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-500">Permissões perigosas</p>
            <div className="grid gap-2 sm:grid-cols-3">
              <PermToggle
                label="Destrutivos"
                hint="rm -rf, mkfs, dd..."
                enabled={guard.allow_destructive}
                onChange={(v) => save({ allow_destructive: v })}
                disabled={saving}
                color="amber"
              />
              <PermToggle
                label="Sudo"
                hint="Execução privilegiada"
                enabled={guard.allow_sudo}
                onChange={(v) => save({ allow_sudo: v })}
                disabled={saving}
                color="red"
              />
              <PermToggle
                label="Reboot"
                hint="Desligar / reiniciar"
                enabled={guard.allow_reboot}
                onChange={(v) => save({ allow_reboot: v })}
                disabled={saving}
                color="red"
              />
            </div>
          </div>

          {/* Tag lists side by side */}
          <div className="grid gap-4 sm:grid-cols-2">
            <TagList
              label="Requer confirmação"
              hint="Pede aprovação antes de executar"
              items={guard.require_confirmation ?? []}
              color="amber"
              onRemove={(v) => removeFromList('require_confirmation', v)}
              inputValue={newConfirmTool}
              onInputChange={setNewConfirmTool}
              onAdd={(v) => addToList('require_confirmation', v)}
            />

            <TagList
              label="Auto-aprovação"
              hint="Sempre executar sem perguntar"
              items={guard.auto_approve ?? []}
              color="emerald"
              onRemove={(v) => removeFromList('auto_approve', v)}
              inputValue={newAutoTool}
              onInputChange={setNewAutoTool}
              onAdd={(v) => addToList('auto_approve', v)}
            />
          </div>

          {(guard.protected_paths ?? []).length > 0 && (
            <div>
              <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-500">Paths protegidos</p>
              <div className="flex flex-wrap gap-1.5">
                {guard.protected_paths.map((p) => (
                  <span key={p} className="rounded-lg bg-zinc-800 px-2.5 py-1 font-mono text-xs text-zinc-400 ring-1 ring-zinc-700/50">{p}</span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </Accordion>
  )
}

/* ── API Keys ── */

function APIKeysSection({ gatewayConfigured, webuiConfigured }: { gatewayConfigured: boolean; webuiConfigured: boolean }) {
  return (
    <Accordion
      icon={<Key className="h-4 w-4 text-cyan-400" />}
      iconColor="bg-cyan-500/10"
      title="Autenticação"
      subtitle="Tokens do gateway e painel web"
    >
      <div className="space-y-2">
        <AuthRow label="Gateway API" hint="Bearer token para API HTTP" configured={gatewayConfigured} />
        <AuthRow label="Web UI" hint="Senha de acesso ao painel" configured={webuiConfigured} warn={!webuiConfigured} />
      </div>
      <div className="mt-4 flex items-center gap-2 text-[11px] text-zinc-600">
        <span>Altere os tokens em</span>
        <Link to="/domain" className="inline-flex items-center gap-1 text-blue-400/70 hover:text-blue-400 transition-colors">
          Domínio & Acesso
          <ExternalLink className="h-2.5 w-2.5" />
        </Link>
      </div>
    </Accordion>
  )
}

function AuthRow({ label, hint, configured, warn }: { label: string; hint: string; configured: boolean; warn?: boolean }) {
  return (
    <div className="flex items-center justify-between rounded-xl bg-zinc-800/30 px-4 py-3 ring-1 ring-zinc-700/20">
      <div>
        <p className="text-sm font-medium text-zinc-200">{label}</p>
        <p className="text-[11px] text-zinc-500">{hint}</p>
      </div>
      {configured ? (
        <span className="flex items-center gap-1.5 text-xs font-medium text-emerald-400">
          <CheckCircle2 className="h-3.5 w-3.5" /> Configurado
        </span>
      ) : warn ? (
        <span className="flex items-center gap-1.5 text-xs font-medium text-amber-400">
          <AlertTriangle className="h-3.5 w-3.5" /> Sem proteção
        </span>
      ) : (
        <span className="flex items-center gap-1.5 text-xs text-zinc-600">
          <XCircle className="h-3.5 w-3.5" /> Não configurado
        </span>
      )}
    </div>
  )
}

/* ── Audit Log ── */

function AuditLogSection({ entryCount }: { entryCount: number }) {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(false)

  const load = () => {
    if (entries.length > 0) return
    setLoading(true)
    api.security.audit(100)
      .then((data) => setEntries(data.entries ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  return (
    <Accordion
      icon={<Activity className="h-4 w-4 text-blue-400" />}
      iconColor="bg-blue-500/10"
      title="Audit Log"
      subtitle={entryCount > 0 ? `${entryCount} registros` : 'Histórico de ações executadas'}
      onOpen={load}
    >
      {loading ? (
        <Spinner />
      ) : entries.length === 0 ? (
        <div className="flex items-center gap-3 py-4">
          <Activity className="h-5 w-5 shrink-0 text-zinc-700" />
          <div>
            <p className="text-sm text-zinc-400">Nenhuma ação registrada ainda</p>
            <p className="text-[11px] text-zinc-600">O histórico aparece conforme o agente executa ferramentas</p>
          </div>
        </div>
      ) : (
        <div className="max-h-[380px] overflow-y-auto -mx-5 -mb-5">
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-(--color-dc-dark)">
              <tr>
                <th className="px-5 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-zinc-600">Ferramenta</th>
                <th className="px-5 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-zinc-600">Caller</th>
                <th className="px-5 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-zinc-600">Status</th>
                <th className="px-5 py-2.5 text-right text-[10px] font-semibold uppercase tracking-wider text-zinc-600">Quando</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/4">
              {entries.map((e) => (
                <tr key={e.id} className="transition-colors hover:bg-white/2">
                  <td className="px-5 py-2.5 font-mono text-zinc-300">{e.tool}</td>
                  <td className="px-5 py-2.5 text-zinc-500">{e.caller || '—'}</td>
                  <td className="px-5 py-2.5">
                    {e.allowed ? (
                      <span className="inline-flex items-center gap-1 text-[10px] font-medium text-emerald-400">
                        <CheckCircle2 className="h-3 w-3" /> OK
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-[10px] font-medium text-red-400">
                        <XCircle className="h-3 w-3" /> Negado
                      </span>
                    )}
                  </td>
                  <td className="px-5 py-2.5 text-right text-zinc-600">{timeAgo(e.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Accordion>
  )
}

/* ── Shared components ── */

function Spinner() {
  return (
    <div className="flex justify-center py-8">
      <div className="h-6 w-6 rounded-full border-2 border-blue-500/30 border-t-blue-500 animate-spin" />
    </div>
  )
}

function EmptyState({ icon, title, description }: { icon: React.ReactNode; title: string; description: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center py-8">
      {icon}
      <p className="mt-3 text-sm font-medium text-zinc-400">{title}</p>
      <p className="mt-1 text-xs text-zinc-600 text-center max-w-xs">{description}</p>
    </div>
  )
}

function Code({ children }: { children: React.ReactNode }) {
  return <code className="rounded bg-zinc-800 px-1.5 py-0.5 text-zinc-300">{children}</code>
}

function PermToggle({
  label,
  hint,
  enabled,
  onChange,
  disabled,
  color = 'amber',
}: {
  label: string
  hint: string
  enabled: boolean
  onChange: (v: boolean) => void
  disabled?: boolean
  color?: 'amber' | 'red'
}) {
  const ringActive = color === 'red' ? 'ring-red-500/20 bg-red-500/5' : 'ring-blue-500/20 bg-blue-500/5'
  const trackActive = color === 'red' ? 'bg-red-500' : 'bg-blue-500'

  return (
    <button
      onClick={() => onChange(!enabled)}
      disabled={disabled}
      className={`flex cursor-pointer items-center gap-3 rounded-xl px-3.5 py-3 text-left ring-1 transition-all ${
        enabled ? ringActive : 'ring-zinc-700/20 bg-zinc-800/30 hover:ring-zinc-700/40'
      } ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
    >
      <div className="min-w-0 flex-1">
        <p className="text-xs font-semibold text-zinc-200">{label}</p>
        <p className="text-[10px] text-zinc-500">{hint}</p>
      </div>
      <div className={`inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${enabled ? trackActive : 'bg-zinc-700'}`}>
        <div className={`h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${enabled ? 'translate-x-4.5' : 'translate-x-0.5'}`} />
      </div>
    </button>
  )
}

function TagList({
  label,
  hint,
  items,
  color,
  onRemove,
  inputValue,
  onInputChange,
  onAdd,
}: {
  label: string
  hint?: string
  items: string[]
  color: 'amber' | 'emerald'
  onRemove: (v: string) => void
  inputValue: string
  onInputChange: (v: string) => void
  onAdd: (v: string) => void
}) {
  const tagClass = color === 'amber'
    ? 'bg-blue-500/10 text-amber-400 ring-blue-500/20'
    : 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'

  return (
    <div className="rounded-xl bg-zinc-800/20 px-4 py-3 ring-1 ring-zinc-700/15">
      <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{label}</p>
      {hint && <p className="mt-0.5 text-[10px] text-zinc-600">{hint}</p>}
      <div className="mt-2.5 flex flex-wrap gap-1.5">
        {items.map((t) => (
          <span key={t} className={`inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 font-mono text-xs ring-1 ${tagClass}`}>
            {t}
            <button onClick={() => onRemove(t)} className="cursor-pointer transition-colors hover:text-red-400">
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <form className="inline-flex" onSubmit={(e) => { e.preventDefault(); onAdd(inputValue) }}>
          <input
            value={inputValue}
            onChange={(e) => onInputChange(e.target.value)}
            placeholder={items.length === 0 ? 'nome_da_tool' : '+ adicionar'}
            className="h-7 w-28 rounded-lg bg-transparent px-2 text-xs text-zinc-400 outline-none placeholder:text-zinc-600 focus:placeholder:text-zinc-500"
          />
        </form>
      </div>
    </div>
  )
}
