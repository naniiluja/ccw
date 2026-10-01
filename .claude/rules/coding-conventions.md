# Coding Conventions

Nguồn: Go blog "Organizing Go code" (không chia package quá nhỏ), Google Go Style Guide, Uber Go Style Guide (lỗi, tránh global thay đổi được, constructor), `go.dev/doc/modules/layout`. Đặt tên nằm ở `naming.md`.

Mỗi luật ghi kèm **cách kiểm**:
- `[tool]`: cổng tự động. `bash scripts/check.sh` chạy đủ: `gofmt -l`, `go vet ./...`, staticcheck v0.8.1 theo `staticcheck.conf`, `gocyclo -over 30` trên file không test, `go test -race ./...` (gồm test kiến trúc và kích thước ở `internal/httpapi/arch_test.go`). CI chạy đúng script này trước mọi bản publish.
- `[review]`: hướng dẫn, không công cụ nào chặn. Người review kiểm bằng lệnh ghi kèm. Luật `[review]` áp cho code **mới hoặc sửa**; chỗ vi phạm có sẵn được liệt kê, không nhân thêm.

## Giới hạn đo được (công cụ ép)
- `gofmt` sạch: `gofmt -l` trả rỗng (tab, thứ tự import do `gofmt` sắp: stdlib, dòng trống, module ngoài và `github.com/naniiluja/ccw/...`). `[tool]`
- `go vet ./...` sạch. `[tool]`
- `staticcheck ./...` sạch với bộ mặc định cộng ST1003, ST1016 (`staticcheck.conf`). Muốn tắt một check phải ghi lý do và số đo vào chính file đó. `[tool]`
- Độ phức tạp cyclomatic mỗi hàm **tối đa 30** (`gocyclo -over 30`, bỏ qua `_test.go`). Hàm cao nhất hiện nay đúng bằng ngưỡng: `(*api).reviewErrorGroup` 30 (`internal/httpapi/errreview.go`), nên thêm một nhánh vào nó là đỏ; tách thành bước có tên trước. `[tool]`
- File `.go` không test **tối đa 1000 dòng**; file test được miễn (bảng case dài ra theo số case). `[tool]` `TestNonTestGoFilesStayUnderTheSizeLimit`. Lớn nhất hiện nay: `httpapi/v1.go` 969, `httpapi/quota.go` 910, `httpapi/quota_resets.go` 874.
- Đúng một comment `// Package x ...` mỗi package (lệnh dùng `// Command x ...`). `[tool]` `TestEachPackageHasExactlyOnePackageComment`.
- Không vòng import, chỉ `cmd/ccw` import `internal/httpapi`, `store` không import `httpapi`/`translate`/`drift`. `[tool]` các test trong `arch_test.go`, chi tiết ở `architecture.md`.

## Cấu trúc package
- Không tạo package mới trước khi có nhu cầu thật (hai nơi dùng độc lập, hoặc một ranh giới phụ thuộc cần bảo vệ). Một package 15 đến 20 file là bình thường (`net/http` có 17). `internal/httpapi` hiện có 21 file không test. `[review]` `go list ./...` trong diff.
- Interface nhỏ, khai báo ở **phía dùng**, chỉ khi có từ hai cài đặt hoặc cần seam cho test. Hiện có ba: `translate.Signatures`, `translate.Flusher`, `websearch.Searcher`. `[review]` `grep -rn '^type [A-Za-z]* interface' --include='*.go' internal`.
- Functional options chỉ khi có từ ba tham số tùy chọn trở lên; ít hơn thì dùng tham số hoặc struct. `[review]`

