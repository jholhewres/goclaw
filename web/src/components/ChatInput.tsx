import { useState, useRef, useEffect, type KeyboardEvent } from 'react'
import { ArrowUp, Square } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ChatInputProps {
  onSend: (message: string) => void
  onAbort?: () => void
  isStreaming?: boolean
  disabled?: boolean
  placeholder?: string
  rows?: number
  autoFocus?: boolean
}

export function ChatInput({
  onSend,
  onAbort,
  isStreaming = false,
  disabled = false,
  placeholder = 'Enviar mensagem...',
  rows = 3,
  autoFocus = false,
}: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }, [value])

  useEffect(() => {
    if (autoFocus) textareaRef.current?.focus()
  }, [autoFocus])

  const handleSend = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (isStreaming) return
      handleSend()
    }
  }

  const canSend = value.trim().length > 0 && !disabled && !isStreaming

  return (
    <div className="bg-[#111827] rounded-xl border border-white/10 transition-all focus-within:border-white/20">
      {/* Textarea */}
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={disabled ? 'Aguarde a resposta...' : placeholder}
        disabled={disabled}
        rows={rows}
        autoFocus={autoFocus}
        className="w-full px-4 pt-3 pb-1.5 text-[15px] text-[#f8fafc] placeholder:text-[#475569] bg-transparent resize-none border-none outline-none focus:ring-0 disabled:opacity-50"
        style={{ boxShadow: 'none' }}
      />

      {/* Action bar */}
      <div className="flex items-center justify-between px-3 pb-2.5">
        {/* Left side - hint */}
        <span className="text-[11px] text-[#475569]">
          Enter para enviar, Shift+Enter para nova linha
        </span>

        {/* Right side - send/stop */}
        <div className="flex items-center gap-1">
          {isStreaming ? (
            <button
              onClick={() => onAbort?.()}
              className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg bg-[#ef4444] text-white transition-all hover:bg-[#dc2626]"
              aria-label="Parar geração"
            >
              <Square className="h-3.5 w-3.5" fill="currentColor" />
            </button>
          ) : (
            <button
              onClick={handleSend}
              disabled={!canSend}
              className={cn(
                'flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg transition-all',
                canSend
                  ? 'bg-[#3b82f6] text-white hover:bg-[#2563eb]'
                  : 'bg-white/10 text-white/30 cursor-not-allowed',
              )}
              aria-label="Enviar mensagem"
            >
              <ArrowUp className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
