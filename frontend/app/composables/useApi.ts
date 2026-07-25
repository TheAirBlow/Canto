import { FetchError } from 'ofetch'
import type { ApiErrorBody } from '~/types/api'

export class ApiError extends Error {
  status: number
  body?: ApiErrorBody

  constructor(status: number, body?: ApiErrorBody) {
    super(body?.detail ?? `request failed with status ${status}`)
    this.status = status
    this.body = body
  }
}

interface UseApiOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  query?: Record<string, string | number | boolean | undefined>
  body?: Record<string, unknown> | FormData
  ssr?: boolean
}

// Central fetch wrapper: always same-origin from the client, always cookie-credentialed, and
// SSR-forwards the incoming request's cookie so a logged-in user's first paint is authenticated.
// Pass ssr: false for anything scoped by a client-persisted preference (e.g. a stats timeframe),
// so a server render doesn't bake in data for a timeframe the client is about to override anyway.
export async function useApi<T>(path: string, opts: UseApiOptions = {}): Promise<T> {
  if (opts.ssr === false && import.meta.server) {
    throw new Error(`useApi: ${path} is client-only, skipped during SSR`)
  }

  const config = useRuntimeConfig()
  const baseURL = import.meta.server ? config.apiBase : config.public.apiBase

  let headers: Record<string, string> | undefined
  if (import.meta.server) {
    try {
      headers = useRequestHeaders(['cookie'])
    } catch {
      headers = undefined
    }
  }

  try {
    return await $fetch<T>(path, {
      baseURL,
      method: opts.method ?? 'GET',
      query: opts.query,
      body: opts.body,
      headers,
      credentials: 'include',
    })
  } catch (err) {
    if (err instanceof FetchError) {
      throw new ApiError(err.status ?? 0, err.data as ApiErrorBody | undefined)
    }
    throw err
  }
}
