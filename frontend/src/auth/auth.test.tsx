import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { API, problem } from '../test/mockApi'
import { server } from '../test/setup'
import { renderApp, startMockApi } from '../test/utils'

const signInAs = async (user: ReturnType<typeof renderApp>['user'], email: string, password: string) => {
  await user.type(await screen.findByLabelText(/E-mail/), email)
  await user.type(screen.getByLabelText(/Senha/), password)
  await user.click(screen.getByRole('button', { name: 'Entrar' }))
}

describe('login', () => {
  it('sends anonymous visitors to the login form and signs in', async () => {
    const db = startMockApi()
    const { user } = renderApp(db, { as: null })
    await signInAs(user, 'admin@example.com', 'admin-pass')
    expect(await screen.findByRole('heading', { name: 'Painel de resultados' })).toBeInTheDocument()
    expect(screen.getByText(/admin@example\.com/)).toBeInTheDocument()
    expect(screen.getByText('Administrador')).toBeInTheDocument()
    // only the refresh token is persisted; the access token stays in memory
    const stored = JSON.stringify({ ...sessionStorage })
    expect(stored).toContain('refresh-')
    expect(stored).not.toContain('access-')
  })

  it('returns to the page that was requested before login', async () => {
    const db = startMockApi()
    const { user } = renderApp(db, { as: null, route: '/clientes' })
    await signInAs(user, 'staff@example.com', 'staff-pass')
    expect(await screen.findByRole('heading', { name: 'Clientes' })).toBeInTheDocument()
  })

  it('says so when the credentials are wrong and stays on the form', async () => {
    const db = startMockApi()
    const { user } = renderApp(db, { as: null })
    await signInAs(user, 'admin@example.com', 'wrong')
    expect(await screen.findByRole('alert')).toHaveTextContent('E-mail ou senha incorretos.')
    expect(screen.getByRole('button', { name: 'Entrar' })).toBeEnabled()
    expect(sessionStorage.length).toBe(0)
  })

  it('validates empty fields locally, without calling the API', async () => {
    const db = startMockApi()
    const { user } = renderApp(db, { as: null })
    await user.click(await screen.findByRole('button', { name: 'Entrar' }))
    expect(screen.getByText('Informe o e-mail.')).toBeInTheDocument()
    expect(screen.getByText('Informe a senha.')).toBeInTheDocument()
    expect(screen.getByLabelText(/E-mail/)).toBeInvalid()
    expect(db.calls.login).toBe(0)
  })

  it('tells how long to wait when the login rate limit is hit', async () => {
    const db = startMockApi()
    server.use(
      http.post(`${API}/v1/auth/login`, () => {
        const res = problem(429, 'rate_limited', 'Too many requests')
        res.headers.set('Retry-After', '42')
        return res
      }),
    )
    const { user } = renderApp(db, { as: null })
    await signInAs(user, 'admin@example.com', 'admin-pass')
    expect(await screen.findByRole('alert')).toHaveTextContent('Tente novamente em 42 s')
  })

  it('does not keep half a session when /me fails right after a successful login', async () => {
    const db = startMockApi()
    server.use(http.get(`${API}/v1/auth/me`, () => problem(503, 'not_ready', 'down')))
    const { user } = renderApp(db, { as: null })
    await signInAs(user, 'admin@example.com', 'admin-pass')
    expect(await screen.findByRole('alert')).toHaveTextContent(/indisponível/)
    expect(sessionStorage.length).toBe(0)
  })
})

describe('session lifecycle', () => {
  it('restores the session after a reload using only the refresh token', async () => {
    const db = startMockApi()
    renderApp(db) // signed in, but with no access token in memory
    expect(await screen.findByRole('heading', { name: 'Painel de resultados' })).toBeInTheDocument()
    expect(db.calls.refresh).toBe(1)
  })

  it('refreshes transparently when the access token expires mid-session, rotating the refresh token', async () => {
    const db = startMockApi()
    const { user } = renderApp(db)
    await screen.findByRole('heading', { name: 'Painel de resultados' })
    const before = sessionStorage.getItem('studio-suite.refresh')
    db.access.clear() // 15 minutes later...
    await user.click(screen.getByRole('link', { name: 'Serviços' }))
    expect(await screen.findByText('Corte de cabelo', { selector: 'strong' })).toBeInTheDocument()
    expect(db.calls.refresh).toBe(2) // once at start-up, once for the expiry
    expect(sessionStorage.getItem('studio-suite.refresh')).not.toBe(before)
  })

  it('a revoked / reused refresh token ends the session and shows the login form', async () => {
    const db = startMockApi()
    const { user } = renderApp(db)
    await screen.findByRole('heading', { name: 'Painel de resultados' })
    db.refresh.clear() // the server revoked the family (e.g. reuse detection)
    db.access.clear()
    await user.click(screen.getByRole('link', { name: 'Clientes' }))
    expect(await screen.findByRole('button', { name: 'Entrar' })).toBeInTheDocument()
    expect(sessionStorage.length).toBe(0)
  })

  it('logs out: tokens are dropped locally and revoked on the server', async () => {
    const db = startMockApi()
    const { user } = renderApp(db)
    await screen.findByRole('heading', { name: 'Painel de resultados' })
    const refresh = sessionStorage.getItem('studio-suite.refresh')!
    await user.click(screen.getByRole('button', { name: 'Sair' }))
    expect(await screen.findByRole('button', { name: 'Entrar' })).toBeInTheDocument()
    expect(sessionStorage.length).toBe(0)
    await waitFor(() => expect(db.refresh.has(refresh)).toBe(false))
  })

  it('offers a retry when the session check fails for a network reason (and keeps the session)', async () => {
    const db = startMockApi()
    server.use(http.get(`${API}/v1/auth/me`, () => HttpResponse.error(), { once: true }))
    const { user } = renderApp(db)
    expect(await screen.findByRole('alert')).toHaveTextContent(/restaurar sua sessão/)
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }))
    expect(await screen.findByRole('heading', { name: 'Painel de resultados' })).toBeInTheDocument()
  })

  it('redirects away from the login page when already signed in', async () => {
    const db = startMockApi()
    renderApp(db, { route: '/login' })
    expect(await screen.findByRole('heading', { name: 'Painel de resultados' })).toBeInTheDocument()
  })
})
