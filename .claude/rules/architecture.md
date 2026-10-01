# Architecture

Một module Go (`github.com/naniiluja/ccw`), 15 package (`go list ./...`): `cmd/ccw` và 14 package dưới `internal/`. 75 file `.go` không test (`find internal cmd -name '*.go' -not -name '*_test.go' | wc -l`), khoảng 22.5k dòng.

## Import graph (đo bằng `grep -o 'ccw/internal/[a-z]*"' --exclude='*_test.go' -r internal cmd`)
- **Lá, không import package nội bộ nào:** `auth`, `filter`, `oauth`, `provider`, `translate`, `upstream`, `usage`, `websearch`, `webui`, `zen`.
- **Trung gian:** `store` → `provider`; `drift` → `store`; `servertools` → `websearch`.
- **Đỉnh:** `internal/httpapi` import cả 13 package nội bộ còn lại (kể cả `webui`, mount ở `/ui/`); `cmd/ccw` chỉ import `auth`, `httpapi`, `store`.
- Hướng một chiều: `cmd/ccw` → `httpapi` → (`servertools`, `drift`) → `store` → lá.

## Package (một trách nhiệm mỗi package)
- `auth`: mật khẩu (PBKDF2, `password.go`), session cookie ký HMAC (`session.go`), `Config` và master token (`config.go`), giới hạn đăng nhập sai `LoginGuard` (`loginguard.go`).
- `store`: SQLite, mọi SQL; schema đánh số bằng `PRAGMA user_version` (danh sách `migrations` trong `store.go`), mỗi domain một file (`apikey.go`, `connection.go`, `drift.go`, `errors.go`, `filter.go`, `models.go`, `oauth.go`, `pending.go`, `providerdef.go`, `settings.go`, `usage.go`).
- `provider`: định nghĩa provider bằng dữ liệu (`Def`, registry). `translate`: dịch shape OpenAI Chat, Anthropic Messages, Responses, Gemini, Zen. `filter`: blacklist field. `drift`: học shape và phát hiện thay đổi. `oauth`: refresh token. `upstream`: client gọi provider và chính sách retry. `usage`: đọc usage từ body. `servertools`: tool phía server (fetch, MCP client, tool search). `websearch`: tìm kiếm web. `zen`: OpenCode Zen.

## Bản đồ file `internal/httpapi` (23 file không test)
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
- `session.go`: `GET /api/session` (công khai, ba boolean `authRequired`, `authenticated`, `admin`, không lộ secret), `uiLoginPath`, `hasSession`. `settings.go`: `GET /api/settings` (`requireToken`), cấu hình hiệu lực chỉ đọc (`authMode` là `password` hoặc `none`, TTL session, múi giờ, web search; không bao giờ có secret).
- `mcp.go`: máy chủ MCP tại `/mcp`, `mcpAdminTools`.
- Test kiến trúc và kích thước: `arch_test.go`.

## Package `webui` và bản đồ `fe/`
- `internal/webui` (`webui.go`): `//go:embed all:static`; `Handler()` phục vụ dưới `/ui/`. Đường dẫn không có đuôi file trả `index.html` (`Cache-Control: no-cache`, để deep link như `/ui/keys` làm mới được), `assets/*` có `immutable`, chưa build thì `503 UI chưa được build`; chỉ `GET`/`HEAD`. `static/` bị gitignore trừ `.gitkeep`, do `scripts/ui-build.sh` điền. Lá, không import package nội bộ nào.
- `fe/src/api/`: client `fetch` và hook TanStack Query theo từng nhóm endpoint (`accounts`, `keys`, `quota`, `session`, `settings`...). `fe/src/app/`: router (`routes.tsx`), `require-auth.tsx`, `create-app.tsx`. `fe/src/components/ui/`: primitive shadcn (không sửa tay). `fe/src/components/app/`: khung ứng dụng (sidebar, command palette, theme). `fe/src/pages/<trang>/`: 10 trang (`overview`, `accounts`, `providers`, `keys`, `usage`, `quota`, `errors`, `drift`, `filters`, `settings`) cộng `login` và `not-found`. `fe/src/test/`: render helper, MSW server. Luật chi tiết ở `frontend.md`.

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
- UI: SPA ở `fe/`, nhúng vào binary bởi package `webui` và phục vụ tại `GET /ui/` (công khai, vì trang đăng nhập phải tải được trước khi có session). `GET /` chuyển `302` sang `/ui/`; `POST /login` và session ở `internal/httpapi/auth.go`. Đổi code `fe/` thì chạy `bash scripts/ui-build.sh` rồi build lại binary. `[review]`

## Auth tiers (không được nới lỏng)
- `requireSession`: route dashboard trình duyệt, cần session cookie; thất bại thì chuyển hướng `302` về `/ui/login` (`uiLoginPath`), hoặc `401` với client gửi `Accept: application/json`.
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
