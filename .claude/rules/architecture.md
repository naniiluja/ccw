# Architecture

Một module Go (`github.com/naniiluja/ccw`), 14 package (`go list ./...`): `cmd/ccw` và 13 package dưới `internal/`. 72 file `.go` không test (`find internal cmd -name '*.go' -not -name '*_test.go' | wc -l`), khoảng 22.3k dòng.

## Import graph (đo bằng `grep -o 'ccw/internal/[a-z]*"' --exclude='*_test.go' -r internal cmd`)
- **Lá, không import package nội bộ nào:** `auth`, `filter`, `oauth`, `provider`, `translate`, `upstream`, `usage`, `websearch`, `zen`.
- **Trung gian:** `store` → `provider`; `drift` → `store`; `servertools` → `websearch`.
- **Đỉnh:** `internal/httpapi` import cả 12 package nội bộ còn lại; `cmd/ccw` chỉ import `auth`, `httpapi`, `store`.
- Hướng một chiều: `cmd/ccw` → `httpapi` → (`servertools`, `drift`) → `store` → lá.

## Package (một trách nhiệm mỗi package)
- `auth`: mật khẩu (PBKDF2, `password.go`), session cookie ký HMAC (`session.go`), `Config` và master token (`config.go`), giới hạn đăng nhập sai `LoginGuard` (`loginguard.go`).
- `store`: SQLite, mọi SQL; schema đánh số bằng `PRAGMA user_version` (danh sách `migrations` trong `store.go`), mỗi domain một file (`apikey.go`, `connection.go`, `drift.go`, `errors.go`, `filter.go`, `models.go`, `oauth.go`, `pending.go`, `providerdef.go`, `settings.go`, `usage.go`).
- `provider`: định nghĩa provider bằng dữ liệu (`Def`, registry). `translate`: dịch shape OpenAI Chat, Anthropic Messages, Responses, Gemini, Zen. `filter`: blacklist field. `drift`: học shape và phát hiện thay đổi. `oauth`: refresh token. `upstream`: client gọi provider và chính sách retry. `usage`: đọc usage từ body. `servertools`: tool phía server (fetch, MCP client, tool search). `websearch`: tìm kiếm web. `zen`: OpenCode Zen.

## Bản đồ file `internal/httpapi` (21 file không test)
Gộp theo khái niệm ở task 020 đến 022. Đếm: `find internal/httpapi -maxdepth 1 -name '*.go' -not -name '*_test.go' | wc -l`.
- `server.go`: kiểu `api`, `newServerWithLog`, `registerRoutes` (mọi route), middleware `requireSession`/`requireToken`/`requireAdmin`/`v1API`, error envelope (`writeError`, `writeAPIError`), `openapi.json` (`go:embed`), `readBody`, `writeJSON`.
- `auth.go`: `POST /login`, `/logout`, cookie thiết bị, `guardRequest` (chặn ghi cross-site), `principal`.
- `v1.go`: handler `/v1`, failover giữa các tài khoản, `send` (lời gọi upstream có log).
- `proxy.go`: dựng request ra (`newOutbound`), relay và stream, dịch shape khi lệch, ghi usage.
- `rotation.go`: xoay vòng tài khoản, `attemptsFor`. `rewrite.go`: echo model và đổi tên model trong phản hồi.
- `models.go`: bảng model, test model và tài khoản, autotest. `modelinfo.go`: thông tin model và biến thể theo provider.
- `providers.go`: phần riêng của Antigravity, Copilot, Zen. `websearch.go`: tìm kiếm web và tool phía server.
- `accounts.go`: tài khoản, provider defs, dashboard API. `keys.go`: API key, giới hạn RPM, múi giờ (`CCW_TZ`).
- `filters.go`: blacklist field, từ chối bộ lọc không an toàn (`unsafeFilter`), `GET /api/providers`. `oauth.go`: refresh OAuth và luồng đăng nhập provider (start, finish, device poll).
- `quota.go`: quota theo provider, header rate limit, bảng `quotaFetchers`. `quota_resets.go`: claim reset quota.
- `drift.go`: API drift và AI drift review. `errlog.go`: bảng `upstream_errors` và API `/errors`. `errreview.go`: AI error review. `errbisect.go`: chia đôi system prompt để tìm đoạn bị provider từ chối.
- `mcp.go`: máy chủ MCP tại `/mcp`, `mcpAdminTools`.
- Test kiến trúc và kích thước: `arch_test.go`.

