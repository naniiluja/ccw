# Task 033: Phục vụ SPA nhúng dưới /ui/

- **Vertical slice:** BE package `webui` + route trong `httpapi` + test kiến trúc
- **Depends on:** 032
- **Spec refs:** `.claude/rules/architecture.md` (import graph, auth tiers), `.claude/rules/coding-conventions.md` (file nhỏ, `newX()`), `.claude/rules/testing.md` (kỷ luật contract)
- **MCP to use:** context7 (Go net/http ServeMux, embed)
- **Gate (must be GREEN before the next slice):** `bash scripts/check.sh` xanh; `cd be && go test -race ./internal/webui/ ./internal/httpapi/` xanh
- **discipline:** on

## Goal (one sentence)
`GET /ui/*` phục vụ bản build SPA nhúng bằng `go:embed`, `GET /` chuyển hướng sang `/ui/`, và route `/v1`, `/api` không bị ảnh hưởng.

## Acceptance criteria (verifiable)
- [ ] Package `internal/webui` có đúng một `// Package webui`, một hàm công khai `Handler() http.Handler`, nhúng `//go:embed all:static`; `webui` không import package nội bộ nào.
- [ ] `static/` chỉ có `.gitkeep` được commit; khi chưa có `index.html`, mọi đường dẫn dưới `/ui/` trả `503` kèm thông báo tiếng Việt "UI chưa được build".
- [ ] Khi có `index.html`: tệp tồn tại được phục vụ bằng `http.FileServerFS`; đường dẫn không có đuôi tệp (deep link như `/ui/accounts`) trả `index.html` với `Cache-Control: no-cache`; `/ui/assets/*` trả `public, max-age=31536000, immutable`; tệp có đuôi mà thiếu trả `404`.
- [ ] `GET /{$}` trả `302` về `/ui/`; `GET /ui` được chuyển về `/ui/` bởi `ServeMux`.
- [ ] `HEAD /ui/` đúng; `POST /ui/` trả `405`.
- [ ] Đăng ký route không làm `ServeMux` panic (không dùng `GET /{path...}`); `GET /v1/models`, `GET /api/accounts`, `POST /login` giữ nguyên hành vi.
- [ ] `/ui/` công khai (không `requireSession`), vì trang đăng nhập phải tải được; vẫn đi qua `guardRequest`.
- [ ] `arch_test.go` thêm `TestWebUIIsALeaf`; `TestOnlyCmdImportsHTTPAPI` vẫn xanh.

## Test first (write before implementing)
Ma trận contract trên `Handler()` bằng `httptest` và `fstest.MapFS` (hàm nội bộ nhận `fs.FS` để test cả hai trạng thái chưa build, đã build): `/ui/` chưa build (503), có build (200 và đúng body), `/ui/accounts` (index), `/ui/assets/app.abc.js` có (200, immutable), `/ui/assets/missing.js` (404), `/ui/missing.png` (404), `HEAD /ui/`, `POST /ui/` (405), đường dẫn chứa `..` (không thoát khỏi FS). Ở `httpapi`: `GET /` (302 về `/ui/`), `GET /v1/models` không token (401 như cũ), `newServer` không panic.

## Files to touch
- be/internal/webui/webui.go
- be/internal/webui/webui_test.go
- be/internal/webui/static/.gitkeep
- be/internal/httpapi/server.go
- be/internal/httpapi/webui_test.go
- be/internal/httpapi/arch_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Go 1.22+ ServeMux: `GET /{path...}` xung đột `/v1/{path...}` và panic khi đăng ký; dùng `GET /{$}` và `GET /ui/` (go.dev/blog/routing-enhancements).
- Khác lựa chọn ban đầu "commit `index.html` placeholder": Vite và bước copy sẽ ghi đè tệp đã tracked, nên chỉ commit `.gitkeep` và handler trả `503` khi thiếu `index.html`.
- Task này chạm `be/internal/httpapi/server.go` cùng 034 và 035, nên `/ccf:cook` xếp chúng vào các wave khác nhau. Pattern `.gitignore` đã được 032 thêm sẵn.
- Số package và số tệp trong spec (14, 72) đổi thành 15 và 73 sau task này; đồng bộ spec thuộc task 046, không sửa ở đây.
