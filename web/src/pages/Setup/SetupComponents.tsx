import { useState, type ReactNode, type FC } from 'react'
import { Eye, EyeOff } from 'lucide-react'

/* ─────────────────────────────────────────────────────────────
   Layout Components
   ───────────────────────────────────────────────────────────── */

export function StepContainer({ children }: { children: ReactNode }) {
  return <div className="space-y-5">{children}</div>
}

export function StepHeader({ title, description }: { title: string; description: string }) {
  return (
    <div>
      <h2 className="text-base font-semibold text-[#f8fafc]">{title}</h2>
      <p className="mt-1 text-sm text-[#94a3b8]">{description}</p>
    </div>
  )
}

export function FieldGroup({ children }: { children: ReactNode }) {
  return <div className="space-y-4">{children}</div>
}

/* ─────────────────────────────────────────────────────────────
   Field & Label
   ───────────────────────────────────────────────────────────── */

interface FieldProps {
  label: string
  icon?: FC<{ className?: string }>
  hint?: string
  children: ReactNode
}

export function Field({ label, icon: Icon, hint, children }: FieldProps) {
  return (
    <div>
      <label className="mb-1.5 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[#64748b]">
        {Icon && <Icon className="h-3.5 w-3.5" />}
        {label}
      </label>
      {children}
      {hint && <p className="mt-1.5 text-xs text-[#64748b]">{hint}</p>}
    </div>
  )
}

/* ─────────────────────────────────────────────────────────────
   Input Components
   ───────────────────────────────────────────────────────────── */

interface InputProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  type?: 'text' | 'password' | 'tel' | 'url'
  mono?: boolean
  className?: string
}

export function Input({
  value,
  onChange,
  placeholder,
  type = 'text',
  mono,
  className = '',
}: InputProps) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      autoComplete="off"
      data-lpignore="true"
      data-form-type="other"
      className={`h-11 w-full rounded-xl border border-white/10 bg-[#0c1222] px-4 text-sm text-[#f8fafc] placeholder:text-[#475569] outline-none transition-all hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-2 focus:ring-[#3b82f6]/20 ${mono ? 'font-mono' : ''} ${className}`}
    />
  )
}

interface PasswordInputProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

export function PasswordInput({ value, onChange, placeholder }: PasswordInputProps) {
  const [show, setShow] = useState(false)
  const [focused, setFocused] = useState(false)

  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setFocused(true)}
        placeholder={placeholder}
        autoComplete="new-password"
        name="devclaw-new-password"
        id="devclaw-new-password"
        data-lpignore="true"
        data-form-type="other"
        data-1p-ignore=""
        readOnly={!focused}
        className="h-11 w-full rounded-xl border border-white/10 bg-[#0c1222] px-4 pr-10 text-sm text-[#f8fafc] placeholder:text-[#475569] outline-none transition-all hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-2 focus:ring-[#3b82f6]/20"
      />
      <button
        type="button"
        onMouseDown={(e) => { e.preventDefault(); setShow(!show) }}
        className="absolute right-3 top-1/2 -translate-y-1/2 cursor-pointer text-[#64748b] hover:text-[#f8fafc] transition-colors"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

interface SelectProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  options?: { value: string; label: string }[]
  groups?: { label: string; options: { value: string; label: string }[] }[]
}

export function Select({ value, onChange, placeholder, options = [], groups }: SelectProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="h-11 w-full cursor-pointer rounded-xl border border-white/10 bg-[#0c1222] px-4 text-sm text-[#f8fafc] outline-none transition-all hover:border-white/20 focus:border-[#3b82f6]/50 focus:ring-2 focus:ring-[#3b82f6]/20"
    >
      {placeholder && <option value="">{placeholder}</option>}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>{opt.label}</option>
      ))}
      {groups?.map((group) => (
        <optgroup key={group.label} label={group.label}>
          {group.options.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </optgroup>
      ))}
    </select>
  )
}

/* ─────────────────────────────────────────────────────────────
   Card Components
   ───────────────────────────────────────────────────────────── */

interface CardProps {
  children: ReactNode
  className?: string
  highlight?: 'blue' | 'green' | 'amber' | 'red'
}

export function Card({ children, className = '', highlight }: CardProps) {
  const highlightStyles = {
    blue: 'border-[#3b82f6]/30 bg-[#3b82f6]/5',
    green: 'border-[#22c55e]/30 bg-[#22c55e]/5',
    amber: 'border-[#f59e0b]/30 bg-[#f59e0b]/5',
    red: 'border-[#ef4444]/30 bg-[#ef4444]/5',
  }

  return (
    <div className={`rounded-xl border p-4 transition-all ${
      highlight
        ? highlightStyles[highlight]
        : 'border-white/[0.06] bg-[#0c1222]/50'
    } ${className}`}>
      {children}
    </div>
  )
}

interface SelectableCardProps {
  selected: boolean
  onClick: () => void
  icon?: FC<{ className?: string }>
  iconColor?: string
  title: string
  description?: string
  accentColor?: string
}

