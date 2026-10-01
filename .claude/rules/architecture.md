# Architecture

## Layering & boundaries
Không có layering chính thức, nhưng import graph hiện tại có hình dạng ổn định (đã đối chiếu bằng `grep` import):
- **Lá, không import package nội bộ nào:** `auth`, `filter`, `oauth`, `provider`, `upstream`, `usage`, `websearch`, `zen`.
- **Trung gian:** `store` → `provider`; `drift` → `store`; `contract` → `drift`, `store`; `translate` → `filter`; `servertools` → `websearch`.
- **Đỉnh:** `internal/httpapi` import tất cả, và `cmd/ccw` chỉ import `auth`, `httpapi`, `store`.

## Dependency direction
- Dependencies flow one way only: `cmd/ccw` → `httpapi` → (`translate`, `contract`, `servertools`) → (`drift`, `store`) → lá.
- Business logic does NOT depend on frameworks/IO directly: package lá không được import `httpapi`, và `store` không được import `httpapi`, `translate` hay `drift`.

## Design patterns
- Backend: handler là method của struct `api` (`internal/httpapi/server.go`) giữ `store`, cache in-memory và cursor xoay vòng. Middleware là hàm bọc `http.HandlerFunc`: `guardRequest`, `requireSession`, `requireToken`, `requireAdmin`, `v1API`.
- Provider khai báo bằng dữ liệu (`provider.Def`), không có code riêng cho mỗi provider trừ chỗ shape khác biệt (`antigravity.go`, `copilot.go`, `zen.go`, `typesafe.go`).
- Frontend: đã gỡ cùng UI, sẽ định nghĩa lại khi viết bản mới.

## Where things go
- Route mới: đăng ký trong `internal/httpapi/server.go`, handler đặt trong file cùng chủ đề (`keys.go`, `filters.go`, ...).
- Route dưới `/v1` phải đi qua `v1API` để có error envelope theo shape của caller và header `Request-Id`.
- SQL mới: chỉ trong `internal/store/`, mỗi domain một file (`connection.go`, `usage.go`, `drift.go`, ...).
- Chuyển đổi shape: `internal/translate/`. Khi caller và provider cùng shape, body đi qua nguyên vẹn, chỉ dịch khi lệch shape.
- Tool phía server (fetch, search, MCP client, tool search): `internal/servertools/`, `internal/websearch/`.
- UI: đã gỡ khỏi repo (đang viết lại). `GET /` và `GET /login` chưa có route; `POST /login` và session vẫn còn trong `internal/httpapi/auth_handlers.go`.

## Auth tiers (không được nới lỏng)
- `requireSession`: route trình duyệt, chuyển hướng về `/login` khi thất bại.
- `requireToken`: `/v1` và đọc `/api/*`; nhận master token, API key trong DB hoặc session; áp giới hạn RPM theo key.
- `requireAdmin`: mọi thao tác ghi cấu hình; chỉ session hoặc master token, dashboard key bị từ chối. Tool MCP ghi cấu hình nằm trong `mcpAdminTools` (`internal/httpapi/mcp.go`).

## Verifiable rules
- Each module has one clear responsibility (Single Responsibility).
- No circular imports between layers: a cycle makes each side untestable in isolation and hides the true dependency direction. `go build ./...` phải sạch.
- Package lá (danh sách ở trên) không được import `internal/httpapi`.
- Không thêm route ghi cấu hình mà thiếu `requireAdmin`.
