# Architecture

Một module Go (`github.com/naniiluja/ccw`), 16 package (`go list ./...`): `cmd/ccw` và 15 package dưới `internal/`. 77 file `.go` không test (`find internal cmd -name '*.go' -not -name '*_test.go' | wc -l`), khoảng 23k dòng.

## Import graph (đo bằng `grep -o 'ccw/internal/[a-z]*"' --exclude='*_test.go' -r internal cmd`)
- **Lá, không import package nội bộ nào:** `auth`, `filter`, `oauth`, `provider`, `translate`, `upstream`, `usage`, `websearch`, `webui`, `zen`.
- **Trung gian:** `store` → `provider`; `drift` → `store`; `servertools` → `websearch`; `diabridge` → `websearch` (bridge chạy web search bằng đúng dịch vụ `/v1` dùng, qua `SetSearch`).
- **Đỉnh:** `internal/httpapi` import cả 12 package nội bộ còn lại (kể cả `webui`, mount ở `/ui/`); `cmd/ccw` chỉ import `auth`, `diabridge`, `httpapi`, `store`.
- Hướng một chiều: `cmd/ccw` → `httpapi` → (`servertools`, `drift`) → `store` → lá.

## Package (một trách nhiệm mỗi package)
- `auth`: mật khẩu (PBKDF2, `password.go`), session cookie ký HMAC (`session.go`), `Config` và master token (`config.go`), giới hạn đăng nhập sai `LoginGuard` (`loginguard.go`).
- `store`: SQLite, mọi SQL; schema đánh số bằng `PRAGMA user_version` (danh sách `migrations` trong `store.go`), mỗi domain một file (`apikey.go`, `connection.go`, `drift.go`, `errors.go`, `filter.go`, `models.go`, `oauth.go`, `pending.go`, `providerdef.go`, `settings.go`, `usage.go`).
- `provider`: định nghĩa provider bằng dữ liệu (`Def`, registry). `translate`: dịch shape OpenAI Chat, Anthropic Messages, Responses, Gemini, Zen. `filter`: blacklist field. `drift`: học shape và phát hiện thay đổi. `oauth`: refresh token. `upstream`: client gọi provider và chính sách retry. `usage`: đọc usage từ body. `servertools`: tool phía server (fetch, MCP client, tool search). `websearch`: tìm kiếm web. `zen`: OpenCode Zen.
- `diabridge`: cầu nối hai đầu giữi `claude` của Dia và tiến trình `handler` mà Dia spawn (`bridge.go`), lấy model từ path Vertex (`extract.go`), viết lại request cho gateway (`compat.go`), chạy web search dùng chung (`search.go`). Không còn là lá vì `search.go` import `websearch`.

## Bản đồ file `internal/httpapi` (23 file không test)
Gộp theo khái niệm ở task 020 đến 022. Đếm: `find internal/httpapi -maxdepth 1 -name '*.go' -not -name '*_test.go' | wc -l`.