## Design patterns
- Handler là method của struct `api` (giữ `store`, cache in-memory, cursor xoay vòng, logger). Middleware là hàm bọc `http.HandlerFunc`.
- Provider khai báo bằng dữ liệu (`provider.Def`), code riêng chỉ ở chỗ shape khác biệt (`providers.go`, `modelinfo.go`, `quota.go`).
- Background loop khởi động trong `newServerWithLog`: `autoTestLoop`, `reviewLoop`, `errorReviewLoop`.

## Where things go
- Route mới: đăng ký trong `registerRoutes` (`server.go`), handler đặt trong file cùng khái niệm ở bản đồ trên. `[review]`
- Route dưới `/v1` phải bọc `v1API` để có error envelope theo shape của caller, header `Request-Id` và logger mang `req_id`. `[review]` đọc `registerRoutes`; hành vi envelope có test ở `v1_errors_test.go`.
- SQL mới: chỉ trong `internal/store/`, đúng file domain; đổi schema là thêm một phần tử vào cuối `migrations`, không sửa phần tử cũ. `[review]` `grep -rln 'database/sql' --include='*.go' internal cmd | grep -v _test.go | grep -v '^internal/store/'` phải rỗng; `[tool]` `migrate_test.go` (`TestOpenUpgradesAnOldDatabase`, `TestFailedMigrationKeepsTheOldVersion`).
- Chuyển đổi shape: `internal/translate/`. Caller và provider cùng shape thì body đi qua nguyên vẹn. `[review]`
- Tool phía server: `internal/servertools/`, `internal/websearch/`. `[review]`
- UI: đã gỡ, đang viết lại. `GET /` và `GET /login` chưa có route; `POST /login` và session ở `internal/httpapi/auth.go`.

## Auth tiers (không được nới lỏng)
- `requireSession`: route dashboard trình duyệt, cần session cookie; thất bại thì chuyển hướng `302` về `/login`.
- `requireToken`: `/v1` và đọc `/api/*`; nhận master token (`CCW_API_TOKEN`), API key trong DB hoặc session; áp giới hạn RPM theo key.
- `requireAdmin`: mọi thao tác ghi qua `/api/*`; chỉ session hoặc master token, API key của dashboard bị từ chối `403`. Tool MCP ghi cấu hình nằm trong `mcpAdminTools` (`mcp.go`). Lưu ý route errors review đăng ký qua biến `read`/`write` trong vòng lặp (`write` = `requireAdmin` khi có tiền tố `/api`), nên grep `requireAdmin` không thấy hết.

## Verifiable rules
- Không vòng import. `[tool]` `TestPackagesHaveNoImportCycles` và `go build ./...`.
- Chỉ `cmd/ccw` được import `internal/httpapi` (chặt hơn "lá không import httpapi"). `[tool]` `TestOnlyCmdImportsHTTPAPI`.
- `store` không import `httpapi`, `translate`, `drift`. `[tool]` `TestStoreDoesNotImportHigherLayers`.
- Package lá ở trên không import package nội bộ nào. `[review]` lệnh grep ở đầu file; chưa có test ép.
- Mỗi package một trách nhiệm, đúng một `// Package x`. `[tool]` cho comment (`TestEachPackageHasExactlyOnePackageComment`), `[review]` cho trách nhiệm.
- Không thêm route ghi cấu hình dưới `/api` mà thiếu `requireAdmin`. `[tool]` một phần: `internal/httpapi/adminroutes_test.go`; `[review]` đọc `registerRoutes`.
- Tất cả test kiến trúc nằm ở `internal/httpapi/arch_test.go`, chạy trong `go test -race ./...` của `scripts/check.sh`.
