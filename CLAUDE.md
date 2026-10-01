# ccw

> Managed by **CCF (Claude Context First)**. Workflow: Explore → Plan → Implement → Commit.
> **Parallel by waves**: tasks with no declared dependency, shared file or shared hotspot run at the same time, each in its own isolated git worktree, and every wave is merged only after a preflight check and a full test run on the merged result. Everything else waits for the wave before it.
> Ground every design decision in Context7 + Microsoft Learn.
> Keep this spec always fresh with `/ccf:updatespec`.

## What this is
ccw là một credential proxy cho các nhà cung cấp LLM, viết bằng Go. Nó giữ credential của nhiều tài khoản trên một máy và phục vụ tất cả sau một base URL `/v1`. Request nêu model (`groq/llama-3.3-70b-versatile` hoặc chỉ `llama-3.3-70b-versatile`), ccw chọn provider và tài khoản, xoay vòng và failover giữa các tài khoản, dịch shape giữa OpenAI Chat Completions và Anthropic Messages khi cần. Phiên đăng nhập TOTP cho API dashboard (UI đã được gỡ, đang viết lại), máy chủ MCP tại `/mcp`, blacklist field, drift detection, contract lab, theo dõi quota và usage. Phân phối dưới dạng binary Go qua gói npm `ccw-gateway`.

## Repo layout
Một module Go duy nhất (`github.com/naniiluja/ccw`), không phải monorepo `be/` + `fe/`. Git init ở root.
- `cmd/ccw/` — entrypoint, flag, subcommand `contract-reset`, test bảo vệ bản phát hành công khai.
- `internal/httpapi/` — router, `/v1`, `/api/*`, dashboard API, `/mcp`, proxy, rotation. Package lớn nhất.
- `internal/translate/` — dịch request/response giữa các API shape.
- `internal/store/` — SQLite, mọi truy cập DB đi qua đây.
- `internal/provider/`, `filter/`, `drift/`, `contract/`, `oauth/`, `upstream/`, `usage/`, `auth/`, `servertools/`, `websearch/`, `zen/` — xem `architecture.md`.
- `npm/ccw-gateway/`, `scripts/npm-build.sh` — đóng gói npm đa nền tảng.

## Rules (imported — detail lives in .claude/rules/)
> Keep this file < 200 lines AND < 12KB, whichever binds first. Check both with `wc -lc CLAUDE.md`, not by eye: a low line count can still hide a heavy single line.
@.claude/rules/tech-stack.md
@.claude/rules/architecture.md
@.claude/rules/coding-conventions.md
@.claude/rules/logging.md
@.claude/rules/testing.md
@.claude/rules/error-handling.md
@.claude/rules/debugging.md
@.claude/rules/tooling.md
@.claude/rules/git-workflow.md

## Current plan
See `.claude/plan/PLAN.md` for the backlog of the CURRENT iteration. Run it with `/ccf:cook`, which executes independent tasks in parallel waves (one worktree-isolated agent per task) and starts a task only after every task in its `Depends on` is merged + tested; a single task can also be implemented directly in the session. Closed iterations move to `.claude/plan/ARCHIVE.md` (task files to `.claude/plan/archive/`), so keep this section about the work in flight, not a running history. Archive it, never delete it.
