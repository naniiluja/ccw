import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api } from './client'

// Shapes mirror be/internal/store/apikey.go (APIKey, KeyUsageRow) and
// be/internal/httpapi/keys.go. A listing carries only the masked key.
export interface ApiKey {
  id: string
  name: string
  masked: string
  enabled: boolean
  /** "<provider>/<model>" ids the key may call; empty means every model. */
  models: string[]
  /** RFC 3339 UTC, empty when the key never expires. */
  expiresAt: string
  /** Requests per 60 seconds, 0 for no cap. */
  rpm: number
  lastUsed: string
  createdAt: string
}

export interface KeyUsageRow {
  day: string
  model: string
  inputTokens: number
  outputTokens: number
  requests: number
}

export interface KeyUsage {
  rows: KeyUsageRow[]
  rpm: number
  lastMinute: number
  expiresAt: string
  lastUsed: string
  today: string
}

export const maxKeyRPM = 100000

export const keysQuery = queryOptions({
  queryKey: ['keys'],
  queryFn: async ({ signal }) =>
    (await api<{ keys: ApiKey[] | null }>('/keys', { signal })).keys ?? [],
})

export const useKeys = () => useQuery(keysQuery)

// The picker's choices: the ids of /v1/models, which accepts the session.
export const modelIdsQuery = queryOptions({
  queryKey: ['models', 'ids'],
  queryFn: async ({ signal }) =>
    (await api<{ data: { id: string }[] }>('/v1/models', { signal })).data
      .map((m) => m.id)
      .sort(),
  staleTime: 60_000,
})

export const useModelIds = () => useQuery(modelIdsQuery)

export const keyUsageQuery = (id: string) =>
  queryOptions({
    queryKey: ['keys', id, 'usage'],
    queryFn: ({ signal }) =>
      api<KeyUsage>(`/keys/${encodeURIComponent(id)}/usage`, { signal }),
    staleTime: 0,
  })

export const useKeyUsage = (id: string) => useQuery(keyUsageQuery(id))

const path = (id: string, action: string) =>
  `/keys/${encodeURIComponent(id)}/${action}`

// The full key of a create or a reveal never enters the query or mutation
// cache: the mutation hands it to onSecret (a local state setter in the
// dialog) and returns only what is safe to keep.
export interface SecretHandler {
  onSecret: (key: string) => void
}

export function useCreateKey() {
  const qc = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: async ({
      name,
      models,
      onSecret,
    }: SecretHandler & { name: string; models: string[] }) => {
      const k = await api<{ id: string; key: string }>('/keys', {
        method: 'POST',
        body: { name, models },
      })
      onSecret(k.key)
      return { id: k.id }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: keysQuery.queryKey }),
  })
}

export function useRevealKey() {
  return useMutation({
    gcTime: 0,
    mutationFn: async ({ id, onSecret }: SecretHandler & { id: string }) => {
      const r = await api<{ key: string }>(path(id, 'reveal'), { method: 'POST' })
      onSecret(r.key)
      return { id }
    },
  })
}

export function useSetKeyActive() {
  const qc = useQueryClient()
  const key = keysQuery.queryKey
  return useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      api(path(id, 'active'), { method: 'POST', body: { active } }),
    async onMutate({ id, active }) {
      await qc.cancelQueries({ queryKey: key })
      const before = qc.getQueryData<ApiKey[]>(key)
      qc.setQueryData<ApiKey[]>(key, (list) =>
        list?.map((k) => (k.id === id ? { ...k, enabled: active } : k)),
      )
      return { before }
    },
    onError(_e, _v, ctx) {
      if (ctx?.before) qc.setQueryData(key, ctx.before)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  })
}

export function useSetKeyModels() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, models }: { id: string; models: string[] }) =>
      api(path(id, 'models'), { method: 'POST', body: { models } }),
    onSettled: () => qc.invalidateQueries({ queryKey: keysQuery.queryKey }),
  })
}

export function useSetKeyLimits() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      rpm,
      expiresAt,
    }: {
      id: string
      rpm: number
      /** RFC 3339, or empty for no expiry. */
      expiresAt: string
    }) => api(path(id, 'limits'), { method: 'POST', body: { rpm, expiresAt } }),
    onSettled: () => qc.invalidateQueries({ queryKey: keysQuery.queryKey }),
  })
}

export function useDeleteKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id }: { id: string }) =>
      api(path(id, 'delete'), { method: 'POST' }),
    onSettled: () => qc.invalidateQueries({ queryKey: keysQuery.queryKey }),
  })
}
