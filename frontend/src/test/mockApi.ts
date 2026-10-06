// A stateful, in-memory fake of the Studio Suite API for tests, written from openapi.json.
// Every payload is typed with the GENERATED schema types, so contract drift breaks `tsc`.
import { http, HttpResponse, type HttpHandler } from 'msw'
import type { Appointment, Customer, FieldError, Problem, Role, Service, User } from '../api/types'
import { addDays, dayOf, todayIn } from '../lib/datetime'

export const API = import.meta.env.VITE_API_BASE as string

type Account = User & { password: string }

export interface Db {
  accounts: Account[]
  services: Service[]
  customers: Customer[]
  appointments: Appointment[]
  /** Valid access tokens. Clear it to simulate the 15-minute expiry. */
  access: Map<string, User>
  refresh: Map<string, { user: User; used: boolean }>
  calls: { login: number; refresh: number; me: number }
  seq: number
}

const iso = (ms: number) => new Date(ms).toISOString()
const HOUR = 3_600_000
const DAY = 24 * HOUR

export function createDb(now = Date.now()): Db {
  const service = (id: string, name: string, duration_min: number, price_cents: number, active = true): Service => ({
    id, name, description: '', duration_min, price_cents, active, created_at: iso(now - 30 * DAY), updated_at: iso(now - 30 * DAY),
  })
  const customer = (id: string, name: string, email: string): Customer => ({
    id, name, email, phone: '31999990000', notes: '', created_at: iso(now - 20 * DAY), updated_at: iso(now - 20 * DAY),
  })
  const appt = (id: string, c: Customer, s: Service, start: number): Appointment => ({
    id, customer_id: c.id, customer_name: c.name, service_id: s.id, service_name: s.name,
    starts_at: iso(start), ends_at: iso(start + s.duration_min * 60_000), duration_min: s.duration_min,
    price_cents: s.price_cents, status: 'scheduled', notes: '', created_at: iso(now - DAY), updated_at: iso(now - DAY),
  })
  const corte = service('s1', 'Corte de cabelo', 30, 5000)
  const massagem = service('s2', 'Massagem', 60, 12000)
  const antigo = service('s3', 'Serviço antigo', 45, 8000, false)
  const ana = customer('c1', 'Ana Souza', 'ana@example.com')
  const bia = customer('c2', 'Bia Lima', 'bia@example.com')
  return {
    accounts: [
      { id: 'u1', email: 'admin@example.com', role: 'admin', password: 'admin-pass' },
      { id: 'u2', email: 'staff@example.com', role: 'staff', password: 'staff-pass' },
    ],
    services: [corte, massagem, antigo],
    customers: [ana, bia],
    appointments: [
      appt('a-past', ana, corte, now - 2 * HOUR), // already started: can be completed
      appt('a-future', bia, massagem, now + 2 * DAY), // not started yet
    ],
    access: new Map(),
    refresh: new Map(),
    calls: { login: 0, refresh: 0, me: 0 },
    seq: 100,
  }
}

export function problem(status: number, code: Problem['code'], detail: string, errors?: FieldError[]) {
  const body: Problem = { type: 'about:blank', title: detail, status, code, detail, request_id: 'req-test', ...(errors ? { errors } : {}) }
  return HttpResponse.json(body, { status, headers: { 'Content-Type': 'application/problem+json' } })
}

function issue(db: Db, user: User) {
  const n = ++db.seq
  const pair = { access_token: `access-${n}`, token_type: 'Bearer' as const, expires_in: 900, refresh_token: `refresh-${n}` }
  db.access.set(pair.access_token, user)
  db.refresh.set(pair.refresh_token, { user, used: false })
  return pair
}

/** Test helper: log a user in against the fake and return the pair the SPA would have stored. */
export function loginPair(db: Db, role: Role) {
  const acc = db.accounts.find((a) => a.role === role)!
  return issue(db, { id: acc.id, email: acc.email, role: acc.role })
}

