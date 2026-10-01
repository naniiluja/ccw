# Implementation Plan — ccw

> **Execution rule: VERTICAL SLICES, RUN IN WAVES.** `/ccf:cook` runs tasks with no link between them (no `Depends on`, no shared file, no shared hotspot) at the same time, each in its own worktree.
> Each task is a thin tracer-bullet that crosses all the layers it touches (DB + service + UI), NOT a horizontal "all-DB-then-all-API" phase, so integration is proven early.
> A task starts only after every task in its `Depends on` has a **GREEN gate** and is merged; each wave's merged result is tested before the next wave starts.
> The `in-progress`/`in-review` status is read by the session-start hook to re-load context after compact, keep status up to date.

## Milestones
- M1: đóng các chỗ lệch best practice mà researcher và kiểm chứng bằng lệnh đã xác nhận (CI không chạy test, migration nuốt lỗi, log không có request ID, thiếu `IdleTimeout`).

> Status: `todo` / `in-progress` / `in-review` / `done` / `blocked`. Lifecycle: `todo → in-progress → in-review → done`. A task becomes `in-review` once its code+test are complete and merged (by `/ccf:cook`, or implemented directly in the session); only `/ccf:updatespec` writes `done` after `/ccf:check` passes.
> Write the status as a **bare word**, no `**bold**` around it. Emphasis carries no information and the status is matched as a whole word.
> Per-task detail in `task-NNN-*.md`.

> **Keep this file to the CURRENT iteration.** When every task of an iteration is `done`, `/ccf:updatespec` moves its sections verbatim into `.claude/plan/ARCHIVE.md` and `git mv`s its `task-NNN-*.md` files into `.claude/plan/archive/`. A closed row left here is counted as live work by the session-start and Stop hooks, while DELETING the history would lose a real record of what shipped and why. So archive, never delete.

<!-- Keep the status guidance ABOVE this line, never below "## Origin": archive retirement groups content from one "## Origin" heading to the next, and only text before the FIRST heading is never cut into ARCHIVE.md. -->

## Origin: Onboarding gaps and lean ccw

Ba task không có liên kết dữ liệu và không chung file, nên `/ccf:cook` chạy được song song trong một wave.

## Task backlog (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 001 | CI chạy `go vet` và `go test -race` trước khi publish | CI workflow | `go vet ./...` và `go test ./...` xanh trên máy, workflow đọc đúng trình tự bằng test đọc YAML | — | in-review |

Hai task còn lại của đợt onboarding (đánh số lại thành 030 và 031) nằm cuối bảng bên dưới, vì chúng phải chạy sau các task xóa, gộp và tách.

## Origin: Tinh gọn source ccw

Nguồn: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md`. Mục tiêu: giảm số file và độ phức tạp, xóa tính năng không cần, đổi đăng nhập sang mật khẩu, và ép bộ luật mới bằng CI. Không tạo package hay interface mới. Thay thế kế hoạch chia package trước đó (task 004 đến 013 cũ nằm ở `.claude/plan/archive/`, bị thay thế, không phải đã xong). Kỷ luật test mức contract: bật cho 018, 023, 024; tắt cho các task chỉ xóa hoặc gộp file.

## Task backlog: tinh gọn source (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 014 | Xóa contract lab và mọi thứ liên quan đến switcher (kèm cờ `trusted` của API key) | store + contract + httpapi + cmd + docs | `grep` switcher và contract lab rỗng; test hiện có xanh | — | in-review |
| 015 | Xóa notify (Telegram, webhook) | store + httpapi | `grep` telegram và webhook rỗng; test review và OAuth xanh | 014 | in-review |
| 016 | Xóa ranking (bảng xếp hạng model) | httpapi | `grep` arena và ranking rỗng; `/v1/models` đủ trường cũ | 015 | in-review |
| 017 | Dọn code chết và lint (`provider.Generic`, `zen.SystemOneNames`, hai cảnh báo test) | provider + zen + httpapi | `staticcheck` xanh, `deadcode` rỗng | 014 | in-review |
| 018 | Đăng nhập bằng mật khẩu thay TOTP (`CCW_PASSWORD`, tự sinh lần đầu, `-reset-password`) | auth + cmd + httpapi | ma trận `CheckPassword`, DB chỉ chứa bản băm, 401 và 429 đúng | 014 | in-review |
| 019 | Chuyển loginguard vào `internal/auth` | auth + httpapi | test loginguard xanh ở `auth`, ngưỡng không đổi | 018 | in-review |
| 020 | Gộp file httpapi nhóm A (server, auth, oauth, tài khoản, key, filter) | httpapi | `go test -race ./...` xanh, một `// Package httpapi` | 016, 019 | todo |
| 021 | Gộp file httpapi nhóm B (model, proxy, provider adapter, web search) | httpapi | `go test -race ./...` xanh, file dưới 1000 dòng | 020 | todo |
| 022 | Gộp quota, drift và heal | httpapi + translate | `go test -race ./...` xanh | 021 | todo |
| 023 | Tách `failover`, `v1`, `relayVia` thành các bước tên rõ | httpapi (refactor) | `go test -race -count=3` xanh, golden Zen không đổi, gocyclo dưới hoặc bằng 30 | 021 | todo |
| 024 | Tách `claimReset`, `claudeResetRows`, `quotaFor`, `newServer` | httpapi (refactor) | `go test -race ./...` xanh, gocyclo dưới hoặc bằng 30 | 022 | todo |
| 025 | Tách translate phía request | translate | `go test -race ./internal/translate/` xanh | 022 | todo |
| 026 | Tách translate phía response và stream | translate | `go test -race` xanh, golden Zen không đổi | 025 | todo |
| 027 | Tách các hàm còn vượt 30 | httpapi + provider | `gocyclo -over 30` rỗng | 023, 024, 026 | todo |
| 028 | Ép luật bằng công cụ trong CI và `scripts/check.sh` | CI + config + test kiến trúc | `scripts/check.sh` xanh, cố ý vi phạm làm đúng cổng đỏ | 001, 027 | todo |
| 029 | Bộ luật mới và đồng bộ spec | spec | `CLAUDE.md` dưới 200 dòng và 12KB, mỗi luật ghi cách kiểm | 028 | todo |
| 030 | Đánh số phiên bản schema bằng `PRAGMA user_version` thay cho nuốt lỗi `duplicate column` (cũ là task 002) | store | `go test ./internal/store/` xanh, gồm test nâng cấp DB cũ | 014 | in-review |
| 031 | `log/slog` có request ID xuyên suốt `/v1` và `IdleTimeout` cho server (cũ là task 003) | httpapi + cmd | `go test ./internal/httpapi/ ./cmd/ccw/` xanh, test khẳng định dòng log mang `req_id` | 027, 019 | todo |

## Ngoài plan
- `README.md`, `SECURITY.md`, `docs/*.md` đã bị xóa có chủ đích; các test và bước đóng gói npm phụ thuộc vào chúng đã được gỡ, `go test ./...` xanh.
- Mã hóa cột secret trong SQLite là một quyết định về mô hình đe dọa, chưa có yêu cầu, nên chưa thành task.