- `server.go`: kiểu `api`, `newServerWithLog`, `registerRoutes` (mọi route), middleware `requireSession`/`requireToken`/`requireAdmin`/`v1API`, error envelope (`writeError`, `writeAPIError`), `openapi.json` (`go:embed`), `readBody`, `writeJSON`.
- `auth.go`: `POST /login`, `/logout`, cookie thiết bị, `guardRequest` (chặn ghi cross-site), `principal`.
- `v1.go`: handler `/v1`, failover giữa các tài khoản, `send` (lời gọi upstream có log).
- `proxy.go`: dựng request ra (`newOutbound`), relay và stream, dịch shape khi lệch, ghi usage.
- `rotation.go`: xoay vòng tài khoản, `attemptsFor`. `rewrite.go`: echo model và đổi tên model trong phản hồi.
- `models.go`: bảng model, test model và tài khoản, autotest. `modelinfo.go`: thông tin model và biến thể theo provider.
- `providers.go`: phần riêng của Antigravity, Copilot, Zen. `websearch.go`: tìm kiếm web và tool phía server.
- `accounts.go`: tài khoản, provider defs, dashboard API. `keys.go`: API key, giới hạn RPM, múi giờ (`CCW_TZ`).
- `filters.go`: blacklist field, từ chối bộ lọc không an toàn (`unsafeFilter`), `GET /api/providers`. `oauth.go`: refresh OAuth và luồng đăng nhập provider (start, finish, device poll).
- `quota.go`: quota theo provider, header rate limit, bảng `quotaFetchers`. `quota_resets.go`: claim reset quota.
- `drift.go`: API drift và AI drift review. `errlog.go`: bảng `upstream_errors` và API `/errors`. `errreview.go`: AI error review. `errbisect.go`: chia đôi system prompt để tìm đoạn bị provider từ chối.
- `session.go`: `GET /api/session` (công khai, ba boolean `authRequired`, `authenticated`, `admin`, không lộ secret), `uiLoginPath`, `hasSession`. `settings.go`: `GET /api/settings` (`requireToken`), cấu hình hiệu lực chỉ đọc (`authMode` là `password` hoặc `none`, TTL session, múi giờ, web search; không bao giờ có secret).
- `mcp.go`: máy chủ MCP tại `/mcp`, `mcpAdminTools`.
- Test kiến trúc và kích thước: `arch_test.go`.

## Package `webui` và bản đồ `fe/`
- `internal/webui` (`webui.go`): `//go:embed all:static`; `Handler()` phục vụ dưới `/ui/`. Đường dẫn không có đuôi file trả `index.html` (`Cache-Control: no-cache`, để deep link như `/ui/keys` làm mới được), `assets/*` có `immutable`, chưa build thì `503 UI chưa được build`; chỉ `GET`/`HEAD`. `static/` bị gitignore trừ `.gitkeep`, do `scripts/ui-build.sh` điền. Lá, không import package nội bộ nào.
- `fe/src/api/`: client `fetch` và hook TanStack Query theo từng nhóm endpoint (`accounts`, `keys`, `quota`, `session`, `settings`...). `fe/src/app/`: router (`routes.tsx`), `require-auth.tsx`, `create-app.tsx`. `fe/src/components/ui/`: primitive shadcn (không sửa tay). `fe/src/components/app/`: khung ứng dụng (sidebar, command palette, theme). `fe/src/pages/<trang>/`: 10 trang (`overview`, `accounts`, `providers`, `keys`, `usage`, `quota`, `errors`, `drift`, `filters`, `settings`) cộng `login` và `not-found`. `fe/src/test/`: render helper, MSW server. Luật chi tiết ở `frontend.md`.

## Design patterns
- Provider khai báo bằng dữ liệu (`provider.Def`), code riêng chỉ ở chỗ shape khác biệt (`providers.go`, `modelinfo.go`, `quota.go`).
- Background loop khởi động trong `newServerWithLog`: `autoTestLoop`, `reviewLoop`, `errorReviewLoop`.


## Package `diabridge` và provider `dia`
Dia không có API HTTP dùng được: `dist/claude` của app gửi **plain Messages JSON** tới `127.0.0.1:<port>` mà tiến trình `handler` mở ra **mới mỗi lượt chat**, kèm token do chính handler sinh; chỉ từ đó mới đi qua IPC tới app, và app mã hoá trước khi ra mạng. `diabridge` ngồi đúng tầng plaintext đó, hai cổng:
- **mặt trước** `20139` (mặc định `-dia-bridge-front`): `ReverseProxy` giữ nguyên mọi header, kể cả `Authorization` mà `claude` tự gửi, nên handler vẫn xác thực đúng — bridge không cần biết token.
- **mặt sau** `20140` (mặc định `-dia-bridge-back`, và là `BaseURL` của entry `dia` trong registry): `POST /v1/messages` đọc `{port, token}` từ file handler ghi ra, dựng path `…/publishers/anthropic/models/<model>:streamRawPredict`, gửi kèm `Bearer <token>` và chuyển nguyên status + body (kể cả SSE) về. `GET /health` báo `handlerPort` và model cuối cùng đi qua mặt trước.

