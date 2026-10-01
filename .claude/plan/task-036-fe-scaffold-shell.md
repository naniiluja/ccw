# Task 036: Dựng fe/ và khung ứng dụng (shell, router, đăng nhập, theme)

- **Vertical slice:** FE toolchain + design tokens + shell + auth flow + CI gate
- **Depends on:** 032
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/tech-stack.md`, `CLAUDE.md` (UI là việc mới, chưa chọn stack trước task này)
- **MCP to use:** shadcn (nếu khởi động được; nếu không, dùng CLI `pnpm dlx shadcn@<phiên bản ghim>`), context7 (react-router, tanstack-query, vite, vitest, msw)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`; `bash scripts/check.sh` xanh (đã gồm cổng FE)
- **discipline:** off

## Goal (one sentence)
`fe/` chạy được với khung ứng dụng hoàn chỉnh (đăng nhập, điều hướng, theme, command palette, trạng thái tải và lỗi) và đủ 12 route, trong đó các trang chức năng là trang giữ chỗ để các task 037 đến 045 điền song song.

## Acceptance criteria (verifiable)
- [ ] `fe/` dùng pnpm, Vite, React, TypeScript strict, Tailwind v4, `shadcn init` kiểu Radix (khớp `comp-163`); phiên bản CLI shadcn được ghim trong script `ui:add`.
- [ ] `fe/components.json` có `"registries": {"@originui": "https://coss.com/origin/r/{name}.json"}`; `@originui/comp-163` được cài thành công và build xanh (kiểm việc nó ghi đè `radio-group` và import `cn`).
- [ ] Cài một lần mọi shadcn item cần cho cả kế hoạch: sidebar, breadcrumb, button, input, label, textarea, select, checkbox, switch, radio-group, tabs, card, badge, table, dialog, alert-dialog, sheet, drawer, dropdown-menu, popover, tooltip, command, skeleton, progress, separator, scroll-area, sonner, chart, alert, empty (nếu có), pagination (nếu có), form hoặc field, calendar không cần. Thư viện kèm theo: `@tanstack/react-query`, `@tanstack/react-table`, `react-router`, `react-hook-form`, `zod`, `@hookform/resolvers`, `recharts`, `@remixicon/react`, `lucide-react`. Các task 037 đến 045 không sửa `package.json` hay `pnpm-lock.yaml`.
- [ ] Mọi tệp `fe/src/components/ui/*` do CLI sinh; không tệp nào ngoài `ui/` import trực tiếp `@radix-ui/*` hay `@base-ui*` (test tự động); ngoài `ui/` chỉ có component ghép từ primitive shadcn, đặt ở `fe/src/components/app/`.
- [ ] `vite.config.ts`: `base` là `/ui/` khi build và `/` khi dev; proxy dev cho `/api`, `/login`, `/logout`, `/accounts`, `/providers`, `/provider-defs`, `/keys`, `/filters`, `/quota`, `/usage`, `/errors`, `/drift`, `/oauth`, `/ui-settings`, `/account-tests`, `/v1`, `/mcp` tới `http://127.0.0.1:20130` và viết lại header `Origin` thành origin của upstream (vì `guardRequest` so `Origin` với `Host`); `build.outDir` là `fe/dist`.
- [ ] Router (`react-router`, `basename` lấy từ `import.meta.env.BASE_URL`) có đúng 12 route khai báo trong MỘT bảng `fe/src/app/routes.tsx`: `/` (tổng quan), `/accounts`, `/providers`, `/keys`, `/quota`, `/usage`, `/errors`, `/drift`, `/filters`, `/settings`, `/login`, `*` (không tìm thấy). Mỗi trang chức năng là `fe/src/pages/<tên>/index.tsx` tải lười, hiện trang giữ chỗ dựng từ `Empty`/`Card` kèm tiêu đề tiếng Việt.
- [ ] Điều hướng sidebar sinh từ bảng route; sidebar thu gọn thành icon, và trên màn nhỏ là `Sheet`; có breadcrumb; thanh trên có nút đổi theme (sáng, tối, theo hệ thống, lưu `localStorage`, script đặt theme trước khi render để không nháy màu) và nút đăng xuất.
- [ ] Command palette (`Command`, Cmd/Ctrl+K) nhảy tới mọi trang và đổi theme; phím tắt đơn ký tự không dùng (WCAG 2.1.4).
- [ ] `fe/src/api/client.ts`: hàm `fetch` bọc, luôn gửi `Accept: application/json`, `credentials: 'same-origin'`, form-encoded cho `POST /login` và `POST /accounts`, JSON cho phần còn lại; lỗi chuẩn hóa từ `{"error": "..."}`; `401` làm `QueryCache` chuyển về `/login` (kèm `redirect`). `QueryClient` có `staleTime` mặc định, `retry:false` cho 4xx.
- [ ] Bảo vệ route: dùng `GET /api/session`; chưa đăng nhập thì chuyển `/login`; khi `authRequired:false` thì vào thẳng.
- [ ] Trang đăng nhập (`/login`, field và form shadcn): trạng thái đang gửi, lỗi sai mật khẩu, lỗi `429` kèm đếm ngược `Retry-After`, cho dán mật khẩu (WCAG 3.3.8).
- [ ] Thiết kế: token màu và bán kính khai báo ở `fe/src/index.css` (một màu nhấn duy nhất, nền trung tính, cả hai chế độ), font hệ thống hoặc Inter qua `@fontsource`, số liệu dùng `tabular-nums`, chuyển động tinh tế qua `tw-animate-css` và tôn trọng `prefers-reduced-motion`, focus ring rõ, vùng bấm tối thiểu 24x24px.
- [ ] Responsive: layout dùng được từ 360px (một cột, sidebar thành Sheet, bảng cuộn ngang trong `ScrollArea` hoặc chuyển thành thẻ), 768px và 1280px trở lên.
- [ ] `scripts/check.sh` chạy cổng FE (`pnpm install --frozen-lockfile`, lint, typecheck, test, build) khi có `fe/package.json`, cùng cách báo cổng đỏ như cổng Go; workflow có job `frontend` (checkout, `pnpm/action-setup` trước `actions/setup-node` với `cache: pnpm`, chạy cùng bốn bước) và job `publish` vẫn `needs: [test, frontend]`.
- [ ] Spike đã ghi trong ghi chú commit hoặc `fe/README` không tạo; kết quả ghi vào phần Notes của task này: proxy dev đăng nhập được qua Chrome, `Origin` được viết lại, `comp-163` cài sạch.

