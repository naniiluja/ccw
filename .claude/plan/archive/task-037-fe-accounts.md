# Task 037: Trang Tài khoản (danh sách, thêm, OAuth, kiểm tra)

- **Vertical slice:** FE trang + hooks API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/testing.md`, `.claude/rules/logging.md` (không đưa secret vào log hay cache)
- **MCP to use:** shadcn (tra markup `comp-163`), context7 (tanstack-query, react-hook-form)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành xem, thêm (khóa API hoặc OAuth), đổi nhãn, bật làm tài khoản chính, kiểm tra và xóa tài khoản provider, đầy đủ và không cần rời trang.

## Acceptance criteria (verifiable)
- [ ] Danh sách từ `GET /accounts` (id, provider, nhãn, trạng thái active, standby, baseUrl) dưới dạng bảng trên màn rộng và thẻ trên màn hẹp; có lọc theo provider và tìm theo nhãn; không bao giờ hiện secret.
- [ ] Nút "Thêm tài khoản" mở `Sheet`/`Dialog`; bước đầu là bộ chọn kiểu bằng `@originui/comp-163` (kiểu: khóa API, đăng nhập OAuth, thiết bị), sau đó form theo `provider.Setup` (`account` cần account id và khóa, `oauth` đi luồng OAuth, còn lại cần khóa và base URL tùy chọn); gửi `POST /accounts` dạng form-encoded, báo lỗi `400` ngay trên field.
- [ ] Luồng OAuth: `POST /oauth/{provider}/start` trả URL hoặc thông tin thiết bị; UI mở URL ở tab mới, có ô dán URL hoặc mã rồi `POST /oauth/{provider}/finish`; với GitHub device flow hiện mã người dùng, tự poll `POST /oauth/github/poll` theo `interval`, dừng khi đóng hộp thoại, báo hết hạn.
- [ ] Đổi nhãn (`POST /accounts/{id}/label`), đặt active (`POST /accounts/{id}/active`) với cập nhật lạc quan và hoàn tác khi lỗi, kiểm tra (`POST /accounts/{id}/test`, hiện kết quả và thời gian), xem model của tài khoản (`GET /accounts/{id}/models`), xóa (`POST /accounts/{id}/delete`) có `AlertDialog` nêu rõ tên tài khoản.
- [ ] Kết quả kiểm tra tự động từ `GET /account-tests` hiện thành huy hiệu trạng thái trên từng dòng, làm mới định kỳ 30 giây khi tab đang mở.
- [ ] Trạng thái tải (`Skeleton`), rỗng (hướng dẫn thêm tài khoản đầu tiên), lỗi (có thử lại); toast Sonner cho thành công và thất bại; bàn phím điều khiển được mọi thao tác; nhãn tiếng Việt.
- [ ] Không secret nào vào query cache, log hay URL; ô khóa là `type="password"` và được xóa khỏi state sau khi gửi.
- [ ] Responsive 360px, 768px, 1280px (kiểm bằng Claude in Chrome).

## Test first (write before implementing)
Test hành vi với MSW dựng từ hình dạng thật của handler (đọc `be/internal/httpapi/accounts.go`, `oauth.go`): danh sách có dữ liệu, rỗng, lỗi `500`; thêm tài khoản khóa API thành công và `400`; chọn kiểu bằng bàn phím (`comp-163`); OAuth dán mã rồi hoàn tất; GitHub device poll đến khi có kết nối; đặt active hoàn tác khi `500`; xóa cần xác nhận; kiểm tra hiện kết quả.

## Files to touch
- fe/src/pages/accounts/**
- fe/src/api/accounts.ts
- fe/src/api/oauth.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Chỉ ghép primitive đã cài ở task 036; không sửa `package.json`, `pnpm-lock.yaml`, `components.json` hay `fe/src/components/ui/*`. Thiếu item thì dừng và báo để bổ sung vào 036.
- Component ghép nghiệp vụ đặt trong `fe/src/pages/accounts/` hoặc `fe/src/components/app/` (chỉ khi thật sự dùng lại), dựng hoàn toàn từ primitive shadcn.
- Hình dạng JSON lấy từ handler thật, không đoán; ghi chú sai khác (nếu có) vào phần Notes.
- Dùng `useMutation` với `onSettled: invalidateQueries`; optimistic chỉ cho đặt active và đổi nhãn.