**Mặt sau dịch request cho khớp gateway (`compat.go`).** Gateway của Dia không phải endpoint Anthropic tổng quát: nó chỉ nhận đúng shape mà Claude Code của Dia tạo ra, còn lại trả `400` — và `400` đó kết thúc turn của handler, kéo theo listener. Hai ràng buộc xảy ra với mọi client bên thứ ba nên mặt sau viết lại trước khi gọi:
- **Tên tool.** Gateway whitelist tên PascalCase của Claude Code (`Bash Read Write Edit Glob Grep TodoWrite Task Skill TaskCreate TaskGet TaskList TaskUpdate Agent AskUserQuestion`) và trả `400 "Request contains an unrecognized tool"` cho tên khác. Mỗi tên của caller được đổi sang tool Claude Code tương đương (`shell`→`Bash`, `read`→`Read`, `todo`→`TodoWrite`…); tên không có tương đương thì **bỏ hẳn**, vì gửi nó dưới tên không khớp schema sẽ khiến model gọi sai tool. **Ngoại lệ là `WebSearch` và `WebFetch`**: gateway không nhận tên đó, nhưng chúng là tool client (Claude Code tự chạy), nên chúng **mượn một tên whitelist mà request chưa dùng** (`spareSlots`, theo thứ tự `Glob Grep TodoWrite Task Skill AskUserQuestion`) và vẫn giữ nguyên schema và description của mình — gateway chỉ kiểm tên, không kiểm schema. Tool thật mà client khai báo được xếp trước, nên `Glob`/`Grep` thật không bị mượn. Tên đã đúng kiểu Claude Code đi qua nguyên vẹn, nên một client Claude Code trỏ vào ccw không cần viết lại gì. Việc đổi tên lan sang `tool_choice`, các khối `tool_use` trong `messages`, và được đảo ngược trên phản hồi (dòng `content_block_start` khi stream, `assembleSSE` khi không stream) để client khớp được kết quả với tool của nó.
- **Thinking.** Gateway không chấp nhận hai dạng và trả `400`, kết thúc turn của handler: `thinking.type=enabled` (`"thinking.type.enabled" is not supported for this model"`) và `thinking.type=disabled` (`"thinking.type.disabled" is not supported for this model`, đây là dạng Claude Code gửi ở **request phụ** của `WebSearch`). Dạng `enabled` lấy `thinking.type=adaptive` cộng `output_config.effort`, ngân sách cố định của caller gấp thành mức effort (`low`/`medium`/`high`); dạng `disabled` thì **bỏ hẳn field `thinking`** (gateway nhận request không có field này). `thinking.type=adaptive` kèm `display` gửi `"updates"` cũng bị từ chối (`"Input should be 'summarized', 'omitted'"`) nên được đổi thành `summarized`.
- **Turn `role:"system"`** trong `messages` bị gateway **lặng lẽ bỏ**. Claude Code đặt output style và các chỉ dẫn của client vào đúng turn đó, nên qua dia mất output style trong khi mọi provider khác nhận được. `translate.HealAnthropic` gộp các turn `system`/`developer` vào field `system` top-level trước khi gửi — sửa ở đó vì `HealAnthropic` chạy cho **mọi** provider dạng Anthropic, không riêng dia.

**Web search dùng chung (`search.go`).** Gateway không có tool search, nên request khai báo `WebSearch`/`web_search` được bridge **chạy trước** rồi gộp kết quả vào system prompt qua `websearch.Rewrite`, y hệt `/v1` làm (`hostedSearch`). Dịch vụ tìm kiếm là **cùng cái** mà `/v1` dùng: `cmd/ccw` xây `api` trước rồi truyền `bridge.SetSearch(ccw.Searcher())`, nên dia tìm qua Antigravity giống mọi provider khác, không có bản implement thứ hai. Cả hai mặt đều chạy: mặt sau cho caller đi qua `/v1`, mặt trước cho `claude` của Dia.

