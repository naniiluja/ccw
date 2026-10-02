import { isApiError } from '@/api/client'

export function messageOf(error: unknown): string {
  if (isApiError(error) || error instanceof Error) return error.message
  return 'Đã có lỗi không xác định.'
}

// fieldOfError finds which form field a server message names. The server
// writes "field: reason" (provider definitions) or "field is ..." (rotation).
export function fieldOfError<F extends string>(
  message: string,
  fields: readonly F[],
): F | undefined {
  const m = /^([A-Za-z]+)(?::|\s+is\b)/.exec(message)
  return fields.find((f) => f === m?.[1])
}

const dateTime = new Intl.DateTimeFormat('vi-VN', {
  dateStyle: 'short',
  timeStyle: 'short',
})

export function formatTime(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : dateTime.format(d)
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds} giây`
  if (seconds < 3600) return `${Math.round(seconds / 60)} phút`
  return `${(seconds / 3600).toFixed(1)} giờ`
}

export const providerName = (p: { id: string; name?: string }) => p.name || p.id
