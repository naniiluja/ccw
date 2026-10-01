# Task 032: Tách repo thành be/ và fe/ (di chuyển Go vào be/)

- **Vertical slice:** refactor repo layout, scripts, CI, spec
- **Depends on:** —
- **Spec refs:** `.claude/rules/git-workflow.md` (mục Monorepo), `.claude/rules/tooling.md`, `.claude/rules/testing.md`, `CLAUDE.md`
- **MCP to use:** none (gopls không resolve trong worktree; dùng `grep` và `go build ./...`)
- **Gate (must be GREEN before the next slice):** `bash scripts/check.sh` xanh (gofmt, vet, staticcheck, gocyclo, `go test -race ./...` chạy trong `be/`); `cd be && go list ./... | wc -l` bằng 14; `git diff --stat -M HEAD` chỉ thấy rename cộng các tệp khai báo bên dưới
- **discipline:** off

## Goal (one sentence)
Toàn bộ mã Go chuyển vào `be/` (module và đường dẫn import không đổi), gốc repo giữ `CLAUDE.md`, `.claude/`, `.github/`, `scripts/`, `npm/`, `docs/`, `LICENSE`, và mọi cổng vẫn xanh.

## Acceptance criteria (verifiable)
- [ ] `be/go.mod`, `be/go.sum`, `be/staticcheck.conf`, `be/cmd/ccw`, `be/internal/*` tồn tại; gốc không còn `go.mod`, `cmd/`, `internal/`.
- [ ] `scripts/check.sh` chạy cổng Go trong `be/` (`cd be`), in rõ tên từng cổng như cũ, và bỏ qua cổng FE khi chưa có `fe/package.json` (cổng FE do task 036 thêm).
- [ ] `scripts/npm-build.sh` build binary từ `be/` (đường dẫn đầu ra tuyệt đối, vì có `cd be`) và `npm pack` vẫn đúng.
- [ ] Workflow dùng `go-version-file: be/go.mod`; job `test` vẫn chạy `bash scripts/check.sh`.
- [ ] `be/cmd/ccw/release_test.go` tìm gốc repo bằng cách đi lên đến thư mục chứa `CLAUDE.md`; các test đọc `LICENSE`, `.gitignore`, workflow, `scripts/check.sh` vẫn xanh; có test mới khẳng định `be/go.mod` tồn tại và gốc không có `go.mod`.
- [ ] `.gitignore` thêm trước các mẫu cho cả kế hoạch: `/be/*.db`, `/be/*.db-shm`, `/be/*.db-wal`, `/be/ccw`, `fe/node_modules/`, `fe/dist/`, `fe/*.tsbuildinfo`, `be/internal/webui/static/*` kèm `!be/internal/webui/static/.gitkeep`.
- [ ] `CLAUDE.md` mô tả layout mới và ghi một câu: các đường dẫn trong `.claude/rules/*` tính từ `be/` trừ khi có tiền tố `fe/`, `scripts/`, `npm/`, `.github/` hoặc `.claude/`.
- [ ] `git-workflow.md` mục Monorepo bỏ câu "không có be/, fe/"; `tooling.md` đổi lệnh chạy thành `cd be && go run ./cmd/ccw ...`.
- [ ] `be/internal/httpapi/arch_test.go` (`moduleRoot`) và walker `goFiles` vẫn bỏ qua thư mục bắt đầu bằng `.`.

## Test first (write before implementing)
- Sửa `be/cmd/ccw/release_test.go`: helper `repoFile` đi lên đến `CLAUDE.md`; chuỗi workflow cần có đổi thành `go-version-file: be/go.mod` (đỏ cho đến khi sửa workflow); thêm `TestGoModuleLivesInBe` (đỏ cho đến khi dời). Làm bước này trước khi `git mv` để thấy đỏ rồi xanh.

## Files to touch
- cmd/** — đường dẫn cũ bị xóa khi dời
- internal/** — đường dẫn cũ bị xóa khi dời
- go.mod — đường dẫn cũ bị xóa khi dời
- go.sum — đường dẫn cũ bị xóa khi dời
- staticcheck.conf — đường dẫn cũ bị xóa khi dời
- be/cmd/** — nơi mới
- be/internal/** — nơi mới
- be/go.mod
- be/go.sum
- be/staticcheck.conf
- scripts/check.sh
- scripts/npm-build.sh
- .github/workflows/github-packages.yml
- .gitignore
- CLAUDE.md
- .claude/rules/git-workflow.md
- .claude/rules/tooling.md
- .claude/plan/PLAN.md

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches: `git mv` từng thư mục và tệp, sửa script, workflow, spec
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Dùng `git mv` để lịch sử theo được; chỉ stage đúng tệp thuộc task, không `git add -A`, không stash, reset hay checkout. Không push, không tạo tag.
- Preflight so `git diff --no-renames`, nên mỗi tệp dời hiện ra thành một xóa và một thêm; vì vậy khai báo glob cho cả đường dẫn cũ và mới.
- Trước khi dời, `grep` các chuỗi `./cmd/ccw`, `./internal`, `go.mod`, `staticcheck.conf` trong `*.sh`, `*.yml`, `*.json`, `*.md` (kể cả `docs/clients.md`, `npm/ccw-gateway/package.json`) và sửa nếu có (bài học ở `debugging.md`: xóa hay dời tệp phải grep tên tệp trước).
- Module path `github.com/naniiluja/ccw` giữ nguyên, nên không import nào đổi. Giá trị `go-version-file` trỏ `be/go.mod`.
- Nguồn: kế hoạch `/Users/naniiluja/.claude/plans/chia-theo-fe-v-fizzy-oasis.md`; quy tắc monorepo của `/ccf:init` (gốc giữ `CLAUDE.md`, `.claude/`, CI/CD).