Vá Dia là `scripts/dia-patch.sh` (idempotent, phải chạy bằng `sudo` vì TCC chặn sửa bundle đã ký), gồm ba thay thế byte tại đúng vị trí, delta 0 nên offset segment Mach-O không dịch: (1) `NLr` — bỏ kiểm tra tiền kiệm tool, nếu không Claude Code CLI bị từ chối với `400 Invalid tool declarations`; (2) `kt.ANTHROPIC_VERTEX_BASE_URL` đổi thành `http://127.0.0.1:20139` (offset 66876196, hàm `bkr` — chỗ duy nhất bundle tra gateway vừa mở, và **cùng phạm vi** với `tc`); (3) ngay sau đó chèn persistent bridge mode: `await(async()=>{ if(/^\/?(bridge|ccw)$/i.test(e?.prompt?.trim()??"")){ ghi {port:p.port,token:tc} bằng Bun.write (fallback require("fs").writeFileSync); await new Promise(r=>t?.onCancel?.addEventListener("abort",r)); throw AbortError } })()`. Profile Seatbelt `agent-claude-code.sb` được vá thêm để cho phép outbound tới `localhost:20139`. **Hệ quả đã chấp nhận:** chữ ký thành ad-hoc nên Sparkle từ chối cập nhật Dia; mỗi lần Dia tự cập nhật phải chạy lại script.

Bridge đọc file **mỗi request**, không cache: handler sinh lại cả cổng lẫn token mỗi lượt chat. Bật persistent bridge mode bằng cách gõ `bridge` hoặc `ccw` trong một tab Dia; handler giữ cổng và token sống liên tục cho tới khi bấm Stop trong Dia. Chưa bật bridge mode thì mặt sau trả `503` kèm lời nhắc gõ 'bridge' trong Dia — đây là trạng thái bình thường, không phải lỗi. **Bật bằng bàn phím theo đúng thứ tự: gõ `bridge`, bấm `cách`, bấm `↓`, rồi `Enter`** — bỏ `cách` thì Dia coi turn là tìm kiếm omnibox, `Enter` ngay chạy Google; bỏ `↓` thì field vẫn ở chế độ browse nên turn không được gửi. Ô gõ giữ lại text của lượt trước, nên phải xoá sạch (`cmd+a` rồi `delete`) trước khi gõ, nếu không ra `bridgebridge` và không khớp. `scripts/dia-bridge.sh` làm đúng chuỗi đó. Một turn bị `400` cũng giết listener, nên `503` sau đó là hệ quả chứ không phải hỏng bridge — gõ lại `bridge` là phục hồi.

**Tự phục hồi (`recover.go`).** Handler dựng gateway mới mỗi turn, và một turn bị `400` cũng giết listener, nên `503` là trạng thái thường gặp chứ không phải hỏng. Mặt sau không trả `503` luôn: nó gọi `liveGateway`, và khi không có gateway thì **tự gõ `bridge` vào Dia rồi chờ** — caller vẫn nhận một câu trả lời bình thường thay vì bị từ chối. Vòng chờ có giới hạn (`recoverWait`) và ngắt theo context của request.

**Cần cấp quyền cho `osascript`, một lần.** Gõ phím từ tiến trình nền đòi macOS **Accessibility** (System Settings → Privacy & Security → Accessibility). Chưa cấp thì log có `-1719 osascript không phải là truy cập hỗ trợ được cho phép` hoặc `(1002) osascript không được phép gửi các thao tác nhấn phím`, và caller nhận `503` kèm lời nhắc gõ tay — không có gì hỏng. Cấp cho `osascript` một lần là xong; không cần cấp riêng cho `ccw`. Nếu Dia đang đóng, `canDriveDia` trả false và cũng vậy.

