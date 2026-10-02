import { QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider } from 'next-themes'
import {
  createBrowserRouter,
  createMemoryRouter,
  RouterProvider,
  type RouteObject,
} from 'react-router'
import { sessionQuery } from '@/api/session'
import { createQueryClient } from '@/api/query-client'
import { AppShell } from '@/components/app/app-shell'
import { PageSkeleton } from '@/components/app/page-skeleton'
import { RouteError } from '@/components/app/route-error'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { RequireAuth } from './require-auth'
import { routeTable, type AppRoute } from './routes'

const lazyPage = (route: AppRoute): RouteObject => ({
  path: route.path,
  lazy: async () => ({ Component: (await route.load()).default }),
})

// buildRoutes turns the route table into router objects: the login page sits
// outside the guard, every other page inside the guard and the shell.
export function buildRoutes(extra: RouteObject[] = []): RouteObject[] {
  const inShell = routeTable.filter((r) => r.shell).map(lazyPage)
  return [
    {
      errorElement: <RouteError />,
      hydrateFallbackElement: <PageSkeleton fullscreen />,
      children: [
        ...routeTable.filter((r) => !r.shell).map(lazyPage),
        {
          element: <RequireAuth />,
          children: [{ element: <AppShell />, children: [...extra, ...inShell] }],
        },
      ],
    },
  ]
}

export interface CreateAppOptions {
  /** Paths for an in-memory router (tests); the browser router is used when absent. */
  initialEntries?: string[]
  extraRoutes?: RouteObject[]
}

// The router is served under /ui/ in a build and under / in dev (vite base).
const basename = import.meta.env.BASE_URL.replace(/\/$/, '') || '/'

export function createApp({ initialEntries, extraRoutes }: CreateAppOptions = {}) {
  const routes = buildRoutes(extraRoutes)
  const router = initialEntries
    ? createMemoryRouter(routes, { initialEntries })
    : createBrowserRouter(routes, { basename })

  const queryClient = createQueryClient(() => {
    // The session ended: mark it so the login page does not bounce straight
    // back, then go to /login and remember where the person was.
    queryClient.setQueryData(sessionQuery.queryKey, (s) =>
      s ? { ...s, authenticated: false } : s,
    )
    const { pathname, search } = router.state.location
    if (pathname === '/login') return
    void router.navigate(`/login?redirect=${encodeURIComponent(pathname + search)}`, {
      replace: true,
    })
  })

  function Root() {
    return (
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        enableSystem
        storageKey="ccw-theme"
        disableTransitionOnChange
      >
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <RouterProvider router={router} />
            <Toaster richColors closeButton />
          </TooltipProvider>
        </QueryClientProvider>
      </ThemeProvider>
    )
  }

  return { router, queryClient, Root }
}
