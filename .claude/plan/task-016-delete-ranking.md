# Task 016 — Xóa ranking (bảng xếp hạng model)

- **Vertical slice:** httpapi
- **Depends on:** 015
- **Spec refs:** `.claude/rules/architecture.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `grep -rniE "arena|ranking" --include='*.go' internal cmd` rỗng; `/v1/models` vẫn trả đủ trường cũ trừ `arena`
- **discipline:** off

## Goal (one sentence)
Xóa bảng xếp hạng model (dữ liệu arena) vốn chỉ phục vụ dashboard đã gỡ.

## Acceptance criteria (verifiable)
- [ ] `httpapi/ranking.go` và `ranking_test.go` bị xóa.
- [ ] Route `/rankings`, `/api/rankings*` bị gỡ; trường `arena`, `arenaMeta` trong `models.go` và `mcp.go` bị gỡ; `loadArena`, `arenaLoop` và field `arena` của `api` bị gỡ.
- [ ] Biến môi trường `CCW_ARENA_URL` không còn được nhắc.
- [ ] `adminroutes_test.go` không còn gán `arenaAPI`.

## Test first (write before implementing)
- Baseline test trước khi sửa; `TestAdminAPIRoutesRefuseADashboardKey` phải xanh.

## Files to touch
- internal/httpapi/ranking.go, internal/httpapi/ranking_test.go — xóa
- internal/httpapi/models.go, internal/httpapi/mcp.go, internal/httpapi/server.go, internal/httpapi/adminroutes_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Khoảng 560 dòng nguồn bị xóa.
