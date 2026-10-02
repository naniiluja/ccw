// Queries and mutations for the Providers page. The JSON shapes follow
// be/internal/httpapi (accounts.go, models.go, rotation.go, providers.go).
// Provider definitions can carry header values and an OAuth client secret for
// an admin; the list query drops them before they reach the query cache, and
// the editor reads one definition straight from the server into local state.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api } from './client'

export interface ProviderInfo {
  id: string
  setup: string
  auth: 'oauth' | 'apikey'
  declared?: boolean
  name?: string
  color?: string
  iconUrl?: string
  flow?: string
  api?: string
}

export interface ProviderModel {
  provider: string
  model: string
  active: boolean
  stale: boolean
  firstSeen: string
  lastSeen: string
  testAt?: string
  testOk: boolean
  testMs?: number
  testMsg?: string
  testConn?: string
}

export interface ModelPolicy {
  autoTest: boolean
  onlyFree: boolean
  lastRun?: string
  lastResult?: string
}

export interface ModelTable {
  models: ProviderModel[]
  ok: boolean
  fetchedAt: string
  policy: ModelPolicy
  running: boolean
}

export interface ModelTestResult {
  ok: boolean
  status: number
  ms: number
  message: string
  account?: string
  model?: string
  active?: boolean
}

export type RotationMode = 'round-robin' | 'fallback'

export interface Rotation {
  mode: RotationMode
  sticky: number
  order: string[] | null
}

export interface RotationState {
  rotation: Rotation
  next?: string
  used?: number
}

export interface PolicyState {
  policy: ModelPolicy
  running: boolean
}

export interface ProviderDef {
  id: string
  name?: string
  color?: string
  icon?: string
  kind: string
  api: string
  baseUrl: string
  authHeader?: string
  authPrefix?: string
  headers?: Record<string, string>
  modelsUrl?: string
  models?: string[]
  oauth?: Record<string, unknown>
  builtin?: boolean
}

export interface ZenSession {
  id: string
  caller: string
  source: string
  uses: number
  created: string
  lastUsed: string
  expiresInSeconds: number
}

export interface ZenSessions {
  count: number
  ttlSeconds: number
  maxSessions: number
  sessions: ZenSession[]
}

const enc = encodeURIComponent

export const providerKeys = {
  all: ['providers'] as const,
  list: ['providers', 'list'] as const,
  counts: ['providers', 'account-counts'] as const,
  defs: ['providers', 'defs'] as const,
  models: (id: string) => ['providers', id, 'models'] as const,
  rotation: (id: string) => ['providers', id, 'rotation'] as const,
  policy: (id: string) => ['providers', id, 'policy'] as const,
  zen: ['providers', 'zen-sessions'] as const,
}

export const providersQuery = queryOptions({
  queryKey: providerKeys.list,
  queryFn: async ({ signal }) =>
    (await api<{ providers: ProviderInfo[] }>('/providers', { signal }))
      .providers,
})

// Only the provider of each account is kept: the list shows a count per provider.
export const accountCountsQuery = queryOptions({
  queryKey: providerKeys.counts,
  queryFn: async ({ signal }) => {
    const { accounts } = await api<{ accounts: { provider: string }[] }>(
      '/accounts',
      { signal },
    )
    const counts: Record<string, number> = {}
    for (const a of accounts) counts[a.provider] = (counts[a.provider] ?? 0) + 1
    return counts
  },
})

function stripSecrets(def: ProviderDef): ProviderDef {
  const out: ProviderDef = { ...def }
  if (def.headers) {
    out.headers = Object.fromEntries(Object.keys(def.headers).map((k) => [k, '']))
  }
  if (def.oauth) {
    const oauth = { ...def.oauth }
    delete oauth.clientSecret
    out.oauth = oauth
  }
  return out
}

export const providerDefsQuery = queryOptions({
  queryKey: providerKeys.defs,
  queryFn: async ({ signal }) =>
    (
      await api<{ providers: ProviderDef[] }>('/provider-defs', { signal })
    ).providers.map(stripSecrets),
})

// fetchProviderDef reads one definition with its secrets; the caller keeps it
// in local state only, never in the query cache.
export const fetchProviderDef = (id: string) =>
  api<ProviderDef>(`/provider-defs/${enc(id)}`)

export const modelTableQuery = (id: string) =>
  queryOptions({
    queryKey: providerKeys.models(id),
    queryFn: ({ signal }) =>
      api<ModelTable>(`/providers/${enc(id)}/model-table`, { signal }),
  })

export const rotationQuery = (id: string) =>
  queryOptions({
    queryKey: providerKeys.rotation(id),
    queryFn: ({ signal }) =>
      api<RotationState>(`/providers/${enc(id)}/rotation`, { signal }),
  })

export const policyQuery = (id: string) =>
  queryOptions({
    queryKey: providerKeys.policy(id),
    queryFn: ({ signal }) =>
      api<PolicyState>(`/providers/${enc(id)}/model-policy`, { signal }),
  })

export const zenSessionsQuery = queryOptions({
  queryKey: providerKeys.zen,
  queryFn: ({ signal }) => api<ZenSessions>('/api/zen/sessions', { signal }),
})

export const useProviders = () => useQuery(providersQuery)
export const useAccountCounts = () => useQuery(accountCountsQuery)
export const useProviderDefs = () => useQuery(providerDefsQuery)
export const useModelTable = (id: string) => useQuery(modelTableQuery(id))
export const useRotation = (id: string) => useQuery(rotationQuery(id))
export const useModelPolicy = (id: string) => useQuery(policyQuery(id))
export const useZenSessions = () => useQuery(zenSessionsQuery)

export function useSetModelsActive(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { models: string[]; active: boolean }) =>
      api<{ updated: number }>(`/providers/${enc(id)}/models/active`, {
        method: 'POST',
        body: v,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: providerKeys.models(id) }),
  })
}

export function useDeleteModels(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (models: string[]) =>
      api<{ deleted: number }>(`/providers/${enc(id)}/models/delete`, {
        method: 'POST',
        body: { models },
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: providerKeys.models(id) }),
  })
}

export function useTestModel(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (model: string) =>
      api<ModelTestResult>(`/providers/${enc(id)}/models/test`, {
        method: 'POST',
        body: { model },
      }),
    onSettled: () => qc.invalidateQueries({ queryKey: providerKeys.models(id) }),
  })
}

export function useSaveRotation(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { mode: RotationMode; sticky: number }) =>
      api<RotationState>(`/providers/${enc(id)}/rotation`, {
        method: 'POST',
        body: v,
      }),
    onSuccess: (data) => qc.setQueryData(providerKeys.rotation(id), data),
  })
}

export function useSavePolicy(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { autoTest: boolean; onlyFree: boolean }) =>
      api<PolicyState>(`/providers/${enc(id)}/model-policy`, {
        method: 'POST',
        body: v,
      }),
    onSuccess: (data) => {
      qc.setQueryData(providerKeys.policy(id), data)
      return qc.invalidateQueries({ queryKey: providerKeys.models(id) })
    },
  })
}

export function useSaveDef() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (def: ProviderDef) =>
      api<ProviderDef>('/provider-defs', { method: 'POST', body: { ...def } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: providerKeys.all }),
  })
}

export function useDeleteDef() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api<{ deleted: boolean }>(`/provider-defs/${enc(id)}/delete`, {
        method: 'POST',
        body: {},
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: providerKeys.all }),
  })
}
