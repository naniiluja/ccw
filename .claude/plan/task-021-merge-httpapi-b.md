# Task 021 — Gộp file httpapi nhóm B (model, proxy, provider adapter, web search)

- **Vertical slice:** httpapi
- **Depends on:** 020
- **Spec refs:** `.claude/rules/coding-conventions.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./...` xanh; không file nào quá 1000 dòng
- **discipline:** off

## Goal (one sentence)
Gộp 15 file của đường chuyển tiếp và model thành 7 file theo khái niệm.

## Acceptance criteria (verifiable)
- [ ] `models.go` ← `autotest.go`, `typesafe.go`.
- [ ] `modelinfo.go` ← `variants.go` (trừ `attemptsFor`, `attempt`).
- [ ] `rotation.go` ← `attemptsFor`, `attempt` (từ `variants.go`).
- [ ] `proxy.go` ← `shapes.go`.
- [ ] `rewrite.go` (mới) ← `echo.go`, `model_rewrite.go`.
- [ ] `providers.go` (mới) ← `antigravity.go`, `copilot.go`, `zen.go`.
- [ ] `websearch.go` ← `servertools.go`.
- [ ] `v1.go` giữ riêng, không nhận thêm dòng nào; không đổi thân hàm.

## Test first (write before implementing)
- Không có test mới; toàn bộ test hiện có chạy nguyên vẹn, golden Zen không đổi.

## Files to touch
- internal/httpapi/models.go, internal/httpapi/autotest.go, internal/httpapi/typesafe.go
- internal/httpapi/modelinfo.go, internal/httpapi/variants.go, internal/httpapi/rotation.go
- internal/httpapi/proxy.go, internal/httpapi/shapes.go
- internal/httpapi/rewrite.go, internal/httpapi/echo.go, internal/httpapi/model_rewrite.go
- internal/httpapi/providers.go, internal/httpapi/antigravity.go, internal/httpapi/copilot.go, internal/httpapi/zen.go
- internal/httpapi/websearch.go, internal/httpapi/servertools.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- `copilotResponsesModels` (`shapes.go:48`) và `droppedTools` (`servertools.go:18`) là biến package, đi theo file chứa chúng. Giữ nguyên thứ tự hàm trong mỗi file gốc để diff dễ đọc.
