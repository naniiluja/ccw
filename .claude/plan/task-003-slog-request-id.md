# Task 003 — slog với request ID và IdleTimeout

- **Vertical slice:** cmd (khởi tạo logger, server timeout) + httpapi (middleware, proxy, log call sites)
- **Depends on:** —
- **Spec refs:** `.claude/rules/logging.md`, `.claude/rules/error-handling.md`, `.claude/rules/architecture.md` (route `/v1` đi qua `v1API`)
- **MCP to use:** none
- **Gate (must be GREEN before the next slice):** `go test ./internal/httpapi/ ./cmd/ccw/` xanh; có test khẳng định một request `/v1` sinh ra dòng log mang đúng `req_id` bằng với header `Request-Id` trả về.

## Goal (one sentence)
Mọi dòng log phát sinh trong một request `/v1` mang `req_id` trùng với header `Request-Id` của phản hồi, và server có `IdleTimeout`.

## Acceptance criteria (verifiable)
- [ ] `v1API` đặt request ID vào `context` và mọi log trong handler `/v1` lấy logger từ context có sẵn `req_id`.
- [ ] Các `log.Printf` trong `internal/httpapi` (40 chỗ toàn repo, phần nằm ở `httpapi` là chủ yếu) chuyển sang `log/slog` với key/value; `grep -rn 'log.Printf' internal/httpapi` không còn kết quả.
- [ ] Log không chứa secret: test cố tình gửi request kèm `Authorization`/`X-Api-Key` rồi khẳng định output log không chứa giá trị đó.
- [ ] `http.Server` trong `cmd/ccw/main.go` có `IdleTimeout` (đề xuất 120s, đặt thành hằng có tên) và vẫn không đặt `WriteTimeout` toàn cục, để stream SSE dài không bị cắt.
- [ ] Wrapper `apiWriter` vẫn có `Unwrap()` để `http.NewResponseController` tìm được `Flusher`.
- [ ] `log.Fatal`/`log.Fatalf` ở `cmd/ccw` có thể giữ nguyên (chỉ chạy lúc khởi động).

## Test first (write before implementing)
- `TestV1LogLinesCarryTheRequestID`: gắn `slog` handler ghi vào buffer, gọi một route `/v1` đơn giản qua `httptest`, đọc header `Request-Id`, khẳng định buffer có dòng chứa `req_id=<đúng giá trị đó>`.
- `TestLogsNeverHoldCallerCredentials`: như trên nhưng với header `Authorization: Bearer sekret-value`, khẳng định `sekret-value` không xuất hiện trong buffer.
- `TestServerHasIdleTimeout`: đọc cấu hình server dựng bởi hàm khởi tạo (tách hàm dựng `http.Server` ra khỏi `main` nếu cần) và khẳng định `IdleTimeout > 0` và `WriteTimeout == 0`.

## Files to touch
- `internal/httpapi/apierror.go` — `v1API` đặt request ID vào context, hàm lấy logger từ context.
- `internal/httpapi/v1.go`, `proxy.go`, `server.go`, `notify.go` và các file khác còn `log.Printf` — chuyển sang `slog`.
- `cmd/ccw/main.go` — khởi tạo logger mặc định (đề xuất `slog.NewJSONHandler` ra stderr) và thêm `IdleTimeout`.
- `internal/httpapi/*_test.go`, `cmd/ccw/main_test.go` — ba test trên.

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- `log/slog`: https://go.dev/blog/slog (handler JSON cho production, `InfoContext`/`ErrorContext` hoặc `logger.With("req_id", id)`). Redact secret bằng `slog.LogValuer` hoặc `ReplaceAttr`; phần redact chưa được kiểm chứng bằng tài liệu, tự xác nhận khi làm.
- Timeout cho stream: chỉ đặt `ReadHeaderTimeout` và `IdleTimeout`, không đặt `WriteTimeout` toàn cục; repo đã dùng `http.NewResponseController` ở `shapes.go`, `proxy.go`, `echo.go` và `ReadHeaderTimeout: 10s` ở `cmd/ccw/main.go:106`. Nguồn: https://pkg.go.dev/net/http (phần `ResponseController`), phần trường timeout của `Server` chưa được fetch.
- Không chạm `internal/store/` để không xung đột với task 002; `store` hiện không gọi `log`.
- Lưu ý tách nhỏ commit nếu diff lớn: đổi tên logger ở 40 chỗ là cơ học, đổi hành vi (`IdleTimeout`, request ID trong context) là phần cần review kỹ.
