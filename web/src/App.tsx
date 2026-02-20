import { useEffect, useState } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { AppLayout } from '@/layouts/AppLayout'
import { SetupLayout } from '@/layouts/SetupLayout'
import { Chat } from '@/pages/Chat'
import { Dashboard } from '@/pages/Dashboard'
import { Sessions } from '@/pages/Sessions'
import { Skills } from '@/pages/Skills'
import { Channels } from '@/pages/Channels'
import { Config } from '@/pages/Config'
import { Domain } from '@/pages/Domain'
import { Webhooks } from '@/pages/Webhooks'
import { Hooks } from '@/pages/Hooks'
import { Security } from '@/pages/Security'
import { Jobs } from '@/pages/Jobs'
import { Login } from '@/pages/Login'
import { SetupWizard } from '@/pages/Setup/SetupWizard'
import { WhatsAppConnect } from '@/pages/WhatsAppConnect'
import { System } from '@/pages/System'

/** Estado global de autenticação obtido de /api/auth/status */
interface AuthState {
  loading: boolean
  authRequired: boolean
  authenticated: boolean
  setupComplete: boolean
}

/**
 * Guard que verifica o estado de auth e setup, redirecionando conforme necessário:
 * - Se não configurado → /setup
 * - Se auth requerida e não autenticado → /login
 * - Caso contrário → renderiza os filhos
 */
function AuthGuard({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const [state, setState] = useState<AuthState>({
    loading: true,
    authRequired: false,
    authenticated: false,
    setupComplete: true,
  })

  useEffect(() => {
    const token = localStorage.getItem('devclaw_token')
    const headers: Record<string, string> = {}
    if (token) headers['Authorization'] = `Bearer ${token}`

    fetch('/api/auth/status', { headers })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json()
      })
      .then((data) => {
        setState({
          loading: false,
          authRequired: data.auth_required ?? false,
          authenticated: data.authenticated ?? false,
          setupComplete: data.setup_complete ?? true,
        })
      })
      .catch(() => {
        // Se o endpoint falhar, assume configurado e sem auth
        setState({
          loading: false,
          authRequired: false,
          authenticated: true,
          setupComplete: true,
        })
      })
  }, [location.pathname])

  if (state.loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-dc-darker">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-blue-500/30 border-t-blue-500" />
      </div>
    )
  }

  // Não configurado → redireciona para o wizard
  if (!state.setupComplete && location.pathname !== '/setup') {
    return <Navigate to="/setup" replace />
  }

  // Já configurado mas na página de setup → redireciona para home
  if (state.setupComplete && location.pathname === '/setup') {
    return <Navigate to="/" replace />
  }

  // Auth requerida e não autenticado → redireciona para login
  if (state.authRequired && !state.authenticated && location.pathname !== '/login') {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

/**
 * Roteamento principal da aplicação.
 *
 * - /setup → Wizard de configuração inicial (layout centrado, sem sidebar)
 * - /login → Página de login (layout centrado, sem sidebar)
 * - / → Chat conversacional (página inicial)
 * - /chat/:sessionId → Chat de sessão específica
 * - /stats → Dashboard com estatísticas
 * - /sessions → Lista de sessões
 * - /skills → Store de skills
 * - /channels → Status dos canais
 * - /config → Provedores LLM
 * - /system → Configurações do sistema (nome, timezone, idioma)
 * - /domain → Domínio & Rede
 * - /webhooks → Webhooks
 * - /hooks → Hooks
 * - /security → Painel de segurança
 * - /jobs → Cron jobs
 */
export function App() {
  return (
    <AuthGuard>
      <Routes>
        {/* Setup wizard — layout separado */}
        <Route element={<SetupLayout />}>
          <Route path="/setup" element={<SetupWizard />} />
        </Route>

        {/* Login — sem layout */}
        <Route path="/login" element={<Login />} />

        {/* App principal — layout com sidebar */}
        <Route element={<AppLayout />}>
          <Route path="/" element={<Chat />} />
          <Route path="/chat/:sessionId" element={<Chat />} />
          <Route path="/stats" element={<Dashboard />} />
          <Route path="/sessions" element={<Sessions />} />
          <Route path="/skills" element={<Skills />} />
          <Route path="/channels" element={<Channels />} />
          <Route path="/channels/whatsapp" element={<WhatsAppConnect />} />
          <Route path="/config" element={<Config />} />
          <Route path="/system" element={<System />} />
          <Route path="/domain" element={<Domain />} />
          <Route path="/webhooks" element={<Webhooks />} />
          <Route path="/hooks" element={<Hooks />} />
          <Route path="/security" element={<Security />} />
          <Route path="/jobs" element={<Jobs />} />
        </Route>
      </Routes>
    </AuthGuard>
  )
}
