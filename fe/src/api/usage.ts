import { queryOptions, useQuery } from '@tanstack/react-query'
import { api } from './client'

/** Daily counters for one account and model (GET /usage). */
export interface UsageRow {
  /** Calendar day, YYYY-MM-DD. */
  day: string
  connectionId: string
  model: string
  inputTokens: number
  outputTokens: number
  requests: number
}

export const usageQuery = queryOptions({
  queryKey: ['usage'],
  queryFn: ({ signal }) =>
    api<{ usage: UsageRow[] }>('/usage', { signal }).then((r) => r.usage ?? []),
})

/** useUsage: `data` is UsageRow[], newest day first, as the server sends it. */
export const useUsage = () => useQuery(usageQuery)

interface AccountRef {
  id: string
  provider: string
  label: string
}

/** useUsageAccounts maps a connection id to a readable account name. */
export const useUsageAccounts = () =>
  useQuery({
    queryKey: ['accounts', 'labels'],
    queryFn: ({ signal }) =>
      api<{ accounts: AccountRef[] }>('/accounts', { signal }).then(
        (r) => r.accounts ?? [],
      ),
    select: (list) => new Map(list.map((a) => [a.id, a.label || a.id])),
  })
