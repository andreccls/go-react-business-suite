import { describe, expect, it } from 'vitest'
import { isEmail, validateAppointment, validateCustomer, validateService, validateUser } from './validation'

const service = { name: 'Corte', description: '', duration_min: '30', price_cents: '50,00' }

describe('validateService', () => {
  it('accepts a valid service', () => {
    expect(validateService(service)).toEqual({})
  })
  it('mirrors the contract limits', () => {
    expect(validateService({ ...service, name: 'A' }).name).toMatch(/ao menos 2/)
    expect(validateService({ ...service, name: '' }).name).toMatch(/obrigatório|ao menos/)
    expect(validateService({ ...service, name: 'x'.repeat(121) }).name).toMatch(/no máximo 120/)
    expect(validateService({ ...service, description: 'x'.repeat(501) }).description).toMatch(/500/)
    for (const d of ['', '4', '481', '30.5', 'abc']) expect(validateService({ ...service, duration_min: d }).duration_min).toBeDefined()
    expect(validateService({ ...service, duration_min: '5' })).toEqual({})
    expect(validateService({ ...service, duration_min: '480' })).toEqual({})
    expect(validateService({ ...service, price_cents: 'abc' }).price_cents).toMatch(/preço válido/)
    expect(validateService({ ...service, price_cents: '100.000,01' }).price_cents).toMatch(/máximo/)
    expect(validateService({ ...service, price_cents: '100.000,00' })).toEqual({})
    expect(validateService({ ...service, price_cents: '0' })).toEqual({})
  })
})

describe('validateCustomer', () => {
  const ok = { name: 'Ana', email: 'ana@example.com', phone: '', notes: '' }
  it('accepts a valid customer, phone optional', () => {
    expect(validateCustomer(ok)).toEqual({})
    expect(validateCustomer({ ...ok, phone: '(31) 99999-0000' })).toEqual({})
  })
  it('flags bad name, e-mail, phone and notes', () => {
    const e = validateCustomer({ name: ' ', email: 'nope', phone: '123', notes: 'x'.repeat(1001) })
    expect(Object.keys(e).sort()).toEqual(['email', 'name', 'notes', 'phone'])
    expect(validateCustomer({ ...ok, phone: '1'.repeat(14) }).phone).toBeDefined()
  })
  it('checks e-mails loosely (the API is the authority)', () => {
    expect(isEmail('a@b.co')).toBe(true)
    expect(isEmail(' a@b.co ')).toBe(true)
    for (const bad of ['', 'a', 'a@b', '@b.co', 'a b@c.de']) expect(isEmail(bad)).toBe(false)
  })
})

describe('validateUser', () => {
  it('requires e-mail, 8-72 char password and a valid role', () => {
    expect(validateUser({ email: 'a@b.co', password: '12345678', role: 'staff' })).toEqual({})
    expect(validateUser({ email: 'a@b.co', password: '12345678', role: 'admin' })).toEqual({})
    expect(validateUser({ email: 'x', password: '1234567', role: 'boss' })).toEqual({
      email: expect.any(String),
      password: expect.any(String),
      role: expect.any(String),
    })
    expect(validateUser({ email: 'a@b.co', password: 'x'.repeat(73), role: 'staff' }).password).toBeDefined()
  })
})

describe('validateAppointment', () => {
  const ok = { customer_id: 'c1', service_id: 's1', date: '2026-10-07', time: '14:00', notes: '' }
  it('accepts a complete form', () => {
    expect(validateAppointment(ok)).toEqual({})
  })
  it('flags every missing piece', () => {
    expect(validateAppointment({ customer_id: '', service_id: '', date: '', time: '', notes: '' })).toEqual({
      customer_id: expect.any(String),
      service_id: expect.any(String),
      date: expect.any(String),
      time: expect.any(String),
    })
    expect(validateAppointment({ ...ok, time: '99:99' }).time).toBeDefined()
    expect(validateAppointment({ ...ok, notes: 'x'.repeat(501) }).notes).toBeDefined()
  })
})
