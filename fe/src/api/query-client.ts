import { QueryCache, QueryClient } from '@tanstack/react-query'
import { isApiError } from './client'

// createQueryClient builds the one client of the app. A 401 from any query
// means the session ended, so onUnauthorized runs once per failing query and
// the app goes back to the login page. A 4xx will not change on a retry.
export function createQueryClient(onUnauthorized: () => void) {
  return new QueryClient({
    queryCache: new QueryCache({
      onError(error) {
        if (isApiError(error) && error.status === 401) onUnauthorized()
      },
    }),
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry(count, error) {
          if (isApiError(error) && error.status >= 400 && error.status < 500) {
            return false
          }
          return count < 2
        },
      },
    },
  })
}
