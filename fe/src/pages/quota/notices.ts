import { isApiError } from '@/api/client'
import type { ClaimResult } from '@/api/quota'
import { blockedReason } from './format'

export interface ClaimNotice {
  ok: boolean
  title: string
  detail?: string
}

const outcomes: Record<string, { ok: boolean; title: string }> = {
  reset: { ok: true, title: 'Đã reset thành công' },
  not_needed: { ok: false, title: 'Chưa cần reset: tài khoản chưa chạm giới hạn' },
  spent: { ok: false, title: 'Lượt reset này đã được dùng trước đó' },
  not_allowed: { ok: false, title: 'Nhà cung cấp không cho reset lúc này' },
  failed: { ok: false, title: 'Claim thất bại' },
  unknown: {
    ok: false,
    title: 'Chưa rõ kết quả claim, hãy làm mới rồi kiểm tra lại sau ít phút',
  },
  likely_spent: { ok: true, title: 'Có vẻ lượt reset đã được dùng' },
}

export function noticeOf(r: ClaimResult): ClaimNotice {
  const o = outcomes[r.outcome] ?? { ok: false, title: `Kết quả: ${r.outcome}` }
  return { ...o, detail: r.message || undefined }
}

export function noticeOfError(e: unknown): ClaimNotice {
  const raw = e instanceof Error ? e.message : ''
  const detail = isApiError(e) ? blockedReason(raw) || raw : raw
  return { ok: false, title: 'Claim thất bại', detail: detail || undefined }
}

