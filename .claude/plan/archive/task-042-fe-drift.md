# Task 042: Trang Drift (thay đổi shape, ack, AI drift review)

- **Vertical slice:** FE trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/architecture.md` (route ghi cần admin)
- **MCP to use:** shadcn, context7 (tanstack-query)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành thấy khi nào một provider đổi shape request hoặc response, xác nhận (ack) thay đổi, và cấu hình AI drift review.

## Acceptance criteria (verifiable)
- [ ] Danh sách từ `GET /drift/changes` với bộ lọc provider, hướng (request hoặc response), chỉ chưa ack, giới hạn; số chưa ack hiển thị nổi bật; bộ lọc phản ánh trên URL.
- [ ] Mỗi thay đổi hiện endpoint, đường dẫn trường, loại cũ và mới, thời điểm, phán quyết AI (`verdict`, độ tin cậy, tự ack); mẫu dữ liệu (`sample`) hiện trong `Sheet` với `ScrollArea`; diff cũ và mới dựng từ `Table`, `Badge`, `ScrollArea` (không tạo primitive mới).
- [ ] Ack từng thay đổi và ack hàng loạt qua `POST /drift/ack` (lạc quan, hoàn tác khi lỗi); `GET /drift/fields` hiển thị trường đang theo dõi theo provider.
- [ ] Cấu hình AI drift review qua `GET/POST /drift/review` (bật, model quyết định, model giải quyết, ngưỡng tự ack, chạy ngay); khởi tạo mẫu qua `POST /api/drift/seed` có `AlertDialog` giải thích tác động.
- [ ] Trạng thái tải, rỗng, lỗi; nhãn tiếng Việt; responsive 360px, 768px, 1280px.

## Test first (write before implementing)
MSW theo hình dạng thật từ `be/internal/httpapi/drift.go`: lọc đổi query; ack hoàn tác khi `500`; ack hàng loạt gửi đúng id; diff hiện cả loại cũ và mới; cấu hình review lưu đúng body; seed cần xác nhận; danh sách rỗng.

## Files to touch
- fe/src/pages/drift/**
- fe/src/api/drift.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Task 044 import `useDriftChanges` từ `fe/src/api/drift.ts` (lấy số chưa ack); giữ tên ổn định.
- Không sửa tệp dùng chung của task 036.
