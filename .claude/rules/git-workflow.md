# Git Workflow

## Most important rule
- **Hỏi trước mỗi commit/push và chờ người dùng đồng ý rõ ràng.** Một commit hay push không được yêu cầu ghi đè lịch sử của người dùng không đảo ngược được, nên xác nhận là bên rẻ của sự bất cân xứng đó.

## Commit attribution (harness ép)
- Attribution do `.claude/settings.json` `attribution` (`{ "commit": "...", "pr": "..." }`) ép, theo `code.claude.com/docs/en/settings`. Setting của harness là tất định và **thắng** văn bản ở đây. (`attribution` thay cho `includeCoAuthoredBy` đã bỏ.)
- Luật này chỉ là **dự phòng**: trailer viết tay phải khớp `settings.json`; nếu `attribution.commit` là `""` thì KHÔNG thêm tay.
- Giá trị hiện tại: `commit` là `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>` (đếm bằng `git log --grep='Co-Authored-By: Claude' --oneline | wc -l`), `pr` là `""` vì chưa có PR nào.

## When asked to commit
- Nhánh mặc định là `master` (nhánh duy nhất, remote `origin` trỏ `github.com/naniiluja/ccw`). Lịch sử bắt đầu lại ở commit `52bec36` (`Start ccw: the intact credential proxy under its new name`). Khi người dùng yêu cầu commit thì làm trên `master` trừ khi họ muốn nhánh riêng.
- Message: câu tiếng Anh, viết hoa chữ đầu, khoảng 50 đến 80 ký tự, **không** dùng tiền tố conventional commits (`feat:`, `fix:`). Dạng `Miền: nội dung` khi thay đổi gói gọn trong một tính năng. Commit của một task CCF mở đầu bằng số task (`031: Log with slog and carry the request ID through /v1`); commit cập nhật kế hoạch mở đầu bằng `Plan:`; merge wave là `merge <nhánh> (ccf wave)`.
- Commit phát hành: `npm <version>: <mô tả>`, đi kèm nâng `version` trong `npm/ccw-gateway/package.json`.
- Thân commit dùng khi thay đổi nhiều điều: giải thích từng thay đổi và lý do, một đoạn ngắn mỗi ý.
- Một thay đổi logic mỗi commit; không gộp việc không liên quan.
- Trước commit: `bash scripts/check.sh` xanh.

## Branch & PR
- Nhánh của `/ccf:cook`: `worktree-<tên-plan>-<số task>`, mỗi nhánh một worktree dưới `.claude/worktrees/`, merge vào `master` sau khi wave qua gate.
- Nhánh khác: chưa có quy ước. Đề xuất `<loại>/<mô-tả-ngắn>` chữ thường, gạch ngang.
- PR: lịch sử chưa có PR. CI chạy `scripts/check.sh` trên mỗi pull request; PR chỉ merge khi job `test` xanh.
- Tag phát hành `v*` kích hoạt job publish. Lịch sử mới chưa có tag nào. Không tự tạo hay đẩy tag.

## An toàn với working tree
- Không `git add -A`, không `git checkout .`, không `git stash` trần, không `git reset --hard`: working tree và stash dùng chung với các worktree và phiên khác. Chỉ stage đúng file thuộc task, gọi tên từng file.
- `README.md`, `SECURITY.md` và phần lớn `docs/` đã bị chủ repo xóa có chủ đích. Không khôi phục chúng.

## Monorepo
- Git chỉ ở root. Module Go nằm trong `be/` (`be/go.mod`), dashboard viết lại sẽ nằm trong `fe/`; gốc giữ `CLAUDE.md`, `.claude/`, `.github/`, `scripts/`, `npm/`, `docs/`, `LICENSE`.
