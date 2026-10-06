import { parseMoneyToCents } from './format'
import { isDay, zonedToInstant } from './datetime'

// Client-side checks mirror the contract's limits so people get feedback before the round trip.
// The API stays the authority: its 422 field errors are merged on top (see `fieldErrorsFrom`).

export type FieldErrors = Record<string, string>

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function isEmail(s: string): boolean {
  return EMAIL_RE.test(s.trim())
}

function length(errors: FieldErrors, field: string, label: string, value: string, min: number, max: number) {
  const n = value.trim().length
  if (n === 0) errors[field] = `${label} é obrigatório.`
  else if (n < min) errors[field] = `${label} deve ter ao menos ${min} caracteres.`
  else if (n > max) errors[field] = `${label} deve ter no máximo ${max} caracteres.`
}

export interface ServiceValues {
  name: string
  description: string
  duration_min: string
  price_cents: string
}

export function validateService(v: ServiceValues): FieldErrors {
  const e: FieldErrors = {}
  length(e, 'name', 'Nome', v.name, 2, 120)
  if (v.description.length > 500) e.description = 'Descrição deve ter no máximo 500 caracteres.'
  const dur = Number(v.duration_min)
  if (v.duration_min.trim() === '' || !Number.isInteger(dur) || dur < 5 || dur > 480) {
    e.duration_min = 'Informe a duração em minutos (de 5 a 480).'
  }
  const cents = parseMoneyToCents(v.price_cents)
  if (cents === null) e.price_cents = 'Informe um preço válido, por exemplo 120,00.'
  else if (cents > 10_000_000) e.price_cents = 'O preço máximo é R$ 100.000,00.'
  return e
}

export interface CustomerValues {
  name: string
  email: string
  phone: string
  notes: string
}

export function validateCustomer(v: CustomerValues): FieldErrors {
  const e: FieldErrors = {}
  length(e, 'name', 'Nome', v.name, 2, 120)
  if (!isEmail(v.email)) e.email = 'Informe um e-mail válido.'
  if (v.phone.trim() !== '') {
    const digits = v.phone.replace(/\D/g, '')
    if (digits.length < 10 || digits.length > 13) e.phone = 'Telefone deve ter de 10 a 13 dígitos.'
  }
  if (v.notes.length > 1000) e.notes = 'Observações devem ter no máximo 1000 caracteres.'
  return e
}

export interface UserValues {
  email: string
  password: string
  role: string
}

export function validateUser(v: UserValues): FieldErrors {
  const e: FieldErrors = {}
  if (!isEmail(v.email)) e.email = 'Informe um e-mail válido.'
  if (v.password.length < 8 || v.password.length > 72) e.password = 'A senha deve ter de 8 a 72 caracteres.'
  if (v.role !== 'admin' && v.role !== 'staff') e.role = 'Escolha um papel.'
  return e
}

export interface AppointmentValues {
  customer_id: string
  service_id: string
  date: string
  time: string
  notes: string
}

export function validateAppointment(v: AppointmentValues): FieldErrors {
  const e: FieldErrors = {}
  if (!v.customer_id) e.customer_id = 'Escolha o cliente.'
  if (!v.service_id) e.service_id = 'Escolha o serviço.'
  if (!isDay(v.date)) e.date = 'Informe a data.'
  if (!v.time || zonedToInstant(isDay(v.date) ? v.date : '2000-01-01', v.time) === null) e.time = 'Informe o horário.'
  if (v.notes.length > 500) e.notes = 'Observações devem ter no máximo 500 caracteres.'
  return e
}
