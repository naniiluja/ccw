# Task 026 — Tách translate phía response và stream

- **Vertical slice:** translate
- **Depends on:** 025
- **Spec refs:** `.claude/rules/coding-conventions.md`
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./internal/translate/ ./internal/httpapi/` xanh, golden Zen không đổi
- **discipline:** off

## Goal (one sentence)
Giảm độ phức tạp các bộ dịch response và stream.

## Acceptance criteria (verifiable)
- [ ] `CollectZenStream` (43), `OpenAIStreamToAnthropic` (36), `GeminiStreamToOpenAI` (32), `OpenAIToResponses` (30) đều dưới hoặc bằng 30.
- [ ] Trạng thái của stream (chỉ số tool call, khối đang mở) đi qua một struct nhỏ thay vì closure dài, nếu cần để tách an toàn.
- [ ] Không đổi thứ tự sự kiện SSE phát ra.

## Test first (write before implementing)
- Test stream hiện có (`stream_trunc_test.go`, `zen_test.go`, `anthropic_reply_test.go`) giữ nguyên; thêm test đặc tả cho nhánh stream cụt và `[DONE]` nếu đo thấy chưa phủ.

## Files to touch
- internal/translate/zen.go
- internal/translate/response.go
- internal/translate/gemini.go
- internal/translate/responses.go
- internal/translate/collect.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- `CollectZenStream` và các hàm collect nằm gần `collect.go`; chỉ tách nội bộ, không đổi cách gọi từ `httpapi`.
