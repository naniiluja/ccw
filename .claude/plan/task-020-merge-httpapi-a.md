# Task 020 — Gộp file httpapi nhóm A (khung server, auth, oauth, tài khoản, key, filter)

- **Vertical slice:** httpapi
- **Depends on:** 016, 019
- **Spec refs:** `.claude/rules/coding-conventions.md` (file dưới 1000 dòng, một khái niệm một file)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./...` xanh; `server.go` vẫn chứa đúng một dòng `// Package httpapi`; `openapi.json` ở cùng thư mục với `go:embed`
- **discipline:** off

## Goal (one sentence)
Gộp 17 file nhỏ của `httpapi` thành 6 file theo khái niệm, đưa các helper dùng chung về đúng chỗ.

## Acceptance criteria (verifiable)
- [ ] `server.go` ← `openapi.go`, `apierror.go`, `body.go`, `writeJSON` (từ `dashboard_api.go`) và `writeError`, `writeLegacyError` (từ `proxy.go`).
- [ ] `auth.go` ← `auth_handlers.go`, `origin.go`, `principal.go`.
- [ ] `oauth.go` ← `login.go` (OAuth đăng nhập provider).
- [ ] `accounts.go` ← `provdefs.go`, `dashboard_api.go`.
- [ ] `keys.go` ← `keylimit.go`, `timezone.go`.
- [ ] `filters.go` ← `contentguard.go`.
- [ ] Không file nào quá 1000 dòng; không đổi chữ ký hay thân hàm; chỉ một `// Package httpapi`.
- [ ] Giữ phép gán trong `init()` nếu có; không có biến package khởi tạo phụ thuộc chéo file.

## Test first (write before implementing)
- Không có test mới; test hiện có giữ nguyên tên file. Gate đo bằng `go build ./...` (trùng tên định danh báo lỗi biên dịch) và toàn bộ test.

## Files to touch
- internal/httpapi/server.go, internal/httpapi/openapi.go, internal/httpapi/apierror.go, internal/httpapi/body.go
- internal/httpapi/auth.go, internal/httpapi/auth_handlers.go, internal/httpapi/origin.go, internal/httpapi/principal.go
- internal/httpapi/oauth.go, internal/httpapi/login.go
- internal/httpapi/accounts.go, internal/httpapi/provdefs.go, internal/httpapi/dashboard_api.go
- internal/httpapi/keys.go, internal/httpapi/keylimit.go, internal/httpapi/timezone.go
- internal/httpapi/filters.go, internal/httpapi/contentguard.go
- internal/httpapi/proxy.go — chỉ để bỏ `writeError`

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Gộp bằng cách dán thân file vào file đích và xóa file nguồn; giữ doc comment đi theo hàm. `release_test.go` ghim đường dẫn `server.go` nên giữ đúng tên đó.
