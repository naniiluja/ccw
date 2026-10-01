# Task 006 — Chuyển loginguard sang internal/auth

- **Vertical slice:** auth + httpapi
- **Depends on:** 004
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/error-handling.md`, `.claude/rules/testing.md` (`TestGodocDoesNotClaimAPassword`)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Đưa bộ giới hạn thử đăng nhập (`loginguard.go`, 207 dòng, chỉ phụ thuộc stdlib) vào `internal/auth`, giữ handler đăng nhập ở `httpapi`.

## Acceptance criteria (verifiable)
- [ ] `loginGuard`, `newLoginGuard`, `reserve`, `refund`, `loginSourceOf`, `clientIP`, các hằng ngưỡng và `deviceCookie` nằm trong `internal/auth`, xuất khẩu tối thiểu (constructor, `Reserve`, `Refund`, `SourceOf`, accessor cho test).
- [ ] `ownerLoopback` có getter hoặc option khi dựng guard; `api.login` vẫn là field của `api` nhưng kiểu thuộc `auth`.
- [ ] `auth_handlers.go:30,31,40` gọi qua API xuất khẩu; ngưỡng, thời gian cửa sổ và lane (local, device, public) giữ nguyên số.
- [ ] `loginguard_test.go` chuyển sang `internal/auth` cùng test helper mở store cục bộ (nếu cần); `TestGodocDoesNotClaimAPassword` quét file mới và vẫn xanh.

## Test first (write before implementing)
- Ma trận BVA theo `loginPerIPMax`, `loginDeviceMax`, `loginPublicMax` (đúng ngưỡng, ngưỡng cộng một, sau khi hết cửa sổ).
- Decision table lane: loopback owner, device có cookie, public; `refund` sau đăng nhập đúng.

## Files to touch
- `internal/httpapi/loginguard.go` → `internal/auth/loginguard.go`
- `internal/httpapi/auth_handlers.go` — gọi API mới
- `internal/httpapi/server.go` — field `login` (dòng 57), khởi tạo (dòng 105)
- `internal/httpapi/loginguard_test.go` → `internal/auth/loginguard_test.go`

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
- Thêm `net/http` vào package lá `auth` (hiện chỉ stdlib khác). `server.go` là hotspot chung 006 đến 012.