**Vì sao phải gõ phím chứ không bấm chuột.** Đường click + clipboard (mở tab, click ô chat, Select All và Paste qua menu Edit) chạy được không cần quyền gì, nhưng **không** bấm được nút gửi: nút đó nằm trong webview của Dia, và System Events trả về `missing value` khi click vào đó — webview bỏ qua sự kiện chuột tổng hợp. Bàn phím thì đi qua, nên `recoverScript` gõ `cmd+t`, gõ `bridge`, bấm `↓` rồi `Enter`. Ba chi tiết tất định, mỗi cái tìm ra từ một lần thất bại: `with timeout` phải bọc cả chuỗi `tell` (System Events bỏ AppleEvent sau 2 giây, các delay cộng lại ~6 giây, không có nó thì `-1712`); dùng `set frontmost` chứ không `activate` (dùng cả hai thì `-609`); và `↓` phải tới khi danh sách gợi ý **còn hiện**, vì mục đầu là tìm kiếm Google nên `Enter` ngay sẽ chạy tìm kiếm.

**Restart agent-server khi Dia kẹt.** Sau một lượt nhiều turn hỏng, agent của Dia trả `Something went wrong` cho *mọi* tin nhắn và không chạm tới code đã vá, nên gửi `bridge` bao nhiêu lần cũng không có gateway — đây là lỗi của Dia, không phải của bridge. `waitFor` vì thế restart `agent-server` một lần sau `recoverRestartAfter` giây; nó tự khởi động lại vài giây sau, nên làm lúc Dia đang mở cũng an toàn.

**Các lời gọi chờ nhau** dùng chung một lần gõ trong cửa sổ `recoverCooldown`, nên một đợt nhiều caller không mở dồn tab trong Dia. `recoverWait` dài hơn `recoverCooldown` có chủ ý: caller đến giữa lúc một lần phục hồi khác đang chạy không bị từ chối, mà chờ nốt rồi tự gửi nếu lần trước không ăn.
Entry `dia` trong registry dùng `Setup: "none"`, `DefaultSecret: "bridge"`, `AuthHeader: ""` (bridge tự gắn token) và `Clean: true` (header của caller không lọt vào tiến trình Dia), nên `SeedKeylessConnections` tự tạo connection và không có nhánh `dia` nào trong `oauth.go`. `shapeFor` trả `("v1/messages")` vì đó là path của chính bridge; bridge tự dựng lại path `streamRawPredict`. Bridge từ chối model chứa `:` vì handler cắt model bằng `([^:]+)`.
**Model của Dia** lấy từ `dist/agents/*/spec.yaml` (`ANTHROPIC_DEFAULT_OPUS_MODEL` và `..._SONNET_MODEL`), không phải từ `strings` của app binary — binary `Dia` đã strip nên `strings` bỏ sót. Năm tên: `claude-opus-5-5`, `claude-opus-5`, `claude-opus-4-8`, `claude-sonnet-5`, `claude-opus-4-6`. Hậu tố `[1m]` trong env var **không** phải tên model: tài liệu của Claude Code trong `handler.map` nói rõ "the canonical model name also matches its dated, [1m], Bedrock and Vertex spellings" — đó là cách viết cho cửa sổ 1 triệu token, chỉ xuất hiện **1 lần** trong toàn bundle. Registry chỉ khai báo tên sạch. `claude-sonnet-5-5` cũng có trong spec nhưng **không** được khai báo: handler phục vụ model đầu tiên của một turn qua Bedrock (`/v1/bedrock/…`) rồi chuyển sang Vertex (`/v1/google/…`) cho mọi lần đổi model sau đó, và Vertex trả `400 "Offering [claude-sonnet-5-5] not whitelisted for use-case [dia-browser]"`. Năm tên trên đã đo bằng cách đổi sang chúng sau khi một model khác đã chạy, tức đúng đường đi qua Vertex.

