import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/errors'
import { createQueryClient, shouldRetry } from './queryClient'

describe('shouldRetry', () => {
  it('never retries a 4xx: the answer is final', () => {
    for (const status of [400, 401, 404, 409, 422, 429]) {
      expect(shouldRetry(0, new ApiError({ status, code: 'validation_failed' }))).toBe(false)
    }
  })
  it('retries a network failure or 5xx once', () => {
    expect(shouldRetry(0, new ApiError({ status: 0, code: 'network_error' }))).toBe(true)
    expect(shouldRetry(0, new ApiError({ status: 503, code: 'not_ready' }))).toBe(true)
    expect(shouldRetry(1, new ApiError({ status: 503, code: 'not_ready' }))).toBe(false)
    expect(shouldRetry(0, new Error('boom'))).toBe(true)
  })
  it('builds a client with that policy', () => {
    expect(createQueryClient().getDefaultOptions().queries?.retry).toBe(shouldRetry)
  })
})
