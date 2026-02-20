import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  AlertTriangle,
  Radio,
  QrCode,
  Wifi,
  WifiOff,
  Smartphone,
  MessageCircle,
  ArrowRight,
  Clock,
  Check,
  X,
} from 'lucide-react'
import { api, type ChannelHealth } from '@/lib/api'
import { timeAgo, cn } from '@/lib/utils'

/**
 * Channel management page.
 * Shows status of all configured channels and allows
 * connecting/reconnecting WhatsApp via QR code.
 */
export function Channels() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [channels, setChannels] = useState<ChannelHealth[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.channels.list()
      .then(setChannels)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const whatsapp = channels.find((ch) => ch.name === 'whatsapp')
  const otherChannels = channels.filter((ch) => ch.name !== 'whatsapp')

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
        <div className="h-8 w-8 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
      </div>
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-4xl mx-auto">
      <div>
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{t('channels.subtitle')}</p>
        <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">{t('channels.title')}</h1>
        <p className="mt-2 text-sm text-[#64748b]">
          Conecte canais de comunicação para enviar e receber mensagens
        </p>
      </div>

      {channels.length === 0 ? (
        <EmptyChannels />
      ) : (
        <div className="mt-8 space-y-4">
          {whatsapp && <WhatsAppCard channel={whatsapp} onNavigate={() => navigate('/channels/whatsapp')} />}
          {otherChannels.map((ch) => <ChannelCard key={ch.name} channel={ch} />)}
        </div>
      )}
    </div>
  )
}

/* ── WhatsApp Card ── */

