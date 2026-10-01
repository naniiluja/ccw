# Debugging (disciplined, no rushing)

Mandatory process when investigating a bug, followed directly in conversation (no separate debug command):
1. **Reproduce** — reconstruct the bug from symptom/input/environment before touching code.
2. **Trace step by step** — follow the correlation ID across logs, read entry+exit at each boundary.
3. **Query the DB read-only** (if an MCP is present) — verify data state step by step. Với SQLite cục bộ, mở bản sao của `ccw.db` bằng `sqlite3 -readonly`, không bao giờ ghi vào DB đang chạy.
4. **Isolate with evidence** — narrow the suspect area with file:line/log/row, not gut feeling.
5. **Failing test** — write a test reproducing the bug (red) first.
6. **Minimal fix** — fix only within scope, no side refactor.

> Never guess and fix on the spot before you have evidence.

## Công cụ có sẵn trong sản phẩm
- Bảng `upstream_errors` (giữ 14 ngày, tối đa 20k dòng) lưu lỗi upstream với body tối đa 64 KB, nhóm theo chữ ký body; xem qua dashboard, trang Errors.
- Drift (`shape_changes`) và contract lab (`contract_findings`) cho thấy khi nào một provider đổi shape hoặc converter làm mất field.
- Golden file Zen trong `internal/httpapi/testdata/zen/` là mẫu để tái hiện lỗi khớp với gateway gốc.

## Known bugs (updated by /ccf:updatespec)
- Symptom: `go test ./cmd/ccw` fail với `open ../../docs/verify-phase1.md: no such file` sau khi `README.md`, `SECURITY.md`, `docs/*.md` bị xóa | Root cause: `release_test.go` đọc các file đó từ cây thư mục thật, và `scripts/npm-build.sh` chạy `cp README.md` | Prevent: khi xóa hoặc đổi tên một file, `grep` tên file trong `*.go`, `*.sh`, `*.json`, `*.yml` trước; đã xử lý bằng cách gỡ 3 test và bỏ `README.md` khỏi bước đóng gói npm | Files: `cmd/ccw/release_test.go`, `scripts/npm-build.sh`, `npm/ccw-gateway/package.json`.
- Symptom: CI publish được một bản dù test đỏ | Root cause: workflow không có bước `go test` | Prevent: `task-001` | Files: `.github/workflows/github-packages.yml`.
