# Task 022 — Gộp quota, drift và heal

- **Vertical slice:** httpapi + translate
- **Depends on:** 021
- **Spec refs:** `.claude/rules/coding-conventions.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./...` xanh; `quota_resets.go`, `mcp.go`, `errlog.go`, `errreview.go`, `errbisect.go` giữ riêng
- **discipline:** off

## Goal (one sentence)
Gộp nốt các file quota và drift trong `httpapi` và hai file heal trong `translate`.

## Acceptance criteria (verifiable)
- [ ] `quota.go` ← `ratelimit.go`, `quota_providers.go`; giữ phép gán `quotaFetchers[...]` trong `init()`.
- [ ] `drift.go` ← `driftreview.go`.
- [ ] `translate/heal.go` (mới) ← `heal_chat.go`, `heal_anthropic.go`; `missingToolResult` giữ làm hằng dùng chung.
- [ ] Không file nào quá 1000 dòng.

## Test first (write before implementing)
- Không có test mới; test heal và quota hiện có giữ nguyên.

## Files to touch
- internal/httpapi/quota.go, internal/httpapi/ratelimit.go, internal/httpapi/quota_providers.go
- internal/httpapi/drift.go, internal/httpapi/driftreview.go
- internal/translate/heal.go, internal/translate/heal_chat.go, internal/translate/heal_anthropic.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Chỉ hai `init()` trong cả `httpapi` (`quota_providers.go:31`, `quota_resets.go:446`) và chúng điền hai map khác nhau, nên thứ tự không ảnh hưởng. `quota.go` sau gộp khoảng 900 dòng.
