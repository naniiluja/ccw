import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api } from './client'

// One blacklist rule, as GET /filters returns it. Provider "*" applies to all.
export interface Filter {
  id: string
  provider: string
  kind: string
  pattern: string
  note: string
  enabled: boolean
}

/** A rule to save: no id creates it, an id replaces that rule. */
export type FilterInput = Omit<Filter, 'id'> & { id?: string }

export interface ProviderOption {
  id: string
  name?: string
}

export const filtersQuery = queryOptions({
  queryKey: ['filters'],
  queryFn: async ({ signal }) =>
    (await api<{ filters: Filter[] }>('/filters', { signal })).filters ?? [],
})

export const filterProvidersQuery = queryOptions({
  queryKey: ['filter-providers'],
  queryFn: async ({ signal }) =>
    (await api<{ providers: ProviderOption[] }>('/providers', { signal }))
      .providers ?? [],
})

export const useFilters = () => useQuery(filtersQuery)
export const useFilterProviders = () => useQuery(filterProvidersQuery)

export const saveFilter = (input: FilterInput) =>
  api<Filter>('/filters', { method: 'POST', body: { ...input } })

export function useSaveFilter() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: saveFilter,
    onSuccess: () => qc.invalidateQueries({ queryKey: filtersQuery.queryKey }),
  })
}

/** Flips a rule at once and puts the old list back when the server refuses. */
export function useToggleFilter() {
  const qc = useQueryClient()
  const key = filtersQuery.queryKey
  return useMutation({
    mutationFn: (f: Filter) => saveFilter({ ...f, enabled: !f.enabled }),
    onMutate: async (f) => {
      await qc.cancelQueries({ queryKey: key })
      const previous = qc.getQueryData<Filter[]>(key)
      qc.setQueryData<Filter[]>(key, (list) =>
        list?.map((x) => (x.id === f.id ? { ...x, enabled: !f.enabled } : x)),
      )
      return { previous }
    },
    onError: (_err, _f, ctx) => {
      if (ctx?.previous) qc.setQueryData(key, ctx.previous)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  })
}

export function useDeleteFilter() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api<{ ok: boolean }>(`/filters/${encodeURIComponent(id)}/delete`, {
        method: 'POST',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: filtersQuery.queryKey }),
  })
}
