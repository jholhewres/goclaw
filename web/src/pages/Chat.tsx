import { useParams, useNavigate } from 'react-router-dom'
import { useEffect, useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, MessageSquare, Clock, Loader2 } from 'lucide-react'
import { ChatMessage } from '@/components/ChatMessage'
import { ChatInput } from '@/components/ChatInput'
import { useChat } from '@/hooks/useChat'
import { api, type SessionInfo } from '@/lib/api'
import { timeAgo, cn } from '@/lib/utils'

/** Generate a unique session ID */
function generateSessionId(): string {
  const timestamp = Date.now().toString(36)
  const random = Math.random().toString(36).substring(2, 8)
  return `webui:${timestamp}-${random}`
}

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
  const navigate = useNavigate()

  // URL session ID (when navigating to existing sessions)
  const urlSessionId = sessionId ? decodeURIComponent(sessionId) : null

  // Local chat state (for new chats without navigation)
  const [localSessionId, setLocalSessionId] = useState<string | null>(null)
  const [initialMessage, setInitialMessage] = useState<string | null>(null)

  // Effective session ID: either from URL or from local state
  const resolvedId = urlSessionId || localSessionId

  // Chat hook
  const { messages, streamingContent, isStreaming, error, isLoadingHistory, sendMessage: chatSend, abort } = useChat(resolvedId)

  const bottomRef = useRef<HTMLDivElement>(null)
  const [recentSessions, setRecentSessions] = useState<SessionInfo[]>([])
  const [showSidebar, setShowSidebar] = useState(false)

  // Keep chatSend in a ref to avoid stale closures
  const chatSendRef = useRef(chatSend)
  useEffect(() => {
    chatSendRef.current = chatSend
  }, [chatSend])

  // Track if initial message has been sent
  const initialMessageSentRef = useRef(false)

  // Send initial message when we have a sessionId and an initial message
  // Use a small delay to ensure useChat has processed the sessionId change
  useEffect(() => {
    if (resolvedId && initialMessage && !initialMessageSentRef.current) {
      initialMessageSentRef.current = true
      // Small delay to ensure useChat's useEffect has run and cleared/initialized state
      const timeoutId = setTimeout(() => {
        chatSendRef.current(initialMessage)
      }, 50)
      return () => clearTimeout(timeoutId)
    }
  }, [resolvedId, initialMessage])

  // Reset state when navigating to a different URL session
  useEffect(() => {
    if (urlSessionId) {
      // URL changed to a specific session - reset local state
      setLocalSessionId(null)
      setInitialMessage(null)
      initialMessageSentRef.current = false
    }
  }, [urlSessionId])

  // Load recent sessions
  useEffect(() => {
    api.sessions.list().then((sessions) => {
      const webuiSessions = sessions
        .filter(s => s.channel === 'webui' || s.id.startsWith('webui:'))
        .slice(0, 10)
      setRecentSessions(webuiSessions)
    }).catch(() => {})
  }, [messages.length])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  // Determine if we should show the chat view (vs hero)
  const hasMessages = messages.length > 0 || streamingContent || isStreaming
  const showChatView = hasMessages || !!resolvedId

  const friendlyErrorLocal = (raw: string) => friendlyError(raw, t)

  // Handle sending a message
  const sendMessage = useCallback((content: string) => {
    if (resolvedId) {
      // Already have a session - send directly
      chatSend(content)
    } else {
      // No session yet - create one locally and set initial message
      // This will trigger the chat view and send the message
      const newSessionId = generateSessionId()
      initialMessageSentRef.current = false
      setInitialMessage(content)
      setLocalSessionId(newSessionId)
    }
  }, [resolvedId, chatSend])

  // Update URL after conversation starts (non-blocking)
  useEffect(() => {
    if (localSessionId && hasMessages && !urlSessionId) {
      // Update URL to reflect the session without navigation
      window.history.replaceState({}, '', `/chat/${encodeURIComponent(localSessionId)}`)
    }
  }, [localSessionId, hasMessages, urlSessionId])

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)]">
      <div className="flex flex-1 overflow-hidden">
        {/* Main chat area */}
        <div className="flex-1 flex flex-col overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            {!showChatView ? (
              <div className="flex flex-col h-full items-center justify-center px-6">
                <div className="w-full max-w-2xl space-y-6">
                  {/* Branding */}
                  <div className="text-center space-y-3">
                    <h1 className="text-3xl md:text-[40px] font-bold text-[#f8fafc] tracking-tight leading-tight">
                      {t('chatPage.howCanHelp')}
                    </h1>
                    <p className="text-sm text-[#64748b] max-w-md mx-auto">
                      {t('chatPage.howCanHelpDesc')}
                    </p>
                  </div>

                  {/* Input */}
                  <ChatInput
                    onSend={sendMessage}
                    onAbort={abort}
                    isStreaming={isStreaming}
                    placeholder={t('chatPage.placeholder')}
                  />

                  {/* Recent sessions */}
                  {recentSessions.length > 0 && (
                    <div className="mt-4">
                      <button
                        onClick={() => setShowSidebar(!showSidebar)}
                        className="flex items-center gap-2 text-sm text-[#64748b] hover:text-[#f8fafc] transition-colors mx-auto"
                      >
                        <Clock className="h-4 w-4" />
                        <span>Conversas recentes</span>
                      </button>
                    </div>
                  )}
                </div>
              </div>
            ) : (
              /* Messages */
              <div className="py-6">
                <div className="mx-auto max-w-3xl px-4 sm:px-6 lg:px-8 space-y-4">
                  {/* Loading indicator when history is loading */}
                  {isLoadingHistory && (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="h-6 w-6 animate-spin text-[#64748b]" />
                    </div>
                  )}
                  {messages.map((msg, i) => (
                    <ChatMessage
                      key={`${msg.role}-${msg.timestamp}-${i}`}
                      role={msg.role}
                      content={msg.content}
                      toolName={msg.tool_name}
                      toolInput={msg.tool_input}
                    />
                  ))}
                  {/* Show streaming message or thinking indicator */}
                  {isStreaming && (
                    <ChatMessage
                      role="assistant"
                      content={streamingContent}
                      isStreaming
                    />
                  )}
                  {error && (
                    <div
                      className="rounded-xl px-4 py-3"
                      style={{
                        background: 'rgba(239, 68, 68, 0.1)',
                        border: '1px solid rgba(239, 68, 68, 0.2)',
                      }}
                    >
                      <p className="text-sm font-medium text-[#f87171]">{friendlyErrorLocal(error)}</p>
                      {error !== friendlyErrorLocal(error) && (
                        <details className="mt-2">
                          <summary className="cursor-pointer text-xs text-[#f87171]/60 hover:text-[#f87171]/80">
                            {t('chatPage.technicalDetails')}
                          </summary>
                          <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap font-mono text-xs text-[#f87171]/50">
                            {error}
                          </pre>
                        </details>
                      )}
                    </div>
                  )}
                  <div ref={bottomRef} />
                </div>
              </div>
            )}
          </div>

          {/* Input when messages exist */}
          {showChatView && (
            <div className="mx-auto w-full max-w-3xl px-4 sm:px-6 lg:px-8 pb-4">
              <ChatInput onSend={sendMessage} onAbort={abort} isStreaming={isStreaming} />
            </div>
          )}
        </div>

        {/* Recent sessions sidebar */}
        {(showSidebar || showChatView) && recentSessions.length > 0 && (
          <div className="w-64 border-l border-white/10 bg-[#111827] overflow-y-auto hidden lg:block">
            <div className="p-4">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-sm font-semibold text-[#f8fafc]">Conversas</h3>
                <button
                  onClick={() => {
                    // Reset local state to start a new chat
                    setLocalSessionId(null)
                    setInitialMessage(null)
                    initialMessageSentRef.current = false
                    navigate('/')
                  }}
                  className="flex items-center gap-1 text-xs text-[#64748b] hover:text-[#3b82f6] transition-colors"
                >
                  <Plus className="h-3 w-3" />
                  <span>Nova</span>
                </button>
              </div>
              <div className="space-y-1">
                {recentSessions.map((session) => {
                  const isActive = resolvedId === session.id
                  return (
                    <button
                      key={session.id}
                      onClick={() => {
                        // Navigate to existing session
                        setLocalSessionId(null)
                        setInitialMessage(null)
                        initialMessageSentRef.current = false
                        navigate(`/chat/${encodeURIComponent(session.id)}`)
                      }}
                      className={cn(
                        'w-full flex items-start gap-3 px-3 py-2.5 rounded-lg transition-all text-left',
                        isActive
                          ? 'bg-[#3b82f6]/10 text-[#f8fafc]'
                          : 'text-[#94a3b8] hover:bg-white/5 hover:text-[#f8fafc]'
                      )}
                    >
                      <MessageSquare className="h-4 w-4 mt-0.5 shrink-0" />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm truncate">{session.id.replace('webui:', '')}</p>
                        <p className="text-xs text-[#64748b] mt-0.5">
                          {session.message_count} msgs · {timeAgo(session.last_message_at)}
                        </p>
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
