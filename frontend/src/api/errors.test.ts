import { describe, expect, it } from 'vitest'
import { ApiError, errorMessage, fieldErrorsFrom, isApiError } from './errors'

describe('errorMessage', () => {
  it.each([
    ['slot_unavailable', /conflita com outro agendamento/],
    ['not_started', /depois do horário de início/],
    ['service_in_use', /Desative-o/],
    ['customer_in_use', /agendamentos/],
    ['email_taken', /e-mail/],
    ['invalid_credentials', /incorretos/],
    ['forbidden', /permissão/],
    ['network_error', /conexão/],
  ] as const)('%s has a friendly pt-BR message', (code, re) => {
    expect(errorMessage(new ApiError({ status: 409, code }))).toMatch(re)
  })

  it('tells how long to wait on a 429 when Retry-After is known', () => {
    expect(errorMessage(new ApiError({ status: 429, code: 'rate_limited', retryAfter: 12 }))).toMatch(/12 s/)
    expect(errorMessage(new ApiError({ status: 429, code: 'rate_limited' }))).toMatch(/Aguarde/)
  })

  it("falls back to the API's own detail for codes without a message, and to a generic text for non-API errors", () => {
    expect(errorMessage(new ApiError({ status: 404, code: 'route_not_found', detail: 'No such route' }))).toBe('No such route')
    expect(errorMessage(new Error('x'))).toMatch(/Algo deu errado/)
    expect(errorMessage('x')).toMatch(/Algo deu errado/)
  })
})

describe('fieldErrorsFrom / isApiError', () => {
  it('maps 422 field errors', () => {
    const e = new ApiError({ status: 422, code: 'validation_failed', fields: [{ field: 'name', message: 'too short' }, { field: 'email', message: 'bad' }] })
    expect(fieldErrorsFrom(e)).toEqual({ name: 'too short', email: 'bad' })
    expect(fieldErrorsFrom(new Error('x'))).toEqual({})
  })
  it('narrows by code', () => {
    const e = new ApiError({ status: 409, code: 'email_taken' })
    expect(isApiError(e)).toBe(true)
    expect(isApiError(e, 'email_taken')).toBe(true)
    expect(isApiError(e, 'slot_unavailable')).toBe(false)
    expect(isApiError(new Error('x'))).toBe(false)
  })
})
