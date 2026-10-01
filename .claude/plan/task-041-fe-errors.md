# Task 041: Trang Lỗi upstream (danh sách, chi tiết, AI error review)

- **Vertical slice:** FE trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/logging.md` (body người dùng chỉ hiện trong chi tiết lỗi đã lưu)
- **MCP to use:** shadcn, context7 (tanstack-query, tanstack-table)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành điều tra lỗi từ provider: lọc, xem thống kê theo nhóm chữ ký, đọc request và response đã lưu, và bật hoặc xem kết quả AI error review.

## Acceptance criteria (verifiable)
- [ ] Danh sách từ `GET /errors` với bộ lọc provider, class (network, timeout, auth, rejected, rate_limit, fake_rate_limit, server, other), signature, status, since, limit; bộ lọc phản ánh trên URL (`searchParams`) để chia sẻ được; phân trang theo `limit` và `since`.
- [ ] Thống kê từ `GET /errors/stats` thành các thẻ và biểu đồ nhỏ theo class và theo provider.
- [ ] Chi tiết từ `GET /errors/{id}` mở trong `Sheet` (màn hẹp toàn màn hình): trạng thái, độ trễ, số lần thử, request và response đã cắt, có `ScrollArea`, nút sao chép, chỉ báo khi body đã bị cắt (64 KiB); làm nổi bật tiêu đề nhạy cảm không được hiện (nếu server đã che).
- [ ] AI error review: đọc và đổi cấu hình qua `GET/POST /errors/review` (bật, model, `minErrors`, replay, chạy ngay), danh sách kết luận từ `GET /errors/verdicts` kèm nguyên nhân đề xuất và mức tin cậy; thao tác ghi dùng đúng tier (session).
- [ ] Làm mới tự động mỗi 30 giây khi tab đang mở, có nút tạm dừng.
- [ ] Trạng thái tải, rỗng ("chưa có lỗi"), lỗi; nhãn tiếng Việt; responsive 360px, 768px, 1280px.

## Test first (write before implementing)
MSW theo hình dạng thật từ `be/internal/httpapi/errlog.go`, `errreview.go`: lọc cập nhật URL và gọi lại với đúng query; danh sách rỗng và có dữ liệu; chi tiết hiện body bị cắt kèm chỉ báo; cấu hình review lưu đúng body; verdict hiện nguyên nhân; polling tạm dừng.

## Files to touch
- fe/src/pages/errors/**
- fe/src/api/errors.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Task 044 (tổng quan) import hook `useErrorStats` và `useErrors` từ `fe/src/api/errors.ts`; giữ tên ổn định.
- Không sửa tệp dùng chung của task 036.
