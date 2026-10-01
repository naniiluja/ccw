# Task 019 — Chuyển loginguard vào internal/auth

- **Vertical slice:** auth + httpapi
- **Depends on:** 018
- **Spec refs:** `.claude/rules/architecture.md` (package lá không import httpapi)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: test loginguard xanh ở `internal/auth`, ngưỡng đếm không đổi
- **discipline:** off

## Goal (one sentence)
Đưa bộ giới hạn đăng nhập sai (`loginguard.go`, 207 dòng, chỉ phụ thuộc stdlib) vào package `auth` có sẵn, không tạo package mới.

## Acceptance criteria (verifiable)
- [ ] `loginGuard`, `newLoginGuard`, `reserve`, `refund`, `loginSourceOf`, `clientIP` và các hằng ngưỡng nằm trong `internal/auth`, xuất khẩu tối thiểu.
- [ ] `auth_handlers.go` và `server.go` gọi qua API xuất khẩu; ngưỡng, cửa sổ thời gian và ba lane (local, device, public) giữ nguyên số.
- [ ] `loginguard_test.go` chuyển sang `internal/auth` cùng helper mở store cục bộ nếu cần.

## Test first (write before implementing)
- Ma trận BVA theo ngưỡng mỗi lane (đúng ngưỡng, ngưỡng cộng một, sau khi hết cửa sổ) chạy trước và sau khi chuyển cho cùng kết quả.

## Files to touch
- internal/httpapi/loginguard.go — chuyển sang internal/auth/loginguard.go
- internal/httpapi/loginguard_test.go — chuyển sang internal/auth/loginguard_test.go
- internal/httpapi/auth_handlers.go, internal/httpapi/server.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- `internal/auth` đã có sẵn `config.go`, `session.go` và password; không thêm package. Thêm `net/http` vào package auth là chấp nhận được vì cookie và IP nằm cạnh đăng nhập.
