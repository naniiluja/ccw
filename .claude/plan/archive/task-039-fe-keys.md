# Task 039: Trang API key (tạo, reveal, giới hạn, usage)

- **Vertical slice:** FE trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/logging.md` (secret chỉ hiện một lần, không vào cache hay log)
- **MCP to use:** shadcn, context7 (react-hook-form, zod)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành tạo và quản lý API key (bật tắt, giới hạn model, RPM, hạn dùng, xem usage, xóa) với khóa đầy đủ chỉ hiện đúng một lần khi tạo.

## Acceptance criteria (verifiable)
- [ ] Danh sách từ `GET /keys` (tên, dạng che, bật tắt, model cho phép, hạn dùng, RPM, lần dùng cuối, ngày tạo); bảng trên màn rộng, thẻ trên màn hẹp; lọc theo trạng thái.
- [ ] Tạo khóa (`POST /keys`, tên và danh sách model): khóa đầy đủ hiện trong `Dialog` một lần với ô chỉ đọc, nút sao chép (toast xác nhận), cảnh báo "không xem lại được", không đóng được trước khi người dùng xác nhận đã lưu; khóa chỉ nằm trong state cục bộ của hộp thoại, không vào query cache.
- [ ] Reveal (`POST /keys/{id}/reveal`) có `AlertDialog` xác nhận, hiện khóa trong hộp thoại như trên.
- [ ] Bật tắt (`POST /keys/{id}/active`) lạc quan và hoàn tác; sửa danh sách model (`POST /keys/{id}/models`) bằng bộ chọn nhiều giá trị; sửa giới hạn (`POST /keys/{id}/limits`: RPM và hạn dùng, kiểm tra bằng `zod`, báo lỗi server cạnh field); xóa (`POST /keys/{id}/delete`) có `AlertDialog` nêu tên khóa.
- [ ] Usage theo khóa (`GET /keys/{id}/usage`): biểu đồ cột hoặc đường bằng `chart`, kèm bảng số liệu có nhãn chữ (không chỉ màu).
- [ ] Trạng thái tải, rỗng, lỗi; toast; bàn phím; nhãn tiếng Việt; responsive 360px, 768px, 1280px.

## Test first (write before implementing)
MSW theo hình dạng thật từ `be/internal/httpapi/keys.go`: tạo khóa hiện khóa đầy đủ một lần và không còn trong DOM hay cache sau khi đóng (khẳng định `queryClient.getQueryCache()` không chứa chuỗi khóa); không đóng được trước khi xác nhận; reveal cần xác nhận; hoàn tác khi bật tắt lỗi; giới hạn hợp lệ, RPM âm, hạn dùng quá khứ; usage rỗng và có dữ liệu.

## Files to touch
- fe/src/pages/keys/**
- fe/src/api/keys.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Không sửa tệp dùng chung của task 036; thiếu item thì dừng và báo.
- Mutation tạo khóa dùng `useMutation` với `gcTime: 0` và không đặt kết quả vào `queryClient`; dùng giá trị trả về trực tiếp trong `onSuccess` để mở hộp thoại.
