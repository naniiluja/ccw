import type { ErrorFilter, ErrorVerdict } from '@/api/errors'

// All Vietnamese strings and display helpers of the errors page.

export const classLabels: Record<string, string> = {
  network: 'Lỗi mạng',
  timeout: 'Quá thời gian',
  auth: 'Xác thực',
  rejected: 'Bị từ chối',
  rate_limit: 'Giới hạn tốc độ',
  fake_rate_limit: 'Giới hạn giả',
  server: 'Lỗi máy chủ',
  other: 'Khác',
}

export const classLabel = (c: string) => classLabels[c] ?? c

export const actionLabels: Record<string, string> = {
  blacklist: 'Thêm vào blacklist',
  disable_model: 'Tắt model',
  disable_account: 'Tắt tài khoản',
  ignore: 'Bỏ qua',
}

export const actionLabel = (a: string) => actionLabels[a] ?? a

/** Confidence of a verdict: a replayed and verified one is the strongest. */
export function confidence(v: ErrorVerdict): {
  label: string
  level: 'high' | 'medium' | 'low'
} {
  if (v.replayed && v.verified) return { label: 'Tin cậy cao', level: 'high' }
  if (v.replayed || v.verified) {
    return { label: 'Tin cậy trung bình', level: 'medium' }
  }
  return { label: 'Suy đoán, chưa kiểm chứng', level: 'low' }
}

export const bodyLimitBytes = 64 * 1024

/** The server cuts a stored body at 64 KiB and appends an ellipsis. */
export function isClipped(body: string | undefined): boolean {
  if (!body || !body.endsWith('…')) return false
  return new TextEncoder().encode(body).length >= bodyLimitBytes
}

const dateTime = new Intl.DateTimeFormat('vi-VN', {
  dateStyle: 'short',
  timeStyle: 'medium',
})

export function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : dateTime.format(d)
}

export function formatLatency(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${ms} ms`
}

export const formatStatus = (status: number) =>
  status === 0 ? 'Không có' : String(status)

// Filters live in the URL so a view can be shared.

export const pageSizes = [25, 50, 100, 200] as const
export const defaultLimit = 50

export const sinceRanges = [
  { value: 'all', label: 'Tất cả', ms: 0 },
  { value: '1h', label: '1 giờ qua', ms: 3_600_000 },
  { value: '24h', label: '24 giờ qua', ms: 86_400_000 },
  { value: '7d', label: '7 ngày qua', ms: 7 * 86_400_000 },
] as const

/** An RFC 3339 time without milliseconds, the way the server stores `at`. */
export function sinceFor(ms: number, now = Date.now()): string {
  return new Date(now - ms).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

export function readFilter(p: URLSearchParams): ErrorFilter {
  const status = Number(p.get('status'))
  const limit = Number(p.get('limit'))
  return {
    provider: p.get('provider') || undefined,
    class: p.get('class') || undefined,
    signature: p.get('signature') || undefined,
    status: Number.isInteger(status) && status > 0 ? status : undefined,
    since: p.get('since') || undefined,
    limit: Number.isInteger(limit) && limit > 0 ? limit : defaultLimit,
  }
}
