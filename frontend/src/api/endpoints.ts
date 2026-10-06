import type { Client } from './client'
import type {
  Appointment,
  AppointmentInput,
  AppointmentQuery,
  AppointmentStatus,
  Customer,
  CustomerInput,
  CustomerPatch,
  CustomerQuery,
  DailySeries,
  NewUserRequest,
  Page,
  PeriodQuery,
  Ranking,
  Service,
  ServiceInput,
  ServicePatch,
  ServiceQuery,
  Summary,
  TokenPair,
  User,
} from './types'

/** One typed function per operation of the contract (types come from the generated schema). */
export function createApi(c: Client) {
  const get = <T>(path: string, query?: object) => c.request<T>('GET', path, { query })
  const send = <T>(method: string, path: string, body: unknown) => c.request<T>(method, path, { body })
  const del = (path: string) => c.request<void>('DELETE', path)

  return {
    login: (email: string, password: string) =>
      c.request<TokenPair>('POST', '/v1/auth/login', { body: { email, password }, auth: false }),
    logout: (refreshToken: string) =>
      c.request<void>('POST', '/v1/auth/logout', { body: { refresh_token: refreshToken }, auth: false }),
    me: () => get<User>('/v1/auth/me'),
    createUser: (input: NewUserRequest) => send<User>('POST', '/v1/users', input),

    listServices: (q: ServiceQuery = {}) => get<Page<Service>>('/v1/services', q),
    createService: (input: ServiceInput) => send<Service>('POST', '/v1/services', input),
    updateService: (id: string, patch: ServicePatch) => send<Service>('PATCH', `/v1/services/${id}`, patch),
    deleteService: (id: string) => del(`/v1/services/${id}`),

    listCustomers: (q: CustomerQuery = {}) => get<Page<Customer>>('/v1/customers', q),
    createCustomer: (input: CustomerInput) => send<Customer>('POST', '/v1/customers', input),
    updateCustomer: (id: string, patch: CustomerPatch) => send<Customer>('PATCH', `/v1/customers/${id}`, patch),
    deleteCustomer: (id: string) => del(`/v1/customers/${id}`),

    listAppointments: (q: AppointmentQuery = {}) => get<Page<Appointment>>('/v1/appointments', q),
    createAppointment: (input: AppointmentInput) => send<Appointment>('POST', '/v1/appointments', input),
    setAppointmentStatus: (id: string, status: AppointmentStatus) =>
      send<Appointment>('PATCH', `/v1/appointments/${id}/status`, { status }),

    summary: (p: PeriodQuery = {}) => get<Summary>('/v1/dashboard/summary', p),
    daily: (p: PeriodQuery = {}) => get<DailySeries>('/v1/dashboard/daily', p),
    topServices: (p: PeriodQuery & { limit?: number } = {}) => get<Ranking>('/v1/dashboard/top-services', p),
    upcoming: (limit = 5) => get<{ data: Appointment[] }>('/v1/dashboard/upcoming', { limit }),
  }
}

export type Api = ReturnType<typeof createApi>
