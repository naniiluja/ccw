import type { QuotaWindow } from '@/api/quota'

export type Level = 'ok' | 'high' | 'critical'

/** Thresholds of the warning: 75% is "gần ngưỡng", 90% is "sắp hết". */
export const highPct = 75
export const criticalPct = 90

export function clampPct(pct: number): number {
  if (!Number.isFinite(pct)) return 0
  return Math.min(100, Math.max(0, Math.round(pct)))
}

export function levelOf(w: QuotaWindow): Level {
  if (w.unlimited) return 'ok'
  if (w.usedPct >= criticalPct) return 'critical'
  if (w.usedPct >= highPct) return 'high'
  return 'ok'
}

const dateTime = new Intl.DateTimeFormat('vi-VN', {
  hour: '2-digit',
  minute: '2-digit',
  day: '2-digit',
  month: '2-digit',
})

/** "Reset sau 2 giờ 5 phút (10:00 01/01)" or "" when there is no reset time. */
export function resetText(iso: string | undefined, now = Date.now()): string {
  if (!iso) return ''
  const at = Date.parse(iso)
  if (Number.isNaN(at)) return ''
  const abs = dateTime.format(at)
  const minutes = Math.round((at - now) / 60_000)
  if (minutes <= 0) return `Đã tới hạn reset (${abs})`
  const days = Math.floor(minutes / 1440)
  const hours = Math.floor((minutes % 1440) / 60)
  const mins = minutes % 60
  const parts = [
    days ? `${days} ngày` : '',
    hours ? `${hours} giờ` : '',
    !days && mins ? `${mins} phút` : '',
  ].filter(Boolean)
  return `Reset sau ${parts.join(' ')} (${abs})`
}

export function clockText(ms: number): string {
  return new Date(ms).toLocaleTimeString('vi-VN')
}

export const sourceText = (source: string) =>
  source === 'api' ? 'Nguồn: API' : source === 'headers' ? 'Nguồn: header' : source

const blockedText: Record<string, string> = {
  just_reset: 'Vừa reset xong, đợi nhà cung cấp cập nhật số lượt còn lại',
  not_found: 'Lượt reset này không còn trong danh sách',
  claim_in_progress: 'Đang có một claim khác của tài khoản này',
}

export const blockedReason = (code: string | undefined) =>
  code ? (blockedText[code] ?? code) : ''
