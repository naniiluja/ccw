// Vietnamese copy of the Filters page.

export const ALL_PROVIDERS = '*'

export const kindLabels: Record<string, string> = {
  field: 'Field',
  schema: 'Schema tool',
  system: 'System prompt',
  header: 'Header',
}

export const kindHints: Record<string, string> = {
  field: 'Đường dẫn JSON trong body, ví dụ thinking hoặc messages.*.cache_control.',
  schema: 'Tên key bị xóa ở mọi độ sâu của JSON schema từng tool, ví dụ $id.',
  system: 'Biểu thức chính quy; dòng khớp bị xóa khỏi system prompt.',
  header: 'Tên header request không được gửi đi, ví dụ anthropic-beta.',
}

export const kinds = Object.keys(kindLabels)

export const providerLabel = (id: string) =>
  id === ALL_PROVIDERS ? 'Tất cả nhà cung cấp' : id

export const text = {
  title: 'Bộ lọc',
  description: 'Danh sách field bị loại khỏi request gửi đi.',
  add: 'Thêm quy tắc',
  filterProvider: 'Lọc theo nhà cung cấp',
  filterKind: 'Lọc theo loại',
  allProviders: 'Mọi nhà cung cấp',
  allKinds: 'Mọi loại',
  loading: 'Đang tải',
  loadFailedTitle: 'Không tải được bộ lọc',
  retry: 'Thử lại',
  emptyTitle: 'Chưa có quy tắc nào',
  emptyBody:
    'Blacklist field loại bỏ những field, key hoặc dòng mà nhà cung cấp từ chối khỏi request trước khi gửi đi.',
  noMatchTitle: 'Không có quy tắc khớp bộ lọc',
  noMatchBody: 'Thử đổi hoặc bỏ bộ lọc nhà cung cấp và loại.',
  clearFilters: 'Bỏ bộ lọc',
  rules: (n: number) => `${n} quy tắc`,
  enableRule: (pattern: string) => `Bật quy tắc ${pattern}`,
  editRule: (pattern: string) => `Sửa quy tắc ${pattern}`,
  deleteRule: (pattern: string) => `Xóa quy tắc ${pattern}`,
  toggleFailed: 'Không đổi được trạng thái quy tắc',
  createTitle: 'Thêm quy tắc',
  editTitle: 'Sửa quy tắc',
  formDescription:
    'Quy tắc được áp dụng cho request gửi tới nhà cung cấp đã chọn.',
  provider: 'Nhà cung cấp',
  kind: 'Loại',
  pattern: 'Mẫu',
  note: 'Ghi chú',
  enabled: 'Bật quy tắc',
  patternRequired: 'Nhập mẫu cần loại bỏ.',
  patternBadRegex: 'Biểu thức chính quy không hợp lệ.',
  noteTooLong: 'Ghi chú tối đa 200 ký tự.',
  save: 'Lưu',
  saving: 'Đang lưu',
  cancel: 'Hủy',
  saved: 'Đã lưu quy tắc',
  saveFailed: 'Không lưu được quy tắc',
  deleteTitle: 'Xóa quy tắc này?',
  deleteBody: 'Mẫu sau sẽ không còn bị loại khỏi request:',
  delete: 'Xóa',
  deleted: 'Đã xóa quy tắc',
  deleteFailed: 'Không xóa được quy tắc',
}