export function handlers(db: Db): HttpHandler[] {
  const auth = (req: Request): User | null => {
    const token = req.headers.get('Authorization')?.replace('Bearer ', '') ?? ''
    return db.access.get(token) ?? null
  }
  const unauthorized = () => problem(401, 'invalid_token', 'Invalid or expired token')
  // Wraps a handler that needs a signed-in user (and optionally the admin role).
  const guard =
    (admin: boolean, fn: (user: User, req: Request, params: Record<string, string | readonly string[] | undefined>) => Response | Promise<Response>) =>
    ({ request, params }: { request: Request; params: Record<string, string | readonly string[] | undefined> }) => {
      const user = auth(request)
      if (!user) return unauthorized()
      if (admin && user.role !== 'admin') return problem(403, 'forbidden', 'Admin only')
      return fn(user, request, params)
    }
  const page = <T,>(items: T[], req: Request) => {
    const u = new URL(req.url)
    const p = Number(u.searchParams.get('page') ?? 1)
    const size = Number(u.searchParams.get('page_size') ?? 20)
    return HttpResponse.json({ data: items.slice((p - 1) * size, p * size), page: p, page_size: size, total: items.length })
  }
  const stamp = () => iso(Date.now())
  const find = <T extends { id: string }>(list: T[], id: unknown) => list.find((x) => x.id === id)

  return [
    http.post(`${API}/v1/auth/login`, async ({ request }) => {
      db.calls.login++
      const b = (await request.json()) as { email: string; password: string }
      const acc = db.accounts.find((a) => a.email === b.email && a.password === b.password)
      if (!acc) return problem(401, 'invalid_credentials', 'Wrong e-mail or password')
      return HttpResponse.json(issue(db, { id: acc.id, email: acc.email, role: acc.role }))
    }),
    http.post(`${API}/v1/auth/refresh`, async ({ request }) => {
      db.calls.refresh++
      const b = (await request.json()) as { refresh_token: string }
      const rec = db.refresh.get(b.refresh_token)
      if (!rec) return problem(401, 'invalid_token', 'Unknown refresh token')
      if (rec.used) {
        // reuse = theft: revoke the whole family
        for (const [k, v] of db.refresh) if (v.user.id === rec.user.id) db.refresh.delete(k)
        return problem(401, 'invalid_token', 'Refresh token reuse detected')
      }
      rec.used = true
      return HttpResponse.json(issue(db, rec.user))
    }),
    http.post(`${API}/v1/auth/logout`, async ({ request }) => {
      const b = (await request.json()) as { refresh_token: string }
      db.refresh.delete(b.refresh_token)
      return new HttpResponse(null, { status: 204 })
    }),
    http.get(`${API}/v1/auth/me`, guard(false, (user) => {
      db.calls.me++
      return HttpResponse.json(user)
    })),
    http.post(`${API}/v1/users`, guard(true, async (_u, req) => {
      const b = (await req.json()) as { email: string; password: string; role: Role }
      if (db.accounts.some((a) => a.email === b.email)) return problem(409, 'email_taken', 'E-mail already registered')
      const acc: Account = { id: `u${++db.seq}`, email: b.email, role: b.role, password: b.password }
      db.accounts.push(acc)
      return HttpResponse.json({ id: acc.id, email: acc.email, role: acc.role }, { status: 201 })
    })),

    // ---- services ----
    http.get(`${API}/v1/services`, guard(false, (_u, req) => {
      const u = new URL(req.url)
      const q = (u.searchParams.get('q') ?? '').toLowerCase()
      const active = u.searchParams.get('active')
      const items = db.services
        .filter((s) => s.name.toLowerCase().includes(q))
        .filter((s) => active === null || String(s.active) === active)
      return page(items, req)
    })),
    http.post(`${API}/v1/services`, guard(false, async (_u, req) => {
      const b = (await req.json()) as Partial<Service>
      if (!b.name || b.name.length < 2) {
        return problem(422, 'validation_failed', 'One or more fields are invalid.', [{ field: 'name', message: 'must be 2 to 120 characters' }])
      }
      const s: Service = {
        id: `s${++db.seq}`, name: b.name, description: b.description ?? '', duration_min: b.duration_min ?? 30,
        price_cents: b.price_cents ?? 0, active: b.active ?? true, created_at: stamp(), updated_at: stamp(),
      }
      db.services.push(s)
      return HttpResponse.json(s, { status: 201 })
    })),
    http.patch(`${API}/v1/services/:id`, guard(false, async (_u, req, params) => {
      const s = find(db.services, params.id)
      if (!s) return problem(404, 'service_not_found', 'Service not found')
      Object.assign(s, await req.json(), { updated_at: stamp() })
      return HttpResponse.json(s)
    })),
    http.delete(`${API}/v1/services/:id`, guard(true, (_u, _r, params) => {
      const s = find(db.services, params.id)
      if (!s) return problem(404, 'service_not_found', 'Service not found')
      if (db.appointments.some((a) => a.service_id === s.id)) return problem(409, 'service_in_use', 'Service is referenced by appointments')
      db.services = db.services.filter((x) => x !== s)
      return new HttpResponse(null, { status: 204 })
    })),

    // ---- customers ----
    http.get(`${API}/v1/customers`, guard(false, (_u, req) => {
      const q = (new URL(req.url).searchParams.get('q') ?? '').toLowerCase()
      return page(db.customers.filter((c) => (c.name + c.email).toLowerCase().includes(q)), req)
    })),
    http.post(`${API}/v1/customers`, guard(false, async (_u, req) => {
      const b = (await req.json()) as Partial<Customer>
      if (db.customers.some((c) => c.email === b.email)) return problem(409, 'email_taken', 'E-mail already registered')
      const c: Customer = {
        id: `c${++db.seq}`, name: b.name ?? '', email: b.email ?? '', phone: (b.phone ?? '').replace(/\D/g, ''), notes: b.notes ?? '',
        created_at: stamp(), updated_at: stamp(),
      }
      db.customers.push(c)
      return HttpResponse.json(c, { status: 201 })
    })),
    http.patch(`${API}/v1/customers/:id`, guard(false, async (_u, req, params) => {
      const c = find(db.customers, params.id)
      if (!c) return problem(404, 'customer_not_found', 'Customer not found')
      Object.assign(c, await req.json(), { updated_at: stamp() })
      for (const a of db.appointments) if (a.customer_id === c.id) a.customer_name = c.name
      return HttpResponse.json(c)
    })),
    http.delete(`${API}/v1/customers/:id`, guard(true, (_u, _r, params) => {
      const c = find(db.customers, params.id)
      if (!c) return problem(404, 'customer_not_found', 'Customer not found')
      if (db.appointments.some((a) => a.customer_id === c.id)) return problem(409, 'customer_in_use', 'Customer has appointments')
      db.customers = db.customers.filter((x) => x !== c)
      return new HttpResponse(null, { status: 204 })
    })),

    // ---- appointments ----
    http.get(`${API}/v1/appointments`, guard(false, (_u, req) => {
      const sp = new URL(req.url).searchParams
      const [status, from, to] = [sp.get('status'), sp.get('from'), sp.get('to')]
      const items = db.appointments
        .filter((a) => !status || a.status === status)
        .filter((a) => !from || dayOf(a.starts_at) >= from)
        .filter((a) => !to || dayOf(a.starts_at) <= to)
        .sort((a, b) => a.starts_at.localeCompare(b.starts_at))
      return page(items, req)
    })),
    http.post(`${API}/v1/appointments`, guard(false, async (_u, req) => {
      const b = (await req.json()) as { customer_id: string; service_id: string; starts_at: string; notes?: string }
      const c = find(db.customers, b.customer_id)
      const s = find(db.services, b.service_id)
      const start = Date.parse(b.starts_at)
      if (!c || !s) return problem(422, 'validation_failed', 'Unknown customer or service')
      if (!s.active) return problem(422, 'service_inactive', 'Service is inactive')
      if (start <= Date.now()) {
        return problem(422, 'validation_failed', 'One or more fields are invalid.', [{ field: 'starts_at', message: 'must be in the future' }])
      }
      const end = start + s.duration_min * 60_000
      const clash = db.appointments.some(
        (a) => (a.status === 'scheduled' || a.status === 'completed') && Date.parse(a.starts_at) < end && start < Date.parse(a.ends_at),
      )
      if (clash) return problem(409, 'slot_unavailable', 'That time slot overlaps another appointment.')
      const a: Appointment = {
        id: `a${++db.seq}`, customer_id: c.id, customer_name: c.name, service_id: s.id, service_name: s.name,
        starts_at: iso(start), ends_at: iso(end), duration_min: s.duration_min, price_cents: s.price_cents,
        status: 'scheduled', notes: b.notes ?? '', created_at: stamp(), updated_at: stamp(),
      }
      db.appointments.push(a)
      return HttpResponse.json(a, { status: 201 })
    })),
    http.patch(`${API}/v1/appointments/:id/status`, guard(false, async (_u, req, params) => {
      const a = find(db.appointments, params.id)
      if (!a) return problem(404, 'appointment_not_found', 'Appointment not found')
      const { status } = (await req.json()) as { status: Appointment['status'] }
      if (a.status !== 'scheduled') return problem(409, 'invalid_transition', 'Only scheduled appointments can change')
      if ((status === 'completed' || status === 'no_show') && Date.parse(a.starts_at) > Date.now()) {
        return problem(409, 'not_started', 'The appointment has not started yet')
      }
      a.status = status
      return HttpResponse.json(a)
    })),

    // ---- dashboard (computed from the same state) ----
    http.get(`${API}/v1/dashboard/summary`, guard(false, (_u, req) => {
      const { from, to, inRange } = period(req)
      const list = db.appointments.filter(inRange)
      const n = (s: Appointment['status']) => list.filter((a) => a.status === s).length
      const done = list.filter((a) => a.status === 'completed')
      const revenue = done.reduce((t, a) => t + a.price_cents, 0)
      return HttpResponse.json({
        from, to, timezone: 'America/Sao_Paulo', appointments_total: list.length,
        by_status: { scheduled: n('scheduled'), completed: n('completed'), cancelled: n('cancelled'), no_show: n('no_show') },
        revenue_cents: revenue, average_ticket_cents: done.length ? Math.round(revenue / done.length) : 0,
        cancellation_rate: list.length ? n('cancelled') / list.length : 0, no_show_rate: list.length ? n('no_show') / list.length : 0,
        new_customers: db.customers.length,
      })
    })),
    http.get(`${API}/v1/dashboard/daily`, guard(false, (_u, req) => {
      const { from, to, inRange } = period(req)
      const data = []
      for (let d = from; d <= to; d = addDays(d, 1)) {
        const day = db.appointments.filter((a) => inRange(a) && dayOf(a.starts_at) === d)
        const done = day.filter((a) => a.status === 'completed')
        data.push({ date: d, appointments: day.length, completed: done.length, revenue_cents: done.reduce((t, a) => t + a.price_cents, 0) })
      }
      return HttpResponse.json({ from, to, timezone: 'America/Sao_Paulo', data })
    })),
    http.get(`${API}/v1/dashboard/top-services`, guard(false, (_u, req) => {
      const { from, to, inRange } = period(req)
      const data = db.services
        .map((s) => {
          const list = db.appointments.filter((a) => inRange(a) && a.service_id === s.id)
          const done = list.filter((a) => a.status === 'completed')
          return { service_id: s.id, name: s.name, appointments: list.length, completed: done.length, revenue_cents: done.reduce((t, a) => t + a.price_cents, 0) }
        })
        .filter((t) => t.appointments > 0)
        .sort((a, b) => b.revenue_cents - a.revenue_cents)
      return HttpResponse.json({ from, to, timezone: 'America/Sao_Paulo', data })
    })),
    http.get(`${API}/v1/dashboard/upcoming`, guard(false, () =>
      HttpResponse.json({
        data: db.appointments.filter((a) => a.status === 'scheduled' && Date.parse(a.starts_at) > Date.now()).sort((a, b) => a.starts_at.localeCompare(b.starts_at)),
      }),
    )),
  ]

  function period(req: Request) {
    const sp = new URL(req.url).searchParams
    const to = sp.get('to') ?? todayIn()
    const from = sp.get('from') ?? addDays(to, -29)
    return { from, to, inRange: (a: Appointment) => dayOf(a.starts_at) >= from && dayOf(a.starts_at) <= to }
  }
}
