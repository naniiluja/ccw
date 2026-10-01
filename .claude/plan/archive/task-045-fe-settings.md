# Task 045: Trang Cài đặt (chỉ đọc) và thông tin kết nối

- **Vertical slice:** FE trang + hook API + test hành vi bằng MSW
- **Depends on:** 036
- **Spec refs:** `.claude/rules/tech-stack.md` (danh sách biến môi trường), `.claude/rules/testing.md`, `docs/clients.md`
- **MCP to use:** shadcn, context7 (tanstack-query)
- **Gate (must be GREEN before the next slice):** `pnpm -C fe lint && pnpm -C fe typecheck && pnpm -C fe test && pnpm -C fe build`
- **discipline:** off

## Goal (one sentence)
Người vận hành thấy cấu hình hiệu lực chỉ đặt được bằng biến môi trường (web search, múi giờ, TTL session, chế độ xác thực), biết biến nào điều khiển, và có sẵn thông tin để nối client vào `/v1` và `/mcp`.

## Acceptance criteria (verifiable)
- [ ] Hợp đồng đọc từ `GET /api/settings` do task 035 định nghĩa: `{"authMode","sessionTtlSeconds","timezone","websearch":{"provider","model","count","url","keySet"}}`; hook `useSettings` ở `fe/src/api/settings.ts`.
- [ ] Mỗi giá trị hiện kèm tên biến môi trường điều khiển nó (`CCW_SEARCH_PROVIDER`, `CCW_SEARCH_MODEL`, `CCW_SEARCH_COUNT`, `CCW_SEARCH_URL`, `CCW_SEARCH_KEY` chỉ hiện "đã đặt" hoặc "chưa đặt", `CCW_TZ`, `CCW_SESSION_TTL`, `CCW_PASSWORD`/`CCW_API_TOKEN` cho chế độ xác thực) và dòng "chỉ đọc, đổi bằng biến môi trường rồi khởi động lại"; không có nút ghi; không hiện hay yêu cầu secret.
- [ ] Mục "Kết nối": base URL `/v1` lấy từ `window.location.origin`, URL máy chủ MCP `/mcp`, đoạn cấu hình mẫu cho Claude Code, Codex và client OpenAI-compatible lấy đúng từ `docs/clients.md`, mỗi đoạn có nút sao chép; ghi chú dùng API key tạo ở trang API key.
- [ ] Mục "Phiên và bảo mật": TTL phiên, chế độ xác thực, nhắc rằng đổi mật khẩu chỉ qua `-reset-password`.
- [ ] Trạng thái tải, lỗi (kể cả `404` khi server chưa có route, hiện thông báo rõ), nhãn tiếng Việt; responsive 360px, 768px, 1280px.

## Test first (write before implementing)
MSW dựng đúng hợp đồng của task 035: hiện đủ giá trị và tên biến; `keySet:false` và `true` hiện đúng chữ; không có phần tử nhập liệu hay nút lưu trong DOM; sao chép đoạn cấu hình ghi vào clipboard (mock) và hiện toast; lỗi `500` hiện thử lại.

## Files to touch
- fe/src/pages/settings/**
- fe/src/api/settings.ts

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Không phụ thuộc task 035 để bắt đầu: hợp đồng đã cố định ở đó; tích hợp thật được kiểm ở task 046.
- Không sửa tệp dùng chung của task 036.
