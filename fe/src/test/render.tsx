import { render } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import type { RouteObject } from 'react-router'
import { createApp } from '@/app/create-app'
import { server } from './server'

export interface SessionState {
  authRequired: boolean
  authenticated: boolean
  admin: boolean
}

// useSession installs a mutable GET /api/session handler and returns the state
// object, so a test can flip authenticated the way a successful login would.
export function useSession(initial: Partial<SessionState> = {}) {
  const state: SessionState = {
    authRequired: true,
    authenticated: true,
    admin: true,
    ...initial,
  }
  server.use(http.get('*/api/session', () => HttpResponse.json(state)))
  return state
}

export function renderApp(path = '/', extraRoutes: RouteObject[] = []) {
  const app = createApp({ initialEntries: [path], extraRoutes })
  const utils = render(<app.Root />)
  return { ...app, ...utils }
}
