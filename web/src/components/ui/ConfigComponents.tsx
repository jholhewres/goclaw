import { useState, type ReactNode } from 'react'
import { ChevronDown, ChevronUp, Save, RotateCcw, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'

// ============================================
// Config Page Wrapper
// ============================================

interface ConfigPageProps {
  title: string
  subtitle?: string
  description?: string
  children: ReactNode
  actions?: ReactNode
  message?: { type: 'success' | 'error'; text: string } | null
}

export function ConfigPage({ title, subtitle, description, children, actions, message }: ConfigPageProps) {
  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-[#0c1222]">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div>
            {subtitle && (
              <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{subtitle}</p>
            )}
            <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">{title}</h1>
            {description && (
              <p className="mt-2 text-base text-[#64748b]">{description}</p>
            )}
          </div>
          {actions && <div className="flex items-center gap-3">{actions}</div>}
        </div>

        {/* Message */}
        {message && (
          <div className={cn(
            'mt-6 rounded-xl px-5 py-4 text-sm border',
            message.type === 'success'
              ? 'bg-[#22c55e]/10 text-[#22c55e] border-[#22c55e]/20'
              : 'bg-[#ef4444]/10 text-[#f87171] border-[#ef4444]/20'
          )}>
            {message.text}
          </div>
        )}

        {/* Content */}
        <div className="mt-10">{children}</div>
      </div>
    </div>
  )
}

// ============================================
// Config Section (Collapsible)
// ============================================

interface ConfigSectionProps {
  icon?: React.ElementType
  title: string
  description?: string
  children: ReactNode
  collapsible?: boolean
  defaultCollapsed?: boolean
  className?: string
  iconColor?: string
}

export function ConfigSection({
  icon: Icon,
  title,
  description,
  children,
  collapsible = false,
  defaultCollapsed = false,
  className,
  iconColor,
}: ConfigSectionProps) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed)

  const content = (
    <div className={cn('space-y-5 rounded-2xl border border-white/10 bg-[#111827] p-6', className)}>
      {children}
    </div>
  )

  if (collapsible) {
    return (
      <section className="mb-10">
        <button
          onClick={() => setIsCollapsed(!isCollapsed)}
          className="flex items-center gap-3 mb-6 w-full text-left group"
        >
          {Icon && (
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-[#111827] group-hover:border-white/20 transition-colors">
              <Icon className="h-5 w-5 text-[#64748b]" />
            </div>
          )}
          <div className="flex-1">
            <h2 className="text-lg font-semibold text-[#f8fafc]">{title}</h2>
            {description && <p className="text-sm text-[#64748b]">{description}</p>}
          </div>
          {isCollapsed ? (
            <ChevronDown className="h-5 w-5 text-[#64748b] group-hover:text-[#f8fafc] transition-colors" />
          ) : (
            <ChevronUp className="h-5 w-5 text-[#64748b] group-hover:text-[#f8fafc] transition-colors" />
          )}
        </button>
        {!isCollapsed && content}
      </section>
    )
  }

  return (
    <section className="mb-10">
      {(Icon || title) && (
        <div className="flex items-center gap-3 mb-6">
          {Icon && (
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-[#111827]">
              <Icon className="h-5 w-5" style={{ color: iconColor || '#64748b' }} />
            </div>
          )}
          <div>
            <h2 className="text-lg font-semibold text-[#f8fafc]">{title}</h2>
            {description && <p className="text-sm text-[#64748b]">{description}</p>}
          </div>
        </div>
      )}
      {content}
    </section>
  )
}

// ============================================
// Config Field (Label + Hint)
// ============================================

interface ConfigFieldProps {
  label: string
  hint?: string
  children: ReactNode
  className?: string
}

export function ConfigField({ label, hint, children, className }: ConfigFieldProps) {
  return (
    <div className={cn('space-y-2', className)}>
      <label className="block text-xs font-semibold uppercase tracking-wider text-[#64748b]">
        {label}
      </label>
      {children}
      {hint && <p className="text-xs text-[#475569]">{hint}</p>}
    </div>
  )
}

// ============================================
// Config Input
// ============================================

interface ConfigInputProps {
  value: string | number
  onChange: (value: string) => void
  placeholder?: string
  type?: 'text' | 'password' | 'number' | 'email' | 'url' | 'time'
  disabled?: boolean
  className?: string
}

export function ConfigInput({
  value,
  onChange,
  placeholder,
  type = 'text',
  disabled = false,
  className,
}: ConfigInputProps) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      className={cn(
        'h-11 w-full rounded-xl border border-white/10 bg-[#111827] px-4 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20',
        disabled && 'opacity-50 cursor-not-allowed',
        className
      )}
    />
  )
}

// ============================================
// Config Textarea
// ============================================

interface ConfigTextareaProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  rows?: number
  disabled?: boolean
  className?: string
}

