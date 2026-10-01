import { useSearchParams } from 'react-router'
import type { DriftChangesFilter, DriftDirection } from '@/api/drift'
import { defaultLimit, limitOptions } from './strings'

export interface Filters {
  filter: DriftChangesFilter & { provider: string; direction: DriftDirection | ''; unacked: boolean; limit: number }
  update: (patch: Record<string, string | null>) => void
}

// useFilters reads the list filters from the URL and writes them back.
export function useFilters(): Filters {
  const [params, setParams] = useSearchParams()
  const direction = params.get('direction')
  const limit = Number(params.get('limit'))
  const filter = {
    provider: params.get('provider') ?? '',
    direction: (direction === 'request' || direction === 'response'
      ? direction
      : '') as DriftDirection | '',
    unacked: params.get('unacked') === '1',
    limit: limitOptions.includes(limit) ? limit : defaultLimit,
  }
  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v)
          else next.delete(k)
        }
        return next
      },
      { replace: true },
    )
  return { filter, update }
}
