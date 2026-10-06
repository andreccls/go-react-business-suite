import { describe, expect, it, vi } from 'vitest'
import { createSessionStore } from '../auth/session'
import { createClient } from './client'
import { ApiError } from './errors'

const json = (status: number, body?: unknown, headers: Record<string, string> = {}) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
const problem = (status: number, code: string, extra: object = {}) => json(status, { type: 'about:blank', title: 't', status, code, ...extra })
const pair = (n: number) => ({ access_token: `a${n}`, token_type: 'Bearer', expires_in: 900, refresh_token: `r${n}` })

function setup(responder: (url: string, init: RequestInit, n: number) => Response | Promise<Response>, opts: { access?: string; refresh?: string } = {}) {
  const store: Record<string, string> = {}
  const storage = {
    getItem: (k: string) => store[k] ?? null,
    setItem: (k: string, v: string) => void (store[k] = v),
    removeItem: (k: string) => void delete store[k],
  } as unknown as Storage
  const session = createSessionStore(storage)
  if (opts.access || opts.refresh) session.set({ access_token: opts.access ?? '', refresh_token: opts.refresh ?? '' })
  if (!opts.access) {
    // only a refresh token (a reload): drop the in-memory access token
    const r = session.getRefresh()
    session.clear()
    if (r) storage.setItem('studio-suite.refresh', r)
  }
  const fetchFn = vi.fn((url: RequestInfo | URL, init?: RequestInit) => Promise.resolve(responder(String(url), init ?? {}, fetchFn.mock.calls.length)))
  const client = createClient({ baseUrl: 'http://x/api', session, fetchFn: fetchFn as unknown as typeof fetch })
  const onClear = vi.fn()
  session.subscribe(onClear)
  const calls = () => fetchFn.mock.calls.map(([u, i]) => `${i?.method ?? 'GET'} ${String(u).replace('http://x/api', '')} ${(i?.headers as Record<string, string>)?.Authorization ?? '-'}`)
  return { client, session, fetchFn, onClear, calls }
}

describe('requests', () => {
  it('sends the bearer token, JSON body and serializes the query (skipping empty values)', async () => {
    const t = setup(() => json(200, { ok: true }), { access: 'tok', refresh: 'r' })
    const out = await t.client.request('POST', '/v1/things', { query: { a: 1, b: undefined, c: '', d: 'x y', e: false }, body: { n: 1 } })
    expect(out).toEqual({ ok: true })
    const [url, init] = t.fetchFn.mock.calls[0]!
    expect(url).toBe('http://x/api/v1/things?a=1&d=x+y&e=false')
    expect((init as RequestInit).body).toBe('{"n":1}')
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer tok', 'Content-Type': 'application/json', Accept: 'application/json' })
  })

  it('does not send a token (nor a content type without a body) on auth:false calls', async () => {
    const t = setup(() => json(200, {}), { access: 'tok', refresh: 'r' })
    await t.client.request('GET', '/v1/x', { auth: false })
    expect((t.fetchFn.mock.calls[0]![1] as RequestInit).headers).toEqual({ Accept: 'application/json' })
  })

  it('returns undefined for 204', async () => {
    const t = setup(() => new Response(null, { status: 204 }), { access: 'tok', refresh: 'r' })
    await expect(t.client.request('DELETE', '/v1/x')).resolves.toBeUndefined()
  })
})

describe('errors', () => {
  it('turns an RFC 9457 problem into ApiError with code, detail, field errors, request id and Retry-After', async () => {
    const t = setup(
      () => problem(422, 'validation_failed', { detail: 'bad', request_id: 'req-1', errors: [{ field: 'name', message: 'short' }] }),
      { access: 'tok', refresh: 'r' },
    )
    const err = (await t.client.request('POST', '/v1/x', { body: {} }).catch((e: unknown) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 422, code: 'validation_failed', message: 'bad', requestId: 'req-1', fields: [{ field: 'name', message: 'short' }] })

    const t2 = setup(() => json(429, { code: 'rate_limited' }, { 'Retry-After': '7' }), { access: 'tok', refresh: 'r' })
    await expect(t2.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 429, code: 'rate_limited', retryAfter: 7 })
  })

  it('maps a non-JSON error (a proxy error page) and a non-JSON success to unexpected_response', async () => {
    const t = setup(() => new Response('<html>Bad gateway</html>', { status: 502 }), { access: 'tok', refresh: 'r' })
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 502, code: 'unexpected_response' })
    const t2 = setup(() => new Response('not json', { status: 200 }), { access: 'tok', refresh: 'r' })
    await expect(t2.client.request('GET', '/v1/x')).rejects.toMatchObject({ code: 'unexpected_response' })
  })

  it('maps a fetch failure to network_error without touching the session', async () => {
    const t = setup(() => {
      throw new TypeError('Failed to fetch')
    }, { access: 'tok', refresh: 'r' })
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 0, code: 'network_error' })
    expect(t.session.getAccess()).toBe('tok')
    expect(t.onClear).not.toHaveBeenCalled()
  })

  it('does not refresh on a 4xx other than 401', async () => {
    const t = setup(() => problem(409, 'slot_unavailable'), { access: 'tok', refresh: 'r' })
    await expect(t.client.request('POST', '/v1/appointments', { body: {} })).rejects.toMatchObject({ code: 'slot_unavailable' })
    expect(t.fetchFn).toHaveBeenCalledTimes(1)
  })
})

