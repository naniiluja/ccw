# Task 034: Route session trả JSON cho SPA (content negotiation)

- **Vertical slice:** BE auth + accounts handlers + route whoami + test
- **Depends on:** 032
- **Spec refs:** `.claude/rules/architecture.md` (Auth tiers, không nới lỏng), `.claude/rules/error-handling.md` (envelope `{"error": ...}`), `.claude/rules/testing.md` (kỷ luật contract)
- **MCP to use:** context7 (net/http, content negotiation)
- **Gate (must be GREEN before the next slice):** `bash scripts/check.sh` xanh; mọi test `auth_test.go`, `adminroutes_test.go`, `accounts_test.go` cũ không đổi kết quả
- **discipline:** on

## Goal (one sentence)
Khi request mang `Accept: application/json`, các route session trả JSON thay vì `302`, và SPA có `GET /api/session` để biết đã đăng nhập chưa; client không gửi header đó (trình duyệt điều hướng, curl) giữ hành vi cũ.

## Acceptance criteria (verifiable)
- [ ] `requireSession`: thiếu hoặc hết hạn session, có `Accept` chứa `application/json` thì trả `401` với `{"error": "..."}`; không có thì `302`, đích đổi từ `/login` thành `/ui/login` (vì `GET /login` không có route).
- [ ] `POST /login` với `Accept: application/json`: đúng mật khẩu trả `200 {"ok":true}` và đặt cả hai cookie như cũ; sai trả `401`, vượt ngân sách trả `429` kèm `Retry-After` (đã là JSON). Không có `Accept` thì vẫn `302` về `/`. Ngân sách `LoginGuard` và cách tính `Reserve/Refund` không đổi.
- [ ] `POST /logout` với `Accept: application/json` trả `200 {"ok":true}` và xóa cookie; không có thì `302` về `/ui/login`.
- [ ] `POST /accounts` và `POST /accounts/{id}/delete` với `Accept: application/json` trả `200 {"ok":true}` thay vì `302`.
- [ ] `GET /api/session` (không yêu cầu đăng nhập, không lộ secret): `200 {"authRequired":bool,"authenticated":bool,"admin":bool}`; khi server không gate (`a.auth == nil`) thì `authRequired:false, authenticated:true, admin:true`.
- [ ] Không route nào bị nới tier: `adminroutes_test.go` xanh nguyên vẹn; `guardRequest` chặn ghi cross-site như cũ cho route mới.
- [ ] Không log hay trả mật khẩu, cookie, token.

## Test first (write before implementing)
Ma trận contract cho từng route đổi: (không Accept, Accept JSON) nhân (không phiên, phiên hợp lệ, phiên hết hạn) cho `requireSession`; `/login` (rỗng, sai, đúng, vượt ngân sách) nhân (có, không có `Accept`); `/logout`; `createAccount` và `deleteAccount` một nhánh thành công mỗi chế độ; `GET /api/session` (gate bật có phiên, gate bật không phiên, gate tắt); `Accept: text/html,application/xhtml+xml,*/*` (trình duyệt) vẫn `302`.

## Files to touch
- be/internal/httpapi/server.go
- be/internal/httpapi/auth.go
- be/internal/httpapi/accounts.go
- be/internal/httpapi/session.go
- be/internal/httpapi/session_test.go
- be/internal/httpapi/auth_test.go
- be/internal/httpapi/accounts_test.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Hàm nhỏ `wantsJSON(r)` đặt trong `session.go`; chỉ cần `strings.Contains` trên `Accept` và không coi `*/*` là JSON.
- Các chỗ `http.Redirect` hiện có: `accounts.go` (hai chỗ), `auth.go` (ba chỗ), `server.go` (`requireSession`). Gọi `grep -n 'http.Redirect' be/internal/httpapi/*.go` trước khi sửa. Test cũ khẳng định `Location: /login` phải được cập nhật thành `/ui/login`; đó là thay đổi có chủ đích.
- Cookie `ccw_session` vẫn `Secure: true` cứng (`auth.go`); không đổi ở task này (Safari dev là rủi ro đã ghi trong kế hoạch).
- Task chạm `server.go` cùng 033 và 035 nên chạy khác wave.
