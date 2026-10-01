# Task 018 — Đăng nhập bằng mật khẩu thay cho authenticator (TOTP)

- **Vertical slice:** auth + cmd + httpapi + tests
- **Depends on:** 014
- **Spec refs:** `.claude/rules/architecture.md` (3 tầng auth không được nới lỏng), `.claude/rules/error-handling.md`, `.claude/rules/logging.md` (không log secret)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./...` xanh gồm ma trận `CheckPassword`; test khẳng định DB chỉ chứa bản băm; đăng nhập đúng, sai và quá nhiều lần sai (429) chạy được trên bản dựng tạm
- **discipline:** on

## Goal (one sentence)
Người vận hành đăng nhập bằng một mật khẩu thay vì mã TOTP, mật khẩu không bao giờ nằm dạng rõ trong DB hay log.

## Acceptance criteria (verifiable)
- [ ] Mật khẩu lấy từ `CCW_PASSWORD` (tối thiểu 12 ký tự, nếu ngắn hơn thì dừng khi khởi động với lỗi rõ); nếu không đặt thì lần chạy đầu tự sinh một mật khẩu ngẫu nhiên, in ra stderr một lần, chỉ lưu bản băm (khóa `password_hash` trong settings); các lần chạy sau dùng bản băm đã lưu.
- [ ] Băm bằng stdlib `crypto/pbkdf2` (PBKDF2-HMAC-SHA256, salt ngẫu nhiên 16 byte, số vòng lặp theo khuyến nghị OWASP hiện hành, ghi hằng số và nguồn trong comment); so sánh bằng `crypto/subtle`; không thêm thư viện ngoài.
- [ ] `Config` có `CheckPassword(string) bool`; xóa `CheckCode`, `lastStep`, `TOTPSecret`, `GenerateTOTPSecret`, `TOTPNow`, `totp.go`, `totp_test.go`, `FromEnv` và biến `CCW_TOTP_SECRET`; `CheckAPIToken`, phiên cookie, device cookie giữ nguyên.
- [ ] `cmd/ccw/main.go`: bỏ cờ `-enroll`, `-show-totp`; thêm cờ `-reset-password` (sinh mật khẩu mới, in một lần, lưu bản băm, rồi thoát); `authDecision` và `-insecure-no-auth` giữ nguyên hành vi.
- [ ] `POST /login` đọc trường form `password`; sai trả 401 và đúng đặt cookie phiên như cũ; quá nhiều lần sai trả 429 qua loginguard.
- [ ] Các test từng dùng `auth.TOTPNow` (`loginguard_test.go`, `auth_test.go`, `origin_test.go`, `accounts_crud_test.go`) chuyển sang mật khẩu thử; `TestGodocDoesNotClaimAPassword` thay bằng bất biến mới: mật khẩu không có dạng rõ trong DB, trong log của một lần khởi động, hay trong comment mẫu; `ya29.` quét mọi `_test.go`.
- [ ] Comment nhắc TOTP trong `server.go`, `loginguard.go`, `auth_handlers.go` được sửa.

## Test first (write before implementing)
- Ma trận hợp đồng cho `CheckPassword` (EP: rỗng, sai, đúng, quá dài 1KB; BVA: 11 và 12 ký tự; đúng nhưng đã đổi bằng `-reset-password`).
- `TestFirstStartPrintsThePasswordOnceAndStoresOnlyAHash`, `TestAShortPasswordRefusesToStart`, `TestLoginRejectsAWrongPasswordWith401`, `TestLoginIsRateLimitedAfterRepeatedFailures` (dùng loginguard hiện có).

## Files to touch
- internal/auth/totp.go, internal/auth/totp_test.go — xóa
- internal/auth/config.go, internal/auth/config_test.go, internal/auth/session.go, internal/auth/password.go, internal/auth/password_test.go
- cmd/ccw/main.go, cmd/ccw/main_test.go, cmd/ccw/release_test.go
- internal/httpapi/auth_handlers.go, internal/httpapi/loginguard.go, internal/httpapi/server.go
- internal/httpapi/loginguard_test.go, internal/httpapi/auth_test.go, internal/httpapi/origin_test.go, internal/httpapi/accounts_crud_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Đây là tính năng mới, tách khỏi mọi task refactor. ccw giữ credential của nhiều nhà cung cấp nên mật khẩu "cho dễ" vẫn phải đủ dài (12 ký tự trở lên) và có loginguard canh. Bản băm lưu cùng chỗ bí mật TOTP cũ (bảng settings). In mật khẩu tự sinh một lần là ngoại lệ duy nhất của luật không log secret.
