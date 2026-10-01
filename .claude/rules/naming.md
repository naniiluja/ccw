# Naming

Nguồn: Google Go Style Guide (naming, getters, initialisms, repetition), Uber Go Style Guide (error naming), Effective Go (MixedCaps). Mỗi luật ghi kèm **cách kiểm**: `[tool]` là cổng tự động trong `scripts/check.sh` (chạy cục bộ và trong CI), `[review]` là hướng dẫn, không công cụ nào chặn, người review kiểm bằng lệnh ghi kèm.

Bật check đặt tên ở `staticcheck.conf`: `checks = ["inherit", "ST1003", "ST1016"]`. `inherit` đã gồm ST1005 và ST1012.

## Package
- Tên package chữ thường, một từ, không gạch dưới, không MixedCaps (`httpapi`, `servertools`). Nguồn: Google Style "Package names". `[tool]` ST1003 báo tên package có gạch dưới hoặc chữ hoa.
- Không đặt `util`, `common`, `helpers`, `misc`, `shared`: tên phải nói package cung cấp gì. Nguồn: Google Style "Util packages", Go blog "Package names". `[review]` `go list ./...` rồi nhìn tên cuối.
- Không lặp tên package trong tên hàm xuất khẩu: `quota.Fetch`, không `quota.FetchQuota`; `auth.NewLoginGuard` là constructor nên được phép. Nguồn: Google Style "Repetition". `[review]` `grep -rnE '^func ([A-Z][a-z]+)' internal/<pkg>` và so với tên package.

## Định danh
- MixedCaps cho mọi định danh, không `snake_case`, không `ALL_CAPS` cho hằng (`maxFileLines`, `MinPasswordLength`). Nguồn: Effective Go "MixedCaps", Google Style "Constant names". `[tool]` ST1003.
- Initialism viết đồng nhất cả cụm: `ID`, `URL`, `API`, `HTTP`, `JSON`, `SQL`, `TTL` (`connID`, `baseURL`, `PendingLoginTTL`); `OAuth` theo cách viết thông dụng. Nguồn: Google Style "Initialisms". `[tool]` ST1003 (đo khi bật: 1 vi phạm, đã sửa ở task 028).
- Getter không có tiền tố `Get`: `Counts()`, không `GetCounts()`. Dùng `Compute`, `Fetch`, `Load` khi hàm tính toán hay gọi xa. Nguồn: Google Style "Getters", Effective Go "Getters". `[review]` `grep -rnE '^func .* Get[A-Z]' --include='*.go' internal`. Ngoại lệ có sẵn, không nhân thêm: `(*Store).GetSetting`, `(*Store).GetUpstreamError`. Handler HTTP tên `getRotation`, `getModelPolicy`, `getDef` đặt theo động từ HTTP `GET`, không phải getter.
- Biến lỗi: `ErrXxx` (xuất khẩu) hoặc `errXxx` (nội bộ). Nguồn: Uber Style "Error Naming". `[tool]` ST1012.
- Kiểu lỗi có hậu tố `Error` (`FetchError`, `RequestError`, `toolSearchError`). Nguồn: Uber Style "Error Naming". `[review]` `grep -rn ') Error() string' --include='*.go' internal`. Ngoại lệ có sẵn: `errSkipAccount` (`internal/httpapi/v1.go`).
- Chuỗi lỗi chữ thường đầu câu, không dấu chấm hay dấu hai chấm cuối, vì nó được nối vào chuỗi khác qua `%w`. Nguồn: Go Code Review Comments "Error Strings", Google Style "Error strings". `[tool]` ST1005.

## Receiver
- Receiver ngắn (một chữ cái hoặc chữ viết tắt của kiểu: `a *api`, `s *Store`, `c *Config`), không `this`/`self`, và **cùng một tên** cho mọi method của một kiểu. Nguồn: Google Style "Receiver names", Go Code Review Comments "Receiver Names". `[tool]` ST1016 kiểm tính nhất quán; độ dài là `[review]`.

## File và test
- File `snake_case.go` chữ thường, một khái niệm mỗi file (`loginguard.go`, `quota_resets.go`); test là `<file>_test.go` cạnh nó, cùng package. Nguồn: quy ước của repo (Go không quy định tên file ngoài `_test.go`). `[review]` `find internal cmd -name '*.go' | LC_ALL=C grep '/[^/]*[A-Z][^/]*$'` phải rỗng (cần `LC_ALL=C`: với locale mặc định của macOS, `[A-Z]` khớp cả chữ thường).
- Tên test `TestĐơnVịVàHànhVi` dạng câu mô tả hành vi: `TestZenStreamLeavesInTheOpenAIShape`, `TestFirstStartPrintsThePasswordOnceAndStoresOnlyAHash`. Nguồn: Google Style "Test names" (tên nói hành vi), quy ước của repo. `[review]` `grep -rn '^func Test' --include='*_test.go'`.
- Lệnh (`package main`) mở đầu bằng `// Command x ...`, package khác bằng `// Package x ...`. Nguồn: Go Doc Comments "Packages", "Commands". `[tool]` `TestEachPackageHasExactlyOnePackageComment` (`internal/httpapi/arch_test.go`).

## Giá trị lưu trong DB
Chuỗi đã ghi vào DB (`By`, `intact-review`, tên principal) giữ nguyên chữ cũ dù không khớp tên mới: đổi chúng là migration dữ liệu, không phải đổi tên. `[review]`
