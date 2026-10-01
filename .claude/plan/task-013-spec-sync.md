# Task 013 — Đồng bộ spec sau khi tách backend

- **Vertical slice:** spec
- **Depends on:** 012
- **Spec refs:** `.claude/rules/*`, `CLAUDE.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `wc -lc CLAUDE.md` (dưới 200 dòng và 12KB), `go list -deps ./internal/... | grep internal/httpapi` chỉ ra `cmd/ccw`, `go build ./...` và `go test ./...` vẫn xanh
- **discipline:** on

## Goal (one sentence)
Cập nhật `CLAUDE.md` và `.claude/rules/*` cho khớp cấu trúc mới và kiểm tra bằng số đo.

## Acceptance criteria (verifiable)
- [ ] `CLAUDE.md` mô tả layout mới, dưới 200 dòng và dưới 12KB (`wc -lc CLAUDE.md`).
- [ ] `architecture.md` có đồ thị import mới, hướng phụ thuộc mới và danh sách package lá cập nhật.
- [ ] `testing.md` ghi cổng `go test -race ./...` và quy ước helper test cục bộ mỗi package.
- [ ] `tech-stack.md`, `tooling.md` không còn nhắc cấu trúc cũ; `.claude/plan/PLAN.md` và các task file có trạng thái đúng.

## Test first (write before implementing)
- Không có test mới; gate là các lệnh đo: `wc -lc CLAUDE.md`, `go list -deps ./internal/... | grep internal/httpapi`, `wc -l internal/httpapi/*.go`.

## Files to touch
- `CLAUDE.md`
- `.claude/rules/architecture.md`, `tech-stack.md`, `testing.md`, `tooling.md`
- `.claude/plan/PLAN.md`

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
- Spec dùng chung là hotspot, nên gom vào một task cuối để các task trước chạy được mà không tranh file.
