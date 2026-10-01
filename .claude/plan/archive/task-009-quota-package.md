# Task 009 — Chuyển quota sang internal/quota

- **Vertical slice:** quota + httpapi
- **Depends on:** 004, 006
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/error-handling.md`, `.claude/rules/testing.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Đưa đọc quota và nhận phần thưởng reset (`quota.go`, `quota_providers.go`, `quota_resets.go`, khoảng 1700 dòng) vào `internal/quota`, phá vòng với proxy bằng interface khai báo ở phía dùng.

## Acceptance criteria (verifiable)
- [ ] `quota` khai báo interface nhỏ: `TokenSource` (lấy secret và force refresh), `RateSource` (đọc rate header đã bắt), `ConnectionLookup`, và cấu hình `BaseOverride`; `proxy` hiện tại (`httpapi`) implement qua adapter.
- [ ] `quotaFetchers`, `claimClaude`, `claimCodex` không còn nhận `*api`.
- [ ] `quotaList` và `claimReset` ở lại `httpapi` làm handler mỏng (`claimReset` tự kiểm session cookie nên là logic HTTP); route và JSON (`AccountQuota`, `QuotaWindow`, `QuotaReset`) giữ nguyên.
- [ ] `errlog.go:182,194`, `login.go:296`, `mcp.go:409-413` gọi API xuất khẩu của `quota`.
- [ ] 5 file `quota_*_test.go` (2041 dòng) chuyển sang `internal/quota` hoặc ở lại `httpapi` nếu chỉ chạm HTTP; `quotaFetchers[...]` gán đè thành tiêm fetcher giả.

## Test first (write before implementing)
- Ma trận `windowsFromHeaders` (EP theo nhà cung cấp, BVA: 0 phần trăm, 100 phần trăm, thiếu header).
- Decision table `claimReset` (unknown, hold, done, xung đột 409).
- Test `quotaFor` đồng thời (singleflight) chạy dưới `-race`.

## Files to touch
- `internal/httpapi/quota.go`, `quota_providers.go`, `quota_resets.go` → `internal/quota/`
- `internal/httpapi/server.go` — fields `quota`, `claimLocks`, `claims` (dòng 46, 63, 104, 106), route 134-136
- `internal/httpapi/errlog.go`, `login.go`, `mcp.go` — gọi API mới
- `internal/httpapi/quota_cache_test.go`, `quota_claim_test.go`, `quota_nullwindow_test.go`, `quota_org_test.go`, `quota_resets_test.go`

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
- Tốn công nhất trong nhóm package lá: test chạm trực tiếp `a.claims`, `a.quota.m`, `quotaFetchers["claude"]`. `claimLocks` dùng cùng kiểu `refreshLocks` với `refresh` (OAuth): quyết định cách chia khi làm. Hai cơ chế override song song (`baseOverride` và biến URL ở `quota_providers.go:21-28`) hợp nhất qua `BaseOverride`.
