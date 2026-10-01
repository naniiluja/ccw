# Task 029 — Bộ luật mới và đồng bộ spec

- **Vertical slice:** spec
- **Depends on:** 028
- **Spec refs:** `.claude/rules/*`, `CLAUDE.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `wc -lc CLAUDE.md` dưới 200 dòng và 12KB; mỗi luật trong bộ mới ghi kèm cách kiểm bằng công cụ nào
- **discipline:** off

## Goal (one sentence)
Viết bộ luật đặt tên, convention và design pattern mới theo best practice Go, khớp cấu trúc sau khi tinh gọn.

## Acceptance criteria (verifiable)
- [ ] `.claude/rules/naming.md` (mới): package, hàm, getter không `Get`, initialism, hằng MixedCaps, `ErrXxx` và `...Error`, receiver, file và test; mỗi mục ghi nguồn (Google Go Style Guide, Uber Go Style Guide) và check ST tương ứng.
- [ ] `coding-conventions.md` viết lại: giới hạn đo được (cyclomatic 30, file 1000 dòng), tham số từ 5 trở lên dùng struct, `context` đầu tiên, handler mỏng, `newX()` cho state struct, không biến global thay đổi được trong code mới, interface nhỏ ở phía dùng, không tạo package trước khi cần.
- [ ] `architecture.md`: bản đồ package và file `httpapi` sau tinh gọn (khoảng 20 file), hướng phụ thuộc, 3 tầng auth, luật không route ghi thiếu `requireAdmin`.
- [ ] `testing.md`: cổng `go test -race ./...`, `scripts/check.sh`, kỷ luật test mức contract bật cho chữ ký công khai mới, golden Zen, bất biến mật khẩu mới.
- [ ] `error-handling.md`, `logging.md`, `tech-stack.md`, `tooling.md` cập nhật (đăng nhập mật khẩu, không còn contract lab, notify, ranking, switcher).
- [ ] `CLAUDE.md` mô tả đúng sản phẩm hiện tại, dưới 200 dòng và 12KB; `.claude/plan/PLAN.md` có trạng thái đúng.

## Test first (write before implementing)
- Không có test mới; gate là các lệnh đo: `wc -lc CLAUDE.md`, `go list -deps ./internal/... | grep internal/httpapi` chỉ ra `cmd/ccw`, `ls internal/httpapi/*.go | grep -vc _test`.

## Files to touch
- CLAUDE.md
- .claude/rules/naming.md
- .claude/rules/coding-conventions.md
- .claude/rules/architecture.md
- .claude/rules/testing.md
- .claude/rules/error-handling.md
- .claude/rules/logging.md
- .claude/rules/tech-stack.md
- .claude/rules/tooling.md
- .claude/plan/PLAN.md

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Spec dùng chung là hotspot nên gom vào một task cuối. Luật nào không có cách đo bằng công cụ phải ghi rõ là hướng dẫn, không phải cổng.
