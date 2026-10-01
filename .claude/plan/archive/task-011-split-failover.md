# Task 011 — Tách failover trong v1.go thành các bước nhỏ

- **Vertical slice:** httpapi (refactor, cùng package)
- **Depends on:** 004
- **Spec refs:** `.claude/rules/error-handling.md`, `.claude/rules/architecture.md`, `.claude/rules/debugging.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Chia `failover` (`v1.go:665-904`, khoảng 240 dòng) thành các bước có tên, trong cùng package, để việc chuyển package ở task 012 chỉ còn là di chuyển.

## Acceptance criteria (verifiable)
- [ ] Closure `prepare` trả về kết quả (`path, send, to, via`) thay vì ghi 4 biến chia sẻ; nó vẫn được gọi hai lần (nhánh Copilot responses-only, `v1.go:821`).
- [ ] Vòng thử account tách thành các bước: chọn account và secret, chuẩn bị request, gửi, quyết định failover, relay.
- [ ] Mọi đường thoát giữ `cap.ReleaseOnAbort()` (`v1.go:690,794,884,894`); `defer resp.Body.Close()` (865) giữ đúng vị trí; thứ tự `defer pw.Close()` và `close(cap.ProducerDone)` trong `relayVia` (`shapes.go:187-196`) không đổi.
- [ ] `errSkipAccount` và chỗ nhận biết `errors.As` (`v1.go:771,788-791`) ở cùng package.
- [ ] `v1.go` không còn hàm nào dài quá 120 dòng; không đổi hành vi (golden Zen và toàn bộ test `v1_*`, `shapes_test`, `proxy_test`, `rotation_test` giữ nguyên xanh).

## Test first (write before implementing)
- Test đặc tả bổ sung trước khi tách (characterization): stream bị ngắt giữa chừng (race), Copilot responses-only gọi `prepare` hai lần, `errSkipAccount` làm `continue` sang account kế, account cuối trả nguyên lỗi upstream, 401 OAuth refresh một lần.
- Chạy `go test -race -count=3 ./internal/httpapi/` làm cổng.

## Files to touch
- `internal/httpapi/v1.go` — chia `failover`
- `internal/httpapi/shapes.go` — chỉ nếu cần để `relayVia` rõ hơn
- `internal/httpapi/v1_*_test.go`, `shapes_test.go` — thêm test đặc tả

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
- Tách refactor khỏi di chuyển theo quy tắc CCF: lỗi nếu có sẽ biết do refactor hay do chuyển package. Trùng file với task-003 (`v1.go`): `/ccf:cook` xếp khác wave.
