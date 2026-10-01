# Tooling (skills / MCP / subagents có sẵn, KHI NÀO dùng)

Danh mục công cụ của dự án kèm **khi nào dùng**, để agent ở phiên sau tự quyết. `/ccf:updatespec` cập nhật file này khi có công cụ mới.

## MCP servers
- **context7** (plugin ccf): tra tài liệu hiện hành của thư viện, framework. **Dùng khi:** cần cú pháp API, best practice, migration. Cách: `resolve-library-id` rồi `query-docs`. Phiên không tương tác có thể bị từ chối quyền gọi; khi đó fetch tài liệu chính thức (`go.dev`, `pkg.go.dev`, `sqlite.org`) và ghi rõ nguồn.
- **microsoft-learn** (plugin ccf): tài liệu Microsoft, .NET, Azure. Dự án Go này thường không cần.
- **shadcn** (khai báo ở `.mcp.json`, chạy `npx -y shadcn@latest mcp`): tra cứu component shadcn/ui. **Dùng khi:** cần tham khảo markup một component để mô phỏng. UI cũ đã gỡ, bản viết lại chưa chọn stack; chưa chạy lệnh `add` cho tới khi có `components.json` và quyết định stack.
- **gopls** (nếu phiên có, không khai báo trong `.mcp.json`): `go_symbol_references`, `go_file_context`, `go_diagnostics`. **Dùng khi:** xóa, gộp hay đổi tên symbol, để không sót tham chiếu. Có thể không resolve bên trong một git worktree; khi đó dùng `grep` và `go build ./...`.

## Skills
- **ccf:plan**: lên kế hoạch một tính năng thành các vertical slice. **Dùng khi:** bắt đầu tính năng mới, ở plan mode.
- **ccf:cook**: chạy backlog theo wave song song, mỗi task một worktree. **Dùng khi:** có nhiều task độc lập trong `.claude/plan/`.
- **ccf:check**: review độc lập theo spec. **Dùng khi:** task chuyển `in-review`.
- **ccf:updatespec**: làm mới spec và memory. **Dùng khi:** cuối phiên có bài học hoặc công cụ mới.
- **claude-api**: tham chiếu Anthropic SDK và tham số API. **Dùng khi:** sửa code dịch Anthropic Messages, tool use, thinking block trong `internal/translate`.

## Subagents (CCF)
Mọi subagent CCF **chỉ đọc**: khám phá, review, tìm best practice, không viết code. Code được viết hoặc trực tiếp trong phiên chính (một task sau `/ccf:plan`), hoặc bởi agent task của `/ccf:cook`, mỗi agent một worktree tạo bằng `isolation: "worktree"`; không writer nào chạy trong checkout chính.
- **ccf-codebase-analyzer**: lập bản đồ hoặc khoanh vùng codebase. **Dùng khi:** onboarding (`/ccf:init`) hoặc trước khi lên kế hoạch (`/ccf:plan`).
- **ccf-spec-checker**: review tuân thủ spec và SOLID. **Dùng khi:** `/ccf:check`.
- **ccf-scope-checker**: kiểm diff có khớp phạm vi task. **Dùng khi:** `/ccf:check`, song song với spec-checker.
- **ccf-best-practice-researcher**: lấy best practice có trích nguồn. **Dùng khi:** cần căn cứ cho một quyết định.

## Lệnh thường dùng
- Kiểm toàn bộ trước commit: `bash scripts/check.sh` (gofmt, go vet, staticcheck, gocyclo, `go test -race ./...` gồm test kiến trúc). Giống hệt CI. Lần đầu tải staticcheck, gocyclo và toolchain `go1.27.1` qua `go run`, nên cần mạng.
- Build: `go build ./...`. Chạy cục bộ: `go run ./cmd/ccw -addr 127.0.0.1:20130 -db ccw.db`. Lần chạy đầu in mật khẩu dashboard **một lần** ra stderr (lưu ngay vào trình quản lý mật khẩu); hoặc đặt `CCW_PASSWORD` (tối thiểu 12 ký tự). Quên mật khẩu: `go run ./cmd/ccw -db ccw.db -reset-password`. Thử nghiệm dùng DB tạm (`-db /tmp/ccw-check.db`), không dùng DB thật.
- Đo nhanh: `find internal cmd -name '*.go' -not -name '*_test.go' | wc -l` (72), `find internal/httpapi -maxdepth 1 -name '*.go' -not -name '*_test.go' | wc -l` (21), `go run golang.org/x/tools/cmd/deadcode@latest -test ./...` khi dọn code chết.
- Build npm đa nền tảng: `scripts/npm-build.sh` (cần `NPM_SCOPE` khi publish). Không publish thủ công; release đi qua tag `v*`.

## System memory vs Spec (ghi ở đâu)
- **Spec** (file này và các rule khác): luật của dự án, suy ra được từ repo. Trọng số thấp hơn (user message).
- **Memory** (`~/.claude/projects/<path>/memory/`): `feedback` chống lặp lỗi và `user` preference. Trọng số cao hơn (system prompt). Cập nhật qua `/ccf:updatespec`. **Không** chép lại nội dung CLAUDE.md.
- **MEMORY.md chỉ là mục lục**: mỗi phiên chỉ nạp 200 dòng hoặc 25KB đầu (cái nào tới trước), nên giữ gọn. Tầng mạnh nhất là `feedback` (ghi cả thắng lẫn thua, bắt buộc có `Why`).
