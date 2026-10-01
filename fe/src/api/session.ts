import { queryOptions, useQuery } from '@tanstack/react-query'
import { api } from './client'

export interface Session {
  authRequired: boolean
  authenticated: boolean
  admin: boolean
}

export const sessionQuery = queryOptions({
  queryKey: ['session'],
  queryFn: ({ signal }) => api<Session>('/api/session', { signal }),
})

export const useSession = () => useQuery(sessionQuery)

export const signIn = (password: string) =>
  api('/login', { method: 'POST', body: { password } })

export const signOut = () => api('/logout', { method: 'POST' })

/** Whether the visitor may see the app: no gate, or a live session. */
export const isSignedIn = (s: Session) => !s.authRequired || s.authenticated

/** A redirect target is kept only when it is a path inside this app. */
export function safeRedirect(value: string | null): string {
  return value && value.startsWith('/') && !value.startsWith('//') ? value : '/'
}