**Claude Code của Dia không có khả năng coding.** Ba lớp, đều đọc từ `dist/handler.map`: (1) `disallowedTools` tính bằng `$2()` từ base list `yce` — 20 tool (`CronCreate`, `Task`, `WebFetch`, `WebSearch`, `Workflow`…); (2) `canUseTool` từ chối mọi tool trừ `AskUserQuestion` với `{behavior:"deny"}`; (3) Seatbelt `agent-claude-code.sb` — ghi file chỉ trong `<CONTEXT_DIR>/work`, `process-exec` chỉ 13 lệnh (`ls cat cp mv mkdir grep awk sed touch find plutil security` + `sh`/`bash`), và `network-outbound` chỉ `localhost:$GATEWAY_PORT`. Sản phẩm là engine suy luận cho connector SaaS (`tool-groups.yaml`: Atlassian, Notion, Amplitude, Granola, Zoom, Salesforce) chứ không phải agent coding.


## Where things go
- Route mới: đăng ký trong `registerRoutes` (`server.go`), handler đặt trong file cùng khái niệm ở bản đồ trên. `[review]`
- Route dưới `/v1` phải bọc `v1API` để có error envelope theo shape của caller, header `Request-Id` và logger mang `req_id`. `[review]` đọc `registerRoutes`; hành vi envelope có test ở `v1_errors_test.go`.
- SQL mới: chỉ trong `internal/store/`, đúng file domain; đổi schema là thêm một phần tử vào cuối `migrations`, không sửa phần tử cũ. `[review]` `grep -rln 'database/sql' --include='*.go' internal cmd | grep -v _test.go | grep -v '^internal/store/'` phải rỗng; `[tool]` `migrate_test.go` (`TestOpenUpgradesAnOldDatabase`, `TestFailedMigrationKeepsTheOldVersion`).
- Chuyển đổi shape: `internal/translate/`. Caller và provider cùng shape thì body đi qua nguyên vẹn. `[review]`
- Tool phía server: `internal/servertools/`, `internal/websearch/`. `[review]`
- UI: SPA ở `fe/`, nhúng vào binary bởi package `webui` và phục vụ tại `GET /ui/` (công khai, vì trang đăng nhập phải tải được trước khi có session). `GET /` chuyển `302` sang `/ui/`; `POST /login` và session ở `internal/httpapi/auth.go`. Đổi code `fe/` thì chạy `bash scripts/ui-build.sh` rồi build lại binary. `[review]`

## Auth tiers (không được nới lỏng)
- `requireSession`: route dashboard trình duyệt, cần session cookie; thất bại thì chuyển hướng `302` về `/ui/login` (`uiLoginPath`), hoặc `401` với client gửi `Accept: application/json`.
- `requireToken`: `/v1` và đọc `/api/*`; nhận master token (`CCW_API_TOKEN`), API key trong DB hoặc session; áp giới hạn RPM theo key.
- `requireAdmin`: mọi thao tác ghi qua `/api/*`; chỉ session hoặc master token, API key của dashboard bị từ chối `403`. Tool MCP ghi cấu hình nằm trong `mcpAdminTools` (`mcp.go`). Lưu ý route errors review đăng ký qua biến `read`/`write` trong vòng lặp (`write` = `requireAdmin` khi có tiền tố `/api`), nên grep `requireAdmin` không thấy hết.

## Verifiable rules
- Không vòng import. `[tool]` `TestPackagesHaveNoImportCycles` và `go build ./...`.
- Chỉ `cmd/ccw` được import `internal/httpapi` (chặt hơn "lá không import httpapi"). `[tool]` `TestOnlyCmdImportsHTTPAPI`.
- `store` không import `httpapi`, `translate`, `drift`. `[tool]` `TestStoreDoesNotImportHigherLayers`.
- Package lá ở trên không import package nội bộ nào. `[review]` lệnh grep ở đầu file; chưa có test ép.
- Mỗi package một trách nhiệm, đúng một `// Package x`. `[tool]` cho comment (`TestEachPackageHasExactlyOnePackageComment`), `[review]` cho trách nhiệm.
- Không thêm route ghi cấu hình dưới `/api` mà thiếu `requireAdmin`. `[tool]` một phần: `internal/httpapi/adminroutes_test.go`; `[review]` đọc `registerRoutes`.
- Tất cả test kiến trúc nằm ở `internal/httpapi/arch_test.go`, chạy trong `go test -race ./...` của `scripts/check.sh`.
