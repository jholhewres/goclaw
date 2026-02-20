import type { HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

const variants = {
  default: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200',
  success: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-700/50 dark:text-zinc-300',
  warning: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400',
  error: 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-400',
}

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: keyof typeof variants
}

export function Badge({ className, variant = 'default', ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        variants[variant],
        className,
      )}
      {...props}
    />
  )
}