function WhatsAppCard({ channel, onNavigate }: { channel: ChannelHealth; onNavigate: () => void }) {
  const connected = channel.connected
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'

  return (
    <div className={cn(
      'rounded-2xl p-6 border transition-all',
      connected
        ? 'bg-[#111827] border-[#22c55e]/20'
        : 'bg-[#111827] border-white/10'
    )}>
      <div className="flex items-start gap-4">
        {/* Icon */}
        <div className={cn(
          'flex h-12 w-12 shrink-0 items-center justify-center rounded-xl transition-colors',
          connected ? 'bg-[#22c55e]/10' : 'bg-[#1e293b]'
        )}>
          <WhatsAppIcon className={cn('h-6 w-6', connected ? 'text-[#22c55e]' : 'text-[#64748b]')} />
        </div>

        {/* Content */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h3 className="text-base font-semibold text-[#f8fafc]">WhatsApp</h3>
            <StatusBadge connected={connected} />
          </div>

          <p className="mt-1 text-sm text-[#64748b]">
            {connected
              ? hasLastMsg
                ? `Última mensagem ${timeAgo(channel.last_msg_at)}`
                : 'Conectado — aguardando mensagens'
              : 'Escaneie o QR code para conectar'}
          </p>

          {/* Actions */}
          <div className="mt-4 flex items-center gap-3">
            <button
              onClick={onNavigate}
              className={cn(
                'flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-all',
                connected
                  ? 'bg-[#1e293b] text-[#94a3b8] border border-white/10 hover:bg-[#334155] hover:text-[#f8fafc]'
                  : 'bg-[#3b82f6] text-white hover:bg-[#2563eb]'
              )}
            >
              {connected ? (
                <>
                  <Smartphone className="h-4 w-4" />
                  Gerenciar
                </>
              ) : (
                <>
                  <QrCode className="h-4 w-4" />
                  Conectar via QR
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
            </button>

            {channel.error_count > 0 && (
              <span className="flex items-center gap-1.5 rounded-lg bg-[#f59e0b]/10 border border-[#f59e0b]/20 px-3 py-2 text-xs font-medium text-[#f59e0b]">
                <AlertTriangle className="h-3.5 w-3.5" />
                {channel.error_count} {channel.error_count === 1 ? 'erro' : 'erros'}
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

/* ── Generic Channel Card ── */

function ChannelCard({ channel }: { channel: ChannelHealth }) {
  const connected = channel.connected
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'

  const channelConfig: Record<string, { name: string; color: string }> = {
    discord: { name: 'Discord', color: '#5865F2' },
    telegram: { name: 'Telegram', color: '#0088cc' },
    slack: { name: 'Slack', color: '#4A154B' },
  }

  const config = channelConfig[channel.name] || { name: channel.name, color: '#64748b' }

  return (
    <div className={cn(
      'rounded-xl p-5 border transition-all',
      connected
        ? 'bg-[#111827] border-white/10'
        : 'bg-[#111827]/50 border-white/5'
    )}>
      <div className="flex items-center gap-4">
        {/* Icon */}
        <div className={cn(
          'flex h-10 w-10 shrink-0 items-center justify-center rounded-lg transition-colors',
          connected ? 'bg-[#1e293b]' : 'bg-[#1e293b]/50'
        )}>
          {connected ? (
            <Wifi className="h-5 w-5 text-[#22c55e]" />
          ) : (
            <WifiOff className="h-5 w-5 text-[#475569]" />
          )}
        </div>

        {/* Content */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h3 className="text-sm font-semibold text-[#f8fafc]">{config.name}</h3>
            <StatusBadge connected={connected} small />
          </div>
          <p className="mt-0.5 text-xs text-[#64748b]">
            {connected
              ? hasLastMsg
                ? `Última mensagem ${timeAgo(channel.last_msg_at)}`
                : 'Conectado'
              : 'Desconectado'}
          </p>
        </div>

        {/* Status indicators */}
        <div className="flex items-center gap-3">
          {channel.error_count > 0 && (
            <span className="flex items-center gap-1 text-xs text-[#f59e0b]">
              <AlertTriangle className="h-3.5 w-3.5" />
              {channel.error_count}
            </span>
          )}
          {hasLastMsg && (
            <span className="flex items-center gap-1.5 text-xs text-[#475569]">
              <Clock className="h-3.5 w-3.5" />
              {timeAgo(channel.last_msg_at)}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

/* ── Empty State ── */

function EmptyChannels() {
  return (
    <div className="mt-8 rounded-2xl border border-white/10 bg-[#111827] p-8">
      <div className="flex flex-col items-center text-center">
        <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-[#1e293b]">
          <Radio className="h-7 w-7 text-[#64748b]" />
        </div>
        <h3 className="mt-4 text-base font-semibold text-[#f8fafc]">Nenhum canal configurado</h3>
        <p className="mt-2 max-w-md text-sm text-[#64748b]">
          Canais permitem que o DevClaw envie e receba mensagens via WhatsApp, Discord, Telegram e Slack.
        </p>
      </div>

      <div className="mt-6 mx-auto max-w-md rounded-xl bg-[#0c1222] border border-white/10 p-4">
        <p className="text-[10px] font-semibold uppercase tracking-wider text-[#475569]">Exemplo em config.yaml</p>
        <pre className="mt-3 overflow-x-auto font-mono text-xs leading-relaxed text-[#94a3b8]">
{`channels:
  whatsapp:
    enabled: true
    owner_phone: "5511999999999"
  discord:
    enabled: true
    token: "\${DEVCLAW_DISCORD_TOKEN}"`}
        </pre>
      </div>

      <div className="mt-5 flex items-center justify-center gap-6 text-xs text-[#475569]">
        <span className="flex items-center gap-2">
          <MessageCircle className="h-3.5 w-3.5" />
          WhatsApp, Discord, Telegram, Slack
        </span>
        <span className="h-3 w-px bg-white/10" />
        <span>Tokens são armazenados no vault</span>
      </div>
    </div>
  )
}

/* ── Status Badge ── */

function StatusBadge({ connected, small = false }: { connected: boolean; small?: boolean }) {
  return (
    <span className={cn(
      'inline-flex items-center gap-1.5 rounded-full font-medium transition-colors',
      small
        ? 'px-2 py-0.5 text-[10px]'
        : 'px-2.5 py-1 text-[11px]',
      connected
        ? 'bg-[#22c55e]/10 text-[#22c55e]'
        : 'bg-[#475569]/20 text-[#64748b]'
    )}>
      {connected ? (
        <Check className={cn('shrink-0', small ? 'h-3 w-3' : 'h-3.5 w-3.5')} />
      ) : (
        <X className={cn('shrink-0', small ? 'h-3 w-3' : 'h-3.5 w-3.5')} />
      )}
      {connected ? 'Online' : 'Offline'}
    </span>
  )
}

/* ── WhatsApp Icon ── */

function WhatsAppIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor">
      <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
    </svg>
  )
}
