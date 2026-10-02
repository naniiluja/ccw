import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api } from './client'

// Shapes follow be/internal/httpapi/errlog.go and errreview.go.

export const errorClasses = [
  'network',
  'timeout',
  'auth',
  'rejected',
  'rate_limit',
  'fake_rate_limit',
  'server',
  'other',
] as const

export type ErrorClass = (typeof errorClasses)[number]

/** One stored upstream error. The list leaves reqBody and respBody empty. */
export interface UpstreamError {
  id: number
  at: string
  provider: string
  connectionId: string
  model: string
  client: string
  clientKeyId?: string
  endpoint: string
  /** 0 means the call itself failed (network, timeout). */
  status: number
  latencyMs: number
  class: string
  signature: string
  message: string
  /** 0 to 1 of the model's quota when read; -1 when unknown. */
  quotaLeft: number
  headers?: string
  respBody?: string
  reqBody?: string
}

/** The errors of one signature. */
export interface ErrorGroup {
  signature: string
  provider: string
  status: number
  classes: Record<string, number>
  count: number
  first: string
  last: string
  models: string[]
  message: string
  lastId: number
  medianMs: number
}

export interface ErrorFilter {
  provider?: string
  class?: string
  signature?: string
  status?: number
  /** RFC 3339 time; only newer errors are returned. */
  since?: string
  limit?: number
}

export interface ErrorReviewState {
  enabled: boolean
  model: string
  minErrors: number
  replay: boolean
  ready: boolean
  lastError: string
  /** Only in the answer to a run: how many groups were judged. */
  judged?: number
}

export interface ErrorReviewSettings {
  enabled?: boolean
  model?: string
  minErrors?: number
  replay?: boolean
  run?: boolean
}

export interface ErrorVerdict {
  id: number
  at: string
  provider: string
  signature: string
  /** blacklist, disable_model, disable_account or ignore. */
  action: string
  applied: boolean
  verified: boolean
  detail: string
  cause: string
  reason: string
  note: string
  by: string
  errors: number
  lastError: string
  replayed: boolean
}

/** How often the page and the overview refresh while the tab is open. */
export const errorsRefreshMs = 30_000

function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== 0) {
      q.set(key, String(value))
    }
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}

export const errorsKey = ['errors'] as const

export const errorsQuery = (filter: ErrorFilter = {}) =>
  queryOptions({
    queryKey: [...errorsKey, 'list', filter],
    queryFn: async ({ signal }) =>
      (
        await api<{ errors: UpstreamError[] | null }>(
          `/errors${query({ ...filter })}`,
          { signal },
        )
      ).errors ?? [],
  })

export const errorStatsQuery = (
  filter: Pick<ErrorFilter, 'provider' | 'since'> = {},
) =>
  queryOptions({
    queryKey: [...errorsKey, 'stats', filter],
    queryFn: async ({ signal }) =>
      (
        await api<{ groups: ErrorGroup[] | null }>(
          `/errors/stats${query({ ...filter })}`,
          { signal },
        )
      ).groups ?? [],
  })

export interface PollOptions {
  /** Poll every errorsRefreshMs while the tab is visible. */
  poll?: boolean
}

const interval = (poll?: boolean) => (poll ? errorsRefreshMs : false)

/** The errors, newest first. Data is an array. */
export function useErrors(filter: ErrorFilter = {}, { poll }: PollOptions = {}) {
  return useQuery({
    ...errorsQuery(filter),
    placeholderData: keepPreviousData,
    refetchInterval: interval(poll),
  })
}

/** The errors grouped by signature. Data is an array. */
export function useErrorStats(
  filter: Pick<ErrorFilter, 'provider' | 'since'> = {},
  { poll }: PollOptions = {},
) {
  return useQuery({
    ...errorStatsQuery(filter),
    placeholderData: keepPreviousData,
    refetchInterval: interval(poll),
  })
}

/** One error with its stored request and response. */
export function useError(id: number | null) {
  return useQuery({
    queryKey: [...errorsKey, 'one', id],
    queryFn: ({ signal }) => api<UpstreamError>(`/errors/${id}`, { signal }),
    enabled: id !== null,
  })
}

const reviewKey = [...errorsKey, 'review'] as const

export const useErrorReview = () =>
  useQuery({
    queryKey: reviewKey,
    queryFn: ({ signal }) => api<ErrorReviewState>('/errors/review', { signal }),
  })

/** Saves the review settings; with run set it also judges the groups due. */
export function useSaveErrorReview() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (settings: ErrorReviewSettings) =>
      api<ErrorReviewState>('/errors/review', {
        method: 'POST',
        body: { ...settings },
      }),
    onSuccess: (state) => {
      client.setQueryData(reviewKey, state)
      void client.invalidateQueries({ queryKey: [...errorsKey, 'verdicts'] })
    },
  })
}

export function useErrorVerdicts({ poll }: PollOptions = {}) {
  return useQuery({
    queryKey: [...errorsKey, 'verdicts'],
    queryFn: async ({ signal }) =>
      (
        await api<{ verdicts: ErrorVerdict[] | null }>('/errors/verdicts', {
          signal,
        })
      ).verdicts ?? [],
    refetchInterval: interval(poll),
  })
}
