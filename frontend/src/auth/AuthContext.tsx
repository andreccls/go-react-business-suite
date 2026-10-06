import { useQueryClient } from '@tanstack/react-query'
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api } from '../api'
import { isApiError } from '../api/errors'
import type { User } from '../api/types'
import { session } from './session'

type State =
  | { status: 'loading' }
  | { status: 'anonymous' }
  | { status: 'error' }
  | { status: 'authenticated'; user: User }

interface AuthValue {
  state: State
  login(email: string, password: string): Promise<void>
  logout(): Promise<void>
  /** Re-run the initial session check (after a network error). */
  retry(): void
}

const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [state, setState] = useState<State>(() => (session.getRefresh() ? { status: 'loading' } : { status: 'anonymous' }))
  const [attempt, setAttempt] = useState(0)

  // The client clears the session when a refresh is refused (expired, reused, revoked): leave.
  useEffect(
    () =>
      session.subscribe(() => {
        qc.clear()
        setState({ status: 'anonymous' })
      }),
    [qc],
  )

  // Reload: the access token is gone, the refresh token (sessionStorage) brings the session back.
  useEffect(() => {
    if (!session.getRefresh()) return
    let cancelled = false
    api
      .me()
      .then((user) => !cancelled && setState({ status: 'authenticated', user }))
      .catch((err: unknown) => {
        if (cancelled) return
        // a 401 already cleared the session (and the subscriber above moved us to anonymous)
        if (!isApiError(err) || err.status !== 401) setState({ status: 'error' })
      })
    return () => {
      cancelled = true
    }
  }, [attempt])

  const login = useCallback(async (email: string, password: string) => {
    session.set(await api.login(email, password))
    setState({ status: 'authenticated', user: await api.me() })
  }, [])

  const logout = useCallback(async () => {
    const refresh = session.getRefresh()
    session.clear()
    if (refresh) await api.logout(refresh).catch(() => undefined) // best effort: the tokens are gone locally anyway
  }, [])

  const retry = useCallback(() => {
    setState({ status: 'loading' })
    setAttempt((n) => n + 1)
  }, [])

  const value = useMemo(() => ({ state, login, logout, retry }), [state, login, logout, retry])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const v = useContext(AuthContext)
  if (!v) throw new Error('useAuth must be used inside <AuthProvider>')
  return v
}

/** The signed-in user; only call below <RequireAuth>. */
export function useUser(): User {
  const { state } = useAuth()
  if (state.status !== 'authenticated') throw new Error('useUser outside an authenticated route')
  return state.user
}
