# Task 004 — Chuẩn bị: gom helper HTTP dùng chung, thêm test kiến trúc và test mcpAdminTools

- **Vertical slice:** httpapi + cmd + spec
- **Depends on:** —
- **Spec refs:** `.claude/rules/architecture.md`, `.claude/rules/coding-conventions.md`, `.claude/rules/error-handling.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Gom `writeError`, `writeLegacyError`, `writeJSON`, `writeAPIError`, `firstNonEmpty`, `truncate` vào `internal/httpapi/httpx.go` và thêm các test canh kiến trúc, để mọi task di chuyển sau đó không bị chặn bởi helper nằm trong file sắp chuyển.

## Acceptance criteria (verifiable)
- [ ] `writeError` và `writeLegacyError` (đang ở `proxy.go:272,276`), `writeJSON` (`dashboard_api.go:83`), `writeAPIError` (`apierror.go:58`), `firstNonEmpty` (`v1.go:953`), `truncate` (`errreview.go:588`) nằm trong `internal/httpapi/httpx.go`, chữ ký không đổi.
- [ ] `contains` (`modelinfo.go:297`) được thay bằng `slices.Contains`, hàm cũ bị xóa.
- [ ] Test kiến trúc đọc import bằng `go/parser`: không package nào dưới `internal/` ngoài `httpapi` import `internal/httpapi`.
- [ ] Test đối chiếu: mọi tool ghi trong `mcpTools` (`mcp.go:106`) có tên trong `mcpAdminTools` (`mcp.go:67`), và ngược lại không có tên thừa.
- [ ] `cmd/ccw/release_test.go` quét comment chứa `password` ở mọi file `.go` trong `internal/auth/` (không chỉ `totp.go`) và `internal/httpapi/server.go`.
- [ ] `.claude/rules/architecture.md` sửa dòng `translate → filter` cho khớp `go list` (translate không import package nội bộ nào).

## Test first (write before implementing)
- `TestNoInternalPackageImportsHttpapi` (đỏ nếu thử thêm một import giả vào package khác).
- `TestEveryWriteMcpToolIsAdminGated` (decision table: tool đọc / tool ghi / tool ghi thiếu trong `mcpAdminTools`).
- `TestGodocDoesNotClaimAPassword` mở rộng sang toàn bộ `internal/auth/*.go` (BVA: file mới chứa chữ `password` làm test đỏ).

## Files to touch
- `internal/httpapi/httpx.go` — mới, chứa helper dùng chung
- `internal/httpapi/proxy.go`, `dashboard_api.go`, `apierror.go`, `v1.go`, `modelinfo.go`, `errreview.go` — bỏ định nghĩa cũ
- `internal/httpapi/arch_test.go` — mới, test kiến trúc
- `internal/httpapi/mcp_test.go` — test đối chiếu `mcpAdminTools`
- `cmd/ccw/release_test.go` — mở rộng quét `internal/auth/`
- `.claude/rules/architecture.md` — sửa đồ thị import

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
- `writeError` có 187 lần gọi ở 28 file: chỉ đổi vị trí, không đổi chữ ký. Đây là task mở đường cho 006 đến 012.