export function SelectableCard({
  selected,
  onClick,
  icon: Icon,
  iconColor,
  title,
  description,
  accentColor,
}: SelectableCardProps) {
  return (
    <button
      onClick={onClick}
      className={`flex w-full items-start gap-3 rounded-xl border px-4 py-3 text-left transition-all ${
        selected
          ? 'border-[#3b82f6]/50 bg-[#3b82f6]/10'
          : 'border-white/[0.06] bg-[#0c1222]/50 hover:border-white/10 hover:bg-[#111827]'
      }`}
      style={selected && accentColor ? { borderColor: accentColor, backgroundColor: `${accentColor}15` } : undefined}
    >
      {Icon && (
        <div className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg ${
          selected ? 'bg-white/5' : 'bg-[#1e293b]'
        }`}>
          <Icon className={`h-3.5 w-3.5 ${selected ? (iconColor || 'text-[#3b82f6]') : 'text-[#64748b]'}`} />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-[#f8fafc]">{title}</span>
          {selected && <span className="h-1.5 w-1.5 rounded-full bg-[#3b82f6]" />}
        </div>
        {description && <p className="mt-0.5 text-xs text-[#94a3b8]">{description}</p>}
      </div>
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Toggle & Checkbox
   ───────────────────────────────────────────────────────────── */

interface ToggleProps {
  enabled: boolean
  onChange: (enabled: boolean) => void
  label?: string
}

export function Toggle({ enabled, onChange, label }: ToggleProps) {
  return (
    <button
      type="button"
      onClick={() => onChange(!enabled)}
      className="flex cursor-pointer items-center gap-2"
    >
      <div className={`relative h-5 w-9 rounded-full transition-colors ${
        enabled ? 'bg-[#3b82f6]' : 'bg-[#1e293b]'
      }`}>
        <span className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${
          enabled ? 'translate-x-4' : 'translate-x-0.5'
        }`} />
      </div>
      {label && <span className="text-xs text-[#94a3b8]">{label}</span>}
    </button>
  )
}

interface CheckboxProps {
  checked: boolean
  onChange: () => void
  children: ReactNode
}

export function Checkbox({ checked, onChange, children }: CheckboxProps) {
  return (
    <button onClick={onChange} className="flex cursor-pointer items-center gap-2.5">
      <div className={`flex h-5 w-5 shrink-0 items-center justify-center rounded border transition-all ${
        checked
          ? 'border-transparent bg-[#3b82f6] text-white'
          : 'border-white/20 bg-[#1e293b] hover:border-white/40'
      }`}>
        {checked && (
          <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        )}
      </div>
      {children}
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Button Components
   ───────────────────────────────────────────────────────────── */

interface ButtonProps {
  children: ReactNode
  onClick: () => void
  disabled?: boolean
  loading?: boolean
  variant?: 'primary' | 'secondary' | 'ghost'
  size?: 'sm' | 'md'
  icon?: FC<{ className?: string }>
}

export function Button({
  children,
  onClick,
  disabled,
  loading,
  variant = 'secondary',
  size = 'md',
  icon: Icon,
}: ButtonProps) {
  const variants = {
    primary: 'bg-[#f8fafc] text-[#0f1419] shadow-lg shadow-white/5 hover:bg-white',
    secondary: 'border border-white/10 bg-[#1e293b] text-[#f8fafc] hover:border-white/20 hover:bg-[#334155]',
    ghost: 'text-[#64748b] hover:text-[#f8fafc]',
  }

  const sizes = {
    sm: 'px-3 py-2 text-xs',
    md: 'px-4 py-2.5 text-sm',
  }

  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className={`flex cursor-pointer items-center justify-center gap-2 rounded-xl font-medium transition-all disabled:cursor-not-allowed disabled:opacity-40 ${variants[variant]} ${sizes[size]}`}
    >
      {loading ? (
        <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current/30 border-t-current" />
      ) : Icon ? (
        <Icon className="h-3.5 w-3.5" />
      ) : null}
      {children}
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Button Group / Grid Select
   ───────────────────────────────────────────────────────────── */

interface OptionButtonProps {
  selected: boolean
  onClick: () => void
  children: ReactNode
  className?: string
}

export function OptionButton({ selected, onClick, children, className = '' }: OptionButtonProps) {
  return (
    <button
      onClick={onClick}
      className={`flex cursor-pointer items-center gap-2 rounded-xl border px-3 py-2.5 text-left transition-all ${
        selected
          ? 'border-[#3b82f6]/50 bg-[#3b82f6]/10 text-[#f8fafc]'
          : 'border-white/10 bg-[#0c1222] text-[#94a3b8] hover:border-white/20 hover:bg-[#111827]'
      } ${className}`}
    >
      {children}
    </button>
  )
}

/* ─────────────────────────────────────────────────────────────
   Info Box
   ───────────────────────────────────────────────────────────── */

interface InfoBoxProps {
  icon?: FC<{ className?: string }>
  children: ReactNode
}

export function InfoBox({ icon: Icon, children }: InfoBoxProps) {
  return (
    <div className="flex items-start gap-2.5 rounded-xl border border-white/[0.06] bg-[#0c1222]/50 px-4 py-3">
      {Icon && (
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-[#1e293b]">
          <Icon className="h-3.5 w-3.5 text-[#64748b]" />
        </div>
      )}
      <p className="text-xs text-[#94a3b8]">{children}</p>
    </div>
  )
}
