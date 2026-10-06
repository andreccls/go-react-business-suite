import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { App } from '../App'
import { AuthProvider } from '../auth/AuthContext'
import { session } from '../auth/session'
import type { Role } from '../api/types'
import { createDb, handlers, loginPair, type Db } from './mockApi'
import { server } from './setup'

/** Fresh fake API state wired into MSW for the current test. */
export function startMockApi(): Db {
  const db = createDb()
  server.use(...handlers(db))
  return db
}

/** Start the app already signed in (as the SPA would be after a reload: only the refresh token is stored). */
export function renderApp(db: Db, opts: { route?: string; as?: Role | null } = {}) {
  if (opts.as !== null) {
    const pair = loginPair(db, opts.as ?? 'admin')
    sessionStorage.setItem('studio-suite.refresh', pair.refresh_token)
    db.access.clear() // like a reload: no access token in memory yet
  }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const user = userEvent.setup()
  const utils = render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[opts.route ?? '/']}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { user, qc, ...utils }
}

export { session }
