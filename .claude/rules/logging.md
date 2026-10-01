# Logging (optimized for AI tracing)

Mục tiêu: log đủ cấu trúc để Claude grep **toàn bộ dấu vết** của một request chỉ bằng correlation ID. Nguồn: `go.dev/blog/slog`, tài liệu `log/slog`. Cách kiểm: `[tool]` là cổng tự động, `[review]` là hướng dẫn kèm lệnh kiểm.

## Hiện trạng (đo bằng grep, sau task 031)
- Toàn bộ log dùng `log/slog`. `cmd/ccw/main.go` đặt `slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))`, nên mỗi dòng là một object JSON trên stderr; 4 lời gọi `log.Fatal`/`log.Fatalf` còn lại (chỉ ở `main.go`, lúc khởi động) cũng đi qua handler này. Không còn `log.Printf` nào trong code không test (`grep -rn 'log\.Printf' --include='*.go' internal cmd | grep -v _test` rỗng).
- `v1API` (`internal/httpapi/server.go`) sinh ID `req_<hex>`, đặt vào header `Request-Id`/`X-Request-Id` và gắn logger `With("req_id", id)` vào context. Handler lấy logger bằng `a.logFor(ctx)`.
- **Có `req_id`:** mọi dòng log trong một lời gọi `/v1` đi qua `a.logFor(ctx)`. Lời gọi upstream `(*api).send` (`v1.go`) log cả vào và ra: `proxy.upstream.start`, rồi `proxy.upstream.done` (status, `duration_ms`) hoặc `proxy.upstream.fail`.
- **Chưa có `req_id`:** route `/api/*`, dashboard và `/mcp` không đi qua `v1API`, nên `a.logFor` rơi về `a.logger()` không có ID; vòng nền (`reviewLoop`, `errorReviewLoop`, `autoTestLoop`) dùng `a.logger()`; `internal/drift/observer.go` và `internal/zen/systemone.go` log qua `slog` mặc định của package.
- **Lời gọi qua ranh giới chưa có log vào/ra kèm thời gian:** `(*api).getModels` (`v1.go`), `oauth.Refresh` (người gọi chỉ log khi lỗi: `oauth.refresh.fail`, `oauth.refresh.force.fail`), `Search` của websearch (người gọi chỉ log `websearch.search.fail`), `fetchJSON` của quota và claim (`quota.go`; claim có `quota.claim.send`/`quota.claim.outcome` ở tầng handler). Thêm log vào/ra cho chúng là việc còn lại.

## Required (áp cho code mới)
- Log bằng `log/slog` với cặp key/value, không nối chuỗi, không `log.Printf`. `[review]` grep ở trên phải rỗng.
- Trong request, lấy logger từ context (`a.logFor(ctx)`) để dòng log mang `req_id`; không gọi `slog.Info` trực tiếp trong handler. `[review]` `grep -rn 'slog\.\(Info\|Warn\|Error\|Debug\)(' --include='*.go' internal/httpapi`.
- Mỗi **lời gọi qua ranh giới** (upstream, DB chậm) log cả vào và ra kèm `duration_ms`, theo mẫu `send`. `[review]`
- Tên sự kiện dạng `<miền>.<hành động>[.<kết quả>]`, chữ thường: `proxy.upstream.start`, `oauth.refresh.fail`, `quota.claim.outcome`. `[review]` `grep -rhoE '\.(Info|Warn|Error|Debug)\("[^"]*"' --include='*.go' internal`.

## Log levels
- `Error`: cần người can thiệp. `Warn`: bất thường nhưng đã xử lý. `Info`: mốc nghiệp vụ. `Debug`: chi tiết phát triển (handler mặc định không in). `[review]`

## Secret và PII
- Không bao giờ log secret: log được sao chép sang nơi kiểm soát truy cập yếu hơn dữ liệu gốc, nên secret ghi một lần là lộ ở mọi nơi log đi tới. Cụ thể không log `connections.secret`, `refresh_token`, `client_secret`, `api_keys.key`, mật khẩu đăng nhập hay bản băm `password_hash`, `CCW_PASSWORD`, `CCW_API_TOKEN`, `CCW_SESSION_KEY`, `CCW_SEARCH_KEY`, `CCW_ANTIGRAVITY_CLIENT_SECRET`, header `Authorization`/`X-Api-Key`/`Cookie`. `[tool]` một phần: `TestAnEnvironmentPasswordIsNeitherLoggedNorStored`, `TestFirstStartPrintsThePasswordOnceAndStoresOnlyAHash` (`cmd/ccw/main_test.go`); phần còn lại `[review]`.
- **Ngoại lệ duy nhất:** mật khẩu tự sinh ở lần chạy đầu (hoặc `-reset-password`) được in **một lần** ra stderr dạng văn bản thường bằng `fmt.Fprint` (`passwordNotice` trong `cmd/ccw/main.go`), không qua `slog`. Chỉ bản băm được lưu.
- Body request/response của người dùng không vào log (ví dụ `websearch.search.fail` chỉ log độ dài câu truy vấn). Nơi cần lưu body đã có bảng riêng có giới hạn: `upstream_errors` (64 KiB mỗi chiều, 8 MiB request đầy đủ mỗi nhóm để replay, giữ 14 ngày, tối đa 20000 dòng; `internal/httpapi/errlog.go`). `[review]`

## Tooling
- Thư viện: `log/slog` (stdlib), handler JSON.
- Đích: stderr của tiến trình. Không có file log, không có sink ngoài, không có kênh cảnh báo (notify Telegram và webhook đã gỡ ở task 015).
