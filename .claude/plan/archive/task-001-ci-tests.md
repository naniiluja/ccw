# Task 001 — CI chạy vet và test trước khi publish

- **Vertical slice:** CI workflow (một layer, nhưng cắt qua toàn bộ pipeline release)
- **Depends on:** —
- **Spec refs:** `.claude/rules/testing.md` (mục Coverage và CI), `.claude/rules/coding-conventions.md` (Linter)
- **MCP to use:** none
- **Gate (must be GREEN before the next slice):** `go vet ./...` và `go test ./...` chạy xanh trên máy; test đọc `.github/workflows/github-packages.yml` xác nhận job publish có `needs` trỏ tới job test.

## Goal (one sentence)
Workflow phát hành không được publish khi `go vet` hoặc `go test -race` đỏ, và các bước này cũng chạy trên push và pull request.

## Acceptance criteria (verifiable)
- [ ] Workflow có job `test` chạy `go vet ./...` và `go test -race ./...` với Go lấy từ `go.mod` (`go-version-file`).
- [ ] Job publish hiện có khai báo `needs: test`, nên tag `v*` không publish được khi test đỏ.
- [ ] Job `test` cũng kích hoạt khi `push` lên `master` và khi `pull_request`, không chỉ trên tag `v*`.
- [ ] Không thêm dependency Go mới; chưa thêm `golangci-lint` trong task này.

## Test first (write before implementing)
- Thêm `TestReleaseWorkflowGatesPublishOnTests` vào `cmd/ccw/release_test.go`, cùng phong cách `repoFile` đang có: đọc `.github/workflows/github-packages.yml` và khẳng định file chứa `go vet ./...`, `go test`, và job publish có dòng `needs:` trỏ tới job test. Chạy trước khi sửa workflow, phải đỏ.

## Files to touch
- `.github/workflows/github-packages.yml` — thêm job `test`, triggers `push` và `pull_request`, `needs: test` cho job publish.
- `cmd/ccw/release_test.go` — thêm test bảo vệ trình tự trên.

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Researcher: workflow nên chạy `go vet ./...` và `go test -race ./...` trên push và pull request, và publish trên tag phải phụ thuộc bước test (`needs:`). Chưa có nguồn chính thức được fetch cho mục này, chỉ là thực hành chung.
- `go test ./...` hiện xanh toàn bộ: các test đọc `README.md`, `SECURITY.md`, `docs/*.md` đã bị gỡ cùng lúc các file đó bị xóa.
- Các test trong `cmd/ccw/release_test.go` đọc cây thư mục thật (`LICENSE`, `.gitignore`, `internal/...`), nên job `test` phải checkout đầy đủ và chạy từ root repo.
