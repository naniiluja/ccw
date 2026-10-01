# Task 015 — Xóa notify (cảnh báo Telegram và webhook)

- **Vertical slice:** store + httpapi
- **Depends on:** 014
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/logging.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `grep -rniE "telegram|webhook|notifyMsg|sendWebhook|alertUnsavedToken" --include='*.go' internal cmd` rỗng; test AI review và OAuth refresh xanh
- **discipline:** off

## Goal (one sentence)
Xóa kênh cảnh báo cho người vận hành (Telegram, webhook) cùng mọi lời gọi cảnh báo trong OAuth và AI review.

## Acceptance criteria (verifiable)
- [ ] `httpapi/notify.go`, `notify_test.go`, `notify_ssrf_test.go` và `store/notify.go` bị xóa; `notifySchema` không còn ở `store.go`.
- [ ] Route `/notify`, `/api/notify*` và các tool MCP tương ứng (`mcp.go:346-362`) bị gỡ.
- [ ] Lời gọi `a.notify`, `alertUnsavedToken`, `alertVerdict`, `alertBursts`, `pauseErrReview` và các hằng `Event*` bị gỡ khỏi `oauth.go`, `driftreview.go`, `errreview.go`; hành vi review và refresh còn lại giữ nguyên.
- [ ] `maskedValue` còn được `provdefs.go` dùng nên chuyển hàm đó sang `provdefs.go`, không xóa.
- [ ] Test `TestWebhookURLNeverReachesADashboardKey` (`principal_test.go`) và các test dùng `telegramAPI` (`errreview_test.go`), `notifyMsg` (`oauth_refresh_test.go`) được sửa hoặc xóa cho khớp.

## Test first (write before implementing)
- Baseline `go test -race ./...` trước khi sửa.
- Test hiện có của `errreview_test.go`, `driftreview_test.go`, `oauth_refresh_test.go` giữ nguyên ý nghĩa sau khi bỏ phần cảnh báo.

## Files to touch
- internal/httpapi/notify.go, internal/httpapi/notify_test.go, internal/httpapi/notify_ssrf_test.go — xóa
- internal/store/notify.go — xóa
- internal/store/store.go — bỏ `notifySchema`
- internal/httpapi/driftreview.go, internal/httpapi/errreview.go, internal/httpapi/oauth.go, internal/httpapi/provdefs.go, internal/httpapi/mcp.go, internal/httpapi/server.go
- internal/httpapi/errreview_test.go, internal/httpapi/oauth_refresh_test.go, internal/httpapi/principal_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Cảnh báo chỉ là thông báo, không nằm trên đường chuyển tiếp request. Khoảng 560 dòng nguồn bị xóa.
