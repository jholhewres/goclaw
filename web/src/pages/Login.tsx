import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Lock, ArrowRight, Loader2, Bot } from 'lucide-react'
import { api, ApiError } from '@/lib/api'

export function Login() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const { token } = await api.auth.login(password)
      setPassword('')
      if (token) {
        localStorage.setItem('devclaw_token', token)
      }
      navigate('/')
    } catch (err) {
      // Extract error message from various error formats
      let errorMessage = t('login.invalidPassword')

      if (err instanceof ApiError) {
        // Try to parse JSON error response
        try {
          const parsed = JSON.parse(err.message)
          errorMessage = parsed.error || t('login.invalidPassword')
        } catch {
          errorMessage = err.status === 401 ? t('login.invalidPassword') : (err.message || t('common.error'))
        }
      } else if (err instanceof Error) {
        // Try to parse JSON from regular Error message too
        try {
          const parsed = JSON.parse(err.message)
          errorMessage = parsed.error || t('login.invalidPassword')
        } catch {
          errorMessage = err.message || t('common.error')
        }
      }

      setError(errorMessage)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#0c1222] flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        {/* Logo */}
        <div className="text-center mb-8">
          <div className="inline-flex h-14 w-14 items-center justify-center rounded-xl bg-[#3b82f6] shadow-lg shadow-blue-500/20">
            <Bot className="h-7 w-7 text-white" />
          </div>
          <h1 className="mt-4 text-2xl font-bold text-[#f8fafc]">
            Dev<span className="text-[#64748b]">Claw</span>
          </h1>
          <p className="mt-1 text-sm text-[#64748b]">{t('login.subtitle')}</p>
        </div>

        {/* Card */}
        <div className="bg-[#111827] rounded-2xl border border-white/10 p-6">
          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label htmlFor="password" className="flex items-center gap-2 mb-2 text-xs font-semibold text-[#64748b] uppercase tracking-wider">
                <Lock className="h-3.5 w-3.5" />
                {t('login.password')}
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => { setPassword(e.target.value); setError('') }}
                placeholder={t('login.passwordPlaceholder')}
                autoComplete="current-password"
                autoFocus
                className="w-full rounded-xl border border-white/10 bg-[#1e293b] px-4 py-3 text-sm text-[#f8fafc] placeholder:text-[#475569] outline-none transition-all focus:border-[#3b82f6]/50 focus:ring-1 focus:ring-[#3b82f6]/20"
              />
            </div>

            {error && (
              <div className="rounded-xl px-4 py-3 text-sm text-[#f87171] bg-red-500/10 border border-red-500/20">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={loading || !password}
              className="flex w-full items-center justify-center gap-2 rounded-xl bg-[#3b82f6] px-4 py-3 text-sm font-semibold text-white transition-all hover:bg-[#2563eb] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <>
                  {t('login.login')}
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
            </button>
          </form>
        </div>

        {/* Footer */}
        <p className="mt-6 text-center text-xs text-[#475569]">
          DevClaw v1.6.0
        </p>
      </div>
    </div>
  )
}
