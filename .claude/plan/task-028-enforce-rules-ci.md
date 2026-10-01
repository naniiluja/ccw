# Task 028 — Ép luật bằng công cụ trong CI và script cục bộ

- **Vertical slice:** CI + config + test kiến trúc
- **Depends on:** 001, 027
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/coding-conventions.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `scripts/check.sh` chạy xanh cục bộ và workflow CI chạy cùng các cổng; thử cố ý vi phạm (hàm vượt 30, tên sai initialism, import `httpapi` từ package lá) làm đúng cổng đỏ
- **discipline:** off

## Goal (one sentence)
Biến bộ luật mới thành các cổng tự động để vibe code không thể lén vi phạm.

## Acceptance criteria (verifiable)
- [ ] CI (mở rộng workflow của task-001) và `scripts/check.sh` chạy cùng danh sách: `gofmt -l .` rỗng, `go vet ./...`, `staticcheck` (kèm `staticcheck.conf`) với nhóm đặt tên (ST1003 initialism và tên, ST1005, ST1012, ST1016) và nhóm mặc định, `gocyclo -over 30`, `go test -race ./...`.
- [ ] Test kiến trúc: không package nào dưới `internal/` ngoài `httpapi` import `internal/httpapi`; mỗi package có đúng một `// Package x`; mọi file `.go` dưới 1000 dòng.
- [ ] Bước đầu đo số vi phạm ST có sẵn bằng `staticcheck -checks ...`; vi phạm nào ít thì sửa trong task này, check nào quá nhiều thì tắt riêng check đó trong `staticcheck.conf` kèm lý do và ghi vào luật (task 029).
- [ ] `scripts/check.sh` in rõ tên cổng nào đỏ.

## Test first (write before implementing)
- Test kiến trúc đỏ khi thêm một import cấm vào file thử rồi xanh khi gỡ.
- Cố ý thêm một hàm cyclomatic 31 để xác nhận `gocyclo` đỏ, rồi gỡ.

## Files to touch
- .github/workflows/github-packages.yml
- staticcheck.conf
- scripts/check.sh
- internal/httpapi/arch_test.go
- cmd/ccw/release_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Dùng `go run honnef.co/go/tools/cmd/staticcheck@<phiên bản cố định>` và `go run github.com/fzipp/gocyclo/cmd/gocyclo@<phiên bản cố định>` thay vì `@latest` để CI lặp lại được; ghi phiên bản trong `scripts/check.sh`. Đây là công cụ phát triển, không thêm vào `go.mod`.
