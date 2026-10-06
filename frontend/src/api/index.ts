import { session } from '../auth/session'
import { createClient } from './client'
import { createApi } from './endpoints'

// `/api` is the same-origin prefix: nginx (production) and Vite's dev proxy both strip it and
// forward to the Go API. Override with VITE_API_BASE to talk to the API directly (then CORS applies).
const baseUrl = (import.meta.env.VITE_API_BASE ?? '/api').replace(/\/$/, '')

export const api = createApi(createClient({ baseUrl, session }))
