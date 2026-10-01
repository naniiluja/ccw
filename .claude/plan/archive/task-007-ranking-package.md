# Task 007 — Chuyển ranking sang internal/ranking

- **Vertical slice:** ranking + httpapi
- **Depends on:** 004
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/testing.md` (test không gọi mạng thật)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Đưa bảng xếp hạng arena (`ranking.go`, 560 dòng) vào `internal/ranking`, giữ ba handler ở `httpapi`.

## Acceptance criteria (verifiable)
- [ ] `arenaState`, `fetchArena`, `refreshArena`, `loadArena`, `arenaFor`, `arenaMeta`, `arenaAliases`, `arenaNorm` và `arenaLoop` nằm trong `internal/ranking`; loop thành `Run(ctx context.Context)`.
- [ ] `arenaAPI` (đọc `CCW_ARENA_URL`) thành field cấu hình khi dựng, không còn biến package gán đè.
- [ ] `rankings`, `refreshRankings`, `setArenaAlias` ở lại `httpapi` làm handler mỏng; route và JSON giữ nguyên (`ArenaScore`, `ArenaModel`, `ArenaMatch`).
- [ ] `mcp.go:184` và `models.go:55` gọi qua API xuất khẩu của `ranking`.

## Test first (write before implementing)
- Ma trận cho `arenaNorm`/`matchName` (EP: tên có và không có dấu ngoặc ghi chú như `(xHigh)`, BVA: chuỗi rỗng, tên trùng alias).
- Test `refreshArena` với dataset giả qua `httptest` (không mạng thật).

## Files to touch
- `internal/httpapi/ranking.go` → `internal/ranking/ranking.go` (+ handler mỏng giữ ở `internal/httpapi/rankings_handlers.go`)
- `internal/httpapi/server.go` — field `arena` (dòng 50), `loadArena`, `arenaLoop`, 6 route
- `internal/httpapi/mcp.go`, `models.go` — gọi API mới
- `internal/httpapi/ranking_test.go`, `adminroutes_test.go` — đổi sang cấu hình

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md`. Căn cứ Context7: `go.dev/doc/modules/layout` (server: logic trong `internal/`, binary ở `cmd/`), `go.dev/doc/faq` (interface thỏa ngầm, khai báo ở phía dùng).
- Quy tắc di chuyển: `git mv` cho file đã track; commit đầu chỉ đổi `package` và import để git nhận rename, commit sau mới sửa nội dung. Chỉ stage file thuộc task, không `git add -A`, không stash, reset hay checkout.
- Package mới không import `internal/httpapi`; log bằng `log.Printf("pkg: ...")` theo tiền lệ `drift`/`zen` cho tới khi task-003 chuyển sang slog; không log secret.
- Không đổi JSON shape của endpoint và không đổi route.
- discipline: on (ma trận EP/BVA/decision-table ở chữ ký public và chạy test thật trước khi gate xanh).
- Phụ thuộc ngoài file chỉ là `writeJSON`/`writeError` trong 3 handler (task 004 đã xử lý) và `store.GetSetting/SetSetting`.
