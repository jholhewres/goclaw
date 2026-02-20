import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Globe,
  Server,
  Save,
  Loader2,
  CheckCircle2,
  XCircle,
  ExternalLink,
  Network,
  Eye,
  EyeOff,
  X,
  Lock,
  Unlock,
  ArrowUpRight,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { DomainConfig } from '@/lib/api'

/**
 * Domain and network configuration page.
 */
export function Domain() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<DomainConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [webuiAddress, setWebuiAddress] = useState('')
  const [webuiToken, setWebuiToken] = useState('')
  const [showWebuiToken, setShowWebuiToken] = useState(false)

  const [gatewayEnabled, setGatewayEnabled] = useState(false)
  const [gatewayAddress, setGatewayAddress] = useState('')
  const [gatewayToken, setGatewayToken] = useState('')
  const [showGatewayToken, setShowGatewayToken] = useState(false)
  const [corsOrigins, setCorsOrigins] = useState<string[]>([])
  const [newCors, setNewCors] = useState('')

  const [tailscaleEnabled, setTailscaleEnabled] = useState(false)
  const [tailscaleServe, setTailscaleServe] = useState(false)
  const [tailscaleFunnel, setTailscaleFunnel] = useState(false)
  const [tailscalePort, setTailscalePort] = useState(8085)

  useEffect(() => {
    api.domain.get()
      .then((data) => {
        setConfig(data)
        setWebuiAddress(data.webui_address || ':8090')
        setGatewayEnabled(data.gateway_enabled)
        setGatewayAddress(data.gateway_address || ':8085')
        setCorsOrigins(data.cors_origins || [])
        setTailscaleEnabled(data.tailscale_enabled)
        setTailscaleServe(data.tailscale_serve)
        setTailscaleFunnel(data.tailscale_funnel)
        setTailscalePort(data.tailscale_port || 8085)
      })
      .catch(() => setMessage({ type: 'error', text: t('common.error') }))
      .finally(() => setLoading(false))
  }, [t])

  const handleSave = async () => {
    setSaving(true)
    setMessage(null)
    try {
      await api.domain.update({
        webui_address: webuiAddress,
        webui_auth_token: webuiToken || undefined,
        gateway_enabled: gatewayEnabled,
        gateway_address: gatewayAddress,
        gateway_auth_token: gatewayToken || undefined,
        cors_origins: corsOrigins,
        tailscale_enabled: tailscaleEnabled,
        tailscale_serve: tailscaleServe,
        tailscale_funnel: tailscaleFunnel,
        tailscale_port: tailscalePort,
      })
      setMessage({ type: 'success', text: t('common.success') })
      setWebuiToken('')
      setGatewayToken('')
    } catch {
      setMessage({ type: 'error', text: t('common.error') })
    } finally {
      setSaving(false)
    }
  }

  const addCorsOrigin = () => {
    const trimmed = newCors.trim()
    if (trimmed && !corsOrigins.includes(trimmed)) {
      setCorsOrigins([...corsOrigins, trimmed])
      setNewCors('')
    }
  }

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-10 w-10 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-dc-darker">
      <div className="mx-auto w-full max-w-3xl flex-1 overflow-y-auto px-8 py-10">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">Rede</p>
            <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Domínio & Acesso</h1>
          </div>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex cursor-pointer items-center gap-2 rounded-xl bg-blue-500 px-5 py-2.5 text-sm font-semibold text-white shadow-lg shadow-blue-500/15 transition-all hover:bg-blue-400 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {saving ? 'Salvando...' : 'Salvar'}
          </button>
        </div>

        {/* Toast */}
        {message && (
          <div className={`mt-5 flex items-center gap-2.5 rounded-xl px-4 py-3 text-sm ring-1 ${
            message.type === 'success'
              ? 'bg-emerald-500/5 text-emerald-400 ring-emerald-500/20'
              : 'bg-red-500/5 text-red-400 ring-red-500/20'
          }`}>
            {message.type === 'success' ? <CheckCircle2 className="h-4 w-4 shrink-0" /> : <XCircle className="h-4 w-4 shrink-0" />}
            {message.text}
          </div>
        )}

        {/* Status overview */}
        {config && (
          <div className="mt-6 grid grid-cols-3 gap-2.5">
            <Endpoint
              label="WebUI"
              url={config.webui_url}
              active
              secure={config.webui_auth_configured}
            />
            <Endpoint
              label="Gateway"
              url={config.gateway_url}
              active={config.gateway_enabled}
              secure={config.gateway_auth_configured}
            />
            <Endpoint
              label="Tailscale"
              url={config.public_url || config.tailscale_url}
              active={config.tailscale_enabled}
              secure
            />
          </div>
        )}

        {/* ── WebUI ── */}
        <Card className="mt-8">
          <CardHeader icon={Globe} title="Web UI" />
          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <Field label="Porta">
              <Input value={webuiAddress} onChange={setWebuiAddress} placeholder=":8090" />
            </Field>
            <Field label="Senha">
              <PasswordInput
                value={webuiToken}
                onChange={setWebuiToken}
                show={showWebuiToken}
                onToggle={() => setShowWebuiToken(!showWebuiToken)}
                placeholder={config?.webui_auth_configured ? '••••••••' : 'Sem senha'}
              />
            </Field>
          </div>
        </Card>

        {/* ── Gateway ── */}
        <Card className="mt-4">
          <div className="flex items-center justify-between">
            <CardHeader icon={Server} title="Gateway API" />
            <Toggle value={gatewayEnabled} onChange={setGatewayEnabled} />
          </div>

          {gatewayEnabled && (
            <div className="mt-5 space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Porta">
                  <Input value={gatewayAddress} onChange={setGatewayAddress} placeholder=":8085" />
                </Field>
                <Field label="Auth Token">
                  <PasswordInput
                    value={gatewayToken}
                    onChange={setGatewayToken}
                    show={showGatewayToken}
                    onToggle={() => setShowGatewayToken(!showGatewayToken)}
                    placeholder={config?.gateway_auth_configured ? '••••••••' : 'Sem token'}
                  />
                </Field>
              </div>

              {/* CORS */}
              <Field label="CORS Origins">
                <div className="flex flex-wrap gap-1.5">
                  {corsOrigins.map((origin) => (
                    <span
                      key={origin}
                      className="group flex items-center gap-1.5 rounded-lg bg-zinc-800 px-2.5 py-1.5 text-xs font-mono text-zinc-300 ring-1 ring-zinc-700/50"
                    >
                      {origin}
                      <button
                        onClick={() => setCorsOrigins(corsOrigins.filter((o) => o !== origin))}
                        className="cursor-pointer text-zinc-600 transition-colors hover:text-red-400"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                  <input
                    value={newCors}
                    onChange={(e) => setNewCors(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && addCorsOrigin()}
                    onBlur={addCorsOrigin}
                    placeholder="+ adicionar origem"
                    className="min-w-[140px] flex-1 rounded-lg bg-transparent px-2 py-1.5 text-xs text-zinc-400 outline-none placeholder:text-zinc-600"
                  />
                </div>
              </Field>
            </div>
          )}
        </Card>

        {/* ── Tailscale ── */}
        <Card className="mt-4 mb-10">
          <div className="flex items-center justify-between">
            <CardHeader icon={Network} title="Tailscale" />
            <Toggle value={tailscaleEnabled} onChange={setTailscaleEnabled} />
          </div>

          {tailscaleEnabled && (
            <div className="mt-5 space-y-3">
              <ToggleRow
                label="Serve"
                description="HTTPS dentro da sua Tailnet"
                value={tailscaleServe}
                onChange={setTailscaleServe}
              />
              <ToggleRow
                label="Funnel"
                description="HTTPS público na internet"
                value={tailscaleFunnel}
                onChange={setTailscaleFunnel}
              />

              <div className="pt-1">
                <Field label="Porta local">
                  <Input
                    value={String(tailscalePort)}
                    onChange={(v) => setTailscalePort(parseInt(v) || 8085)}
                    placeholder="8085"
                    type="number"
                  />
                </Field>
              </div>

              {config?.tailscale_hostname && (
                <div className="flex items-center gap-3 rounded-xl bg-emerald-500/5 px-4 py-3 ring-1 ring-emerald-500/15">
                  <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-400" />
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium text-zinc-200 truncate">{config.tailscale_hostname}</p>
                    {config.tailscale_url && (
                      <a
                        href={config.tailscale_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-1 text-xs text-emerald-400/70 hover:text-emerald-400 transition-colors"
                      >
                        {config.tailscale_url}
                        <ArrowUpRight className="h-3 w-3" />
                      </a>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}

/* ── Components ── */

function Card({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={`rounded-2xl border border-white/6 bg-(--color-dc-dark)/80 p-5 ${className}`}>
      {children}
    </div>
  )
}

function CardHeader({ icon: Icon, title }: { icon: React.FC<{ className?: string }>; title: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-500/10">
        <Icon className="h-4 w-4 text-blue-400" />
      </div>
      <h2 className="text-sm font-bold text-white">{title}</h2>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1.5 block text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{label}</label>
      {children}
    </div>
  )
}

function Input({
  value,
  onChange,
  placeholder,
  type = 'text',
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="flex h-10 w-full rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/10"
    />
  )
}

function PasswordInput({
  value,
  onChange,
  show,
  onToggle,
  placeholder,
}: {
  value: string
  onChange: (v: string) => void
  show: boolean
  onToggle: () => void
  placeholder?: string
}) {
  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="flex h-10 w-full rounded-lg border border-zinc-700/50 bg-zinc-800/50 px-3 pr-9 text-sm text-white placeholder:text-zinc-600 outline-none transition-all focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/10"
      />
      <button
        type="button"
        onClick={onToggle}
        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-zinc-600 hover:text-zinc-300 transition-colors"
      >
        {show ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
      </button>
    </div>
  )
}

function Toggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={value}
      onClick={() => onChange(!value)}
      className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors ${
        value ? 'bg-blue-500' : 'bg-zinc-700'
      }`}
    >
      <span
        className={`inline-block h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${
          value ? 'translate-x-5' : 'translate-x-0.5'
        }`}
      />
    </button>
  )
}

function ToggleRow({
  label,
  description,
  value,
  onChange,
}: {
  label: string
  description: string
  value: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between rounded-xl bg-zinc-800/30 px-4 py-3 ring-1 ring-zinc-700/20">
      <div>
        <span className="text-sm font-medium text-zinc-200">{label}</span>
        <p className="text-[11px] text-zinc-500">{description}</p>
      </div>
      <Toggle value={value} onChange={onChange} />
    </div>
  )
}

function Endpoint({
  label,
  url,
  active,
  secure,
}: {
  label: string
  url?: string
  active: boolean
  secure: boolean
}) {
  return (
    <div className={`rounded-xl px-3.5 py-2.5 ring-1 transition-colors ${
      active
        ? 'bg-emerald-500/3 ring-emerald-500/15'
        : 'bg-zinc-800/30 ring-zinc-700/20'
    }`}>
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">{label}</span>
        <div className="flex items-center gap-1">
          {active ? (
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
          ) : (
            <span className="h-1.5 w-1.5 rounded-full bg-zinc-600" />
          )}
          {active && (secure ? (
            <Lock className="h-3 w-3 text-emerald-400/60" />
          ) : (
            <Unlock className="h-3 w-3 text-amber-400/60" />
          ))}
        </div>
      </div>
      {url ? (
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="mt-1 flex items-center gap-1 text-[11px] font-mono text-zinc-400 hover:text-blue-400 transition-colors truncate"
        >
          {url.replace(/^https?:\/\//, '')}
          <ExternalLink className="h-2.5 w-2.5 shrink-0" />
        </a>
      ) : (
        <p className="mt-1 text-[11px] text-zinc-600">{active ? 'Ativo' : 'Off'}</p>
      )}
    </div>
  )
}
