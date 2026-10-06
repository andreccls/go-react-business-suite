import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/errors'

/** Retry only what can change on a retry: network blips and 5xx — never 4xx (the answer is final). */
export function shouldRetry(failures: number, err: unknown): boolean {
  if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false
  return failures < 1
}

export function createQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: shouldRetry, staleTime: 10_000 } } })
}