export function ConfigTextarea({
  value,
  onChange,
  placeholder,
  rows = 3,
  disabled = false,
  className,
}: ConfigTextareaProps) {
  return (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      rows={rows}
      disabled={disabled}
      className={cn(
        'w-full rounded-xl border border-white/10 bg-[#111827] px-4 py-3 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20 resize-none',
        disabled && 'opacity-50 cursor-not-allowed',
        className
      )}
    />
  )
}

// ============================================
// Config Select
// ============================================

interface SelectOption {
  value: string
  label: string
}

interface ConfigSelectProps {
  value: string
  onChange: (value: string) => void
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
  className?: string
}

export function ConfigSelect({
  value,
  onChange,
  options,
  placeholder,
  disabled = false,
  className,
}: ConfigSelectProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      className={cn(
        'h-11 w-full cursor-pointer appearance-none rounded-xl border border-white/10 bg-[#111827] px-4 pr-10 text-sm text-[#f8fafc] outline-none transition-all hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20',
        disabled && 'opacity-50 cursor-not-allowed',
        className
      )}
      style={{
        backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%2364748b' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E")`,
        backgroundRepeat: 'no-repeat',
        backgroundPosition: 'right 12px center',
      }}
    >
      {placeholder && (
        <option value="" disabled>
          {placeholder}
        </option>
      )}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
  )
}

// ============================================
// Config Toggle
// ============================================

interface ConfigToggleProps {
  enabled: boolean
  onChange: (value: boolean) => void
  label: string
  description?: string
  disabled?: boolean
}

export function ConfigToggle({
  enabled,
  onChange,
  label,
  description,
  disabled = false,
}: ConfigToggleProps) {
  return (
    <button
      type="button"
      onClick={() => !disabled && onChange(!enabled)}
      disabled={disabled}
      className={cn(
        'flex items-start gap-3 group',
        !disabled && 'cursor-pointer',
        disabled && 'opacity-50 cursor-not-allowed'
      )}
    >
      <div
        className={cn(
          'relative h-6 w-11 rounded-full transition-colors flex-shrink-0 mt-0.5',
          enabled ? 'bg-[#3b82f6]' : 'bg-[#1e293b]'
        )}
      >
        <div
          className={cn(
            'absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform',
            enabled && 'translate-x-5'
          )}
        />
      </div>
      <div className="flex flex-col items-start">
        <span className="text-sm text-[#94a3b8] group-hover:text-[#f8fafc] transition-colors">
          {label}
        </span>
        {description && (
          <span className="text-xs text-[#475569] mt-0.5">{description}</span>
        )}
      </div>
    </button>
  )
}

// ============================================
// Config Tag List
// ============================================

interface ConfigTagListProps {
  tags: string[]
  onAdd?: (tag: string) => void
  onRemove?: (tag: string) => void
  addPlaceholder?: string
  readOnly?: boolean
  emptyMessage?: string
}

export function ConfigTagList({
  tags,
  onAdd,
  onRemove,
  addPlaceholder = 'Add item...',
  readOnly = false,
  emptyMessage = 'No items',
}: ConfigTagListProps) {
  const [inputValue, setInputValue] = useState('')

  const handleAdd = () => {
    if (inputValue.trim() && onAdd) {
      onAdd(inputValue.trim())
      setInputValue('')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleAdd()
    }
  }

  return (
    <div className="space-y-3">
      {/* Tags */}
      <div className="flex flex-wrap gap-2">
        {tags.length === 0 && !readOnly && (
          <span className="text-sm text-[#475569] italic">{emptyMessage}</span>
        )}
        {tags.map((tag) => (
          <span
            key={tag}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#1e293b] text-sm text-[#94a3b8] group"
          >
            {tag}
            {!readOnly && onRemove && (
              <button
                onClick={() => onRemove(tag)}
                className="text-[#64748b] hover:text-[#f87171] transition-colors"
              >
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </span>
        ))}
      </div>

      {/* Add Input */}
      {!readOnly && onAdd && (
        <div className="flex gap-2">
          <input
            type="text"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={addPlaceholder}
            className="flex-1 h-10 rounded-lg border border-white/10 bg-[#111827] px-3 text-sm text-[#f8fafc] outline-none transition-all placeholder:text-[#475569] hover:border-white/20 focus:border-[#3b82f6]/50"
          />
          <button
            onClick={handleAdd}
            disabled={!inputValue.trim()}
            className="px-4 h-10 rounded-lg bg-[#3b82f6] text-sm font-medium text-white hover:bg-[#2563eb] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            Add
          </button>
        </div>
      )}
    </div>
  )
}

// ============================================
// Config Actions (Save/Reset buttons)
// ============================================

interface ConfigActionsProps {
  onSave: () => void
  onReset?: () => void
  saving?: boolean
  hasChanges?: boolean
  saveLabel?: string
  savingLabel?: string
  resetLabel?: string
}

export function ConfigActions({
  onSave,
  onReset,
  saving = false,
  hasChanges = true,
  saveLabel = 'Save',
  savingLabel = 'Saving...',
  resetLabel = 'Reset',
}: ConfigActionsProps) {
  return (
    <div className="flex items-center gap-3">
      {onReset && hasChanges && (
        <button
          onClick={onReset}
          disabled={saving}
          className="flex cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-[#111827] px-5 py-3 text-sm font-medium text-[#94a3b8] transition-all hover:border-white/20 hover:text-[#f8fafc] disabled:opacity-50"
        >
          <RotateCcw className="h-4 w-4" />
          {resetLabel}
        </button>
      )}
      <button
        onClick={onSave}
        disabled={!hasChanges || saving}
        className="flex cursor-pointer items-center gap-2 rounded-xl bg-[#3b82f6] px-5 py-3 text-sm font-semibold text-white transition-all hover:bg-[#2563eb] disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {saving ? (
          <>
            <Loader2 className="h-4 w-4 animate-spin" />
            {savingLabel}
          </>
        ) : (
          <>
            <Save className="h-4 w-4" />
            {saveLabel}
          </>
        )}
      </button>
    </div>
  )
}

