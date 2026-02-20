import { useState, useRef, useEffect, type KeyboardEvent } from 'react'
import { ArrowUp, Square } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ChatInputProps {
  onSend: (message: string) => void
  onAbort?: () => void
  isStreaming?: boolean
  disabled?: boolean
  placeholder?: string
}

export function ChatInput({
  onSend, onAbort, isStreaming = false, disabled = false, placeholder = 'Pergunte algo ou descreva uma tarefa...',
}: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }, [value])

  useEffect(() => { textareaRef.current?.focus() }, [])

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

  return (
    <div className="px-6 pb-5 pt-3">
      <div
        className={cn(
          'flex items-end gap-3 rounded-2xl border bg-zinc-900/80 px-4 py-3',
          'transition-all',
          isStreaming
            ? 'border-blue-500/20 ring-2 ring-blue-500/5'
            : 'border-zinc-700/40 focus-within:border-blue-500/30 focus-within:ring-2 focus-within:ring-blue-500/10',
        )}
      >
        <textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={disabled}
          rows={1}
          className="flex-1 resize-none bg-transparent text-sm leading-relaxed text-white outline-none placeholder:text-zinc-600 max-h-[200px] disabled:opacity-50"
        />
        {isStreaming ? (
          <button
            onClick={() => onAbort?.()}
            className="flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-xl bg-red-500/15 text-red-400 ring-1 ring-red-500/20 transition-all hover:bg-red-500/25"
            aria-label="Parar geração"
          >
            <Square className="h-4 w-4" fill="currentColor" />
          </button>
        ) : (
          <button
            onClick={handleSend}
            disabled={!value.trim() || disabled}
            className={cn(
              'flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-xl transition-all',
              value.trim()
                ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/20 hover:bg-blue-400'
                : 'bg-zinc-800 text-zinc-600',
            )}
            aria-label="Enviar mensagem"
          >
            <ArrowUp className="h-4.5 w-4.5" />
          </button>
        )}
      </div>
      <p className="mt-2 text-center text-[11px] text-zinc-700">
        DevClaw pode cometer erros. Verifique informações importantes.
      </p>
    </div>
  )
}
