import type { components, operations } from './schema'

type S = components['schemas']

export type Service = S['Service']
export type ServiceInput = S['ServiceInput']
export type ServicePatch = S['ServicePatch']
export type Customer = S['Customer']
export type CustomerInput = S['CustomerInput']
export type CustomerPatch = S['CustomerPatch']
export type Appointment = S['Appointment']
export type AppointmentInput = S['AppointmentInput']
export type AppointmentStatus = S['AppointmentStatus']
export type Summary = S['Summary']
export type Day = S['Day']
export type DailySeries = S['DailySeries']
export type TopService = S['TopService']
export type Ranking = S['Ranking']
export type User = S['User']
export type Role = S['Role']
export type NewUserRequest = S['NewUserRequest']
export type TokenPair = S['TokenPair']
export type Problem = S['Problem']
export type FieldError = S['FieldError']
export type ErrorCode = NonNullable<Problem['code']>

export type Page<T> = { data: T[]; page: number; page_size: number; total: number }

type Query<K extends keyof operations> = operations[K] extends { parameters: { query?: infer Q } } ? NonNullable<Q> : never
export type ServiceQuery = Query<'listServices'>
export type CustomerQuery = Query<'listCustomers'>
export type AppointmentQuery = Query<'listAppointments'>
export type PeriodQuery = Query<'dashboardSummary'>