// ============================================
// Config Card (for items like servers, webhooks)
// ============================================

interface ConfigCardProps {
  title: string
  subtitle?: string
  icon?: React.ElementType
  iconColor?: string
  status?: 'success' | 'error' | 'warning' | 'neutral'
  actions?: ReactNode
  children?: ReactNode
  className?: string
}

export function ConfigCard({
  title,
  subtitle,
  icon: Icon,
  iconColor = '#64748b',
  status = 'neutral',
  actions,
  children,
  className,
}: ConfigCardProps) {
  const statusColors = {
    success: 'bg-[#22c55e]/10',
    error: 'bg-[#ef4444]/10',
    warning: 'bg-[#f59e0b]/10',
    neutral: 'bg-[#1e293b]',
  }

  const iconColors = {
    success: 'text-[#22c55e]',
    error: 'text-[#f87171]',
    warning: 'text-[#f59e0b]',
    neutral: 'text-[#64748b]',
  }

  return (
    <div className={cn('rounded-2xl border border-white/10 bg-[#111827] p-6', className)}>
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-3">
          {Icon && (
            <div className={cn('flex h-10 w-10 items-center justify-center rounded-xl', statusColors[status])}>
              <Icon className={cn('h-5 w-5', iconColors[status])} style={{ color: status === 'neutral' ? iconColor : undefined }} />
            </div>
          )}
          <div>
            <h3 className="text-base font-semibold text-[#f8fafc]">{title}</h3>
            {subtitle && <p className="text-sm text-[#64748b]">{subtitle}</p>}
          </div>
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      {children && <div className="mt-4 pt-4 border-t border-white/5">{children}</div>}
    </div>
  )
}

// ============================================
// Config Empty State
// ============================================

interface ConfigEmptyStateProps {
  icon?: React.ElementType
  title: string
  description?: string
  action?: ReactNode
}

export function ConfigEmptyState({ icon: Icon, title, description, action }: ConfigEmptyStateProps) {
  return (
    <div className="rounded-2xl border border-white/10 bg-[#111827] p-8 text-center">
      {Icon && <Icon className="h-12 w-12 text-[#475569] mx-auto mb-4" />}
      <p className="text-sm text-[#64748b]">{title}</p>
      {description && <p className="text-xs text-[#475569] mt-2">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

// ============================================
// Config Info Box
// ============================================

interface ConfigInfoBoxProps {
  title?: string
  items: string[]
}

export function ConfigInfoBox({ title, items }: ConfigInfoBoxProps) {
  return (
    <div className="rounded-2xl border border-white/5 bg-[#111827]/50 p-6 mb-10">
      {title && <h3 className="text-sm font-semibold text-[#64748b] mb-3">{title}</h3>}
      <ul className="space-y-2 text-xs text-[#475569]">
        {items.map((item, index) => (
          <li key={index}>• {item}</li>
        ))}
      </ul>
    </div>
  )
}

// ============================================
// Loading Spinner
// ============================================

export function LoadingSpinner() {
  return (
    <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
      <div className="h-10 w-10 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
    </div>
  )
}

// ============================================
// Error State
// ============================================

interface ErrorStateProps {
  message?: string
  onRetry?: () => void
  retryLabel?: string
}

export function ErrorState({ message = 'Error', onRetry, retryLabel = 'Retry' }: ErrorStateProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center bg-[#0c1222]">
      <p className="text-sm text-[#f87171]">{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-3 text-xs text-[#64748b] hover:text-[#f8fafc] transition-colors cursor-pointer"
        >
          {retryLabel}
        </button>
      )}
    </div>
  )
}
