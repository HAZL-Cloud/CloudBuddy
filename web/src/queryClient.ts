import { QueryClient } from '@tanstack/react-query'
import { ApiError } from './api'

// Client errors (401, 404, 409 ...) will not fix themselves, so only retry network and server failures.
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false
  return failureCount < 2
}

export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: shouldRetry, refetchOnWindowFocus: true } },
  })
}
