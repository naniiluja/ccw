import type { Account } from './accounts'
import { api } from './client'

// Sign-in flows. Their answers carry one-time values (a state, a device code),
// so the pages keep them in component state and never in the query cache.

const enc = encodeURIComponent

export interface CodeStart {
  url: string
  state: string
  redirect: string
}

export interface DeviceStart {
  device: true
  deviceCode: string
  userCode: string
  verificationUri: string
  /** Seconds between polls. */
  interval: number
  /** Seconds until the code expires. */
  expiresIn: number
}

export type OAuthStart = CodeStart | DeviceStart

export const isDeviceStart = (s: OAuthStart): s is DeviceStart =>
  'device' in s && s.device === true

export const startOAuth = (provider: string, label?: string) =>
  api<OAuthStart>(`/oauth/${enc(provider)}/start`, {
    method: 'POST',
    body: label ? { label } : {},
  })

export const finishOAuth = (
  provider: string,
  input: { state: string; input: string; label?: string },
) =>
  api<{ connection: Account }>(`/oauth/${enc(provider)}/finish`, {
    method: 'POST',
    body: input,
  })

export interface PollResult {
  status: 'pending' | 'done'
  connection?: Account
}

// GitHub has its own poll route; a declared device provider uses the generic one.
export const pollDevice = (
  provider: string,
  deviceCode: string,
  label?: string,
  signal?: AbortSignal,
) =>
  api<PollResult>(
    provider === 'github' ? '/oauth/github/poll' : `/oauth/${enc(provider)}/poll`,
    { method: 'POST', body: { deviceCode, ...(label ? { label } : {}) }, signal },
  )
