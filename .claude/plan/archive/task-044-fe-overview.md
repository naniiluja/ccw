# Task 044: Trang Tổng quan (dashboard chủ)

- **Vertical slice:** FE trang tổng hợp từ các hook đã có + test hành vi bằng MSW
- **Depends on:** 036, 040, 041, 042
- **Spec refs:** `.claude/rules/testing.md`, `dataviz` skill
- **MCP to use:** shadcn, context7 (tanstack-query, recharts)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Trang chủ cho thấy trong một màn hình: tình trạng tài khoản và quota, lỗi gần đây, thay đổi drift chưa xác nhận và usage gần nhất, kèm đường tắt vào từng trang.

## Acceptance criteria (verifiable)
- [ ] Hàng thẻ số liệu: số tài khoản (và số đang lỗi), tài khoản gần hết quota (từ `useQuota`), lỗi 24 giờ qua (từ `useErrorStats`), drift chưa ack (từ `useDriftChanges`); mỗi thẻ là liên kết tới trang tương ứng.
- [ ] Biểu đồ usage 14 ngày (từ `useUsage`) với nhãn trục, legend và bảng số liệu thay thế cho trình đọc màn hình.
- [ ] Danh sách "cần chú ý": tài khoản quota thấp, lỗi mới nhất, thay đổi drift mới nhất, mỗi mục có liên kết sâu.
- [ ] Mỗi khối tải độc lập (một khối lỗi không làm hỏng khối khác), có `Skeleton` riêng và nút thử lại.
- [ ] Chỉ import hook từ `fe/src/api/{accounts,quota,usage,errors,drift}.ts`; không gọi API trực tiếp, không tạo hook trùng lặp.
- [ ] Nhãn tiếng Việt; responsive 360px (một cột), 768px (hai cột), 1280px (lưới đầy đủ).

## Test first (write before implementing)
MSW: tất cả khối có dữ liệu; một khối lỗi `500` còn lại vẫn hiện; tất cả rỗng hiện hướng dẫn thêm tài khoản đầu tiên; liên kết sâu trỏ đúng route; thẻ quota thấp sắp xếp theo mức dùng.

## Files to touch
- fe/src/pages/overview/**
- fe/src/api/overview.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- `fe/src/api/overview.ts` chỉ chứa phần gộp thuần (chọn tài khoản quota thấp, đếm), không gọi mạng. Task phụ thuộc 040, 041, 042 để dùng chung hook, giữ DRY.
- Không sửa tệp dùng chung của task 036.
