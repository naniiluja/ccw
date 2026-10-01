# Task 017 — Dọn code chết và lint

- **Vertical slice:** provider + zen + httpapi
- **Depends on:** 014
- **Spec refs:** `.claude/rules/coding-conventions.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `staticcheck ./...` xanh và `deadcode -test ./...` không còn mục nào
- **discipline:** off

## Goal (one sentence)
Xóa hai hàm chết thật và sửa các cảnh báo công cụ đã đo.

## Acceptance criteria (verifiable)
- [ ] `provider.Generic` (`registry.go:78`) và `zen.SystemOneNames` (`profiles.go:85`) bị xóa (đã xác nhận không có người gọi, kể cả test); `GenericAPI` giữ.
- [ ] `websearch_test.go:163` (S1021) và `zen_test.go:86` (SA4000, so sánh hai vế giống nhau) được sửa đúng ý của test.
- [ ] Hai vòng lặp được thay bằng `slices.Contains` (`proxy.go:348`, `v1.go:295`).
- [ ] Các hàm chỉ test dùng nhưng nằm trong file sản phẩm (`auth.FromEnv`) được xử lý ở task 018, không ở đây.

## Test first (write before implementing)
- `staticcheck` trước và sau; test cũ giữ nguyên.

## Files to touch
- internal/provider/registry.go
- internal/zen/profiles.go
- internal/httpapi/websearch_test.go
- internal/zen/zen_test.go
- internal/httpapi/proxy.go
- internal/httpapi/v1.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Số đo gốc: `staticcheck U1000` 0 symbol không dùng, `dupl` 0 đoạn trùng, `deadcode` 2 hàm chết thật.
