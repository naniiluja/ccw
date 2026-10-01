# Logging (optimized for AI tracing)

Goal: log structured enough that Claude can grep the **full trace** of a request using only the correlation ID.

## Hiện trạng (đã đo bằng grep)
- Chỉ dùng `log` stdlib: 40 chỗ `log.Printf`, 6 chỗ `log.Fatal`/`log.Fatalf` (ở `cmd/ccw`). Không có `log/slog`, không có JSON.
- Mỗi lời gọi `/v1` đã có request ID dạng `req_<hex>` (`v1API` trong `internal/httpapi/apierror.go`), gắn vào header `Request-Id` và `X-Request-Id`, nhưng ID này **chưa vào dòng log nào**. Chỉ 3 dòng `log.Printf` nhắc tới request.
- Lệch so với best practice: `go.dev/blog/slog` khuyên `log/slog` có cấu trúc, mang request ID theo context. Việc chuyển đổi nằm ở `task-031`.

## Required (áp dụng cho code mới)
- Code mới ghi log bằng `log/slog` với key/value, không nối chuỗi thủ công.
- Gắn request ID vào mọi dòng log của cùng một request; sinh ở `v1API`, truyền qua `context`.
- Each **cross-boundary call** (gọi upstream, chạm DB chậm) logs both entry and exit with the request ID + timing.
- Event-name prefix: `<miền>.<hành động>` (ví dụ `proxy.upstream.retry`, `oauth.refresh.fail`). Code cũ dùng `log.Printf("models %s: %v", ...)` giữ nguyên tới khi `task-031` chuyển.

## Log levels
- `error`: needs attention. `warn`: abnormal but handled. `info`: business milestones. `debug`: development detail.
- Never log secrets/PII: logs replicate to systems with weaker access control than the source data, so a secret written once is exposed everywhere the log travels. Cụ thể không log `connections.secret`, `refresh_token`, `client_secret`, `api_keys.key`, `notify_channels.config`, giá trị `CCW_*_SECRET` hay header `Authorization`/`X-Api-Key`. Comment tại `internal/store/connection.go` đã cảnh báo điều này.
- Body request/response của người dùng không đi vào log; nơi cần lưu (upstream error, contract trace) đã có bảng riêng với giới hạn 64 KB hoặc 4 MiB.

## Tooling
- Logging library: `log` stdlib hiện tại; đích đến là `log/slog`.
- Where logs go: stderr của tiến trình (không có file log, không có sink ngoài). Thông báo cho người vận hành đi qua kênh alert (Telegram, webhook) do `internal/store/notify.go` cấu hình, không qua log.
