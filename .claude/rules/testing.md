# Testing (verification-first)

Philosophy: **viết test đỏ TRƯỚC** khi cài đặt hoặc sửa, vì đó là cách rẻ nhất để Claude tự kiểm việc mình làm.

## Rules
- Mỗi task: viết test tái hiện acceptance criteria (đỏ) → cài đặt → test xanh. Acceptance criteria là test cụ thể, không phải mô tả mơ hồ. `[review]` diff của task phải có file `_test.go`.
- Sửa bug: viết test tái hiện bug (đỏ) trước, rồi sửa. `[review]`
- **Kỷ luật test mức contract** (ma trận EP/BVA trên chữ ký công khai) **bật** cho task thêm hoặc đổi chữ ký công khai (ví dụ `CheckPassword`, `ValidatePassword`: rỗng, sai, đúng, rất dài; biên 11 và 12 ký tự ở `internal/auth/password_test.go`), **tắt** cho task chỉ xóa hoặc gộp file (gate là test có sẵn cộng số đo). Task file ghi rõ ở dòng `discipline`. `[review]`
- Refactor tách hàm (giữ hành vi) viết test đặc tả trước cho nhánh chưa có test, rồi chạy `go test -race -count=3` trên package đó. `[review]`

## Cổng (một lệnh)
- `bash scripts/check.sh` chạy đủ cổng, giống hệt CI: `gofmt -l`, `go vet ./...`, staticcheck v0.8.1 (`staticcheck.conf`), `gocyclo -over 30`, `go test -race ./...`. Script chạy hết mọi cổng rồi liệt kê cổng đỏ. Chạy trước mỗi commit. `[tool]`
- `go test -race ./...` gồm cả test kiến trúc và kích thước file (`internal/httpapi/arch_test.go`) và bộ test bảo vệ bản phát hành (`cmd/ccw/release_test.go`).
- CI (`.github/workflows/github-packages.yml`): job `test` chạy `bash scripts/check.sh` trên mỗi push vào `master`, mỗi pull request và mỗi tag; job `frontend` chạy `pnpm install --frozen-lockfile`, `lint`, `typecheck`, `test`, `build` trong `fe/`; job `publish` có `needs: [test, frontend]`, cài pnpm và Node trước bước Build, và chỉ chạy với tag `v*` hoặc chạy tay. `[tool]` `TestReleaseWorkflowGatesPublishOnTests` giữ ràng buộc này (chấp nhận `needs` dạng đơn hoặc danh sách), `TestPublishBuildsTheUIBeforeTheBinaries` giữ thứ tự Node, pnpm rồi `ui-build.sh` rồi `go build`.

## Cổng frontend (`fe/`)
- `bash scripts/check.sh` chạy thêm, khi có `fe/package.json`: `pnpm install --frozen-lockfile`, `pnpm lint` (oxlint `--deny-warnings`), `pnpm typecheck` (`tsc -b`), `pnpm test` (Vitest, 13 file test), `pnpm build`. `[tool]`
- Test FE dùng Vitest, jsdom, Testing Library và MSW (`fe/src/test/server.ts`); không gọi mạng thật. `fe/src/components/ui-boundary.test.ts` chặn import Radix hoặc Base UI ngoài `components/ui`. Test kiểm hành vi người dùng thấy, không kiểm chi tiết cài đặt. `[tool]`
- `be/internal/webui/webui_test.go` đọc `static/index.html` thật khi đã build (bỏ qua có chú thích khi chưa chạy `ui-build.sh`) và khẳng định `/ui/` trả `200` kèm `<div id="root">`. `[tool]`
- Kiểm đầu cuối trên binary thật bằng trình duyệt là bước tay của task, không nằm trong cổng.
- Còn một lỗi chập chờn ở `fe/src/pages/keys/keys.test.tsx` (xem `CLAUDE.md`).

## Tooling & location
- Framework: package `testing`, `net/http/httptest` cho HTTP, không thư viện assert.
- Lệnh: `go test ./...`, một package `go test ./internal/translate/`.
- Test nằm ở `<file>_test.go` cạnh file được test, cùng package. 89 file test được track (`git ls-files '*_test.go' | wc -l`): `internal/httpapi` 50, `internal/store` 10, `internal/translate` 9, `internal/auth` 4, các package khác 1 đến 3. File test được miễn giới hạn 1000 dòng.
- Helper dùng chung gọi `t.Helper()` (ví dụ `mustJSON` trong `internal/translate/translate_test.go`, `moduleRoot` trong `arch_test.go`).
- Test nhiều case cùng cấu trúc viết table-driven với `t.Run` (hiện 12 file dùng `t.Run`: `git ls-files '*_test.go' | xargs grep -l 't\.Run(' | wc -l`); còn lại kiểm trực tiếp trên JSON.
- Golden Zen: `internal/httpapi/testdata/zen/` (`cases.json`, 4 file `*.golden.json`), dùng bởi `TestZenRequestsMatchTheGatewayItReplaces`. Đọc `README.md` cùng thư mục trước khi ghi lại; cập nhật golden phải xem diff, không ghi đè mù. Refactor `translate` hay `providers.go` không được đổi golden.

## Bộ test bảo vệ bản phát hành công khai (`cmd/ccw/release_test.go`)
Đọc **cây thư mục thật** (không fixture), bỏ qua thư mục ẩn như `.claude/worktrees`:
- `TestLicenseIsMIT`: `LICENSE` là MIT.
- `TestGitignoreExcludesEnvFiles`: `.gitignore` có `*.env` và `ccw.env`.
- `TestCommentsShowNoSamplePassword`: không comment nào trong file `.go` không test ghi mẫu `CCW_PASSWORD=<giá trị>`. Thay cho luật cũ cấm chữ "password" (đã bỏ cùng TOTP).
- `TestReleaseWorkflowGatesPublishOnTests`: workflow chạy `scripts/check.sh` và publish chờ test.
- `TestFixturesLookLikePlaceholders`: không file `_test.go` nào chứa chuỗi `ya29.`.

Repo không còn `README.md` và `SECURITY.md`; `docs/` chỉ còn `docs/clients.md`. Không thêm test đọc các đường dẫn đã xóa.

## Bất biến mật khẩu (`cmd/ccw/main_test.go`, `internal/auth/*_test.go`)
- Lần chạy đầu in mật khẩu đúng một lần và chỉ lưu bản băm (`TestFirstStartPrintsThePasswordOnceAndStoresOnlyAHash`).
- Mật khẩu từ `CCW_PASSWORD` không bị log, không bị lưu dạng rõ (`TestAnEnvironmentPasswordIsNeitherLoggedNorStored`); mật khẩu dưới 12 ký tự từ chối khởi động (`TestAShortPasswordRefusesToStart`); `-reset-password` vô hiệu mật khẩu cũ (`TestResetPasswordRetiresTheOldOne`).
- Server không gate chỉ khởi động khi chọn rõ `-insecure-no-auth` (`TestAuthDecisionRefusesAnUngatedServer`, `TestInsecureOptInStartsWithNoGate`); server có `IdleTimeout` (`TestServerHasIdleTimeout`).

## Coverage
- Không đặt con số. Đường nghiệp vụ lõi phải có test trước khi task chuyển `done`: rotation và failover, dịch shape, error envelope, auth tier (`adminroutes_test.go`), đăng nhập và `LoginGuard`, migration store (`migrate_test.go`).
- Không có hook ép test; cổng là `scripts/check.sh` cục bộ và trong CI.
