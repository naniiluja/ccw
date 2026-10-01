# Task 024 — Tách claimReset, claudeResetRows, quotaFor và newServer

- **Vertical slice:** httpapi (refactor, cùng package)
- **Depends on:** 022
- **Spec refs:** `.claude/rules/architecture.md` (3 tầng auth), `.claude/rules/error-handling.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./...` xanh; `gocyclo` của bốn hàm đều dưới hoặc bằng 30; `newServer` giữ chữ ký `(*api, http.Handler)`
- **discipline:** on

## Goal (one sentence)
Giảm độ phức tạp của nhận phần thưởng reset quota và của hàm dựng server (172 dòng).

## Acceptance criteria (verifiable)
- [ ] `claimReset` tách thành `claimPreflight` (xác thực, kiểm Sec-Fetch-Site, content-type, giải mã body), phần kiểm bảng done và unknown, và `recordClaimOutcome`; khóa theo connection và các `defer Unlock`, `defer tcancel` ở lại hàm điều phối; nhả mutex giữa hai pha vẫn đúng chỗ như cũ.
- [ ] `claudeResetRows` tách thành `weeklyResetRow`, `grantResetRows`, `grantRow`, `grantBlockedReason`; logic `seenIDs` giữ nguyên.
- [ ] `quotaFor` tách các pha singleflight, đọc cache, gọi fetcher.
- [ ] `newServer`: mỗi state struct có `newX()` ngay cạnh kiểu (tiền lệ `newZenState`, `newLoginGuard`), `registerRoutes(mux)` tách riêng, thứ tự dựng và `judgeAdapter`-kiểu back-fill không còn (contract lab đã xóa); các route giữ nguyên cùng bọc `requireSession/Token/Admin`.

## Test first (write before implementing)
- Test đặc tả cho `claimReset` (các nhánh unknown, hold, done, 409, 401 refresh) trước khi tách.
- `TestAdminAPIRoutesRefuseADashboardKey` phải xanh để chắc không route ghi nào mất `requireAdmin`.

## Files to touch
- internal/httpapi/quota_resets.go
- internal/httpapi/quota.go
- internal/httpapi/server.go
- internal/httpapi/quota_claim_test.go
- internal/httpapi/quota_resets_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Tiền lệ phong cách tốt trong repo: `processTrace` đã bị xóa cùng contract lab, nên lấy `httpapi/zen.go zenRequest` và `proxy.go` làm mẫu (hàm có tên rõ, trả `(T, error)`, không closure).