describe('transparent refresh', () => {
  it('on 401 refreshes once, then retries the original request with the new token', async () => {
    const t = setup(
      (url, init) => {
        if (url.endsWith('/v1/auth/refresh')) return json(200, pair(2))
        return (init.headers as Record<string, string>).Authorization === 'Bearer a2' ? json(200, { ok: 1 }) : problem(401, 'invalid_token')
      },
      { access: 'a1', refresh: 'r1' },
    )
    await expect(t.client.request('GET', '/v1/x')).resolves.toEqual({ ok: 1 })
    expect(t.calls()).toEqual(['GET /v1/x Bearer a1', 'POST /v1/auth/refresh -', 'GET /v1/x Bearer a2'])
    expect(t.session.getRefresh()).toBe('r2') // rotated: the old one is spent
    expect((t.fetchFn.mock.calls[1]![1] as RequestInit).body).toBe('{"refresh_token":"r1"}')
  })

  it('concurrent requests that all hit 401 share ONE refresh (the refresh token is single use)', async () => {
    let refreshes = 0
    const t = setup(
      async (url, init) => {
        if (url.endsWith('/v1/auth/refresh')) {
          refreshes++
          await new Promise((r) => setTimeout(r, 20))
          return json(200, pair(2))
        }
        return (init.headers as Record<string, string>).Authorization === 'Bearer a2' ? json(200, { ok: url }) : problem(401, 'invalid_token')
      },
      { access: 'a1', refresh: 'r1' },
    )
    const results = await Promise.all(['/v1/a', '/v1/b', '/v1/c', '/v1/d'].map((p) => t.client.request('GET', p)))
    expect(results).toHaveLength(4)
    expect(refreshes).toBe(1)
  })

  it('a request that 401s AFTER another one already refreshed just retries with the new token', async () => {
    let refreshes = 0
    let slowSeen401 = false
    const t = setup(
      async (url, init) => {
        const auth = (init.headers as Record<string, string>).Authorization
        if (url.endsWith('/v1/auth/refresh')) {
          refreshes++
          return json(200, pair(2))
        }
        if (auth === 'Bearer a2') return json(200, { url })
        if (url.endsWith('/slow')) {
          await new Promise((r) => setTimeout(r, 30)) // answers 401 only after the fast one finished its refresh
          slowSeen401 = true
        }
        return problem(401, 'invalid_token')
      },
      { access: 'a1', refresh: 'r1' },
    )
    await Promise.all([t.client.request('GET', '/fast'), t.client.request('GET', '/slow')])
    expect(slowSeen401).toBe(true)
    expect(refreshes).toBe(1)
  })

  it('after a reload (refresh token only) it refreshes BEFORE the first call', async () => {
    const t = setup(
      (url) => (url.endsWith('/v1/auth/refresh') ? json(200, pair(2)) : json(200, { ok: 1 })),
      { refresh: 'r1' },
    )
    await t.client.request('GET', '/v1/me')
    expect(t.calls()).toEqual(['POST /v1/auth/refresh -', 'GET /v1/me Bearer a2'])
  })

  it('a refused refresh (expired / REUSED / revoked) ends the session: tokens cleared, subscribers told, 401 surfaces', async () => {
    const t = setup(
      (url) => (url.endsWith('/v1/auth/refresh') ? problem(401, 'invalid_token') : problem(401, 'invalid_token')),
      { access: 'a1', refresh: 'r1' },
    )
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 401, code: 'invalid_token' })
    expect(t.session.getAccess()).toBeNull()
    expect(t.session.getRefresh()).toBeNull()
    expect(t.onClear).toHaveBeenCalledTimes(1)
  })

  it('a refresh that fails for another reason (network, 5xx) keeps the session so the user can retry', async () => {
    const t = setup(
      (url) => (url.endsWith('/v1/auth/refresh') ? problem(503, 'timeout') : problem(401, 'invalid_token')),
      { access: 'a1', refresh: 'r1' },
    )
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 503 })
    expect(t.session.getRefresh()).toBe('r1')
    expect(t.onClear).not.toHaveBeenCalled()
    // and the next call can try again
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 503 })
    expect(t.calls().filter((c) => c.includes('refresh'))).toHaveLength(2)
  })

  it('with no tokens at all, an authenticated call ends in a cleared session and a 401 (missing_token)', async () => {
    const t = setup(() => problem(401, 'missing_token'))
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 401, code: 'missing_token' })
    expect(t.onClear).toHaveBeenCalled()
  })

  it('gives up after ONE retry: a second 401 is surfaced, not looped', async () => {
    const t = setup((url) => (url.endsWith('/v1/auth/refresh') ? json(200, pair(2)) : problem(401, 'invalid_token')), { access: 'a1', refresh: 'r1' })
    await expect(t.client.request('GET', '/v1/x')).rejects.toMatchObject({ status: 401 })
    expect(t.calls()).toHaveLength(3)
  })

  it('if the session ended meanwhile (another request lost it), the 401 is surfaced without another refresh', async () => {
    const t = setup(
      async (url) => {
        if (url.endsWith('/slow')) {
          await new Promise((r) => setTimeout(r, 30))
          return problem(401, 'invalid_token')
        }
        return url.endsWith('/v1/auth/refresh') ? problem(401, 'invalid_token') : problem(401, 'invalid_token')
      },
      { access: 'a1', refresh: 'r1' },
    )
    const [a, b] = await Promise.allSettled([t.client.request('GET', '/fast'), t.client.request('GET', '/slow')])
    expect(a.status).toBe('rejected')
    expect(b.status).toBe('rejected')
    expect(t.calls().filter((c) => c.includes('refresh'))).toHaveLength(1)
  })
})
