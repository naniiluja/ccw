# Git Workflow

## Most important rule
- **Ask before every commit/push, and wait for the user's explicit go-ahead.** An unrequested commit or push rewrites the user's history irreversibly, so the confirmation is the cheap side of that asymmetry.

## Commit attribution (harness-enforced)
- Attribution is enforced by `.claude/settings.json` `attribution` (`{ "commit": "...", "pr": "..." }`), per `code.claude.com/docs/en/settings`. Harness-level settings are deterministic and **supersede** this narrative — settings win over prose. (`attribution` replaces the deprecated `includeCoAuthoredBy`.)
- This rule is only the **backup**: keep manual commit/PR trailers consistent with `settings.json`; if `attribution.commit` is `""`, do NOT add one by hand.
- Giá trị hiện tại lấy từ lịch sử: 67 trên 126 commit có `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`, nên `commit` giữ trailer đó. Repo chưa có PR nào mang attribution, nên `pr` là `""`. Dòng `Claude-Session: <url>` trong các commit cũ do harness thêm theo từng phiên, không ghi tay.

## When asked to commit
- If on the default branch (`master`): repo hiện chỉ có `master` và mọi commit đi thẳng vào đó; khi người dùng yêu cầu commit thì làm trên `master` trừ khi họ muốn nhánh riêng.
- Commit messages follow the repo convention: câu tiếng Anh thường, thể mệnh lệnh hoặc mô tả, viết hoa chữ đầu, khoảng 50 đến 80 ký tự, **không** dùng tiền tố conventional commits (`feat:`, `fix:`). Ví dụ đúng: `API keys: expiry, requests per minute, and usage per key`, `Standby accounts: serve only when every other active account fails`. Dạng `Miền: nội dung` được dùng khi thay đổi gói gọn trong một tính năng.
- Commit phát hành: `npm <version>: <mô tả>`, ví dụ `npm 0.1.8: /v1 follows the OpenAI and Anthropic specs`. Đi kèm nâng `version` trong `npm/ccw-gateway/package.json`.
- Thân commit dùng khi thay đổi nhiều điều: giải thích từng thay đổi và lý do, một đoạn ngắn mỗi ý (ví dụ commit `d5d3d02`).
- One logical change per commit; don't bundle unrelated work.

## Branch & PR
- Branch naming convention: chưa có quy ước (chỉ có `master`, remote `origin/master`). Nếu cần nhánh: `<loại>/<mô-tả-ngắn>` chữ thường, dấu gạch ngang (đề xuất, chưa được lịch sử xác nhận).
- PR rules: lịch sử chưa có PR. Đề xuất: mô tả nêu thay đổi và cách kiểm chứng, `go vet ./...` và `go test ./...` phải xanh trước khi merge.
- Tag phát hành: `v0.1.1`, `v0.1.2`, `v0.1.3` (đẩy tag `v*` kích hoạt workflow publish). Không tự tạo hay đẩy tag.

## Working tree đang có thay đổi chưa commit
Lúc chạy `/ccf:init`, working tree có nhiều sửa đổi dang dở (tính năng web search, server tools, Zen), và `README.md`, `SECURITY.md`, `docs/*.md` đã bị chủ repo xóa có chủ đích (chưa commit). Không khôi phục chúng. Không `git add -A`, không `git checkout .`, không `git stash`, không `git reset --hard`: chúng chứa việc chưa lưu của chủ repo. Chỉ stage đúng các file thuộc task đang làm.

## Monorepo
- git lives at the root only. Repo này là một module Go, không có `be/`, `fe/`.
