# Task 023 — Tách failover, v1 và relayVia thành các bước tên rõ

- **Vertical slice:** httpapi (refactor, cùng package)
- **Depends on:** 021
- **Spec refs:** `.claude/rules/error-handling.md`, `.claude/rules/architecture.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race -count=3 ./internal/httpapi/` xanh, golden Zen không đổi; `gocyclo` của `failover`, `v1`, `relayVia` đều dưới hoặc bằng 30
- **discipline:** on

## Goal (one sentence)
Giảm độ phức tạp của lõi `/v1` (failover 71, v1 51, relayVia 34) bằng cách tách các bước trong cùng package, hành vi giữ nguyên.

## Acceptance criteria (verifiable)
- [ ] Test đặc tả viết TRƯỚC khi tách, cho các nhánh chưa có test (`failover` phủ 86,2 phần trăm, `v1` 88,5 phần trăm): nhánh Copilot `/responses` retry (gọi `prepare` hai lần), `errSkipAccount` làm `continue` sang account kế, account cuối trả nguyên lỗi upstream, 401 OAuth refresh đúng một lần, stream bị ngắt giữa chừng.
- [ ] `prepare` thành method `a.buildUpstreamCall(...) (path, send, to, via, err)` thay cho closure ghi 4 biến; tách thêm `resolveAttempt`, `sendWithRecovery`, `relayAnswer` cho `failover`; `parseV1Model` và `selectTargets` cho `v1`.
- [ ] `defer resp.Body.Close()` ở lại nơi nó đang phục vụ pipe của `relayVia`; không thêm đường thoát nào mà không đúng một lần đóng; thứ tự `defer pw.Close()` trong producer giữ nguyên.
- [ ] `errSkipAccount` và chỗ nhận biết `errors.As` ở cùng package; chữ ký `failover` vẫn tương thích với hai chỗ gọi (`v1.go`, `servertools.go` với `rec`).

## Test first (write before implementing)
- Ma trận quyết định cho vòng thử account (số account, standby, mã lỗi retryable hay không, account cuối).
- Chạy `go test -race -count=3` để bắt race ở pipe.

## Files to touch
- internal/httpapi/v1.go
- internal/httpapi/proxy.go
- internal/httpapi/websearch.go
- internal/httpapi/failover_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Sau task 014 `failover` đã bớt con trỏ `cap`, nên việc tách đơn giản hơn kế hoạch gốc. Không chuyển sang package khác.
