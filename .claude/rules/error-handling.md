# Error Handling

## Taxonomy
- Distinguish **expected business errors** (validation, not-found, key hết hạn, vượt RPM) from **system errors** (bug, upstream hỏng).
- User-facing vs internal errors: different messages; never leak internal detail outward, since a stack trace or query string in a user-facing response hands an attacker a map of the system.
- Lỗi upstream được relay nguyên cho caller (kèm `Request-Id` của upstream nếu có), lỗi do ccw tự sinh đi qua envelope bên dưới.

## Verifiable rules
- **No silent catch.** A caught error must be logged (with the request ID) or returned with context.
- Wrap errors with context when crossing a boundary using `fmt.Errorf("...: %w", err)`; don't swallow the original cause. Hiện 115 trên 175 `fmt.Errorf` dùng `%w`, code mới phải dùng `%w`.
- So sánh lỗi bằng `errors.Is`/`errors.As`. Không dùng `strings.Contains(err.Error(), ...)` cho luồng điều khiển mới (chỗ đang có: migration `"duplicate column"` trong `internal/store`, sửa ở `task-002`).
- Retry only transient errors (429, 500, 503, timeout) with backoff; never retry business errors. Client upstream nằm ở `internal/upstream/client.go`; OAuth 401 được refresh và thử lại đúng một lần.
- Không `panic` trong đường xử lý request. `log.Fatal` chỉ dùng ở khởi động trong `cmd/ccw`.

## Error format
- Standard error type/shape: mỗi API có envelope riêng, chọn theo path bởi `v1API`. `/v1/messages*` trả envelope Anthropic; các route `/v1` khác trả envelope OpenAI; route dashboard và `/api/*` trả `{"error": "<message>"}`. Mọi phản hồi `/v1` mang header `Request-Id` và `X-Request-Id` dạng `req_<hex>`.
- Error code / HTTP status mapping: giữ status HTTP của upstream khi relay; `401` cho thiếu hoặc sai credential, `403` cho dashboard key gọi route admin, `429` khi vượt RPM của key. Đặc tả đầy đủ có trong `internal/httpapi/openapi.json`; sửa envelope thì cập nhật file này và test tương ứng trong `apierror`.
