import { queryOptions } from '@tanstack/react-query'
import { ApiError, api } from './client'

// The server never sends a credential for an account, so none of these types
// carries one. A key typed into the add form goes out once and is not kept.

export interface Account {
  id: string
  provider: string
  label: string
  isActive: boolean
  /** A standby account is tried only after every other active one. */
  standby: boolean
  baseUrl: string
  meta?: Record<string, string>
}

/** The last test of an account, as GET /account-tests and the test route give it. */
export interface AccountTest {
  ok: boolean
  status: number
  ms: number
  message: string
  model: string
  at: string
}

/** What a provider needs besides a label: a key, an account id, nothing, or OAuth. */
export interface ProviderInfo {
  id: string
  setup: 'key' | 'account' | 'none' | 'oauth'
  auth: 'oauth' | 'apikey'
  declared?: boolean
  name?: string
  /** "code" or "device" for a declared OAuth provider. */
  flow?: string
  api?: string
}

export const accountsQuery = queryOptions({
  queryKey: ['accounts'],
  // A failed list shows its own retry button, so the query does not retry.
  retry: false,
  queryFn: async ({ signal }) => {
    const res = await api<{ accounts: Account[] | null }>('/accounts', { signal })
    return res.accounts ?? []
  },
})

export const accountTestsQuery = queryOptions({
  queryKey: ['account-tests'],
  // The server tests accounts in the background; the badges follow it while
  // the tab is visible (a hidden tab does not poll by default).
  refetchInterval: 30_000,
  queryFn: ({ signal }) =>
    api<Record<string, AccountTest>>('/account-tests', { signal }),
})

export const accountProvidersQuery = queryOptions({
  queryKey: ['accounts', 'providers'],
  staleTime: 5 * 60_000,
  retry: false,
  queryFn: async ({ signal }) => {
    const res = await api<{ providers: ProviderInfo[] | null }>('/providers', {
      signal,
    })
    return res.providers ?? []
  },
})

const enc = encodeURIComponent

// These two routes answer with a redirect to the dashboard instead of JSON.
// The browser must not follow it (it would land on the HTML page), so the
// redirect itself is the success signal.
async function postForRedirect(
  path: string,
  form?: Record<string, string>,
): Promise<void> {
  let res: Response
  try {
    res = await fetch(path, {
      method: 'POST',
      credentials: 'same-origin',
      redirect: 'manual',
      headers: {
        Accept: 'application/json',
        ...(form ? { 'Content-Type': 'application/x-www-form-urlencoded' } : {}),
      },
      body: form ? new URLSearchParams(form).toString() : undefined,
    })
  } catch {
    throw new ApiError('Không kết nối được tới máy chủ', 0)
  }
  if (
    res.ok ||
    res.type === 'opaqueredirect' ||
    (res.status >= 300 && res.status < 400)
  ) {
    return
  }
  let message = `Yêu cầu thất bại (${res.status})`
  try {
    const data: unknown = await res.json()
    if (data && typeof data === 'object' && 'error' in data) {
      const text = (data as { error: unknown }).error
      if (typeof text === 'string' && text) message = text
    }
  } catch {
    // Not JSON; keep the generic message.
  }
  throw new ApiError(message, res.status)
}

export interface NewAccount {
  provider: string
  label?: string
  secret?: string
  base_url?: string
  account_id?: string
  api?: string
}

export function createAccount(input: NewAccount): Promise<void> {
  const form: Record<string, string> = {}
  for (const [k, v] of Object.entries(input)) {
    if (v) form[k] = v
  }
  return postForRedirect('/accounts', form)
}

export const deleteAccount = (id: string) =>
  postForRedirect(`/accounts/${enc(id)}/delete`)

export const setLabel = (id: string, label: string) =>
  api(`/accounts/${enc(id)}/label`, { method: 'POST', body: { label } })

export const setActive = (id: string, active: boolean, standby?: boolean) =>
  api(`/accounts/${enc(id)}/active`, {
    method: 'POST',
    body: standby === undefined ? { active } : { active, standby },
  })

export const testAccount = (id: string) =>
  api<AccountTest>(`/accounts/${enc(id)}/test`, { method: 'POST', body: {} })

/** The provider answers in its own shape; this reads the model ids from any of them. */
export function modelIds(raw: unknown): string[] {
  const list: unknown[] = Array.isArray(raw)
    ? raw
    : raw && typeof raw === 'object'
      ? (['data', 'models'] as const).flatMap((k) => {
          const v = (raw as Record<string, unknown>)[k]
          return Array.isArray(v) ? v : []
        })
      : []
  const ids = list.flatMap((m) => {
    if (typeof m === 'string') return [m]
    if (m && typeof m === 'object') {
      const o = m as Record<string, unknown>
      const id = o.id ?? o.name ?? o.model
      return typeof id === 'string' ? [id] : []
    }
    return []
  })
  return [...new Set(ids.map((id) => id.replace(/^models\//, '')))]
}

export const accountModels = (id: string, signal?: AbortSignal) =>
  api<unknown>(`/accounts/${enc(id)}/models`, { signal }).then(modelIds)
