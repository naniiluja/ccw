# Task 014 — Xóa contract lab và mọi thứ liên quan đến switcher

- **Vertical slice:** store + contract + httpapi + cmd + docs
- **Depends on:** —
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/error-handling.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `grep -rniE "switcher|contract lab|ClaimTraced|contractMgr|X-Ccw-Trace" --include='*.go' --include='*.md' internal cmd docs` rỗng; `/v1/chat/completions` và `/v1/messages` vẫn pass toàn bộ test hiện có
- **discipline:** off

## Goal (one sentence)
Xóa hẳn contract lab (so sánh shape với switcher) cùng cờ `trusted` của API key mà chỉ nó dùng, để luồng relay `/v1` không còn con trỏ capture.

## Acceptance criteria (verifiable)
- [ ] Package `internal/contract` bị xóa hoàn toàn (6 file nguồn và test).
- [ ] `store/contract.go`, `store/contract_test.go` bị xóa; `contractSchema` và `MigrateContract` không còn ở `store/store.go`.
- [ ] `httpapi/contracts.go`, `contractreview.go`, `judge_adapter.go` và ba file test tương ứng bị xóa; các route `/api/contracts*` và tool MCP `list_contract_findings` bị gỡ khỏi `server.go` và `mcp.go`.
- [ ] Không còn `cap *contract.Capture`, `ReleaseOnAbort`, `ProducerDone`, `captureWriter` ở `v1.go`, `shapes.go`, `proxy.go`; `failover` và `relayVia` giữ nguyên thứ tự đóng `resp.Body` và pipe.
- [ ] Header `X-Ccw-Trace` không còn được đọc hay xóa; vòng `contractPruneLoop` và lệnh con `contract-reset` ở `cmd/ccw` bị xóa.
- [ ] Cờ `trusted` của API key bị xóa: route `POST /keys/{id}/trusted`, `setKeyTrusted`, `isCallerTrusted`, `Store.SetAPIKeyTrusted/IsKeyTrusted`, trường `Trusted` và cột `trusted` trong schema (trước khi xóa, dùng `go_symbol_references` xác nhận chỉ contract lab dùng).
- [ ] `docs/clients.md` không còn nhắc CC Switch hay switcher; hướng dẫn Claude Code viết lại bằng cấu hình thuần (`ANTHROPIC_BASE_URL`, key).
- [ ] `internal/drift` giữ nguyên vì AI review còn dùng.

## Test first (write before implementing)
- Chạy trước khi xóa `go test -race ./...` làm baseline, lưu kết quả.
- Sau khi xóa: `go build ./...` rồi `go vet ./...` để tìm mọi tham chiếu còn sót; test của `failover` và `relayVia` (`v1_test.go`, `shapes_test.go`, `proxy_test.go`, `rotation_test.go`) giữ nguyên và xanh.
- Test mới: `TestKeyCreationHasNoTrustedFlag` (tạo key rồi đọc lại, không có trường `trusted`).

## Files to touch
- internal/contract/ — xóa cả thư mục
- internal/store/contract.go, internal/store/contract_test.go — xóa
- internal/store/store.go — bỏ `contractSchema`, `MigrateContract`
- internal/store/apikey.go — bỏ cờ `trusted`
- internal/httpapi/contracts.go, internal/httpapi/contractreview.go, internal/httpapi/judge_adapter.go — xóa
- internal/httpapi/contracts_test.go, internal/httpapi/contractreview_test.go, internal/httpapi/judge_adapter_test.go — xóa
- internal/httpapi/v1.go, internal/httpapi/shapes.go, internal/httpapi/proxy.go — bỏ capture
- internal/httpapi/server.go — bỏ wiring, route, `isCallerTrusted`, `contractPruneLoop`
- internal/httpapi/mcp.go — bỏ tool contract
- internal/httpapi/keys.go — bỏ `setKeyTrusted`
- internal/httpapi/v1_test.go — bỏ nhắc switcher
- cmd/ccw/main.go, cmd/ccw/main_test.go — bỏ `contract-reset`
- docs/clients.md — bỏ CC Switch

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Task này đơn giản hóa `failover` rất nhiều (con trỏ `cap` rải ở `v1.go:690,794,884,894` và `shapes.go:187-238`). Làm trên nền test xanh, đừng đổi hành vi nào khác. Số dòng xóa dự kiến khoảng 4.200 (không tính test).
