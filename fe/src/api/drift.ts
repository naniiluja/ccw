import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { api } from './client'

export type DriftDirection = 'request' | 'response'

// DriftChange mirrors store.ShapeChange in the Go server.
export interface DriftChange {
  id: number
  at: string
  direction: DriftDirection
  provider: string
  endpoint: string
  event: string
  path: string
  kind: string
  oldType: string
  newType: string
  /** Empty unless the caller is an admin. */
  sample: string
  acked: boolean
  client?: string
  clientKeyId?: string
  verdict?: string
  verdictConf?: number
  verdictAt?: string
  autoAcked?: boolean
  verdictBy?: string
  verdictNote?: string
  resolved?: boolean
}

export interface DriftChangesFilter {
  provider?: string
  direction?: DriftDirection | ''
  unacked?: boolean
  limit?: number
}

export interface DriftChangesResponse {
  changes: DriftChange[]
  unacked: number
}

// DriftField mirrors store.ShapeField.
export interface DriftField {
  /** "direction|provider|endpoint@variant|..." as the observer keys it. */
  key: string
  path: string
  type: string
  seen: number
  firstObs: number
  lastObs: number
  gone: boolean
  lastAt: string
  legacy?: boolean
}

export interface DriftReview {
  enabled: boolean
  decisionModel: string
  resolverModel: string
  ackConfidence: number
  ready: boolean
  lastError: string
  /** Set only by a run: how many changes were judged. */
  judged?: number
}

export interface DriftReviewConfig {
  enabled: boolean
  decisionModel: string
  resolverModel: string
  ackConfidence: number
}

export interface DriftSeedRequest {
  direction: DriftDirection
  provider: string
  endpoint: string
  sse: boolean
  documents: string[]
}

const changesKey = ['drift', 'changes'] as const

function changesPath(f: DriftChangesFilter): string {
  const q = new URLSearchParams()
  if (f.provider) q.set('provider', f.provider)
  if (f.direction) q.set('direction', f.direction)
  if (f.unacked) q.set('unacked', '1')
  if (f.limit) q.set('limit', String(f.limit))
  const s = q.toString()
  return s ? `/drift/changes?${s}` : '/drift/changes'
}

/**
 * useDriftChanges lists recorded shape changes. The response carries the
 * global unacked count in `data.unacked` (the overview page reads it with
 * `useDriftChanges({ unacked: true, limit: 1 })`).
 */
export function useDriftChanges(filter: DriftChangesFilter = {}) {
  return useQuery({
    queryKey: [...changesKey, filter.provider ?? '', filter.direction ?? '', !!filter.unacked, filter.limit ?? 0],
    queryFn: ({ signal }) =>
      api<DriftChangesResponse>(changesPath(filter), { signal }),
    placeholderData: keepPreviousData,
    retry: false,
  })
}

export function useDriftFields() {
  return useQuery({
    queryKey: ['drift', 'fields'],
    queryFn: ({ signal }) =>
      api<{ fields: DriftField[] }>('/drift/fields', { signal }),
    retry: false,
  })
}

type Snapshot = [readonly unknown[], DriftChangesResponse | undefined][]

function markAcked(qc: QueryClient, ids: number[]) {
  const set = new Set(ids)
  qc.setQueriesData<DriftChangesResponse>({ queryKey: changesKey }, (old) => {
    if (!old) return old
    let newly = 0
    const changes = old.changes.map((c) => {
      if (!set.has(c.id) || c.acked) return c
      newly++
      return { ...c, acked: true }
    })
    return { changes, unacked: Math.max(0, old.unacked - newly) }
  })
}

/**
 * useAckChanges acknowledges the given ids. It never sends an empty list: the
 * server reads that as "acknowledge everything". The cache is updated at once
 * and restored if the server refuses.
 */
export function useAckChanges() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: number[]) => {
      if (ids.length === 0) throw new Error('no changes to acknowledge')
      return api<{ acked: number }>('/drift/ack', {
        method: 'POST',
        body: { ids },
      })
    },
    async onMutate(ids) {
      await qc.cancelQueries({ queryKey: changesKey })
      const snapshot: Snapshot = qc.getQueriesData<DriftChangesResponse>({
        queryKey: changesKey,
      })
      markAcked(qc, ids)
      return { snapshot }
    },
    onError(_error, _ids, ctx) {
      for (const [key, data] of ctx?.snapshot ?? []) qc.setQueryData(key, data)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: changesKey }),
  })
}

const reviewKey = ['drift', 'review'] as const

export function useDriftReview() {
  return useQuery({
    queryKey: reviewKey,
    queryFn: ({ signal }) => api<DriftReview>('/drift/review', { signal }),
    retry: false,
  })
}

export function useSaveDriftReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (config: DriftReviewConfig) =>
      api<DriftReview>('/drift/review', { method: 'POST', body: { ...config } }),
    onSuccess: (data) => qc.setQueryData(reviewKey, data),
  })
}

export function useRunDriftReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api<DriftReview>('/drift/review', { method: 'POST', body: { run: true } }),
    onSuccess(data) {
      qc.setQueryData(reviewKey, data)
      return qc.invalidateQueries({ queryKey: changesKey })
    },
  })
}

export function useSeedDrift() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: DriftSeedRequest) =>
      api<{ learned: number }>('/api/drift/seed', {
        method: 'POST',
        body: { ...req },
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['drift', 'fields'] }),
  })
}
