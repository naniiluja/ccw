# Error Handling

Nguồn: Go blog "Working with Errors in Go 1.13" (`%w`, `errors.Is/As`), Uber Go Style Guide (đặt tên lỗi, xử lý lỗi một lần). Cách kiểm: `[tool]` là cổng của `scripts/check.sh`, `[review]` là hướng dẫn kèm lệnh kiểm.

## Taxonomy
- Phân biệt **lỗi nghiệp vụ dự kiến** (validation, not-found, key hết hạn, vượt RPM, mật khẩu sai) với **lỗi hệ thống** (bug, upstream hỏng). `[review]`
- Lỗi cho người dùng và lỗi nội bộ có thông điệp khác nhau; không lộ chi tiết nội bộ ra ngoài, vì stack trace hay câu SQL trong phản hồi là bản đồ hệ thống cho kẻ tấn công. `writeError` không bao giờ nêu credential. `[review]`
- Lỗi upstream được relay nguyên cho caller (kèm `Request-Id` của upstream nếu có); lỗi do ccw tự sinh đi qua envelope bên dưới. `[tool]` qua test ở `v1_errors_test.go`, `failover_test.go`.

## Verifiable rules
- **Không nuốt lỗi.** Lỗi bắt được phải được log (trong `/v1` thì qua `a.logFor(ctx)` để có `req_id`) hoặc trả lên kèm ngữ cảnh. `[review]`; staticcheck chỉ bắt vài dạng (ví dụ SA4006, giá trị gán rồi không dùng), không phải cổng cho luật này.
- Bọc lỗi khi qua ranh giới bằng `fmt.Errorf("...: %w", err)`. Đo bằng grep theo dòng: 76 trên 129 dòng `fmt.Errorf(` có `%w` (`grep -rh 'fmt\.Errorf(' --include='*.go' internal cmd | grep -c '%w'`); code mới phải dùng `%w` khi bọc một lỗi. `[review]`; `go vet` (printf) chỉ chặn `%w` dùng sai.
- So sánh lỗi bằng `errors.Is`/`errors.As`, không `strings.Contains(err.Error(), ...)` trong code mới. `[review]` `grep -rn 'err.Error()' --include='*.go' internal | grep -E 'Contains|HasPrefix'`. Ngoại lệ có sẵn: `store/errors.go` (`"no rows"`), `httpapi/errlog.go` (`"deadline"`), `servertools/fetch.go` (`errPrivate`), `httpapi/quota_resets.go` (`HasPrefix "401:"`). Ngoại lệ `"duplicate column"` của migration đã hết: schema đánh số bằng `PRAGMA user_version` (`internal/store/store.go`).
- Đặt tên lỗi: `ErrXxx`/`errXxx` `[tool]` ST1012; kiểu lỗi hậu tố `Error` `[review]`; chuỗi lỗi chữ thường, không dấu chấm cuối `[tool]` ST1005. Chi tiết ở `naming.md`.
- Chỉ retry lỗi tạm thời (429, 500, 503, lỗi mạng) có backoff; không retry lỗi nghiệp vụ. Chính sách ở `retryable` (`internal/upstream/client.go`). OAuth 401 được refresh và thử lại đúng một lần (`TestFailoverRefreshesOAuthOnceOn401`). `[tool]` qua test.
- Không `panic` trong đường xử lý request (hiện không có `panic(` nào ngoài test). `log.Fatal` chỉ ở khởi động trong `cmd/ccw/main.go`. `[review]` `grep -rn 'panic(\|log\.Fatal' --include='*.go' internal`.

## Error format
- Mỗi API có envelope riêng, chọn theo path bởi `v1API` (`server.go`): `/v1/messages*` trả envelope Anthropic, route `/v1` khác trả envelope OpenAI; route dashboard và `/api/*` trả `{"error": "<message>"}`. Mọi phản hồi `/v1` mang header `Request-Id` và `X-Request-Id` dạng `req_<hex>`. `[tool]` `v1_errors_test.go`, `v1_test.go`.
- Status: giữ status HTTP của upstream khi relay; `401` khi thiếu hoặc sai credential (cả `POST /login` sai mật khẩu), `403` khi API key dashboard gọi route `requireAdmin`, `429` khi vượt RPM của key hoặc vượt ngân sách đăng nhập sai của `LoginGuard`. `[tool]` `v1_errors_test.go`, `adminroutes_test.go`, `auth_test.go`, `keylimits_test.go`.
- Đặc tả đầy đủ ở `internal/httpapi/openapi.json` (phục vụ tại `GET /openapi.json`); sửa envelope thì cập nhật file này và test tương ứng (`v1_errors_test.go`). `[review]`