## Test first (write before implementing)
Vitest + Testing Library + MSW (`onUnhandledRequest: 'error'`, jsdom, polyfill `ResizeObserver`, `matchMedia`, `PointerEvent`, `scrollIntoView`, `hasPointerCapture` trong `setupFiles`): shell hiện đủ mục điều hướng từ bảng route; đăng nhập đúng chuyển vào `/`; sai hiện lỗi; `429` hiện đếm ngược; `401` ở bất kỳ query nào chuyển về `/login`; `authRequired:false` vào thẳng; test biên giới `components/ui` (quét import); test bảng route có đúng 12 mục và mỗi mục có tệp trang; test đổi theme ghi `localStorage` và đặt class `dark`.

## Files to touch
- fe/**
- scripts/check.sh
- .github/workflows/github-packages.yml

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- shadcn: `init -t vite`, Tailwind v4 không cần `tailwind.config`, registry có namespace (ui.shadcn.com/docs/components-json, /installation/vite). Data Table là hướng dẫn cộng `@tanstack/react-table`, không phải registry item.
- `originui.com` trả 301 sang `coss.com`; URL đúng là `https://coss.com/origin/r/{name}.json`. `comp-163` là nhóm radio dạng thẻ phụ thuộc `radio-group` của coss và `@remixicon/react`.
- Vite: `base` có điều kiện, khóa proxy phải mang tiền tố base nếu `base` khác `/` (vite.dev/config/server-options). React Router `basename`.
- TanStack Query v5: `refetchInterval` cho polling, `invalidateQueries` ở `onSettled`; `isPending` thay `isLoading`.
- Cookie `ccw_session` đặt `Secure: true` cứng; Chrome và Firefox lưu được trên `http://127.0.0.1`, Safari thì không. Kiểm bằng Claude in Chrome.
- Shadcn MCP có thể lỗi cache `npx` (`Cannot find module '../llhttp/llhttp-wasm.js'`); dùng CLI trực tiếp, không xóa cache hệ thống.
- Mỗi trang tự giữ chuỗi tiếng Việt trong thư mục của nó, không có từ điển chung (tránh tệp dùng chung giữa các task song song). UI tiếng Việt, không i18n.
- Nếu một item shadcn cần cho trang nào đó chưa có, thêm vào danh sách ở đây thay vì để task trang sửa `package.json`.
