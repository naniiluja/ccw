## Origin: Dashboard ccw, tách be/ và fe/

Nguồn: `/Users/naniiluja/.claude/plans/chia-theo-fe-v-fizzy-oasis.md`. Mục tiêu: Go vào `be/`, giao diện vào `fe/`, SPA phục vụ dưới `/ui/` và nhúng vào binary. Kỷ luật test mức contract: bật cho 033, 034, 035; tắt cho các task còn lại. Task 032 chạy một mình (chạm mọi đường dẫn). 033 và 036 chạy cùng wave vì `be/` và `fe/` rời nhau. Ba task BE (033, 034, 035) cùng chạm `be/internal/httpapi/server.go` nên khác wave. Các trang FE 037 đến 043 và 045 chạy song song sau 036, 044 chạy sau 040, 041, 042 để dùng chung hook.

## Task backlog: dashboard (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 032 | Tách repo: Go vào `be/`, script, CI và spec theo layout mới | repo layout + scripts + CI | `bash scripts/check.sh` xanh, 14 package, chỉ rename trong diff | — | done |
| 033 | Package `webui` và route `/ui/` (SPA nhúng, `GET /` chuyển hướng) | BE webui + httpapi | `bash scripts/check.sh` xanh, ma trận contract của `Handler` | 032 | done |
| 034 | Route session trả JSON khi `Accept: application/json`, thêm `GET /api/session` | BE auth + accounts | `bash scripts/check.sh` xanh, ma trận route session | 032 | done |
| 035 | `GET /api/settings` chỉ đọc, không lộ secret | BE httpapi | `bash scripts/check.sh` xanh, test không lộ secret | 032 | done |
| 036 | `fe/`: Vite, shadcn, Origin UI, shell, router 12 route, đăng nhập, theme, cổng FE trong CI | FE toolchain + shell + CI | `pnpm -C fe lint typecheck test build` xanh, `bash scripts/check.sh` xanh | 032 | done |
| 037 | Trang Tài khoản (thêm, OAuth, kiểm tra, xóa) | FE | cổng FE xanh, test MSW | 036 | done |
| 038 | Trang Provider và Model (định nghĩa, bảng model, xoay vòng, chính sách, Zen) | FE | cổng FE xanh, test MSW | 036 | done |
| 039 | Trang API key (khóa hiện một lần, giới hạn, usage) | FE | cổng FE xanh, test MSW | 036 | done |
| 040 | Trang Quota và Usage (đồng hồ, claim reset, biểu đồ) | FE | cổng FE xanh, test MSW | 036 | done |
| 041 | Trang Lỗi upstream (lọc, chi tiết, AI error review) | FE | cổng FE xanh, test MSW | 036 | done |
| 042 | Trang Drift (thay đổi shape, ack, AI drift review) | FE | cổng FE xanh, test MSW | 036 | done |
| 043 | Trang Bộ lọc (blacklist field) | FE | cổng FE xanh, test MSW | 036 | done |
| 044 | Trang Tổng quan (gộp từ các hook đã có) | FE | cổng FE xanh, test MSW | 036, 040, 041, 042 | done |
| 045 | Trang Cài đặt chỉ đọc và thông tin kết nối | FE | cổng FE xanh, test MSW | 036 | done |
| 046 | Đóng gói SPA vào binary và npm, đồng bộ spec, kiểm đầu cuối bằng trình duyệt | build + CI + spec + e2e | `bash scripts/ui-build.sh && bash scripts/check.sh` xanh, kiểm bằng Claude in Chrome | 033, 034, 035, 036, 037, 038, 039, 040, 041, 042, 043, 044, 045 | done |

## Origin: Onboarding gaps and lean ccw

Ba task không có liên kết dữ liệu và không chung file, nên `/ccf:cook` chạy được song song trong một wave.

## Task backlog (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 001 | CI chạy `go vet` và `go test -race` trước khi publish | CI workflow | `go vet ./...` và `go test ./...` xanh trên máy, workflow đọc đúng trình tự bằng test đọc YAML | — | done |

Hai task còn lại của đợt onboarding (đánh số lại thành 030 và 031) nằm cuối bảng bên dưới, vì chúng phải chạy sau các task xóa, gộp và tách.

## Origin: Tinh gọn source ccw

