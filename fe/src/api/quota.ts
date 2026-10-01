import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api } from './client'

/** One quota window of an account, e.g. "5 giờ" or "Hằng tuần". */
export interface QuotaWindow {
  name: string
  /** Percent used, 0 to 100 (it can exceed 100 when the provider says so). */
  usedPct: number
  used?: string
  limit?: string
  /** RFC 3339 time of the next reset. */
  resetAt?: string
  unlimited?: boolean
}

/** A reset the account can spend (POST /quota/{id}/reset). */
export interface QuotaReset {
  id: string
  kind: 'weekly' | 'grant' | 'credit' | string
  title: string
  description?: string
  clears: string[]
  left: number
  total: number
  validFrom?: string
  expiresAt?: string
  nextAt?: string
  usable: boolean
  blocked?: string
  status?: string
}

export interface AccountQuota {
  connectionId: string
  provider: string
  label: string
  plan?: string
  /** "api": read from the provider; "headers": from the last answer. */
  source: 'api' | 'headers' | string
  windows: QuotaWindow[]
  resets: QuotaReset[]
  resetsError?: string
  /** Set when the quota of this one account could not be read. */
  error?: string
  fetchedAt: string
}

interface QuotaResponse {
  accounts: AccountQuota[]
}

/** The page re-reads the quota this often while the tab is open. */
export const quotaRefreshMs = 60_000

export const quotaKey = ['quota'] as const

const readQuota = (refresh: boolean, signal?: AbortSignal) =>
  api<QuotaResponse>(refresh ? '/quota?refresh=1' : '/quota', { signal }).then(
    (r) => r.accounts ?? [],
  )

export const quotaQuery = queryOptions({
  queryKey: quotaKey,
  queryFn: ({ signal }) => readQuota(false, signal),
  refetchInterval: quotaRefreshMs,
  refetchIntervalInBackground: false,
})

/**
 * useQuota reads GET /quota. `data` is AccountQuota[] sorted by the server
 * (provider, then label). It re-reads every 60 seconds while the tab is open.
 * The overview page reads the same cache entry.
 */
export const useQuota = () => useQuery(quotaQuery)

/** Forces the server to read every account again (GET /quota?refresh=1). */
export function useRefreshQuota() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => readQuota(true),
    onSuccess: (accounts) => client.setQueryData(quotaKey, accounts),
  })
}

export type QuotaView = 'cards' | 'table'

const viewKey = ['ui-settings', 'quota-view'] as const

/** The saved view lives on the server (GET/POST /ui-settings/quota-view). */
export function useQuotaView() {
  const client = useQueryClient()
  const query = useQuery({
    queryKey: viewKey,
    queryFn: ({ signal }) =>
      api<{ view?: QuotaView }>('/ui-settings/quota-view', { signal }),
    select: (s): QuotaView => (s?.view === 'table' ? 'table' : 'cards'),
  })
  const save = useMutation({
    mutationFn: (view: QuotaView) =>
      api('/ui-settings/quota-view', { method: 'POST', body: { view } }),
    onMutate: (view) => client.setQueryData(viewKey, { view }),
    onError: () => client.invalidateQueries({ queryKey: viewKey }),
  })
  return { view: query.data ?? 'cards', setView: save.mutate }
}

export interface ClaimResult {
  outcome:
    | 'reset'
    | 'not_needed'
    | 'spent'
    | 'not_allowed'
    | 'failed'
    | 'unknown'
    | 'likely_spent'
    | string
  providerCode?: string
  message?: string
  requestId: string
  quota?: AccountQuota
}

export interface ClaimVars {
  connectionId: string
  resetId: string
}

const newRequestId = () =>
  `req-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`

/** Spends one reset. A 409 (blocked) arrives as an ApiError. */
export function useClaimReset() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ connectionId, resetId }: ClaimVars) =>
      api<ClaimResult>(`/quota/${encodeURIComponent(connectionId)}/reset`, {
        method: 'POST',
        body: { resetId, requestId: newRequestId() },
      }),
    onSettled: () => client.invalidateQueries({ queryKey: quotaKey }),
  })
}
