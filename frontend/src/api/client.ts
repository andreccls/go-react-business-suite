import type { SessionStore } from '../auth/session'
import { ApiError } from './errors'
import type { Problem, TokenPair } from './types'

export interface ClientOptions {
  baseUrl: string
  session: SessionStore
  fetchFn?: typeof fetch
}

export interface RequestOptions {
  query?: object
  body?: unknown
  /** Default true. `false` for login/refresh/logout (they carry no access token). */
  auth?: boolean
}

export interface Client {
  request<T>(method: string, path: string, opts?: RequestOptions): Promise<T>
}

/**
 * Thin fetch wrapper: JSON in/out, RFC 9457 problems → ApiError, and a transparent refresh.
 *
 *  - A 401 on an authenticated call triggers ONE refresh and ONE retry.
 *  - All concurrent callers share a single in-flight refresh (the refresh token is single use:
 *    two parallel refreshes would look like theft to the server and revoke the whole family).
 *  - A refresh that answers 401 (expired, already used, revoked) ends the session: tokens are
 *    cleared and subscribers (the AuthProvider) send the user to the login screen. Any other
 *    refresh failure (network, 5xx, 429) keeps the session so the user can retry.
 */
export function createClient({ baseUrl, session, fetchFn }: ClientOptions): Client {
  let inflight: Promise<void> | null = null

  const doFetch: typeof fetch = (...args) => (fetchFn ?? fetch)(...args)

  function url(path: string, query?: object): string {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(query ?? {})) {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
    }
    const s = qs.toString()
    return `${baseUrl}${path}${s ? `?${s}` : ''}`
  }

  async function send(method: string, path: string, opts: RequestOptions, token: string | null): Promise<Response> {
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
    if (token) headers.Authorization = `Bearer ${token}`
    try {
      return await doFetch(url(path, opts.query), {
        method,
        headers,
        body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      })
    } catch {
      throw new ApiError({ status: 0, code: 'network_error' })
    }
  }

  async function parse<T>(res: Response): Promise<T> {
    if (res.ok) {
      if (res.status === 204) return undefined as T
      try {
        return (await res.json()) as T
      } catch {
        throw new ApiError({ status: res.status, code: 'unexpected_response' })
      }
    }
    const retry = Number(res.headers.get('Retry-After'))
    let problem: Partial<Problem> | null = null
    try {
      problem = (await res.json()) as Partial<Problem>
    } catch {
      /* not JSON (e.g. a proxy error page) */
    }
    throw new ApiError({
      status: res.status,
      code: problem?.code ?? 'unexpected_response',
      detail: problem?.detail,
      fields: problem?.errors,
      requestId: problem?.request_id,
      retryAfter: retry > 0 ? retry : undefined,
    })
  }

  function refresh(): Promise<void> {
    inflight ??= (async () => {
      const refreshToken = session.getRefresh()
      if (!refreshToken) {
        session.clear()
        throw new ApiError({ status: 401, code: 'missing_token' })
      }
      try {
        const res = await send('POST', '/v1/auth/refresh', { body: { refresh_token: refreshToken } }, null)
        session.set(await parse<TokenPair>(res))
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) session.clear()
        throw err
      }
    })().finally(() => {
      inflight = null
    })
    return inflight
  }

  async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
    if (opts.auth === false) return parse<T>(await send(method, path, opts, null))

    // After a reload the access token is gone but the refresh token is not: get a new pair first.
    if (!session.getAccess() && session.getRefresh()) await refresh()

    const used = session.getAccess()
    const res = await send(method, path, opts, used)
    if (res.status !== 401) return parse<T>(res)

    // Someone else may already have refreshed while this request was in flight.
    if (session.getAccess() === used) await refresh()
    else if (!session.getAccess()) return parse<T>(res) // the session ended meanwhile: surface this 401
    return parse<T>(await send(method, path, opts, session.getAccess()))
  }

  return { request }
}
