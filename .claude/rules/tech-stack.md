# Tech Stack

Philosophy: prefer the **most stable, most widely-supported, least-buggy** stack, mainstream, avoid bleeding-edge. Repo hiện tại gần như chỉ dùng stdlib, giữ nguyên tinh thần đó.

## Chosen stack
- Ngôn ngữ: Go 1.25.0 (`go.mod:3`), module `github.com/naniiluja/ccw`.
- HTTP: stdlib `net/http` với `ServeMux` pattern Go 1.22+ (`GET /path`, `/v1/{path...}`, `GET /{$}`). Không có router ngoài.
- Database: SQLite qua `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`), chế độ WAL, file `ccw.db` quyền `0o600`.
- Dashboard UI: đã gỡ, sẽ viết lại. Chưa chọn stack.
- Phân phối: gói npm `ccw-gateway` (Node >=18) kéo binary theo nền tảng qua `optionalDependencies`. CI publish lên GitHub Packages khi có tag `v*`.

## Core libraries (one fixed choice per concern)
- Routing: `net/http.ServeMux`. DB driver: `modernc.org/sqlite`. Log: `log` stdlib (xem `logging.md` về hướng chuyển sang `log/slog`).
- Auth: TOTP tự cài trong `internal/auth`, không thư viện ngoài. Session cookie ký bằng `CCW_SESSION_KEY`.
- Test: package `testing` + `net/http/httptest`, không có thư viện assert.
- Mọi dependency trong `go.mod` đang đánh dấu `// indirect`, chỉ `modernc.org/sqlite` được import trực tiếp.

## Cấu hình runtime (biến môi trường)
`CCW_TOTP_SECRET`, `CCW_API_TOKEN`, `CCW_SESSION_KEY`, `CCW_SESSION_TTL`, `CCW_INSECURE_NO_AUTH`, `CCW_OWNER_LOOPBACK`, `CCW_TZ`, `CCW_ARENA_URL`, `CCW_SEARCH_*`, `CCW_ANTIGRAVITY_CLIENT_SECRET`. Flag: `-addr` (mặc định `127.0.0.1:20130`), `-db` (mặc định `ccw.db`), `-enroll`, `-show-totp`, `-insecure-no-auth`.

## Rules
- Do not add a new library outside the list above without comparing best practices via Context7 and updating this file.
- Pin versions in `go.sum`; major upgrades require a recorded reason.
- Giữ `CGO_ENABLED=0`: build đa nền tảng của `scripts/npm-build.sh` phụ thuộc vào điều này, nên không thêm dependency cần cgo.
