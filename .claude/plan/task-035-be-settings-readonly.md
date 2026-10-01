# Task 035: GET /api/settings (cấu hình hiệu lực, chỉ đọc)

- **Vertical slice:** BE route đọc cấu hình từ env và auth + test
- **Depends on:** 032
- **Spec refs:** `.claude/rules/architecture.md` (tier `requireToken` cho đọc), `.claude/rules/logging.md` (không lộ secret), `.claude/rules/tech-stack.md` (danh sách biến môi trường), `.claude/rules/testing.md`
- **MCP to use:** context7 (net/http)
- **Gate (must be GREEN before the next slice):** `bash scripts/check.sh` xanh
- **discipline:** on

## Goal (one sentence)
Dashboard đọc được cấu hình hiệu lực mà trước đây chỉ có ở biến môi trường (web search, múi giờ, TTL session, chế độ xác thực), và không bao giờ nhận secret.

## Acceptance criteria (verifiable)
- [ ] `GET /api/settings` đi qua `requireToken` (session, master token hoặc API key đều đọc được, như các route đọc `/api/*` khác).
- [ ] Body: `{"authMode": "password"|"token"|"none", "sessionTtlSeconds": number, "timezone": string, "websearch": {"provider": string, "model": string, "count": number, "url": string, "keySet": boolean}}`; giá trị lấy từ đúng các hàm đang dùng (múi giờ từ `CCW_TZ` như `keys.go`, web search như `websearch.go`, TTL từ `auth.Config`), không đọc env lần hai theo cách khác.
- [ ] Không có giá trị của `CCW_SEARCH_KEY`, `CCW_API_TOKEN`, `CCW_SESSION_KEY`, `CCW_PASSWORD`, `CCW_ANTIGRAVITY_CLIENT_SECRET`, `password_hash` trong body; chỉ có cờ `keySet`. URL web search bị cắt thông tin đăng nhập nếu có (`user:pass@`).
- [ ] Thiếu token khi server có gate trả `401` JSON; `POST /api/settings` không tồn tại (`405` hoặc `404` theo `ServeMux`).
- [ ] Giá trị mặc định đúng khi không đặt biến nào (provider rỗng, `keySet:false`, múi giờ mặc định của `keys.go`).

## Test first (write before implementing)
Ma trận contract: gate bật (không token, token sai, token đúng, session) nhân gate tắt; env web search (không có, có đủ, key rỗng); `CCW_TZ` (rỗng, hợp lệ, không hợp lệ); body quét chuỗi secret đã đặt qua env của test (đặt giá trị dò `canary-secret-value` vào từng biến nhạy cảm và khẳng định không xuất hiện trong body).

## Files to touch
- be/internal/httpapi/settings.go
- be/internal/httpapi/settings_test.go
- be/internal/httpapi/server.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Hợp đồng JSON này là nguồn sự thật cho task 045 (FE): đổi tên trường ở đây thì phải đổi ở đó.
- Đọc cấu hình bằng hàm đã có, không thêm biến global mới (`coding-conventions.md`). Nếu một giá trị chỉ tính được trong lúc xử lý request (ví dụ múi giờ), gọi đúng helper hiện có.
- Chạm `server.go` cùng 033 và 034, nên chạy khác wave.
- Phạm vi mở rộng có chủ ý: `be/internal/websearch/websearch.go` thêm `DefaultCount` và `Config.EffectiveCount()` để `settings.go` dùng chung logic kẹp số kết quả với searcher thật (tiêu chí "không đọc env lần hai theo cách khác"); hành vi của `New` không đổi. Hai test `keys_test.go`, `accounts_crud_test.go` và `quota_claim_test.go` chỉ đổi mong đợi `Location` sang `/ui/login` theo task 034.
