import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { Loading } from '../components/States'
import { useAuth, useUser } from './AuthContext'

export function RequireAuth({ children }: { children: ReactNode }) {
  const { state, retry } = useAuth()
  const location = useLocation()
  if (state.status === 'loading') {
    return (
      <div className="center-screen">
        <Loading label="Restaurando sua sessão…" />
      </div>
    )
  }
  if (state.status === 'error') {
    return (
      <div className="center-screen">
        <div className="state error" role="alert">
          <span>Não foi possível restaurar sua sessão. Verifique a conexão com o servidor.</span>
          <button type="button" className="btn" onClick={retry}>
            Tentar novamente
          </button>
        </div>
      </div>
    )
  }
  if (state.status === 'anonymous') return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
  return <>{children}</>
}

/** Admin-only screens: staff sees an explanation instead of the screen (the API enforces it too). */
export function RequireAdmin({ children }: { children: ReactNode }) {
  const user = useUser()
  if (user.role !== 'admin') {
    return (
      <div className="card" role="alert">
        <h1>Acesso restrito</h1>
        <p className="muted">Esta área é exclusiva de administradores.</p>
      </div>
    )
  }
  return <>{children}</>
}
