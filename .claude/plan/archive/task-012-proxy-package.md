# Task 012 — Chuyển lõi /v1 sang internal/proxy

- **Vertical slice:** proxy + httpapi
- **Depends on:** 009, 010, 011
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/error-handling.md`, `.claude/rules/logging.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Chuyển lõi `/v1` (failover, relay, shapes, rotation, variants, model catalog, OAuth refresh, errlog, principal) sang `internal/proxy`, giữ route và handler mỏng ở `httpapi`.

## Acceptance criteria (verifiable)
- [ ] `proxy.Engine` giữ state `cat`, `rate`, `sigs`, `auto`, `rrMu`, `rrNext`, `copilot`, `refresh` (trước nằm trong `api`); `api` chỉ còn cấu hình, auth, router và các service còn lại.
- [ ] Các provider adapter (`copilot.go`, `antigravity.go`, `typesafe.go`, `zen.go`, `servertools.go`, `websearch.go`, `autotest.go`, `oauth.go`) đi cùng `proxy`; không tách thêm package.
- [ ] `principal` (`Principal`, `ccwJob`, `withPrincipal`, `clientOf`) xuất từ `proxy` (hoặc giữ ở `httpapi` nếu việc kiểm tra cho thấy rẻ hơn, ghi lý do) và `isCallerTrusted` (`server.go:281-295`) hoạt động như cũ.
- [ ] `copilotResponsesModels` (`shapes.go:48`) và `droppedTools` (`servertools.go:18`) thành field của `Engine`; test không còn rò rỉ trạng thái giữa các test.
- [ ] Route trong `server.go:125-127,187-199,204-205,220-222` giữ nguyên, handler `models`, `testAccount`, `accountTests`, `providerModelTable`, `setModelsActive`, `deleteModels`, `testModel`, `getRotation`, `setRotation` ở lại `httpapi`; adapter cho `quota.TokenSource` và `review.Asker`/`Sender` nằm ở `httpapi`.
- [ ] `TestAdminAPIRoutesRefuseADashboardKey` và toàn bộ test auth tier xanh; golden Zen không đổi; `go test -race ./...` xanh.

## Test first (write before implementing)
- Test đặc tả từ task 011 chạy nguyên vẹn sau khi chuyển.
- Ma trận rotation và failover (decision table: số account, trạng thái standby, mã lỗi retryable hay không, account cuối).
- Test `internal/proxy` dựng `Engine` bằng constructor, không qua `httpapi`.

## Files to touch
- `internal/httpapi/v1.go`, `shapes.go`, `proxy.go`, `rotation.go`, `variants.go`, `models.go` (logic), `model_rewrite.go`, `echo.go`, `errlog.go` (recording), `oauth.go`, `copilot.go`, `antigravity.go`, `typesafe.go`, `zen.go`, `servertools.go`, `websearch.go`, `autotest.go`, `principal.go`, `body.go`, `timezone.go`, `ratelimit.go` → `internal/proxy/`
- `internal/httpapi/server.go` — nối dây, đăng ký route, bỏ các field đã chuyển
- `internal/httpapi/*_test.go` liên quan — chuyển theo hoặc đổi sang API xuất khẩu

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
- Task cao rủi ro nhất; chỉ bắt đầu khi 009, 010, 011 đã xanh. `writeError`/`writeJSON` đã ở `httpx.go` (task 004) nên `proxy` không cần chúng. Quyết định `principal` và nơi `errlog` cần chốt khi làm bằng `go_symbol_references`.
