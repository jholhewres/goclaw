import { useParams } from 'react-router-dom'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Terminal,
  GitBranch,
  Database,
  Globe,
  FileCode,
  Server,
  Wrench,
  Zap,
} from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'

/** Extracts a user-friendly message from raw LLM/API errors. */
function friendlyError(raw: string, t: (key: string) => string): string {
  if (raw.includes('404')) return t('chatPage.errorModel')
  if (raw.includes('401') || raw.includes('authentication')) return t('chatPage.errorAuth')
  if (raw.includes('429') || raw.includes('rate_limit')) return t('chatPage.errorRateLimit')
  if (raw.includes('500') || raw.includes('server_error')) return t('chatPage.errorServer')
  if (raw.includes('timeout') || raw.includes('ETIMEDOUT')) return t('chatPage.errorTimeout')
  if (raw.includes('ECONNREFUSED')) return t('chatPage.errorConnect')
  if (raw.includes('LLM call failed')) {
    const match = raw.match(/API returned (\d+)/)
    if (match) return `${t('chatPage.errorGeneric')} (${match[1]})`
    return t('chatPage.errorGeneric')
  }
  return raw
}

export function Chat() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()
  const resolvedId = sessionId ? decodeURIComponent(sessionId) : 'webui:default'
  const { messages, streamingContent, isStreaming, error, sendMessage, abort } = useChat(resolvedId)
  const bottomRef = useRef<HTMLDivElement>(null)

  const SUGGESTIONS = [
    { icon: GitBranch, label: t('chatPage.gitStatus'), prompt: t('chatPage.gitStatusPrompt') },
    { icon: Server, label: t('chatPage.processes'), prompt: t('chatPage.processesPrompt') },
    { icon: Database, label: t('chatPage.dbSchema'), prompt: t('chatPage.dbSchemaPrompt') },
    { icon: FileCode, label: t('chatPage.analyzeCode'), prompt: t('chatPage.analyzeCodePrompt') },
    { icon: Globe, label: t('chatPage.apiTest'), prompt: t('chatPage.apiTestPrompt') },
    { icon: Wrench, label: t('chatPage.dockerPs'), prompt: t('chatPage.dockerPsPrompt') },
  ]

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  const hasMessages = messages.length > 0 || streamingContent

  const friendlyErrorLocal = (raw: string) => friendlyError(raw, t)

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-dc-darker">
      <div className="flex-1 overflow-y-auto">
        {!hasMessages ? (
          <div className="flex h-full flex-col items-center justify-center px-6">
            <div className="flex flex-col items-center -mt-12">
              {/* Logo */}
              <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-linear-to-br from-blue-500/20 to-blue-500/10 ring-1 ring-blue-500/20">
                <Terminal className="h-8 w-8 text-blue-400" />
              </div>

              <h2 className="mt-5 text-xl font-bold text-white">{t('chatPage.whatDo')}</h2>
              <p className="mt-1.5 text-sm text-zinc-500">{t('chatPage.askOrPick')}</p>

              {/* Suggestions grid */}
              <div className="mt-8 grid w-full max-w-lg grid-cols-2 gap-2 sm:grid-cols-3">
                {SUGGESTIONS.map((s) => (
                  <button
                    key={s.label}
                    onClick={() => sendMessage(s.prompt)}
                    className="group flex cursor-pointer items-center gap-2.5 rounded-xl bg-zinc-800/40 px-3.5 py-3 text-left ring-1 ring-zinc-700/20 transition-all hover:bg-zinc-800/60 hover:ring-blue-500/20"
                  >
                    <s.icon className="h-4 w-4 shrink-0 text-zinc-500 transition-colors group-hover:text-blue-400" />
                    <span className="text-xs font-medium text-zinc-400 transition-colors group-hover:text-zinc-200">{s.label}</span>
                  </button>
                ))}
              </div>

              {/* Quick tips */}
              <div className="mt-6 flex items-center gap-4 text-[11px] text-zinc-600">
                <span className="flex items-center gap-1.5">
                  <Zap className="h-3 w-3 text-blue-500/50" />
                  {t('chatPage.nativeTools')}
                </span>
                <span className="h-3 w-px bg-zinc-700/50" />
                <span>{t('chatPage.enterToSend')}</span>
              </div>
            </div>
          </div>
        ) : (
          <div className="mx-auto max-w-3xl space-y-1 px-6 py-8">
            {messages.map((msg, i) => (
              <ChatMessage key={`${msg.role}-${msg.timestamp}-${i}`} role={msg.role} content={msg.content} toolName={msg.tool_name} toolInput={msg.tool_input} />
            ))}
            {streamingContent && (
              <ChatMessage role="assistant" content={streamingContent} isStreaming />
            )}
            {error && (
              <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-5 py-4">
                <p className="text-sm font-medium text-red-400">{friendlyErrorLocal(error)}</p>
                {error !== friendlyErrorLocal(error) && (
                  <details className="mt-2">
                    <summary className="cursor-pointer text-xs text-red-400/60 hover:text-red-400/80">{t('chatPage.technicalDetails')}</summary>
                    <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap font-mono text-xs text-red-400/50">{error}</pre>
                  </details>
                )}
              </div>
            )}
            <div ref={bottomRef} />
          </div>
        )}
      </div>

      <div className="mx-auto w-full max-w-3xl">
        <ChatInput onSend={sendMessage} onAbort={abort} isStreaming={isStreaming} />
      </div>
    </div>
  )
}
