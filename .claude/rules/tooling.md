# Tooling (available skills / MCP / subagents — WHEN TO USE)

A catalog of this project's tools, with **when to use** each, so future-session agents can decide. Updated by `/ccf:updatespec` whenever a new tool is added.

## MCP servers
- **context7** — look up current docs for libraries/frameworks. **Use when:** you need a lib's API syntax/best practice/migration. How: `resolve-library-id` → `query-docs`. Ghi chú: phiên chạy ở chế độ không tương tác có thể bị từ chối quyền gọi công cụ này; khi đó fetch tài liệu chính thức (`go.dev`, `sqlite.org`) thay thế và ghi rõ nguồn.
- **microsoft-learn** — look up Microsoft/.NET/Azure docs. **Use when:** working with a Microsoft platform. Dự án Go này thường không cần.
- **shadcn** — tra cứu và cài component từ registry shadcn/ui (khai báo ở `.mcp.json`, chạy qua `npx -y shadcn@latest mcp`). **Use when:** cần tham khảo thiết kế/markup một component shadcn để mô phỏng lại. Ghi chú: UI cũ đã gỡ, bản viết lại chưa chọn stack; chưa nên chạy lệnh `add` cho tới khi có `components.json` và quyết định stack.

## Skills
- **ccf:plan** — lên kế hoạch một tính năng thành các vertical slice. **Use when:** bắt đầu tính năng mới, ở plan mode.
- **ccf:cook** — chạy backlog theo wave song song. **Use when:** có nhiều task độc lập trong `.claude/plan/`.
- **ccf:check** — review độc lập theo spec. **Use when:** sau khi task chuyển `in-review`.
- **ccf:updatespec** — làm mới spec và memory. **Use when:** cuối phiên có bài học hoặc công cụ mới.
- **claude-api** — tham chiếu Anthropic SDK và tham số API. **Use when:** sửa code dịch Anthropic Messages, tool use, thinking blocks trong `internal/translate`.

## Subagents (CCF)
Every CCF subagent is READ-ONLY: discovery, review or best-practice grounding, never coding. Code is written either directly in the main session (a single task after /ccf:plan) or by /ccf:cook's task agents, one per task of a wave, each in its own worktree created with `isolation: "worktree"`; a writer never runs in the main checkout.
- **ccf-codebase-analyzer** — map or scope the codebase. **Use when:** onboarding (/ccf:init) or before planning a change (/ccf:plan).
- **ccf-spec-checker** — review conformance/SOLID. **Use when:** /ccf:check.
- **ccf-best-practice-researcher** — fetch best practices. **Use when:** grounding a decision.

## Lệnh thường dùng
- Build: `go build ./...`. Chạy cục bộ: `go run ./cmd/ccw -addr 127.0.0.1:20130 -db ccw.db`. Lần đầu: `-enroll` rồi `-show-totp` để lấy TOTP secret.
- Build npm đa nền tảng: `scripts/npm-build.sh` (cần `NPM_SCOPE` khi publish). Không chạy publish thủ công; release đi qua tag `v*`.

## System memory vs Spec (WHEN to write where)
- **Spec** (this file + other rules): project rules that are derivable / belong to the repo. Lower weight (user message).
- **Memory** (`~/.claude/projects/<path>/memory/`): `feedback` anti-mistakes + `user` preferences. Higher weight (system prompt) → Claude repeats fewer mistakes. Updated via `/ccf:updatespec`. **Do not duplicate** CLAUDE.md content.
- **MEMORY.md is a pure index** — only its first **200 lines or 25KB** (whichever first) load per session, so keep it lean (curate when near). The strongest tier is `feedback` (record wins + losses, with a mandatory `Why`).
