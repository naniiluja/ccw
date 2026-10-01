# Task 005 — Tách store/contract.go theo miền

- **Vertical slice:** store
- **Depends on:** —
- **Spec refs:** `.claude/rules/coding-conventions.md` (file tối đa khoảng 1000 dòng; `contract.go` 1165 dòng là ngoại lệ không được phình thêm), `.claude/rules/architecture.md` (SQL chỉ trong `internal/store/`, mỗi domain một file)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`) để kiểm tra không sót tham chiếu khi di chuyển; context7 nếu cần tra lại tài liệu Go
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s), cộng test ma trận EP/BVA ở chữ ký public của package mới
- **discipline:** on

## Goal (one sentence)
Chia `internal/store/contract.go` (1165 dòng) thành các file theo miền trong cùng package `store`, không đổi hành vi.

## Acceptance criteria (verifiable)
- [ ] Mỗi file mới dưới 1000 dòng: `contract_traces.go`, `contract_shapes.go`, `contract_findings.go`, `contract_signatures.go`, `contract_learned.go`, `contract_maintenance.go` (traces, shapes, findings, signatures, learned và counters, reset và prune), cùng `contract.go` còn types, key và schema.
- [ ] Chỉ một `contractSchema` và lời gọi ở `store.go:109` giữ nguyên; `MigrateContract` giữ nguyên hành vi.
- [ ] `PruneContracts` (233 dòng, 5 bước, mỗi bước một transaction) tách thành 5 hàm nhỏ có tên rõ, kết quả và thứ tự y như cũ.
- [ ] Không đổi chữ ký method `(*Store)` nào; `go build ./...` không cần sửa caller nào ngoài `store`.

## Test first (write before implementing)
- `TestPruneContracts...` (ma trận BVA theo ngưỡng giữ lại của từng bước prune, trước và sau khi tách cho cùng kết quả).
- Chạy lại `internal/store/contract_test.go` nguyên vẹn (5 test) làm test đặc tả hành vi.

## Files to touch
- `internal/store/contract.go` — còn types, key, schema
- `internal/store/contract_*.go` — các file mới theo miền
- `internal/store/contract_test.go` — giữ, thêm test prune

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md`. Căn cứ Context7: `go.dev/doc/modules/layout` (server: logic trong `internal/`, binary ở `cmd/`), `go.dev/doc/faq` (interface thỏa ngầm, khai báo ở phía dùng).
- Quy tắc di chuyển: `git mv` cho file đã track; commit đầu chỉ đổi `package` và import để git nhận rename, commit sau mới sửa nội dung. Chỉ stage file thuộc task, không `git add -A`, không stash, reset hay checkout.
- Package mới không import `internal/httpapi`; log bằng `log.Printf("pkg: ...")` theo tiền lệ `drift`/`zen` cho tới khi task-003 chuyển sang slog; không log secret.
- Không đổi JSON shape của endpoint và không đổi route.
- discipline: on (ma trận EP/BVA/decision-table ở chữ ký public và chạy test thật trước khi gate xanh).
- Trùng file với task-002 (`MigrateContract` dòng 934 dùng `strings.Contains`); `/ccf:cook` sẽ xếp khác wave. Giữ nguyên chuỗi so sánh lỗi, task-002 mới đổi nó.
