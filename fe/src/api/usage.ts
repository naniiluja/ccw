import { queryOptions, useQuery } from '@tanstack/react-query'
import { accountsQuery } from './accounts'
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

/** useUsageAccounts maps a connection id to a readable account name. */
export const useUsageAccounts = () =>
  useQuery({
    ...accountsQuery,
    select: (list) => new Map(list.map((a) => [a.id, a.label || a.id])),
  })
