# Debugging (kỷ luật, không vội)

Quy trình bắt buộc khi điều tra một bug, làm ngay trong hội thoại (không có lệnh debug riêng):
1. **Tái hiện**: dựng lại bug từ triệu chứng, đầu vào, môi trường trước khi chạm vào code.
2. **Lần từng bước**: lấy `req_id` từ header `Request-Id` của phản hồi `/v1`, rồi grep log JSON trên stderr theo `"req_id":"req_..."`; đọc cặp `proxy.upstream.start` và `proxy.upstream.done`/`proxy.upstream.fail` ở mỗi lời gọi upstream. Route ngoài `/v1` chưa có `req_id` (xem `logging.md`).
3. **Truy vấn DB chỉ đọc**: với SQLite cục bộ, mở bản sao của `ccw.db` bằng `sqlite3 -readonly`, không bao giờ ghi vào DB đang chạy. `PRAGMA user_version` cho biết schema đang ở bước migration nào.
4. **Khoanh vùng bằng bằng chứng**: thu hẹp bằng file:line, dòng log, dòng DB, không bằng cảm giác.
5. **Test đỏ**: viết test tái hiện bug trước.
6. **Sửa tối thiểu**: chỉ sửa trong phạm vi, không refactor kèm.

> Không đoán rồi sửa tại chỗ khi chưa có bằng chứng.

## Công cụ có sẵn trong sản phẩm
- Bảng `upstream_errors` (giữ 14 ngày, tối đa 20000 dòng, body tối đa 64 KiB mỗi chiều, request đầy đủ tối đa 8 MiB mỗi nhóm để replay) lưu lỗi upstream, nhóm theo chữ ký body; đọc qua `GET /api/errors`, `/api/errors/stats`, `/api/errors/{id}`. AI error review (`errreview.go`) và chia đôi system prompt (`errbisect.go`) đề xuất nguyên nhân.
- Drift (`shape_changes`, `GET /api/drift/changes`) cho thấy khi nào một provider đổi shape; AI drift review chấm từng thay đổi.
- Golden Zen trong `internal/httpapi/testdata/zen/` là mẫu để tái hiện lỗi khớp với gateway gốc.

## Known bugs (cập nhật bởi /ccf:updatespec)
- Symptom: `go test ./cmd/ccw` fail với `open ../../docs/verify-phase1.md: no such file` sau khi `README.md`, `SECURITY.md`, `docs/*.md` bị xóa | Root cause: `release_test.go` đọc các file đó từ cây thư mục thật, và `scripts/npm-build.sh` chạy `cp README.md` | Prevent: khi xóa hoặc đổi tên một file, `grep` tên file trong `*.go`, `*.sh`, `*.json`, `*.yml` trước; đã xử lý bằng cách gỡ các test đó và bỏ `README.md` khỏi bước đóng gói npm | Files: `cmd/ccw/release_test.go`, `scripts/npm-build.sh`, `npm/ccw-gateway/package.json`.
- Symptom: CI publish được một bản dù test đỏ | Root cause: workflow không có bước test | **Đã ngăn** (task 001 và 028): job `test` chạy `bash scripts/check.sh`, job `publish` có `needs: test`; `TestReleaseWorkflowGatesPublishOnTests` đỏ nếu ràng buộc này bị gỡ | Files: `.github/workflows/github-packages.yml`, `scripts/check.sh`, `cmd/ccw/release_test.go`.

## Bẫy đã chặn sẵn
- Worktree của `/ccf:cook` là bản sao repo nằm dưới `.claude/worktrees/`, nên một walker đi cả cây sẽ đọc code cũ. Mọi walker hiện có bỏ qua thư mục bắt đầu bằng `.` (`arch_test.go`, `release_test.go`, hàm `goFiles` của `scripts/check.sh`); walker mới phải làm giống vậy. Lệnh `grep -r` từ gốc repo cũng phải loại `.claude` (`--exclude-dir=.claude`).
