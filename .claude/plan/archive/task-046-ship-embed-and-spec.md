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
- Kết quả kiểm đầu cuối bằng Chrome (binary nhúng SPA, DB tạm): chuyển hướng `/` sang `/ui/login`, đăng nhập, deep link `/ui/keys` sau khi tải lại, tạo khóa API, đổi theme, Command palette, đăng xuất. Đo độ rộng bằng CDP ở 360, 768 và 1280 px cho cả 10 trang: không còn tràn ngang sau các sửa ở commit `6a5ffb9` (trang Khóa API vỡ vì `models: null`, lưới Provider, dòng lỗi Quota, ngưỡng thanh bên 1024 px).
- Sau `/ccf:check` đầu tiên: `createAccount` và `deleteAccount` dùng `api()` (route trả JSON khi có `Accept`), bộ chọn kiểu API tùy chỉnh dùng `Select` của shadcn, thêm test múi giờ hiệu lực cho `GET /api/settings`, job `test` của CI cài pnpm và Node.
- WARN còn mở: `comp-163` chỉ được mô phỏng cấu trúc, chưa cài như registry item; ví dụ Codex và OpenAI ở trang Cài đặt chưa lấy từ `docs/clients.md`; trang Drift chưa liên kết sâu tới từng thay đổi; thông báo lỗi tiếng Anh của máy chủ hiện nguyên văn; hook `/providers` và `/accounts` lặp ở vài tệp `fe/src/api`; một lần `go test -race ./internal/httpapi` đỏ ngẫu nhiên khi tích hợp wave, chạy lại thì xanh.
- `/ccf:check` lần hai: BE sạch, scope không có FAIL, FE `PARTIAL:` (chưa xét thân trang overview, usage, errors, drift, settings và criterion 040 đến 045). Đã sửa `frontend.md` (zod cho form, kiểu TS cho phản hồi API). WARN còn mở: `ViewSwitch` ở `pages/quota/index.tsx` dựng role radio bằng `Button` (nên dùng `toggle-group` qua `ui:add`), bảng `sr-only` thô ở `pages/overview/usage-block.tsx`, form tạo khóa, thêm tài khoản và đổi nhãn chưa dùng zod, lỗi server tiếng Anh hiện nguyên văn, `websearch.go` ngoài `Files to touch` của 035 (thêm `EffectiveCount` để settings dùng chung logic kẹp số kết quả).
- `/ccf:check` lần ba (FE các trang 040 đến 045): không FAIL, không PARTIAL. Đã sửa: `ViewSwitch` dùng `toggle-group`, bảng `sr-only` dùng `Table`, nhãn trục X của Usage, `useUsageAccounts` dùng chung `accountsQuery`. WARN còn mở, cần quyết định thiết kế: liên kết "cần chú ý" ở Tổng quan chưa trỏ sâu tới từng mục quota hay drift (Drift chưa đọc tham số mở một thay đổi), chi tiết lỗi thiếu "số lần thử" vì `upstream_errors` không có trường attempt (mỗi lần thử là một dòng), biểu đồ thanh nhỏ ở `errors/stats-cards.tsx` dựng từ `span` thay vì `Progress` hoặc chart.
