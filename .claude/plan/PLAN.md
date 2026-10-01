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

## Origin: Đóng các chỗ lệch best practice (onboarding)

Ba task không có liên kết dữ liệu và không chung file, nên `/ccf:cook` chạy được song song trong một wave.

## Task backlog (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 001 | CI chạy `go vet` và `go test -race` trước khi publish | CI workflow | `go vet ./...` và `go test ./...` xanh trên máy, workflow đọc đúng trình tự bằng test đọc YAML | — | todo |
| 002 | Đánh số phiên bản schema bằng `PRAGMA user_version` thay cho nuốt lỗi `duplicate column` | store | `go test ./internal/store/` xanh, gồm test nâng cấp DB cũ | — | todo |
| 003 | `log/slog` có request ID xuyên suốt `/v1` và `IdleTimeout` cho server | httpapi + cmd | `go test ./internal/httpapi/ ./cmd/ccw/` xanh, test khẳng định dòng log mang `req_id` | — | todo |

## Origin: Tổ chức lại cấu trúc backend

Nguồn: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md`. Mục tiêu: chia `internal/httpapi` (49 file, 13.5k dòng, struct `api` 26 field) thành các package theo miền, hành vi giữ nguyên. Bước 0 (người dùng): commit nền sạch trước khi task đầu tiên chạy. Các task 004 đến 013 chạy song song với 001 đến 003 theo quyết định của người dùng; `/ccf:cook` tự xếp khác wave những cặp chung file (`v1.go`, `proxy.go`, `server.go`, `notify.go`, `store/contract.go`). Kỷ luật test mức contract: bật.

## Task backlog: tổ chức lại backend (in execution order)
| # | Slice | Layers | Gate (tests green) | Depends on | Status |
|---|-------|--------|--------------------|-----------|--------|
| 004 | Gom helper HTTP dùng chung vào `httpx.go`, thêm test kiến trúc và test `mcpAdminTools` | httpapi + cmd + spec | `gofmt`, `go vet`, `go test -race ./...` xanh, test không package nào ngoài `httpapi` import `httpapi` | — | todo |
| 005 | Tách `store/contract.go` theo miền | store | `go test -race ./internal/store/` xanh, `contract_test.go` giữ nguyên | — | todo |
| 006 | `loginguard` sang `internal/auth` | auth + httpapi | `go test -race ./...` xanh, ma trận BVA ngưỡng đăng nhập | 004 | todo |
| 007 | `ranking` sang `internal/ranking` | ranking + httpapi | `go test -race ./...` xanh, ma trận `arenaNorm` | 004 | todo |
| 008 | `notify` sang `internal/notify` | notify + httpapi | `go test -race ./...` xanh, ma trận SSRF | 004 | todo |
| 009 | `quota` sang `internal/quota` bằng interface `TokenSource` | quota + httpapi | `go test -race ./...` xanh, 5 file test quota đã chuyển | 004, 006 | todo |
| 010 | `review` sang `internal/review` bằng `Asker`/`Sender` | review + httpapi | `go test -race ./...` xanh, test review dùng fake | 004, 008 | todo |
| 011 | Tách `failover` trong `v1.go` thành các bước nhỏ | httpapi (refactor) | `go test -race -count=3 ./internal/httpapi/` xanh, golden Zen không đổi | 004 | todo |
| 012 | Lõi `/v1` sang `internal/proxy` | proxy + httpapi | `go test -race ./...` xanh, auth tier và golden Zen giữ nguyên | 009, 010, 011 | todo |
| 013 | Đồng bộ spec sau khi tách backend | spec | `CLAUDE.md` dưới 200 dòng và 12KB, chỉ `cmd/ccw` import `httpapi` trong `go list -deps` | 012 | todo |

## Ngoài plan
- `README.md`, `SECURITY.md`, `docs/*.md` đã bị xóa có chủ đích; các test và bước đóng gói npm phụ thuộc vào chúng đã được gỡ, `go test ./...` xanh.
- Mã hóa cột secret trong SQLite là một quyết định về mô hình đe dọa, chưa có yêu cầu, nên chưa thành task.
