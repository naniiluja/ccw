import { queryOptions, useQuery } from '@tanstack/react-query'
import { api } from './client'

export type AuthMode = 'password' | 'token' | 'none'

// Settings is the body of GET /api/settings: the effective configuration that
// only environment variables can change. It never carries a secret; the search
// key is reported as a flag.
export interface Settings {
  authMode: AuthMode
  sessionTtlSeconds: number
  timezone: string
  websearch: {
    provider: string
    model: string
    count: number
    url: string
    keySet: boolean
  }
}

export const settingsQuery = queryOptions({
  queryKey: ['settings'],
  queryFn: ({ signal }) => api<Settings>('/api/settings', { signal }),
  // The page has its own retry button, so a failure shows at once.
  retry: false,
})

export const useSettings = () => useQuery(settingsQuery)
