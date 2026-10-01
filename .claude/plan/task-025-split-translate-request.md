# Task 025 — Tách translate phía request

- **Vertical slice:** translate
- **Depends on:** 022
- **Spec refs:** `.claude/rules/coding-conventions.md` (dùng `json.Number`, giữ số thực)
- **MCP to use:** gopls (`go_symbol_references`, `go_file_context`, `go_diagnostics`) để kiểm tra không sót tham chiếu khi xóa hoặc gộp; context7 để tra Go style nếu cần
- **Gate (must be GREEN before the next slice):** `gofmt -l .` rỗng, `go vet ./...`, `go build ./...` và `go test -race ./...` xanh (baseline 97s); cộng: `go test -race ./internal/translate/` xanh; `gocyclo` của năm hàm đều dưới hoặc bằng 30
- **discipline:** off

## Goal (one sentence)
Giảm độ phức tạp các bộ dịch request giữa OpenAI, Anthropic và Gemini.

## Acceptance criteria (verifiable)
- [ ] `AnthropicToOpenAI` (49) tách thành `anthropicSystemMessage`, `anthropicMessages` (trả lỗi), `anthropicTools`, `anthropicToolChoice`, `copyScalarFields`; thứ tự `resultImages` của tool result giữ nguyên.
- [ ] `OpenAIToGemini` (49) tách thành `geminiContents`, `geminiGenerationConfig` (gồm thinking và `maxOut` ở cùng một hàm), `geminiTools`; luật chữ ký thought dùng chỉ số vòng lặp giữ trong nhánh assistant.
- [ ] `OpenAIToAnthropic` (32), `cleanSchema` (44) và `HealAnthropic` (45) giảm xuống dưới 30; `HealAnthropic` mỗi bước trả `(kết quả, changed)`, `sameBlocks` vẫn so con trỏ nên không sao chép block.

## Test first (write before implementing)
- `cleanSchema` chỉ phủ 72,7 phần trăm: thêm test đặc tả trước khi tách. Test hiện có của `translate` (độ phủ 87 đến 94 phần trăm) giữ nguyên và xanh.

## Files to touch
- internal/translate/request.go
- internal/translate/gemini.go
- internal/translate/heal.go
- internal/translate/schema.go

## Steps (thin end-to-end slice)
1. Write the failing test (cover the slice's user-visible behavior, not just one layer)
2. Implement minimally across the layers the slice touches
3. Run the test / verify actual output, the gate above must be GREEN, then mark the task `in-review` (NOT `done`)
4. `/ccf:check` → `/ccf:updatespec`, `done` is set ONLY here, after the review passes

## Notes / best-practice sources
- Kế hoạch gốc: `/Users/naniiluja/.claude/plans/optimized-giggling-gizmo.md` (hướng tinh gọn tại chỗ, không tạo package hay interface mới). Căn cứ Context7: Go blog "Organizing Go code" (không chia quá nhỏ), Google Go Style Guide (đặt tên), Uber Go Style Guide (lỗi, tránh global thay đổi được).
- Chỉ stage đúng file thuộc task, không `git add -A`, không push, không stash, reset hay checkout. Dùng `git mv` khi đổi tên hoặc gộp file; commit đầu chỉ di chuyển, sửa nội dung ở commit sau.
- Không đổi JSON shape và route của các endpoint còn lại. Không thêm thư viện ngoài. Giá trị đang lưu trong DB (`By`, `intact-review`) giữ nguyên chữ cũ.
- Hai hàm đầu có người gọi ở `shapes.go` (nay trong `proxy.go`) và `v1.go`; chữ ký công khai không đổi.
