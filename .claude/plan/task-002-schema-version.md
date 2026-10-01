# Task 002 — Đánh số phiên bản schema SQLite

- **Vertical slice:** store (schema + migration + test nâng cấp từ DB cũ)
- **Depends on:** —
- **Spec refs:** `.claude/rules/error-handling.md` (so sánh lỗi bằng `errors.Is`, không so chuỗi), `.claude/rules/architecture.md` (SQL chỉ trong `internal/store/`)
- **MCP to use:** none
- **Gate (must be GREEN before the next slice):** `go test ./internal/store/` xanh, trong đó có test mở một DB ở trạng thái cũ (chưa có `user_version`, đã có sẵn một số cột) và nâng lên phiên bản mới mà dữ liệu còn nguyên.

## Goal (one sentence)
`store.Open` nâng schema theo danh sách migration đánh số, ghi phiên bản vào `PRAGMA user_version`, và không còn dựa vào việc nuốt lỗi `"duplicate column"`.

## Acceptance criteria (verifiable)
- [ ] `PRAGMA user_version` của một DB mới bằng số migration cuối; mở lại DB đó không chạy lại migration nào.
- [ ] Mở DB tạo bởi bản trước (không có `user_version`, các cột `ALTER TABLE ... ADD COLUMN` đã có sẵn) không lỗi và không mất dữ liệu.
- [ ] Mỗi bước migration chạy trong một transaction; migration lỗi giữa chừng để `user_version` không tăng.
- [ ] `grep -rn 'duplicate column' internal/store` không còn kết quả trong code chạy thật.
- [ ] Các chỗ hiện tại dùng `ALTER TABLE ... ADD COLUMN`: `models.go` (`test_conn`), `errors.go` (`client_key_id`, `replayed`), `drift.go` (9 cột verdict, `legacy`), `oauth.go` (cột OAuth, `base_url`, `meta`, `standby`), `apikey.go` đều đi qua cơ chế mới.

## Test first (write before implementing)
- `TestOpenUpgradesAnOldDatabase`: dựng DB bằng schema cũ (chạy tay các `CREATE TABLE` không có cột mới, chèn vài dòng mẫu vào `connections` và `upstream_errors`), gọi `store.Open`, kiểm tra cột mới tồn tại, dòng mẫu còn nguyên, `user_version` đúng.
- `TestOpenIsIdempotent`: mở hai lần liên tiếp, phiên bản không đổi, không lỗi.
- `TestFailedMigrationKeepsTheOldVersion`: ép một migration lỗi, kiểm tra `user_version` giữ nguyên và DB vẫn mở lại được ở phiên bản cũ.

## Files to touch
- `internal/store/store.go` — danh sách migration đánh số, chạy theo `user_version`, mỗi bước trong transaction.
- `internal/store/models.go`, `errors.go`, `drift.go`, `oauth.go`, `apikey.go` — chuyển các `ALTER TABLE` nằm rải rác vào danh sách migration.
- `internal/store/contract.go` — `MigrateContract` cũng dùng `strings.Contains(err.Error(), "duplicate column")`, nên phải được xử lý để tiêu chí grep rỗng đúng.
- `internal/store/store_test.go` (hoặc file test mới cùng thư mục) — ba test trên.

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- SQLite: `PRAGMA user_version` dành cho ứng dụng theo dõi phiên bản schema, SQLite không tự dùng nó (https://www.sqlite.org/pragma.html#pragma_user_version). Context7 bị từ chối quyền trong phiên onboarding nên chưa có tài liệu riêng của `modernc.org/sqlite`; kiểm chứng lại khi thực hiện.
- Không nối chuỗi tên cột vào SQL từ giá trị không phải hằng số.
- Đọc lại vòng đời hiện tại ở `store.go:57-116` và các nơi `ALTER TABLE` trước khi gom; `drift.go` còn bước đánh dấu `legacy` cho path `{*}` cũ, phải giữ đúng hành vi.
- Chỉ đụng `internal/store/`, không đụng `internal/httpapi/`, để task này chạy song song được với task 003.
