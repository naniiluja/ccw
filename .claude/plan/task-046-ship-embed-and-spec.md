# Task 046: Đóng gói SPA vào binary, đồng bộ spec, kiểm bằng trình duyệt

- **Vertical slice:** build script + npm build + CI publish + spec + kiểm đầu cuối
- **Depends on:** 033, 034, 035, 036, 037, 038, 039, 040, 041, 042, 043, 044, 045
- **Spec refs:** toàn bộ `.claude/rules/*`, `CLAUDE.md`
- **MCP to use:** claude-in-chrome (kiểm đầu cuối), context7 (go embed)
- **Gate (must be GREEN before the next slice):** `bash scripts/ui-build.sh && bash scripts/check.sh` xanh; `cd be && go build ./cmd/ccw` xong và binary phục vụ `/ui/`; `wc -lc CLAUDE.md` dưới 200 dòng và 12KB
- **discipline:** off

## Goal (one sentence)
Binary ccw (và gói npm) chứa SPA đã build, chạy được đầu cuối qua trình duyệt thật, và spec phản ánh đúng cấu trúc be/ và fe/ mới.

## Acceptance criteria (verifiable)
- [ ] `scripts/ui-build.sh`: `pnpm -C fe install --frozen-lockfile`, `pnpm -C fe build`, xóa nội dung `be/internal/webui/static/` trừ `.gitkeep`, copy `fe/dist/.` vào đó; chạy lại nhiều lần cho kết quả giống nhau.
- [ ] `scripts/npm-build.sh` gọi `ui-build.sh` trước `go build`; workflow job `publish` cài Node và pnpm trước bước Build; `release_test.go` có test khẳng định điều đó.
- [ ] Test Go đọc `static/index.html` thật khi đã build (bỏ qua có chú thích khi chưa build) và khẳng định `/ui/` trả `200` kèm `<div id="root">`.
- [ ] Kiểm đầu cuối bằng Claude in Chrome trên binary thật với DB tạm (`-db /tmp/ccw-check.db`): `/` chuyển sang `/ui/login`; đăng nhập bằng mật khẩu in ở stderr; duyệt đủ 10 trang; làm mới ở deep link `/ui/keys`; thêm tài khoản khóa API giả, tạo và xóa API key (khóa hiện một lần); đổi theme sáng, tối; mở Command palette; đăng xuất; thử ở 360px, 768px, 1280px (dùng `resize_window`); không có lỗi trong console. Ghi kết quả vào Notes.
- [ ] Spec: `CLAUDE.md` (layout `be/` và `fe/`, mô tả dashboard, số package 15 và số tệp), `architecture.md` (package `webui`, bản đồ `fe/`, tier `/ui/` công khai, `GET /api/session`, `GET /api/settings`), `tech-stack.md` (stack FE: React, Vite, TypeScript, Tailwind v4, shadcn Radix, TanStack Query, React Router, Vitest, MSW, pnpm), `testing.md` (cổng FE), `tooling.md` (lệnh `pnpm`, `ui-build.sh`, shadcn MCP, sự cố cache `npx`), `coding-conventions.md`, `git-workflow.md` nếu còn lệch.
- [ ] `.claude/rules/frontend.md` (mới): luật không chế component (mọi `fe/src/components/ui/*` do `shadcn add`; ngoài `ui/` chỉ ghép từ primitive shadcn; registry `@originui` ở `coss.com`), cách kiểm bằng lệnh, đặt tên, UI tiếng Việt không i18n, mỗi luật ghi `[tool]` hoặc `[review]` kèm lệnh.
- [ ] Các việc ngoài phạm vi ghi ở mục "Current plan" của `CLAUDE.md`: sửa web search, múi giờ, mật khẩu từ UI; thu hồi session khi `-reset-password`; cập nhật `openapi.json` cho `/api/*`; cookie `Secure` theo `requestIsTLS`.

## Test first (write before implementing)
`release_test.go`: workflow `publish` chạy `ui-build.sh` trước `npm-build.sh` (đỏ cho đến khi sửa); test Go cho `index.html` đã build; kiểm đầu cuối là bước chấp nhận cuối cùng, không phải test tự động.

## Files to touch
- scripts/ui-build.sh
- scripts/npm-build.sh
- .github/workflows/github-packages.yml
- be/cmd/ccw/release_test.go
- be/internal/webui/webui_test.go
- CLAUDE.md
- .claude/rules/architecture.md
- .claude/rules/tech-stack.md
- .claude/rules/testing.md
- .claude/rules/tooling.md
- .claude/rules/coding-conventions.md
- .claude/rules/git-workflow.md
- .claude/rules/frontend.md
- .claude/plan/PLAN.md

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Spec dùng chung là hotspot nên gom vào task cuối này.
- Sửa mọi lỗi giao diện hay tích hợp tìm thấy khi kiểm đầu cuối ngay trong task này nếu nhỏ; nếu lớn, ghi thành WARN trong Notes và không mở rộng phạm vi.
- Không push, không tạo tag trong task này; việc đẩy nhánh và tạo PR do phiên chính làm sau khi `/ccf:check` xong.