Nguồn: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md`. Mục tiêu: giảm số file và độ phức tạp, xóa tính năng không cần, đổi đăng nhập sang mật khẩu, và ép bộ luật mới bằng CI. Không tạo package hay interface mới. Thay thế kế hoạch chia package trước đó (task 004 đến 013 cũ nằm ở `.claude/plan/archive/`, bị thay thế, không phải đã xong). Kỷ luật test mức contract: bật cho 018, 023, 024; tắt cho các task chỉ xóa hoặc gộp file.

## Task backlog: tinh gọn source (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 014 | Xóa contract lab và mọi thứ liên quan đến switcher (kèm cờ `trusted` của API key) | store + contract + httpapi + cmd + docs | `grep` switcher và contract lab rỗng; test hiện có xanh | — | done |
| 015 | Xóa notify (Telegram, webhook) | store + httpapi | `grep` telegram và webhook rỗng; test review và OAuth xanh | 014 | done |
| 016 | Xóa ranking (bảng xếp hạng model) | httpapi | `grep` arena và ranking rỗng; `/v1/models` đủ trường cũ | 015 | done |
| 017 | Dọn code chết và lint (`provider.Generic`, `zen.SystemOneNames`, hai cảnh báo test) | provider + zen + httpapi | `staticcheck` xanh, `deadcode` rỗng | 014 | done |
| 018 | Đăng nhập bằng mật khẩu thay TOTP (`CCW_PASSWORD`, tự sinh lần đầu, `-reset-password`) | auth + cmd + httpapi | ma trận `CheckPassword`, DB chỉ chứa bản băm, 401 và 429 đúng | 014 | done |
| 019 | Chuyển loginguard vào `internal/auth` | auth + httpapi | test loginguard xanh ở `auth`, ngưỡng không đổi | 018 | done |
| 020 | Gộp file httpapi nhóm A (server, auth, oauth, tài khoản, key, filter) | httpapi | `go test -race ./...` xanh, một `// Package httpapi` | 016, 019 | done |
| 021 | Gộp file httpapi nhóm B (model, proxy, provider adapter, web search) | httpapi | `go test -race ./...` xanh, file dưới 1000 dòng | 020 | done |
| 022 | Gộp quota, drift và heal | httpapi + translate | `go test -race ./...` xanh | 021 | done |
| 023 | Tách `failover`, `v1`, `relayVia` thành các bước tên rõ | httpapi (refactor) | `go test -race -count=3` xanh, golden Zen không đổi, gocyclo dưới hoặc bằng 30 | 021 | done |
| 024 | Tách `claimReset`, `claudeResetRows`, `quotaFor`, `newServer` | httpapi (refactor) | `go test -race ./...` xanh, gocyclo dưới hoặc bằng 30 | 022 | done |
| 025 | Tách translate phía request | translate | `go test -race ./internal/translate/` xanh | 022 | done |
| 026 | Tách translate phía response và stream | translate | `go test -race` xanh, golden Zen không đổi | 025 | done |
| 027 | Tách các hàm còn vượt 30 | httpapi + provider | `gocyclo -over 30` rỗng | 023, 024, 026 | done |
| 028 | Ép luật bằng công cụ trong CI và `scripts/check.sh` | CI + config + test kiến trúc | `scripts/check.sh` xanh, cố ý vi phạm làm đúng cổng đỏ | 001, 027 | done |
| 029 | Bộ luật mới và đồng bộ spec | spec | `CLAUDE.md` dưới 200 dòng và 12KB, mỗi luật ghi cách kiểm | 028 | done |
| 030 | Đánh số phiên bản schema bằng `PRAGMA user_version` thay cho nuốt lỗi `duplicate column` (cũ là task 002) | store | `go test ./internal/store/` xanh, gồm test nâng cấp DB cũ | 014 | done |
| 031 | `log/slog` có request ID xuyên suốt `/v1` và `IdleTimeout` cho server (cũ là task 003) | httpapi + cmd | `go test ./internal/httpapi/ ./cmd/ccw/` xanh, test khẳng định dòng log mang `req_id` | 027, 019 | done |

## Ngoài plan
- `README.md`, `SECURITY.md`, `docs/*.md` đã bị xóa có chủ đích; các test và bước đóng gói npm phụ thuộc vào chúng đã được gỡ, `go test ./...` xanh.
- Mã hóa cột secret trong SQLite là một quyết định về mô hình đe dọa, chưa có yêu cầu, nên chưa thành task.
