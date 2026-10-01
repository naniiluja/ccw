import type { ApiKey } from '@/api/keys'

export type KeyStatus = 'active' | 'disabled' | 'expired'

export function keyStatus(k: ApiKey, now = Date.now()): KeyStatus {
  if (k.expiresAt) {
    const t = Date.parse(k.expiresAt)
    if (Number.isNaN(t) || t <= now) return 'expired'
  }
  return k.enabled ? 'active' : 'disabled'
}

const numberFormat = new Intl.NumberFormat('vi-VN')
export const formatNumber = (n: number) => numberFormat.format(n)

const dateTime = new Intl.DateTimeFormat('vi-VN', {
  dateStyle: 'medium',
  timeStyle: 'short',
})

/** A readable local time, or the fallback when the value is empty or invalid. */
export function formatTime(iso: string, fallback: string): string {
  if (!iso) return fallback
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? fallback : dateTime.format(d)
}

const pad = (n: number) => String(n).padStart(2, '0')

/** An RFC 3339 time as the value of a datetime-local input (browser zone). */
export function toLocalInput(iso: string): string {
  const d = new Date(iso)
  if (!iso || Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
