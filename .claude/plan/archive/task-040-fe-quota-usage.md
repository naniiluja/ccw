# Task 040: Trang Quota và Usage (đồng hồ quota, claim reset, biểu đồ usage)

- **Vertical slice:** FE hai trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`, `dataviz` skill (màu biểu đồ, trục, legend)
- **MCP to use:** shadcn, context7 (recharts, tanstack-query)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành thấy ngay tài khoản nào sắp hết quota, làm mới, claim reset khi được, và xem usage theo ngày, model, tài khoản.

## Acceptance criteria (verifiable)
- [ ] Trang Quota từ `GET /quota` (refresh bằng tham số hỗ trợ): mỗi tài khoản một thẻ gồm provider, gói, nguồn (`api` hoặc `headers`), các cửa sổ quota với thanh tiến độ (`Progress`, `role="progressbar"` có `aria-valuenow`, nhãn chữ kèm phần trăm và thời điểm reset, không chỉ dựa vào màu), cảnh báo khi vượt ngưỡng; lỗi từng tài khoản hiện ngay trên thẻ.
- [ ] Nút làm mới (cưỡng bức làm mới qua `?refresh=`), tự làm mới mỗi 60 giây khi tab đang mở; chế độ xem (thẻ hoặc bảng) lưu qua `GET/POST /ui-settings/quota-view`.
- [ ] Claim reset (`POST /quota/{id}/reset`): chỉ hiện khi `resets` có mục; `AlertDialog` giải thích hậu quả; hiện kết quả thành công, thất bại.
- [ ] Trang Usage từ `GET /usage`: biểu đồ theo ngày (`chart`, màu theo palette đã chọn cho toàn app, trục có nhãn, legend, chú thích số liệu), bộ lọc theo model và tài khoản, bảng chi tiết (ngày, tài khoản, model, token vào, token ra, số request) có sắp xếp.
- [ ] Trạng thái tải, rỗng, lỗi; nhãn tiếng Việt; số liệu `tabular-nums`; responsive 360px, 768px, 1280px (biểu đồ co giãn, bảng cuộn).

## Test first (write before implementing)
MSW theo hình dạng thật từ `be/internal/httpapi/quota.go`, `quota_resets.go`, `accounts.go` (hàm `usage`): thẻ quota hiện phần trăm và `aria-valuenow` đúng; cửa sổ không giới hạn (`unlimited`); lỗi một tài khoản; claim reset có xác nhận và hiện kết quả; chế độ xem được lưu; usage rỗng và có dữ liệu; lọc theo model.

## Files to touch
- fe/src/pages/quota/**
- fe/src/pages/usage/**
- fe/src/api/quota.ts
- fe/src/api/usage.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Task 044 (tổng quan) import hook từ `fe/src/api/quota.ts` và `fe/src/api/usage.ts`; giữ tên hook ổn định (`useQuota`, `useUsage`).
- Biểu đồ dùng biến `var(--chart-N)` của shadcn, không màu cố định; đọc skill `dataviz` trước khi viết mã biểu đồ.
- Không sửa tệp dùng chung của task 036.
