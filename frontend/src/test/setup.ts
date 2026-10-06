import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { setupServer } from 'msw/node'
import { session } from '../auth/session'

// Any request a test did not mock is a bug in the test: fail loudly.
export const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => {
  cleanup() // unmount first so the session reset below does not poke live components
  server.resetHandlers()
  session.clear()
  sessionStorage.clear()
})
afterAll(() => server.close())
