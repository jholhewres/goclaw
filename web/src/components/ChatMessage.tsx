import { memo, useState, useMemo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  Copy,
  Check,
  Terminal,
  ChevronDown,
  ChevronRight,
  Bot,
  User,
  FileText,
  FileEdit,
  Search,
  Globe,
  Database,
  Lock,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'

const MAX_PREVIEW = 50

function truncate(s: string, max: number): string {
  if (s.length <= max) return s
  return s.slice(0, max) + '…'
}

function toTildePath(path: string): string {
  const m = path.match(/^\/home\/[^/]+\/(.*)$/)
  return m ? `~/${m[1]}` : path
}

interface ToolSummary {
  summary: string
  icon: LucideIcon
}

function getToolSummary(toolName: string, toolInput: string | undefined): ToolSummary {
  const fallback: ToolSummary = {
    summary: `Using ${toolName}`,
    icon: Wrench,
  }
  if (!toolInput?.trim()) return fallback

  let input: Record<string, unknown>
  try {
    input = JSON.parse(toolInput) as Record<string, unknown>
  } catch {
    return fallback
  }

  const path = input.path as string | undefined
  const command = input.command as string | undefined
  const query = input.query as string | undefined
  const url = input.url as string | undefined
  const name = input.name as string | undefined
  const content = input.content as string | undefined

  switch (toolName) {
    case 'read_file':
      return { summary: path ? `Reading ${toTildePath(path)}` : 'Reading file', icon: FileText }
    case 'write_file':
      return { summary: path ? `Writing ${toTildePath(path)}` : 'Writing file', icon: FileEdit }
    case 'edit_file':
      return { summary: path ? `Editing ${toTildePath(path)}` : 'Editing file', icon: FileEdit }
    case 'bash':
    case 'exec':
      return { summary: command ? `Running: ${truncate(command, MAX_PREVIEW)}` : 'Running command', icon: Terminal }
    case 'web_search':
      return { summary: query ? `Searching: ${truncate(query, MAX_PREVIEW)}` : 'Searching the web', icon: Search }
    case 'web_fetch':
      return { summary: url ? `Fetching: ${truncate(url, MAX_PREVIEW)}` : 'Fetching URL', icon: Globe }
    case 'memory_save':
      return { summary: content ? `Saving to memory: ${truncate(content, MAX_PREVIEW)}` : 'Saving to memory', icon: Database }
    case 'memory_search':
      return { summary: query ? `Searching memory: ${truncate(query, MAX_PREVIEW)}` : 'Searching memory', icon: Database }
    case 'vault_save':
      return { summary: name ? `Vault: save ${name}` : 'Vault: save', icon: Lock }
    case 'vault_get':
      return { summary: name ? `Vault: get ${name}` : 'Vault: get', icon: Lock }
    case 'vault_list':
      return { summary: 'Vault: list', icon: Lock }
    case 'vault_delete':
      return { summary: name ? `Vault: delete ${name}` : 'Vault: delete', icon: Lock }
    default: {
      const keys = Object.keys(input).filter((k) => input[k] !== undefined && input[k] !== '')
      const preview = keys.length > 0
        ? ` (${keys.slice(0, 2).map((k) => `${k}=${truncate(String(input[k]), 20)}`).join(', ')}${keys.length > 2 ? '…' : ''})`
        : ''
      return { summary: `Using ${toolName}${preview}`, icon: Wrench }
    }
  }
}

interface ChatMessageProps {
  role: 'user' | 'assistant' | 'tool'
  content: string
  toolName?: string
  toolInput?: string
  isStreaming?: boolean
}

export const ChatMessage = memo(function ChatMessage({
  role, content, toolName, toolInput, isStreaming,
}: ChatMessageProps) {
  if (role === 'tool') {
    return <ToolMessage toolName={toolName} toolInput={toolInput} content={content} />
  }

  if (role === 'user') {
    return (
      <div className="flex gap-4 py-5 animate-fade-in">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-orange-500/10 ring-1 ring-orange-500/15">
          <User className="h-5 w-5 text-orange-400" />
        </div>
        <div className="min-w-0 flex-1 pt-0.5">
          <p className="mb-1.5 text-[11px] font-bold uppercase tracking-[0.15em] text-orange-400/70">Você</p>
          <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-zinc-200">{content}</p>
        </div>
      </div>
    )
  }

  const isEmpty = !content || content.trim() === ''

  return (
    <div className={cn(
      'flex gap-4 py-5',
      isStreaming ? 'animate-slide-in' : 'animate-fade-in',
    )}>
      <div className={cn(
        'flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ring-1 transition-colors',
        isStreaming
          ? 'bg-emerald-500/15 ring-emerald-500/25 stream-glow'
          : 'bg-emerald-500/10 ring-emerald-500/15',
      )}>
        <Bot className="h-5 w-5 text-emerald-400" />
      </div>
      <div className="min-w-0 flex-1 pt-0.5">
        <div className="mb-1.5 flex items-center gap-2">
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-emerald-400/70">DevClaw</p>
          {isStreaming && (
            <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[9px] font-semibold text-emerald-400/80 ring-1 ring-emerald-500/20">
              gerando
            </span>
          )}
        </div>

        {isStreaming && isEmpty ? (
          <TypingDots />
        ) : (
          <div className={cn(
            'prose prose-sm max-w-none text-[15px] leading-relaxed text-zinc-300',
            'prose-headings:text-white prose-headings:font-bold prose-strong:text-white',
            'prose-code:text-orange-400 prose-a:text-orange-400',
            'prose-pre:bg-transparent prose-pre:p-0',
            'prose-p:text-[15px] prose-li:text-[15px]',
            isStreaming && 'stream-shimmer',
          )}>
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: CodeBlock }}>
              {content}
            </ReactMarkdown>
            {isStreaming && (
              <span className="ml-0.5 inline-block h-[18px] w-[2px] animate-cursor rounded-full bg-emerald-400 align-text-bottom" />
            )}
          </div>
        )}
      </div>
    </div>
  )
})

