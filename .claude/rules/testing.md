# Testing (verification-first)

Philosophy: **write a failing test FIRST** before implementing/fixing — this is the single highest-leverage thing to let Claude verify its own work.

## Rules
- Each task: write a test reproducing the acceptance criteria (red) → implement → test green.
- Bug fix: write a test reproducing the bug (red) first, then fix.
- A task's acceptance criteria = concrete tests, not vague descriptions.

## Tooling & location
- Test framework: package `testing` của Go, `net/http/httptest` cho HTTP, không thư viện assert.
- Run command: `go test ./...` (một package: `go test ./internal/translate/`). Trước khi merge chạy thêm `go vet ./...`.
- Tests live in: `<file>_test.go` cạnh file được test, cùng package. Có 92 file test đã track. Nhóm lớn nhất: `internal/httpapi`, `internal/translate`, `internal/contract`.
- Helper dùng chung gọi `t.Helper()` (ví dụ `mustJSON` trong `internal/translate/translate_test.go`).
- Golden file nằm ở `internal/httpapi/testdata/zen/` (`cases.json`, `*.golden.json`). Đọc `README.md` cùng thư mục trước khi ghi lại; khi cập nhật golden phải xem diff, không ghi đè mù.
- Kiểu viết test hỗn hợp: 22 file dùng table-driven, còn lại là kiểm tra trực tiếp trên JSON. Test mới có nhiều case cùng cấu trúc thì dùng table-driven với `t.Run`.

## Bộ test bảo vệ bản phát hành công khai
`cmd/ccw/release_test.go` đọc **cây thư mục thật** (không dùng fixture) và kiểm tra: `LICENSE` là MIT, `.gitignore` có `*.env` và `ccw.env`, comment trong `internal/auth/totp.go` và `internal/httpapi/server.go` không nhắc "password", fixture không chứa chuỗi `ya29.`. Repo không còn `README.md`, `SECURITY.md` và `docs/`, nên các test từng đọc chúng đã bị gỡ; đừng thêm test mới đọc những đường dẫn này.

## Coverage
- Coverage target: không đặt con số. Core business paths must have tests before marking a task `done`: rotation/failover, dịch shape, error envelope, auth tier, migration store.
- Không có cổng test ép buộc bằng hook (`prompt-only`); kỷ luật ma trận test (EP/BVA) không được bật cho dự án này.
- CI hiện **không chạy test** (`.github/workflows/github-packages.yml` chỉ build và publish khi có tag). `task-001` sửa điều này. Trên cây làm việc hiện tại `go test ./...` xanh toàn bộ.
