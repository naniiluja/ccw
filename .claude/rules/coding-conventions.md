# Coding Conventions

## Formatting (verifiable)
- Indentation: tab (theo `gofmt`). 
- Formatter: run `gofmt -l .` before committing, danh sách trả về phải rỗng (hiện đang rỗng).
- Linter: `go vet ./...` must pass. Repo chưa có `golangci-lint` hay `staticcheck`, thêm chúng nằm trong plan (`task-001`).

## Naming
- Files: `snake_case.go` gọn, một chủ đề mỗi file (`keylimit.go`, `loginguard.go`, `quota_resets.go`); test là `<file>_test.go`.
- Variables/functions: camelCase cho identifier không export, PascalCase cho export, theo Go chuẩn.
- Constants: camelCase hoặc PascalCase theo phạm vi export (`PendingLoginTTL`); hằng SQL schema đặt tên dạng `xxxSchema`, `xxxCols`.
- Test: `TestTênĐơnVịVàHànhVi` mô tả hành vi, ví dụ `TestZenStreamLeavesInTheOpenAIShape`.

## File structure
- A file is at most ~1000 lines; split if it exceeds. Các file lớn nhất hiện nay đều dưới mức đó (`store/contract.go` 1165 và `httpapi/v1.go` 989 là hai ngoại lệ đã có, không làm chúng phình thêm).
- Import order: stdlib, một dòng trống, rồi module ngoài và `github.com/naniiluja/ccw/...` (đúng như `gofmt`/`goimports` sắp xếp).

## General rules
- No dead code / unused imports: dead code still costs reading time and misleads searches; delete it, git history keeps it.
- Comments must match the existing codebase's language: **tiếng Anh**, câu đầy đủ, mở đầu bằng tên hàm hoặc kiểu; comment package bắt đầu bằng `Package x ...`. Comment giải thích lý do, không nhắc lại code.
- Enforceable coding rules live HERE in `.claude/rules` (subagents auto-load these); do NOT keep them only in an output style — an output style modifies the main loop and does not reach spawned subagents.
- Dùng `json.Number` (hoặc giữ nguyên byte) khi chuyển body giữa các provider để không đổi số thực (`internal/translate`).
- Không đặt code hoặc fixture trông như secret thật; test `TestFixturesLookLikePlaceholders` cấm chuỗi `ya29.`.
- Không comment nào trong `internal/auth/totp.go` hoặc `internal/httpapi/server.go` được nhắc tới "password": đăng nhập chỉ bằng TOTP (`TestGodocDoesNotClaimAPassword`).