function TypingDots() {
  return (
    <div className="dot-pulse flex items-center gap-1 py-2">
      <span className="h-2 w-2 rounded-full bg-emerald-400/60" />
      <span className="h-2 w-2 rounded-full bg-emerald-400/60" />
      <span className="h-2 w-2 rounded-full bg-emerald-400/60" />
    </div>
  )
}

function ToolMessage({ toolName, toolInput, content }: { toolName?: string; toolInput?: string; content: string }) {
  const [expanded, setExpanded] = useState(false)
  const { summary, icon: Icon } = useMemo(
    () => getToolSummary(toolName || 'tool', toolInput),
    [toolName, toolInput],
  )
  return (
    <div className="ml-14 animate-fade-in py-2">
      <div className="rounded-lg border-l-2 border-orange-500/40 bg-zinc-800/30">
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex w-full cursor-pointer items-center gap-2.5 px-3 py-2.5 text-left text-xs text-zinc-400 transition-colors hover:bg-zinc-800/50"
        >
          <Icon className="h-3.5 w-3.5 shrink-0 text-orange-500/80" />
          <span className="min-w-0 flex-1 font-medium text-zinc-300">{summary}</span>
          {expanded ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
        </button>
        {expanded && (
          <div className="border-t border-zinc-700/20">
            {toolInput && (
              <div className="border-b border-zinc-700/20 px-4 py-3">
                <p className="mb-1.5 text-[10px] font-bold uppercase tracking-[0.15em] text-zinc-600">Input</p>
                <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs text-zinc-400">{toolInput}</pre>
              </div>
            )}
            <div className="px-4 py-3">
              <p className="mb-1.5 text-[10px] font-bold uppercase tracking-[0.15em] text-zinc-600">Output</p>
              <pre className="max-h-60 overflow-x-auto overflow-y-auto whitespace-pre-wrap font-mono text-xs text-zinc-400">{content}</pre>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function CodeBlock({ className, children, ...props }: React.HTMLAttributes<HTMLElement> & { children?: React.ReactNode }) {
  const [copied, setCopied] = useState(false)
  const isInline = !className

  if (isInline) {
    return (
      <code className="rounded-md bg-zinc-800 px-1.5 py-0.5 text-[13px] text-orange-400" {...props}>
        {children}
      </code>
    )
  }

  const text = String(children).replace(/\n$/, '')
  const lang = className?.replace('language-', '') || ''

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* clipboard not available */
    }
  }

  return (
    <div className="group relative not-prose my-4">
      {lang && (
        <div className="flex items-center justify-between rounded-t-xl border border-b-0 border-zinc-700/30 bg-zinc-800/60 px-4 py-2.5">
          <span className="text-[10px] font-bold uppercase tracking-[0.15em] text-zinc-500">{lang}</span>
          <button onClick={handleCopy} aria-label="Copiar código" className="cursor-pointer text-zinc-600 transition-colors hover:text-zinc-300">
            {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
          </button>
        </div>
      )}
      <pre
        className={cn(
          'overflow-x-auto border border-zinc-700/30 bg-dc-darker p-4 text-[13px] leading-relaxed text-zinc-300',
          lang ? 'rounded-b-xl' : 'rounded-xl',
        )}
      >
        <code className={className} {...props}>{children}</code>
      </pre>
      {!lang && (
        <button
          onClick={handleCopy}
          aria-label="Copiar código"
          className="absolute right-3 top-3 cursor-pointer rounded-lg p-1.5 text-zinc-600 opacity-0 transition-all hover:bg-zinc-800 hover:text-zinc-300 group-hover:opacity-100"
        >
          {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      )}
    </div>
  )
}
