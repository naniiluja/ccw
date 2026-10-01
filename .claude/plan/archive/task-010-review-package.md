# Task 010 — Chuyển review sang internal/review

- **Vertical slice:** review + httpapi
- **Depends on:** 004, 008
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/error-handling.md`, `.claude/rules/debugging.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Đưa drift review, error review, contract review, content guard và judge adapter (khoảng 1900 dòng) vào `internal/review`, thay lời gọi `a.v1` bằng request giả bằng interface `Asker` và `Sender`.

## Acceptance criteria (verifiable)
- [ ] `review` khai báo `Asker` (hỏi một model, thay `chatOnce`, `askJev`, `askResolver` đang gọi `a.v1` qua `httptest`) và `Sender` (gửi thô, thay `a.send`) cùng các interface hành động cần thiết (đọc và ghi filter, tắt model, tắt account); `httpapi` có adapter implement, adapter giữ `ccwJob`.
- [ ] `contractJudgeAdapter` dựng bằng interface, không còn back-fill `judgeAdapter.a = a` (`server.go:91,111`).
- [ ] Nơi đặt `ErrorGroup` và recording lỗi upstream (`errlog.go`) được chốt trong task; mặc định: `proxy` ghi và phân loại, `review` đọc qua `store`.
- [ ] Các vòng nền (`reviewLoop`, `errorReviewLoop`, `contractPruneLoop`) thành `Run(ctx)`; hành vi giữ nguyên.
- [ ] `unsafeFilter` (`contentguard.go`) vẫn được `filters.go:48,85` và `mcp.go:486` dùng, qua API xuất khẩu.

## Test first (write before implementing)
- Test review dùng `Asker`/`Sender` giả thay vì dựng server HTTP thật; decision table cho các nhánh `tryBlacklist`, `tryDisableModel`, `tryDisableAccount`.
- Ma trận `contentguard` (EP: filter an toàn, filter xóa model/tools/system prompt nguyên khối; BVA: pattern rỗng).

## Files to touch
- `internal/httpapi/driftreview.go`, `errreview.go`, `errbisect.go`, `contractreview.go`, `contentguard.go`, `judge_adapter.go` → `internal/review/`
- `internal/httpapi/filters.go`, `mcp.go`, `oauth.go`, `errlog.go` — gọi API mới
- `internal/httpapi/server.go` — fields `review`, `errReview`, adapter, 4 route
- test review tương ứng

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
- Cạnh review → proxy là chỗ rủi ro vòng import lớn nhất (`driftreview.go:187,305`, `errreview.go:187,298`). Sau task này `review` không import `proxy` và không import `httpapi`.
