# Frontend (`fe/`)

Dashboard là SPA React trong `fe/`, build ra `fe/dist`, được `scripts/ui-build.sh` copy vào `be/internal/webui/static/` và nhúng vào binary. Stack ở `tech-stack.md`, bản đồ thư mục ở `architecture.md`. Mỗi luật ghi **cách kiểm**: `[tool]` là cổng tự động (`bash scripts/check.sh` chạy `pnpm lint`, `typecheck`, `test`, `build` trong `fe/`), `[review]` là hướng dẫn kèm lệnh kiểm. Lệnh chạy từ gốc repo.

## Không chế component
- Mọi tệp trong `fe/src/components/ui/` do `shadcn add` sinh ra, từ shadcn/ui hoặc registry `@originui` (`https://coss.com/origin/r/{name}.json`, khai báo ở `fe/components.json`). Thêm bằng `pnpm -C fe ui:add <tên>` (hoặc `@originui/<tên>`), tra trước bằng shadcn MCP (`tooling.md`). Không viết tay một primitive mới vào `ui/`. `[review]` xem diff của `fe/src/components/ui/` có kèm thay đổi `fe/components.json` hoặc `fe/package.json` khi thêm mới; mọi tệp có `data-slot` (trừ `sonner.tsx`): `grep -L 'data-slot' fe/src/components/ui/*.tsx`.
- Ngoài `components/ui/`, chỉ ghép từ primitive shadcn, không tự dựng hộp thoại, menu, popover, bảng từ phần tử thô. Cần biến thể mới thì thêm bằng `shadcn add` hoặc ghép primitive có sẵn. `[review]`
- Chỉ `components/ui/` được import `radix-ui`, `@radix-ui/*` hay `@base-ui`. `[tool]` `fe/src/components/ui-boundary.test.ts` (`pnpm -C fe test`).
- Sửa một tệp `ui/` đã sinh (chỉnh style theo thiết kế) phải nhỏ và ghi lý do trong commit; không đổi API công khai của primitive. `[review]`

## Đặt tên và cấu trúc
- Tệp `fe/src` dùng `kebab-case` (`app-sidebar.tsx`, `use-mobile.ts`); không tệp nào có chữ hoa. `[review]` `find fe/src -type f \( -name '*.ts' -o -name '*.tsx' \) | grep -E '/[^/]*[A-Z][^/]*$'` phải rỗng.
- Component và kiểu `PascalCase`, hàm và biến `camelCase`, hook bắt đầu bằng `use`. `[review]`
- Mỗi trang ở `fe/src/pages/<trang>/index.tsx` kèm các tệp con cùng thư mục; gọi API ở `fe/src/api/` (hook TanStack Query), không `fetch` trực tiếp trong component. `[review]` `grep -rnE '(^|[^A-Za-z.])fetch\(' fe/src --include='*.ts' --include='*.tsx' | grep -v '^fe/src/api/'` phải rỗng.
- Alias `@/` trỏ tới `fe/src`. Test đặt cạnh tệp (`*.test.ts(x)`). `[tool]` `pnpm -C fe typecheck`.
- Form có ràng buộc phức tạp kiểm bằng zod: `filters/filter-form.tsx`, `providers/rotation-tab.tsx`, `providers/def-dialog.tsx`, `keys/limits-dialog.tsx`, `login/index.tsx`. Ngoại lệ có sẵn, không nhân thêm: form chỉ có ràng buộc đơn giản (bắt buộc, độ dài, số nguyên) kiểm bằng `register` của react-hook-form hoặc tay (`accounts/add-account-dialog.tsx`, `accounts/account-dialogs.tsx`, `accounts/oauth-flow.tsx`, `drift/config-tab.tsx`, `errors/review-panel.tsx`, `errors/filter-bar.tsx`, `keys/create-dialog.tsx`, `keys/models-dialog.tsx`, `providers/policy-tab.tsx`). Phản hồi API dùng kiểu TypeScript khai báo ở `fe/src/api/`, không kiểm lúc chạy, vì hợp đồng do cùng repo giữ. Không `any`. `[review]` `grep -rlE 'onSubmit|useForm' fe/src/pages | xargs grep -L zod` chỉ được ra các tệp ngoại lệ ở trên; `[tool]` `pnpm -C fe lint` (oxlint `--deny-warnings`) và `pnpm -C fe typecheck` chặn `any`.

## Ngôn ngữ giao diện
- Chữ hiển thị cho người dùng viết **tiếng Việt**, không dùng i18n: không thư viện dịch, không khóa ngôn ngữ. Chuỗi của mỗi trang gom ở `strings.ts` cạnh trang (vd `fe/src/pages/keys/strings.ts`). Comment trong code viết tiếng Anh như phía Go. `[review]`
- Không hiển thị lỗi thô của server hay secret; khóa API chỉ hiện một lần lúc tạo. `[review]`

## Test (biên giới đang có)
- Vitest với jsdom, Testing Library và MSW (`fe/src/test/server.ts`, `render.tsx`); test kiểm hành vi người dùng thấy, chạy không cần mạng hay backend thật. `[tool]` `pnpm -C fe test`.
- Mỗi trang có một tệp test cạnh nó (13 tệp test hiện nay), cộng `app.test.tsx`, `routes.test.ts` và `ui-boundary.test.ts`. Một test của `fe/src/pages/keys/keys.test.tsx` thỉnh thoảng đỏ (chập chờn, xem `CLAUDE.md`); chạy lại một lần trước khi kết luận. `[review]`
- Kiểm đầu cuối trên binary thật bằng trình duyệt là bước tay, không có test tự động. `[review]`
- Bản build nhúng (`be/internal/webui/static/`) không bao giờ được commit. `[review]` `git status --short` không được liệt kê tệp trong `static/` ngoài `.gitkeep`.
