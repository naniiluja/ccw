# Task 027 — Tách các hàm còn vượt 30

- **Vertical slice:** httpapi + provider (refactor)
- **Depends on:** 023, 024, 026
- **Spec refs:** `.claude/rules/coding-conventions.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `gocyclo -over 30 -ignore '_test.go$' .` không in dòng nào
- **discipline:** off

## Goal (one sentence)
Đưa mọi hàm còn lại về cyclomatic dưới hoặc bằng 30 để luật CI của task 028 xanh ngay khi bật.

## Acceptance criteria (verifiable)
- [ ] Chạy `gocyclo -over 30` ở đầu task để lấy danh sách thật; dự kiến còn `readInfo` (35, `modelinfo.go`), `Def.Normalize` (34, `provider/def.go`), `reviewErrorGroup` (33, `errreview.go`), và bất kỳ hàm nào chưa được các task 023 đến 026 xử lý.
- [ ] Mỗi hàm tách thành bước tên rõ trong cùng file hoặc cùng package, không đổi hành vi.
- [ ] `Def.Normalize` chỉ phủ 59,6 phần trăm nên thêm test đặc tả (đầu vào hợp lệ, `verifyUrl` javascript, icon `data:`) trước khi tách.

## Test first (write before implementing)
- Test đặc tả cho `Normalize` và `reviewErrorGroup` (75 phần trăm) trước khi tách.

## Files to touch
- internal/httpapi/modelinfo.go
- internal/provider/def.go
- internal/httpapi/errreview.go
- internal/provider/def_test.go
- internal/httpapi/errreview_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Danh sách file chính xác được cập nhật vào task này khi chạy `gocyclo` lần đầu; nếu có hàm nằm ở file khác thì ghi thêm vào `Files to touch` trước khi implement.