## Hàm và handler
- `context.Context` luôn là tham số đầu. `[review]` `grep -rnE ', ctx context\.Context' --include='*.go' internal cmd`. Ngoại lệ có sẵn: bảng `quotaFetchers` (`quotaCodex(a *api, ctx, ...)` và năm hàm cùng chữ ký ở `internal/httpapi/quota.go`).
- Hàm có từ 5 tham số trở lên, không tính `ctx`, gom vào một struct. `[review]` Ngoại lệ có sẵn: `oauth.Refresh`, `(*api).send`, `writeAPIError`.
- Handler mỏng: giải mã và kiểm tra đầu vào, gọi một hàm bước trả `(T, error)`, gọi `writeError` ngay tại chỗ gọi. Hàm điều phối không chứa logic nghiệp vụ dài (tiền lệ: `v1Request`/`v1Refusal` ở `v1.go`, `claimRequest`/`refuseClaim` ở `quota_resets.go`). `[review]`, độ dài nhánh bị `gocyclo` chặn gián tiếp.

## State và global
- Mỗi state struct có constructor `newX()` ngay cạnh kiểu; `newServerWithLog` gọi constructor, không rải composite literal. Tiền lệ: `newCatalog`, `newQuotaCache`, `newKeyLimiter`, `newZenState`, `newCopilotCache`, `newSigStore`, `newAutoState`, `newClaimTables`, `newRateHeaders`. Ngoại lệ có sẵn: `newCatalog` nằm ở `server.go` chứ không cạnh kiểu ở `v1.go`, và `rrNext` còn là literal `map[string]rrCursor{}`. `[review]` `grep -rn '^func new[A-Z]' --include='*.go' internal`.
- Không thêm biến global thay đổi được trong code mới; tiêm qua field của `api` hoặc tham số. `[review]` `grep -rn '^var ' --include='*.go' internal cmd | grep -v _test` (lệnh này hiện in 92 dòng, kể cả các khối `var (`; các override cho test như `maxV1Body`, `copilotTokenURL`, `antigravityProdURL`, `autoTestRetry` là nợ cũ, chuyển sang field để làm sau).
- `init()` chỉ để điền bảng tĩnh. Hiện có ba và đều đúng luật: `translate/gemini.go` (`geminiDropKeys`), `httpapi/quota.go` (`quotaFetchers`), `httpapi/quota_resets.go` (`resetClaimers`). Gộp file phải giữ nguyên các phép gán đó. `[review]` `grep -rn '^func init()' --include='*.go' internal cmd`.

## Lỗi và log
Chi tiết ở `error-handling.md` và `logging.md`. Tóm tắt: bọc lỗi bằng `%w`, so sánh bằng `errors.Is`/`errors.As`, không so chuỗi lỗi trong code mới; log bằng `log/slog` có key/value; không bao giờ log secret (ngoại lệ duy nhất: mật khẩu tự sinh in một lần lúc khởi động đầu). `[review]`, trừ ST1005 cho chuỗi lỗi `[tool]`.

## Luật chung
- Không code chết, không import thừa: git giữ lịch sử. `[tool]` một phần: `go vet` và trình biên dịch chặn import thừa, staticcheck U1000 chặn định danh không dùng. Hàm xuất khẩu không ai gọi thì staticcheck không thấy: `go run golang.org/x/tools/cmd/deadcode@latest -test ./...` khi dọn (`[review]`).
- Comment viết **tiếng Anh**, câu đầy đủ, mở đầu bằng tên hàm hoặc kiểu; giải thích lý do, không nhắc lại code. `[review]`; mở đầu bằng tên là ST1020/ST1021/ST1022, không bật.
- Comment không được ghi mẫu giá trị mật khẩu (`CCW_PASSWORD=<giá trị>`); nêu tên biến thì được. `[tool]` `TestCommentsShowNoSamplePassword` (`cmd/ccw/release_test.go`).
- Không fixture trông như secret thật; mọi `_test.go` cấm chuỗi `ya29.`. `[tool]` `TestFixturesLookLikePlaceholders`.
- Chuyển body giữa provider dùng `json.Number` hoặc giữ nguyên byte để không đổi số thực (`internal/translate`). `[review]`
- Luật ép được đặt ở `.claude/rules` (subagent tự nạp), không chỉ ở output style. `[review]`
