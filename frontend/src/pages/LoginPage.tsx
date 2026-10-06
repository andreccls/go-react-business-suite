import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router'
import { errorMessage } from '../api/errors'
import { useAuth } from '../auth/AuthContext'
import { Field } from '../components/Field'
import { session } from '../auth/session'

export function LoginPage() {
  const { state, login } = useAuth()
  const navigate = useNavigate()
  const from = (useLocation().state as { from?: string } | null)?.from ?? '/'
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({})
  const [busy, setBusy] = useState(false)

  if (state.status === 'authenticated') return <Navigate to={from} replace />

  async function submit(e: FormEvent) {
    e.preventDefault()
    const fe: typeof fieldErrors = {}
    if (!email.trim()) fe.email = 'Informe o e-mail.'
    if (!password) fe.password = 'Informe a senha.'
    setFieldErrors(fe)
    setError(null)
    if (fe.email || fe.password) return
    setBusy(true)
    try {
      await login(email.trim(), password)
      void navigate(from, { replace: true })
    } catch (err) {
      session.clear() // login ok but /me failed: do not keep half a session
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-wrap">
      <main className="card login-card">
        <h1>Studio Suite</h1>
        <p className="muted mb">
          Entre para gerenciar agenda, clientes e serviços.
        </p>
        <form onSubmit={(e) => void submit(e)} noValidate className="form-grid">
          {error ? (
            <p className="banner banner-error" role="alert">
              {error}
            </p>
          ) : null}
          <Field label="E-mail" error={fieldErrors.email} required>
            {(c) => <input {...c} type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} />}
          </Field>
          <Field label="Senha" error={fieldErrors.password} required>
            {(c) => (
              <input {...c} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            )}
          </Field>
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {busy ? 'Entrando…' : 'Entrar'}
          </button>
        </form>
      </main>
    </div>
  )
}
