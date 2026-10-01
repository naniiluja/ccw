import { Navigate, Outlet, useLocation } from 'react-router'
import { isSignedIn, useSession } from '@/api/session'
import { RouteError } from '@/components/app/route-error'
import { PageSkeleton } from '@/components/app/page-skeleton'

// RequireAuth keeps the app behind the session: GET /api/session says whether
// the server wants a login at all, and whether this browser has one.
export function RequireAuth() {
  const session = useSession()
  const location = useLocation()

  if (session.isPending) return <PageSkeleton fullscreen />
  if (session.isError) {
    return <RouteError error={session.error} onRetry={() => void session.refetch()} />
  }
  if (!isSignedIn(session.data)) {
    const redirect = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?redirect=${redirect}`} replace />
  }
  return <Outlet />
}
