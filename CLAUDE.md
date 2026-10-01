# ccw

> Managed by **CCF (Claude Context First)**. Workflow: Explore → Plan → Implement → Commit.
> **Parallel by waves**: tasks with no declared dependency, shared file or shared hotspot run at the same time, each in its own isolated git worktree, and every wave is merged only after a preflight check and a full test run on the merged result. Everything else waits for the wave before it.
> Ground every design decision in Context7 + Microsoft Learn.
> Keep this spec always fresh with `/ccf:updatespec`.

## What this is
ccw là một credential proxy cho các nhà cung cấp LLM, viết bằng Go. Nó giữ credential của nhiều tài khoản trên một máy và phục vụ tất cả sau một base URL `/v1`. Request nêu model (`groq/llama-3.3-70b-versatile` hoặc chỉ `llama-3.3-70b-versatile`), ccw chọn provider và tài khoản, xoay vòng và failover giữa các tài khoản, dịch shape giữa OpenAI Chat Completions, Anthropic Messages, Responses và Gemini khi cần.

Kèm theo: đăng nhập dashboard bằng **mật khẩu** (`CCW_PASSWORD` tối thiểu 12 ký tự, hoặc tự sinh ở lần chạy đầu và in một lần; chỉ lưu bản băm PBKDF2; `-reset-password` để thay) có `LoginGuard` chặn đoán mò; API key với giới hạn RPM và danh sách model; máy chủ MCP tại `/mcp`; blacklist field; drift detection và AI drift review; log lỗi upstream và AI error review; theo dõi quota, claim reset và usage; web search và tool phía server; OpenCode Zen. UI dashboard đã gỡ, đang viết lại (API vẫn còn). Phân phối dưới dạng binary Go qua gói npm `ccw-gateway`.

Đã gỡ khỏi source (đừng tìm, đừng khôi phục): contract lab và switcher (`internal/contract`, `X-Ccw-Trace`, `contract-reset`), notify (Telegram, webhook), ranking (`CCW_ARENA_URL`), đăng nhập TOTP, cờ `trusted` của API key.

## Repo layout
Một module Go (`github.com/naniiluja/ccw`), 14 package, 72 file `.go` không test (khoảng 22.3k dòng). Git ở root.
- `cmd/ccw/`: entrypoint, flag, mật khẩu lần đầu, `http.Server` (`IdleTimeout`), test bảo vệ bản phát hành.
- `internal/httpapi/` (21 file): route, middleware auth, `/v1` và failover, proxy, rotation, `/api/*`, `/mcp`, quota, drift, errors. Bản đồ file ở `architecture.md`. Chỉ `cmd/ccw` được import nó.
- `internal/translate/`: dịch request/response giữa các shape. `internal/store/`: SQLite, mọi SQL, schema đánh số bằng `PRAGMA user_version`.
- `internal/auth/` (mật khẩu, session, `LoginGuard`), `provider/`, `filter/`, `drift/`, `oauth/`, `upstream/`, `usage/`, `servertools/`, `websearch/`, `zen/`: xem `architecture.md`.
- `scripts/check.sh`: mọi cổng (gofmt, vet, staticcheck, gocyclo, `go test -race`), CI chạy y hệt. `staticcheck.conf`: bộ check.
- `npm/ccw-gateway/`, `scripts/npm-build.sh`: đóng gói npm đa nền tảng.

## Rules (imported, chi tiết ở .claude/rules/)
> Keep this file < 200 lines AND < 12KB, whichever binds first. Check both with `wc -lc CLAUDE.md`, not by eye: a low line count can still hide a heavy single line.
> Luật trong `naming`, `coding-conventions`, `architecture`, `error-handling`, `logging`, `testing`, `tech-stack` ghi cách kiểm: `[tool]` là cổng tự động của `bash scripts/check.sh`, `[review]` là hướng dẫn kèm lệnh kiểm. `git-workflow`, `tooling`, `debugging` là quy trình. Chạy `bash scripts/check.sh` trước mỗi commit.
@.claude/rules/tech-stack.md
@.claude/rules/architecture.md
@.claude/rules/naming.md
@.claude/rules/coding-conventions.md
@.claude/rules/logging.md
@.claude/rules/testing.md
@.claude/rules/error-handling.md
@.claude/rules/debugging.md
@.claude/rules/tooling.md
@.claude/rules/git-workflow.md

## Current plan
Không có iteration nào đang chạy: `.claude/plan/PLAN.md` trống và iteration tinh gọn ccw (19 task, 001 và 014 đến 031) đã `done`, lưu ở `.claude/plan/ARCHIVE.md` (task file ở `.claude/plan/archive/`). Việc còn mở sau `/ccf:check` (đều là `WARN:`, chưa thành task): `internal/httpapi/proxy.go` bỏ qua lỗi của `copyFlushing` mà không log; `-reset-password` chưa thu hồi session đang sống; comment cũ ở `internal/auth/config.go`. Kế hoạch mới đi qua `/ccf:plan`, chạy bằng `/ccf:cook`. Archive iteration đã đóng, không xóa.
