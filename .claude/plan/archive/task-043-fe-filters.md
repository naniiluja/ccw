# Task 043: Trang Bộ lọc (blacklist field)

- **Vertical slice:** FE trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`
- **MCP to use:** shadcn, context7 (react-hook-form, zod)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành tạo, bật tắt và xóa quy tắc blacklist field cho từng provider, và thấy rõ khi server từ chối quy tắc không an toàn.

## Acceptance criteria (verifiable)
- [ ] Danh sách từ `GET /filters` (provider, loại như `schema` hoặc `system`, mẫu, ghi chú, bật tắt), nhóm theo provider, lọc theo provider và loại.
- [ ] Tạo và sửa (`POST /filters`) bằng form (provider chọn từ `GET /providers`, loại, mẫu, ghi chú, kiểm tra bằng `zod`); khi server từ chối (`unsafeFilter` hoặc `400`) hiện thông điệp ngay cạnh field mẫu.
- [ ] Bật tắt lạc quan, xóa (`POST /filters/{id}/delete`) có `AlertDialog` hiện mẫu sẽ bị xóa.
- [ ] Trạng thái tải, rỗng (giải thích blacklist là gì trong một câu), lỗi; nhãn tiếng Việt; responsive 360px, 768px, 1280px.

## Test first (write before implementing)
MSW theo hình dạng thật từ `be/internal/httpapi/filters.go`: tạo thành công; `400` do bộ lọc không an toàn hiện cạnh field; bật tắt hoàn tác khi lỗi; xóa cần xác nhận; lọc theo provider; danh sách rỗng.

## Files to touch
- fe/src/pages/filters/**
- fe/src/api/filters.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Không sửa tệp dùng chung của task 036.
