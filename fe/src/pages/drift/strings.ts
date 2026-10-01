import type { DriftDirection } from '@/api/drift'

export const directionLabel: Record<DriftDirection, string> = {
  request: 'Yêu cầu',
  response: 'Phản hồi',
}

export const kindLabel: Record<string, string> = {
  added: 'Thêm trường',
  removed: 'Mất trường',
  returned: 'Trường quay lại',
  changed: 'Đổi kiểu',
}

export const causeLabel: Record<string, string> = {
  new_client_usage: 'Client dùng tính năng mới',
  data_noise: 'Nhiễu dữ liệu người dùng',
  optional_field_flap: 'Trường tùy chọn lúc có lúc không',
  provider_format_change: 'Nhà cung cấp đổi định dạng',
}

export const limitOptions = [25, 50, 100, 200]
export const defaultLimit = 50

const dateTime = new Intl.DateTimeFormat('vi-VN', {
  dateStyle: 'short',
  timeStyle: 'short',
})

export function formatTime(iso: string): string {
  const t = new Date(iso)
  return Number.isNaN(t.getTime()) ? iso : dateTime.format(t)
}

export const percent = (v: number) => `${Math.round(v * 100)}%`
