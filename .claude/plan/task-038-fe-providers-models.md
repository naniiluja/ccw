# Task 038: Trang Provider và Model (định nghĩa, bảng model, xoay vòng, chính sách, Zen)

- **Vertical slice:** FE trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/architecture.md` (tier route ghi)
- **MCP to use:** shadcn, context7 (tanstack-table, react-hook-form)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành quản lý provider (kể cả provider tùy chỉnh), bảng model từng provider, chính sách xoay vòng và chính sách model, và xem phiên Zen.

## Acceptance criteria (verifiable)
- [ ] Danh sách provider từ `GET /providers` (số tài khoản mỗi provider) và `GET /provider-defs`; chọn một provider mở chi tiết với `Tabs`: Model, Xoay vòng, Chính sách model, Định nghĩa.
- [ ] Tab Model: bảng (`@tanstack/react-table`, sắp xếp, lọc, phân trang phía client) từ `GET /providers/{id}/model-table`; bật hoặc tắt hàng loạt (`POST /providers/{id}/models/active`), xóa (`/models/delete`, có `AlertDialog`), thử model (`/models/test`, hiện kết quả từng model); hàng nhiều cột tự thu gọn trên màn hẹp.
- [ ] Tab Xoay vòng: đọc và lưu `GET/POST /providers/{id}/rotation` bằng form có kiểm tra bằng `zod`, báo lỗi từ server.
- [ ] Tab Chính sách model: đọc và lưu `GET/POST /providers/{id}/model-policy`.
- [ ] Tab Định nghĩa: tạo, sửa (`POST /provider-defs`), xóa (`POST /provider-defs/{id}/delete`) provider tùy chỉnh; hiện lỗi `400` của server cạnh field; xóa cần xác nhận.
- [ ] Mục Zen: bảng phiên từ `GET /api/zen/sessions` (hiện khi provider là Zen hoặc có mục riêng), có trạng thái rỗng và lỗi.
- [ ] Trạng thái tải, rỗng, lỗi; toast; bàn phím; nhãn tiếng Việt; responsive 360px, 768px, 1280px.

## Test first (write before implementing)
MSW theo hình dạng thật từ `be/internal/httpapi/accounts.go`, `models.go`, `rotation.go`, `providers.go`: bảng model có dữ liệu, rỗng, lọc, sắp xếp; bật tắt hàng loạt gửi đúng body; xóa cần xác nhận; thử model hiện kết quả; lưu rotation và model-policy; tạo và xóa provider-def; `400` hiện cạnh field.

## Files to touch
- fe/src/pages/providers/**
- fe/src/api/providers.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Không sửa tệp dùng chung của task 036 (`package.json`, lockfile, `components.json`, `fe/src/components/ui/*`, `fe/src/app/routes.tsx`, `fe/src/api/client.ts`); thiếu item thì dừng và báo.
- Đọc hình dạng JSON từ handler thật trước khi viết MSW.
- Data Table theo hướng dẫn shadcn: `add table` cộng `@tanstack/react-table` (đã cài ở 036).
