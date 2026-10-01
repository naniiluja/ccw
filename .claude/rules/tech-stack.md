# Tech Stack

Philosophy: chọn stack **ổn định nhất, phổ biến nhất, ít lỗi nhất**, tránh bleeding-edge. Repo gần như chỉ dùng stdlib, giữ nguyên tinh thần đó.

## Chosen stack
- Ngôn ngữ: Go 1.25.0 (`go.mod`), module `github.com/naniiluja/ccw`.
- HTTP: stdlib `net/http` với `ServeMux` pattern Go 1.22+ (`GET /path`, `/v1/{path...}`). Không router ngoài. `http.Server` đặt `ReadHeaderTimeout` 10s và `IdleTimeout` 120s; `WriteTimeout` cố ý để trống để không cắt stream dài (`newHTTPServer` trong `cmd/ccw/main.go`).
- Database: SQLite qua `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`), chế độ WAL, file `ccw.db` và file `-wal`/`-shm` quyền `0o600`. Schema đánh số bằng `PRAGMA user_version`.
- Log: `log/slog` với handler JSON ra stderr.
- Dashboard UI: đã gỡ, sẽ viết lại. Chưa chọn stack.
- Phân phối: gói npm `ccw-gateway` (Node >=18) kéo binary theo nền tảng qua `optionalDependencies`, build bằng `scripts/npm-build.sh`. CI publish lên GitHub Packages khi có tag `v*`, sau khi job `test` xanh.

## Core libraries (một lựa chọn cố định mỗi việc)
- Routing `net/http.ServeMux`. DB driver `modernc.org/sqlite`. Log `log/slog`.
- Auth: tự cài trong `internal/auth` bằng stdlib, không thư viện ngoài. Mật khẩu băm PBKDF2-HMAC-SHA256 (`crypto/pbkdf2`, 600000 vòng theo OWASP, salt 16 byte), so sánh bằng `crypto/subtle`, lưu ở bảng settings khóa `password_hash`. Session cookie ký HMAC bằng `CCW_SESSION_KEY`.
- Test: `testing` + `net/http/httptest`, không thư viện assert.
- Mọi dòng trong `go.mod` đang đánh dấu `// indirect`, kể cả `modernc.org/sqlite` v1.59.0, dù nó là import ngoài duy nhất (blank import ở `internal/store/store.go`).
- Công cụ lint **không** nằm trong `go.mod`: `scripts/check.sh` chạy qua `go run module@version`, ghim staticcheck `v0.8.1` (2026.2.1, chạy trên `GOTOOLCHAIN=go1.27.1` vì v0.8.x cần Go 1.26 trở lên) và gocyclo `v0.6.0`.

## Cấu hình runtime
- Biến môi trường (đối chiếu bằng `grep -rhoE 'CCW_[A-Z_]+' --include='*.go' internal cmd | sort -u`): `CCW_PASSWORD` (tối thiểu 12 ký tự, ưu tiên hơn mật khẩu đã lưu), `CCW_API_TOKEN` (master token cho máy), `CCW_SESSION_KEY`, `CCW_SESSION_TTL`, `CCW_INSECURE_NO_AUTH`, `CCW_OWNER_LOOPBACK`, `CCW_TZ`, `CCW_SEARCH_PROVIDER`, `CCW_SEARCH_KEY`, `CCW_SEARCH_URL`, `CCW_SEARCH_COUNT`, `CCW_SEARCH_MODEL`, `CCW_ANTIGRAVITY_CLIENT_SECRET`.
- Flag: `-addr` (mặc định `127.0.0.1:20130`), `-db` (mặc định `ccw.db`), `-reset-password` (sinh mật khẩu mới, in một lần, lưu bản băm, rồi thoát), `-insecure-no-auth`.
- Lần chạy đầu không có `CCW_PASSWORD` và DB chưa có `password_hash`: ccw tự sinh mật khẩu, in một lần ra stderr, chỉ lưu bản băm.
- Đã gỡ: `CCW_TOTP_SECRET`, `-enroll`, `-show-totp` (đăng nhập TOTP), `CCW_ARENA_URL` (ranking), lệnh con `contract-reset` (contract lab), cấu hình notify.

## Rules
- Không thêm thư viện ngoài danh sách trên khi chưa so sánh best practice qua Context7 và cập nhật file này. `[review]` `git diff go.mod`.
- Ghim phiên bản trong `go.sum`; nâng major phải ghi lý do. Nâng phiên bản công cụ lint là sửa biến ở đầu `scripts/check.sh`. `[review]`
- Giữ `CGO_ENABLED=0`: build đa nền tảng của `scripts/npm-build.sh` phụ thuộc vào điều này, nên không thêm dependency cần cgo. `[review]`: `scripts/npm-build.sh` build với `CGO_ENABLED=0` và đỏ nếu cần cgo, nhưng nó chỉ chạy lúc publish, không nằm trong `scripts/check.sh`.
