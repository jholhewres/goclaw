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
} from 'lucide-react'
import { api, type ChannelHealth } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

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
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-8 w-8 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto bg-dc-darker">
      <div className="mx-auto max-w-3xl px-8 py-10">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('channels.subtitle')}</p>
          <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('channels.title')}</h1>
        </div>

        {channels.length === 0 ? (
          <EmptyChannels />
        ) : (
          <div className="mt-6 space-y-3">
            {whatsapp && <WhatsAppCard channel={whatsapp} onNavigate={() => navigate('/channels/whatsapp')} />}
            {otherChannels.map((ch) => <ChannelCard key={ch.name} channel={ch} />)}
          </div>
        )}
      </div>
    </div>
  )
}

/* ── WhatsApp Card ── */

function WhatsAppCard({ channel, onNavigate }: { channel: ChannelHealth; onNavigate: () => void }) {
  const connected = channel.connected
  const hasLastMsg = channel.last_msg_at && channel.last_msg_at !== '0001-01-01T00:00:00Z'

  return (
    <div className={`rounded-2xl p-5 ring-1 transition-colors ${
      connected ? 'bg-emerald-500/3 ring-emerald-500/15' : 'ring-zinc-700/30 bg-zinc-800/20'
    }`}>
      <div className="flex items-start gap-4">
        <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-xl ${
          connected ? 'bg-emerald-500/10' : 'bg-blue-500/10'
        }`}>
          <WhatsAppIcon className={`h-5 w-5 ${connected ? 'text-emerald-400' : 'text-blue-400'}`} />
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h3 className="text-sm font-bold text-white">WhatsApp</h3>
            <StatusDot connected={connected} />
          </div>

          <p className="mt-0.5 text-[11px] text-zinc-500">
            {connected
              ? hasLastMsg
                ? `Last message ${timeAgo(channel.last_msg_at)}`
                : 'Connected — waiting for messages'
              : 'Scan QR code to connect'}
          </p>

          <div className="mt-3 flex items-center gap-2">
            <button
              onClick={onNavigate}
              className={`flex cursor-pointer items-center gap-2 rounded-lg px-3.5 py-2 text-xs font-semibold transition-all ${
                connected
                  ? 'bg-zinc-800 text-zinc-300 ring-1 ring-zinc-700/30 hover:bg-zinc-700'
                  : 'bg-blue-500 text-white shadow-lg shadow-blue-500/15 hover:bg-blue-400'
              }`}
            >
              {connected ? (
                <>
                  <Smartphone className="h-3.5 w-3.5" />
                  Manage
                </>
              ) : (
                <>
                  <QrCode className="h-3.5 w-3.5" />
                  Connect via QR Code
                  <ArrowRight className="h-3 w-3" />
                </>
              )}
            </button>

            {channel.error_count > 0 && (
              <span className="flex items-center gap-1 rounded-lg bg-blue-500/10 px-2.5 py-1.5 text-[11px] font-medium text-amber-400 ring-1 ring-blue-500/15">
                <AlertTriangle className="h-3 w-3" />
                {channel.error_count} error{channel.error_count !== 1 ? 's' : ''}
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

  const channelNames: Record<string, string> = {
    discord: 'Discord',
    telegram: 'Telegram',
    slack: 'Slack',
  }

  return (
    <div className={`rounded-xl px-5 py-4 ring-1 transition-colors ${
      connected ? 'bg-emerald-500/3 ring-emerald-500/15' : 'bg-zinc-800/30 ring-zinc-700/20'
    }`}>
      <div className="flex items-center gap-4">
        <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${
          connected ? 'bg-emerald-500/10' : 'bg-zinc-800'
        }`}>
          {connected ? <Wifi className="h-4 w-4 text-emerald-400" /> : <WifiOff className="h-4 w-4 text-zinc-500" />}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-bold text-white">{channelNames[channel.name] || channel.name}</h3>
            <StatusDot connected={connected} />
          </div>
          <p className="text-[11px] text-zinc-500">
            {connected
              ? hasLastMsg
                ? `Last message ${timeAgo(channel.last_msg_at)}`
                : 'Connected'
              : 'Disconnected'}
          </p>
        </div>

        <div className="flex items-center gap-2">
          {channel.error_count > 0 && (
            <span className="flex items-center gap-1 text-[11px] text-amber-400">
              <AlertTriangle className="h-3 w-3" />
              {channel.error_count}
            </span>
          )}
          {hasLastMsg && (
            <span className="flex items-center gap-1 text-[11px] text-zinc-600">
              <Clock className="h-3 w-3" />
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
    <div className="mt-8 rounded-2xl border border-white/6 bg-(--color-dc-dark)/80 px-6 py-12">
      <div className="flex flex-col items-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-blue-500/10">
          <MessageCircle className="h-6 w-6 text-blue-400" />
        </div>
        <h3 className="mt-4 text-sm font-bold text-zinc-300">No channels configured</h3>
        <p className="mt-1.5 max-w-sm text-center text-xs text-zinc-500">
          Channels allow DevClaw to send and receive messages via WhatsApp, Discord, Telegram, and Slack.
        </p>
      </div>

      <div className="mt-6 mx-auto max-w-md rounded-xl bg-zinc-800/30 px-4 py-3 ring-1 ring-zinc-700/20">
        <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">Example in config.yaml</p>
        <pre className="mt-2 overflow-x-auto font-mono text-xs leading-relaxed text-zinc-400">
{`channels:
  whatsapp:
    enabled: true
    owner_phone: "5511999999999"
  discord:
    enabled: true
    token: "\${DEVCLAW_DISCORD_TOKEN}"`}</pre>
      </div>

      <div className="mt-4 flex items-center justify-center gap-4 text-[11px] text-zinc-600">
        <span className="flex items-center gap-1.5">
          <Radio className="h-3 w-3 text-zinc-500" />
          WhatsApp, Discord, Telegram, Slack
        </span>
        <span className="h-3 w-px bg-zinc-700/50" />
        <span>Tokens are stored in the vault</span>
      </div>
    </div>
  )
}

/* ── Shared ── */

function StatusDot({ connected }: { connected: boolean }) {
  return (
    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
      connected
        ? 'bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/20'
        : 'bg-zinc-800 text-zinc-500 ring-1 ring-zinc-700/30'
    }`}>
      <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-400' : 'bg-zinc-600'}`} />
      {connected ? 'Online' : 'Offline'}
    </span>
  )
}

function WhatsAppIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor">
      <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
    </svg>
  )
}
