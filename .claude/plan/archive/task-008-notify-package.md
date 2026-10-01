# Task 008 — Chuyển notify sang internal/notify

- **Vertical slice:** notify + httpapi
- **Depends on:** 004
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/logging.md` (không log secret), `.claude/rules/error-handling.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Đưa kênh alert, SSRF guard và throttle (`notify.go`, 472 dòng) vào `internal/notify`, giữ các handler ở `httpapi` để MCP vẫn gọi qua `callHandler`.

## Acceptance criteria (verifiable)
- [ ] `Notifier`, `Msg` (trước là `notifyMsg`), các hằng `Event*`, `sendTelegram`, `sendWebhook`, `blockedHost`, `guardedTransport`, `maskSecret` nằm trong `internal/notify`; `notify` chỉ import `store`.
- [ ] `telegramAPI`, `dialGuard`, `webhookClient` thành field của `Notifier`, không còn biến package gán đè.
- [ ] `notifyInfo`, `putChannel`, `deleteChannel`, `testChannel` ở lại `httpapi`; `mcp.go:346-362` không đổi cách gọi.
- [ ] `oauth.go:145,193`, `driftreview.go:354,439`, `errreview.go:608,611,677,721` gọi `Notifier` xuất khẩu; `maskedValue` còn được `provdefs.go:56,62` dùng nên giữ nguyên chỗ dùng chung (xuất từ `notify` hoặc để ở `httpapi`, chọn một).
- [ ] Không có thay đổi nào làm lộ `notify_channels.config` ra log.

## Test first (write before implementing)
- Ma trận SSRF (EP: IP công khai, loopback, link-local `169.254.169.254`, IPv6, tên miền trỏ vào IP riêng; BVA: biên các dải CIDR) trước và sau khi chuyển.
- Decision table throttle: cùng key trong và ngoài cửa sổ, kênh bật và tắt.

## Files to touch
- `internal/httpapi/notify.go` → `internal/notify/notify.go` (+ handler giữ ở `internal/httpapi/notify_handlers.go`)
- `internal/httpapi/driftreview.go`, `errreview.go`, `oauth.go`, `provdefs.go` — gọi API mới
- `internal/httpapi/server.go` — field `notes` (dòng 54), 5 route
- `internal/httpapi/notify_test.go`, `notify_ssrf_test.go`, `errreview_test.go`, `oauth_refresh_test.go`, `principal_test.go`

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
- Trùng file với task-003 (`notify.go` có `log.Printf`); `/ccf:cook` xếp khác wave hoặc rebase. Review và oauth import `notify` trực tiếp (nó là lá), không cần interface.
