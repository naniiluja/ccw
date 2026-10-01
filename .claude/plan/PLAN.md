# Implementation Plan — ccw

> **Execution rule: VERTICAL SLICES, RUN IN WAVES.** `/ccf:cook` runs tasks with no link between them (no `Depends on`, no shared file, no shared hotspot) at the same time, each in its own worktree.
> Each task is a thin tracer-bullet that crosses all the layers it touches (DB + service + UI), NOT a horizontal "all-DB-then-all-API" phase, so integration is proven early.
> A task starts only after every task in its `Depends on` has a **GREEN gate** and is merged; each wave's merged result is tested before the next wave starts.
> The `in-progress`/`in-review` status is read by the session-start hook to re-load context after compact, keep status up to date.

## Milestones
- M2: tách repo thành `be/` và `fe/`, dựng dashboard cho toàn bộ chức năng của ccw bằng shadcn và Origin UI (không chế component), phục vụ dưới `/ui/` từ binary Go.

> Status: `todo` / `in-progress` / `in-review` / `done` / `blocked`. Lifecycle: `todo → in-progress → in-review → done`. A task becomes `in-review` once its code+test are complete and merged (by `/ccf:cook`, or implemented directly in the session); only `/ccf:updatespec` writes `done` after `/ccf:check` passes.
> Write the status as a **bare word**, no `**bold**` around it. Emphasis carries no information and the status is matched as a whole word.
> Per-task detail in `task-NNN-*.md`.

> **Keep this file to the CURRENT iteration.** When every task of an iteration is `done`, `/ccf:updatespec` moves its sections verbatim into `.claude/plan/ARCHIVE.md` and `git mv`s its `task-NNN-*.md` files into `.claude/plan/archive/`. A closed row left here is counted as live work by the session-start and Stop hooks, while DELETING the history would lose a real record of what shipped and why. So archive, never delete.

<!-- Keep the status guidance ABOVE this line, never below "## Origin": archive retirement groups content from one "## Origin" heading to the next, and only text before the FIRST heading is never cut into ARCHIVE.md. -->

## Origin: Dashboard ccw, tách be/ và fe/

Nguồn: `/Users/naniiluja/.claude/plans/chia-theo-fe-v-fizzy-oasis.md`. Mục tiêu: Go vào `be/`, giao diện vào `fe/`, SPA phục vụ dưới `/ui/` và nhúng vào binary. Kỷ luật test mức contract: bật cho 033, 034, 035; tắt cho các task còn lại. Task 032 chạy một mình (chạm mọi đường dẫn). 033 và 036 chạy cùng wave vì `be/` và `fe/` rời nhau. Ba task BE (033, 034, 035) cùng chạm `be/internal/httpapi/server.go` nên khác wave. Các trang FE 037 đến 043 và 045 chạy song song sau 036, 044 chạy sau 040, 041, 042 để dùng chung hook.

## Task backlog: dashboard (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 032 | Tách repo: Go vào `be/`, script, CI và spec theo layout mới | repo layout + scripts + CI | `bash scripts/check.sh` xanh, 14 package, chỉ rename trong diff | — | in-review |
| 033 | Package `webui` và route `/ui/` (SPA nhúng, `GET /` chuyển hướng) | BE webui + httpapi | `bash scripts/check.sh` xanh, ma trận contract của `Handler` | 032 | todo |
| 034 | Route session trả JSON khi `Accept: application/json`, thêm `GET /api/session` | BE auth + accounts | `bash scripts/check.sh` xanh, ma trận route session | 032 | todo |
| 035 | `GET /api/settings` chỉ đọc, không lộ secret | BE httpapi | `bash scripts/check.sh` xanh, test không lộ secret | 032 | todo |
| 036 | `fe/`: Vite, shadcn, Origin UI, shell, router 12 route, đăng nhập, theme, cổng FE trong CI | FE toolchain + shell + CI | `pnpm -C fe lint typecheck test build` xanh, `bash scripts/check.sh` xanh | 032 | todo |
| 037 | Trang Tài khoản (thêm, OAuth, kiểm tra, xóa) | FE | cổng FE xanh, test MSW | 036 | todo |
| 038 | Trang Provider và Model (định nghĩa, bảng model, xoay vòng, chính sách, Zen) | FE | cổng FE xanh, test MSW | 036 | todo |
| 039 | Trang API key (khóa hiện một lần, giới hạn, usage) | FE | cổng FE xanh, test MSW | 036 | todo |
| 040 | Trang Quota và Usage (đồng hồ, claim reset, biểu đồ) | FE | cổng FE xanh, test MSW | 036 | todo |
| 041 | Trang Lỗi upstream (lọc, chi tiết, AI error review) | FE | cổng FE xanh, test MSW | 036 | todo |
| 042 | Trang Drift (thay đổi shape, ack, AI drift review) | FE | cổng FE xanh, test MSW | 036 | todo |
| 043 | Trang Bộ lọc (blacklist field) | FE | cổng FE xanh, test MSW | 036 | todo |
| 044 | Trang Tổng quan (gộp từ các hook đã có) | FE | cổng FE xanh, test MSW | 036, 040, 041, 042 | todo |
| 045 | Trang Cài đặt chỉ đọc và thông tin kết nối | FE | cổng FE xanh, test MSW | 036 | todo |
| 046 | Đóng gói SPA vào binary và npm, đồng bộ spec, kiểm đầu cuối bằng trình duyệt | build + CI + spec + e2e | `bash scripts/ui-build.sh && bash scripts/check.sh` xanh, kiểm bằng Claude in Chrome | 033, 034, 035, 036, 037, 038, 039, 040, 041, 042, 043, 044, 045 | todo |
