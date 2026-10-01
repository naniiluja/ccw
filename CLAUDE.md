# ccw

> Managed by **CCF (Claude Context First)**. Workflow: Explore → Plan → Implement → Commit.
> **Parallel by waves**: tasks with no declared dependency, shared file or shared hotspot run at the same time, each in its own isolated git worktree, and every wave is merged only after a preflight check and a full test run on the merged result. Everything else waits for the wave before it.
> Ground every design decision in Context7 + Microsoft Learn.
> Keep this spec always fresh with `/ccf:updatespec`.

## What this is
ccw là một credential proxy cho các nhà cung cấp LLM, viết bằng Go. Nó giữ credential của nhiều tài khoản trên một máy và phục vụ tất cả sau một base URL `/v1`. Request nêu model (`groq/llama-3.3-70b-versatile` hoặc chỉ `llama-3.3-70b-versatile`), ccw chọn provider và tài khoản, xoay vòng và failover giữa các tài khoản, dịch shape giữa OpenAI Chat Completions, Anthropic Messages, Responses và Gemini khi cần.

Kèm theo: đăng nhập dashboard bằng **mật khẩu** (`CCW_PASSWORD` tối thiểu 12 ký tự, hoặc tự sinh ở lần chạy đầu và in một lần; chỉ lưu bản băm PBKDF2; `-reset-password` để thay) có `LoginGuard` chặn đoán mò; API key với giới hạn RPM và danh sách model; máy chủ MCP tại `/mcp`; blacklist field; drift detection và AI drift review; log lỗi upstream và AI error review; theo dõi quota, claim reset và usage; web search và tool phía server; OpenCode Zen. Dashboard là SPA React ở `fe/`, build xong được nhúng vào binary và phục vụ công khai tại `/ui/` (package `webui`); `/` chuyển sang `/ui/`. Phân phối dưới dạng binary Go qua gói npm `ccw-gateway`.

Đã gỡ khỏi source (đừng tìm, đừng khôi phục): contract lab và switcher (`internal/contract`, `X-Ccw-Trace`, `contract-reset`), notify (Telegram, webhook), ranking (`CCW_ARENA_URL`), đăng nhập TOTP, cờ `trusted` của API key.

## Repo layout
Repo chia hai phần: `be/` giữ module Go (`github.com/naniiluja/ccw`, `be/go.mod`, `be/go.sum`, `be/staticcheck.conf`), 15 package, 75 file `.go` không test (khoảng 22.5k dòng); `fe/` giữ dashboard React (134 file `ts`/`tsx` không test, 13 file test), bản build được copy vào `be/internal/webui/static/` (gitignore trừ `.gitkeep`) bởi `scripts/ui-build.sh`. Git ở root; gốc giữ `CLAUDE.md`, `.claude/`, `.github/`, `scripts/`, `npm/`, `docs/`, `LICENSE`.
Các đường dẫn trong `.claude/rules/*` tính từ `be/` trừ khi có tiền tố `fe/`, `scripts/`, `npm/`, `.github/` hoặc `.claude/`.
- `cmd/ccw/`: entrypoint, flag, mật khẩu lần đầu, `http.Server` (`IdleTimeout`), test bảo vệ bản phát hành.
- `internal/httpapi/` (23 file): route, middleware auth, `/v1` và failover, proxy, rotation, `/api/*`, `/mcp`, quota, drift, errors. Bản đồ file ở `architecture.md`. Chỉ `cmd/ccw` được import nó.
- `internal/translate/`: dịch request/response giữa các shape. `internal/store/`: SQLite, mọi SQL, schema đánh số bằng `PRAGMA user_version`.
- `internal/webui/`: nhúng và phục vụ SPA. `internal/auth/` (mật khẩu, session, `LoginGuard`), `provider/`, `filter/`, `drift/`, `oauth/`, `upstream/`, `usage/`, `servertools/`, `websearch/`, `zen/`: xem `architecture.md`.
- `scripts/check.sh`: mọi cổng (gofmt, vet, staticcheck, gocyclo, `go test -race`), CI chạy y hệt; cổng Go chạy trong `be/`, cổng FE chạy khi có `fe/package.json`. `scripts/ui-build.sh`: build `fe/` rồi copy vào `be/internal/webui/static/`. `be/staticcheck.conf`: bộ check.
- `npm/ccw-gateway/`, `scripts/npm-build.sh`: đóng gói npm đa nền tảng.

## Rules (imported, chi tiết ở .claude/rules/)
> Keep this file < 200 lines AND < 12KB, whichever binds first. Check both with `wc -lc CLAUDE.md`, not by eye: a low line count can still hide a heavy single line.
> Luật trong `naming`, `coding-conventions`, `architecture`, `error-handling`, `logging`, `testing`, `tech-stack` ghi cách kiểm: `[tool]` là cổng tự động của `bash scripts/check.sh`, `[review]` là hướng dẫn kèm lệnh kiểm. `git-workflow`, `tooling`, `debugging` là quy trình. Chạy `bash scripts/check.sh` trước mỗi commit.
@.claude/rules/tech-stack.md
@.claude/rules/architecture.md
@.claude/rules/frontend.md
@.claude/rules/naming.md
@.claude/rules/coding-conventions.md
@.claude/rules/logging.md
@.claude/rules/testing.md
@.claude/rules/error-handling.md
@.claude/rules/debugging.md
@.claude/rules/tooling.md
@.claude/rules/git-workflow.md

## Current plan
Iteration tinh gọn ccw (19 task) đã `done`, lưu ở `.claude/plan/ARCHIVE.md` (task file ở `.claude/plan/archive/`; không xóa). Iteration dashboard (be và fe, task 032 đến 046) đang `in-review`: xem `.claude/plan/PLAN.md`; kế hoạch mới đi qua `/ccf:plan`, chạy bằng `/ccf:cook`.
Việc còn mở (đều là `WARN:`, chưa thành task):
- `internal/httpapi/proxy.go` bỏ qua lỗi của `copyFlushing` mà không log; comment cũ ở `internal/auth/config.go`.
- `-reset-password` chưa thu hồi session đang sống.
- Ngoài phạm vi của dashboard: sửa web search, múi giờ và đổi mật khẩu từ UI (trang Settings chỉ đọc, `GET /api/settings`); cập nhật `openapi.json` cho `/api/*`; cookie session luôn `Secure: true` (`internal/httpapi/auth.go`), nên đổi sang theo `requestIsTLS` như cookie thiết bị.
- `GET /api/settings` không bao giờ báo `authMode: "token"` (chỉ `password` hoặc `none`, `settings.go`).
- Một lỗi chập chờn của `go test -race ./internal/httpapi` chỉ thấy ở lần chạy đầu trong script tích hợp wave (chạy lại thì qua, chưa ghi được tên test); một lỗi chập chờn ở `fe/src/pages/keys/keys.test.tsx`.
- Bộ chọn loại tài khoản ở trang Accounts dựng từ primitive `RadioGroup`, chưa phải `@originui/comp-163`.
