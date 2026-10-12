# Dia Claude Opus qua cầu nối native: chặn plaintext ở tầng handler, expose ra ccw

## Context

Provider `dia` đã được tích hợp vào ccw theo kế hoạch trước (`local://dia-opus-integration-plan.md`), nhưng chạy thật cho thấy **đường HTTP trực tiếp không dùng được**: `aiproxy.diabrowser.engineering/api/dia-claude-stream` trả `401 Unauthorized` với token hợp lệ (đã thử bằng `curl` với JWT bắt được từ Proxyman). Endpoint thật mà app dùng là `ai-service.diabrowser.engineering/aiproxy` với `Content-Type: application/octet-stream` và `X-Server-Data-Encrypted` — payload đã mã hoá AWS KMS envelope.

Khảo sát trong phiên này đọc được mã nguồn thật của `dist/handler` (bundle JavaScript, có `handler.map` 5.3 MB) và xác định chính xác chuỗi 5 tầng:

| Tầng | Thành phần | Dạng | Xác minh |
|---|---|---|---|
| 1 | `dist/claude` (Mach-O arm64, 217 MB) | HTTP **thuần** tới `127.0.0.1:<port>` | log `claude auth env: ... VERTEX_BASE_URL=http://127.0.0.1:49161` |
| 2 | `dist/handler` (Mach-O chứa JS, spawn mỗi lượt) | JSON, có cổng auth | đọc trực tiếp mã ở offset 66776400 |
| 3 | `dist/agent-server` (Mach-O chứa JS) | JSON qua IPC | `case "LLM_REQUEST": { this.forwardRequest(a); break; }` |
| 4 | `Contents/MacOS/Dia` (Swift, **đã strip**) | mã hoá Watchtower + AWS KMS | `nm \| grep -ci watchtower` = 0 trong 682 symbol |
| 5 | Browser Helper `network.mojom.NetworkService` | ciphertext tới `ai-service...` | `Content-Type: application/octet-stream` |

Ba phát hiện quyết định hình dạng kế hoạch:

**(a) `handler` có cổng auth.** Đọc mã tại offset 66776400:
```js
let a = OLr((l, u) => {
  if (l.headers.authorization !== `Bearer ${r}`) {
    u.writeHead(401, { "content-type": "application/json" }), u.end(JSON.stringify({ error: "Unauthorized" }));
    return;
  }
  let d = PLr(l.url ?? "/");
  ...
```
Token `r` do chính handler sinh: ``let tc = `sk-${akr(32).toString("hex")}` ``, rồi truyền cho tiến trình `claude` con qua biến môi trường `ANTHROPIC_AUTH_TOKEN`. Nó bị redact thành chữ `set` trong log (`Ie = new Set(["ANTHROPIC_AUTH_TOKEN", ...])`) nên **không đọc được từ log**. Hệ quả: gọi thẳng vào cổng của handler mà không có token sẽ nhận `401` — và lấy token từ log là con đường đóng.

**(b) Nhưng ta kiểm soát được chính `claude`.** `VERTEX_BASE_URL` do handler gán, nên vá nó trỏ sang một proxy của ta. `claude` tự gửi kèm `Authorization: Bearer <token>` của chính nó, và handler vẫn xác thực đúng vì token đó là token thật. **Không cần đọc token, không cần sửa logic auth.**

**(c) `handler` xử lý được cả stream lẫn non-stream.** Đọc offset 66778700: nhánh `if (d.endpoint === "streamRawPredict")` gọi `e(S, g, JSON.stringify(h), { stream: true, onChunk })`; nhánh `else` phía dưới xử lý non-stream và có cache cho `countTokens`. Endpoint được dispatch qua `PLr(l.url)`, nên proxy phải gọi đúng path mà `claude` dựng ra.

Hai lỗi trong kế hoạch cũ phải sửa: (1) binary `Dia` đã strip nên hook `send()` là sai chỗ — plaintext không đi qua đó; (2) model `claude-opus-4-6[1m]` không tồn tại trong Dia 1.51.1 — binary chính chỉ có `claude-opus-4-6` (offset `0x60d51c8`) và `claude-opus-4-8` (`0x60d51d8`), `strings | grep -E "1m\]"` rỗng, và `handler` chỉ liệt kê `"claude-opus-4-6"`, `"claude-opus-4-8"`. Hậu tố `[1m]` là annotation của Claude Code cho tool, không phải tên model.

Kết quả cuối: `dia` trở thành provider ngang hàng `antigravity`/`claude` — tự động hoá, có failover, quota, drift.
## Approach

### Bước 1 — Vá `handler`: `VERTEX_BASE_URL` trỏ vào proxy của ta

`dist/handler` tự mở HTTP server trên cổng ngẫu nhiên rồi gán cho `claude` con. Dòng gán duy nhất trong toàn bộ bundle (5 chỗ `ANTHROPIC_VERTEX_BASE_URL` còn lại là danh sách hằng, không phải gán — grep đã xác minh từng chỗ):

```js
kt.ANTHROPIC_VERTEX_BASE_URL = `http://127.0.0.1:${p.port}`
```

Đổi thành cổng cố định của proxy:

```js
kt.ANTHROPIC_VERTEX_BASE_URL = `http://127.0.0.1:20139`
```

Chỉ một hằng số. Xác minh bằng `grep -a -oE 'ANTHROPIC_VERTEX_BASE_URL = .http://127.0.0.1:\\\$\{[a-z]\.port\}'` phải trả **0 dòng**, còn `grep -ac 'ANTHROPIC_VERTEX_BASE_URL'` vẫn bằng 6. Handler vẫn tự mở server trên cổng ngẫu nhiên của riêng nó (đoạn `a.listen(0, "127.0.0.1")`), chỉ không ai trỏ tới nữa — vô hại.

### Bước 2 — `diabridge`: proxy hai đầu, thấy plaintext và token tự nhiên

Tạo `be/internal/diabridge/` (Go, trong module `github.com/naniiluja/ccw`). Bridge lắng nghe **hai cổng**:

| Cổng | Vai trò | Ai gọi |
|---|---|---|
| `20139` | mặt trước — `claude` gọi vào | `dist/claude` (sau khi vá `handler`) |
| `20140` | mặt sau — ccw gọi vào | ccw, qua provider `dia` |

Luồng một lượt chat của Dia: `claude` → `20139` (bridge ghi lại `Authorization`) → cổng ngẫu nhiên của `handler` → handler vẫn xác thực thành công vì token đó là token thật do chính handler sinh → handler emit `LLM_REQUEST` qua IPC → `Dia` mã hoá → `ai-service...`.

Luồng của ccw: ccw → `20140` (bridge thêm token đã học) → cổng của `handler` → nhận SSE → trả về ccw.

**`bridge.go`** — type `Bridge`:
- `New() *Bridge`
- `Start(frontAddr, backAddr string) error`: lắng nghe hai `http.Server`. `frontAddr` mặc định `127.0.0.1:20139`, `backAddr` mặc định `127.0.0.1:20140`.
- `Addr() string`: trả `frontAddr` đã bind, dùng cho log.
- `Close() error`: đóng cả hai listener.
- Mặt trước dùng `ServeMux` với handler mặc định `forwardToHandler`: đọc `r.Header.Get("Authorization")`, lưu vào `b.token` dưới `sync.RWMutex` nếu khác rỗng, rồi `httputil.ReverseProxy` sang `http://127.0.0.1:<handlerPort>` giữ nguyên mọi header.
- Mặt sau đăng ký `POST /v1/messages` và `GET /health`.

**`POST /v1/messages`** (mặt sau): đọc `model` từ body JSON, dựng path `v1/projects/p/locations/l/publishers/anthropic/models/<model>:streamRawPredict` — khớp đúng cách `claude` dựng, đọc từ mã handler: ``let A = `http://127.0.0.1:${d.port}/v1/projects/p/locations/l/publishers/anthropic/models/${o}:streamRawPredict` ``. Gửi kèm `Authorization` đã học và `content-type: application/json`, rồi copy nguyên vẹn status + header + body (kể cả SSE chunked) về client. Không parse, không dịch — ccw đảm nhiệm.

Body rỗng, `model` thiếu, hoặc `json.Unmarshal` lỗi → `400` với `{"error":"bad request body"}` và **không** gọi handler.

**`GET /health`**: `{"ok":true,"handlerPort":<int>}`; `handlerPort` là `0` nếu chưa đọc được file cổng.

**`handlerPort`** — handler ghi cổng của nó ra file. Vá thêm một dòng vào `handler` ngay sau `let c = a.address().port;`:
```js
try { require("fs").writeFileSync("/tmp/dia-handler.port", String(c)); } catch {}
```
`require("fs")` đã có sẵn trong bundle (`grep -a -c 'require("fs")'` trả `1`). Bridge đọc file này mỗi lần cần `handlerPort` (không cache), vì handler sinh cổng mới cho mỗi lượt chat. Đường dẫn đọc từ `os.Getenv("DIA_HANDLER_PORT_FILE")`, mặc định `/tmp/dia-handler.port` — biến môi trường này chỉ để test, không phải cấu hình người dùng.

**Khi `b.token` rỗng** (chưa có lượt chat nào đi qua mặt trước): `POST /v1/messages` trả `503` kèm `{"error":"no dia token yet: open Dia and send one chat message"}` và không gọi handler. Không có vòng poll nền.

**`extract.go`** — chuyển model trong path về id sạch: `strings.Cut(s, "/publishers/anthropic/models/")` rồi `strings.CutSuffix(..., ":streamRawPredict")`. Không có sẵn tương đương trong repo (`grep -rn streamRawPredict be/` trả rỗng).

### Bước 3 — Test bridge bằng `httptest`, không cần Dia chạy

`be/internal/diabridge/bridge_test.go`: handler giả là `httptest.NewServer`, trả SSE `event: content_block_delta` khi path chứa `publishers/anthropic/models/`, trả `401` nếu `Authorization` lệch với giá trị cố định `"Bearer sk-test"`. Bridge trong test khởi động với `frontAddr` và `backAddr` cùng `127.0.0.1:0`, đọc cổng thật qua `Addr()` và `BackAddr()`. Ba test:

- `TestBridgeLearnsTokenFromClaudeThenServesCcw`: `POST` vào mặt trước kèm `Authorization: Bearer sk-test` → handler giả nhận đúng; sau đó `POST` vào mặt sau **không** kèm `Authorization` → handler giả vẫn thấy `Bearer sk-test` và trả `200` với SSE. Đây là hành vi trung tâm của cả kế hoạch.
- `TestBridgeRejectsBadBody`: `POST` vào mặt sau với body không phải JSON → `400`, handler giả không được gọi.
- `TestBridgeWithoutTokenIsUnavailable`: bridge mới, chưa học token → `503` với đúng message.

`t.Setenv("DIA_HANDLER_PORT_FILE", filepath.Join(t.TempDir(), "port"))` và ghi cổng của handler giả vào đó, để test không đụng file thật của Dia.

Không sao chép `DiaSniffer` (`be/internal/httpapi/dia_sniff.go`, xoá ở bước 6) — nó là HTTP proxy một đầu, không học token qua mặt trước.
### Bước 4 — Sửa registry và `shapeFor` trỏ vào bridge

`be/internal/provider/registry.go` — entry `dia` (dòng 270-287), sửa 5 trường:

- `BaseURL`: `"https://aiproxy.diabrowser.engineering/api"` → `"http://127.0.0.1:20140"`
- `Models`: `[]string{"claude-opus-4-8", "claude-opus-4-6[1m]"}` → `[]string{"claude-opus-4-8", "claude-opus-4-6"}`
- `Setup`: `"oauth"` → `"none"`, kèm `DefaultSecret: "bridge"` — không có API key để dán, và `SeedKeylessConnections` (`be/internal/store/connection.go:64`) tự tạo connection. Đây là lý do xoá các nhánh `dia` trong `oauth.go` ở bước 6.
- `AuthHeader`: `"Authorization"` → `""` — bridge tự thêm token, không cần ccw gửi credential nào
- Thêm `Clean: true` — không để header của caller (`X-Api-Key`, `Cookie`) lọt vào tiến trình Dia

Giữ nguyên `API: "anthropic"`, `Watch: true`, `NoModelList: true`, `Identity`, `Defaults` — chúng đã đúng.

`be/internal/httpapi/proxy.go:381-384` — `shapeFor`, đổi giá trị trả về cho provider `dia`:

```go
if p.ID == "dia" {
    return translate.Anthropic, "v1/messages"
}
```

Đây là path của chính bridge (bước 2), không phải path handler nhận — bridge tự dựng lại path `streamRawPredict` khi gọi xuống. `buildUpstreamCall` (`be/internal/httpapi/v1.go:814`) chuyển `c.path = wantPath` rồi `newOutbound` ghép `base + "/" + path`.

### Bước 5 — Khôi phục `parseV1Model` về hành vi cắt hậu tố gốc

`be/internal/httpapi/v1.go:164-173` hiện có nhánh bảo vệ đặc biệt cho `claude-opus-4-6[1m]`. Không còn model nào mang hậu tố đó, nên xoá nhánh và trả về dạng gốc:

```go
if base, found := strings.CutSuffix(model, "[1m]"); found && base != "" {
    if renamed, ok := setModel(body, base); ok {
        body, model = renamed, base
    }
}
```

Cập nhật test đi theo:
- Xoá `TestDiaPreservesOneMillionModelSuffix` trong `be/internal/httpapi/dia_test.go`
- Trong `TestDiaShapeForRoutesToDiaClaudeStream`, `TestDiaModelCatalog`, đổi `claude-opus-4-6[1m]` → `claude-opus-4-6`
- `TestDiaProviderRegistered` tại `be/internal/provider/registry_test.go:90`: `wantModels` → `{"claude-opus-4-8", "claude-opus-4-6"}`
- `TestDiaMessagesRequestRelayedToEndpoint`, `TestDiaChatCompletionsTranslated`: đổi `gotPath` mong đợi từ `/dia-claude-stream` sang `/v1/messages`
- `TestDiaFailoverAcrossAccounts`: giữ nguyên, chỉ đổi tên model

`TestTheOneMillionMarkIsNotPartOfTheModelName` (`be/internal/httpapi/server_test.go:136`) giữ nguyên — sau bước 5 nó lại đúng nghĩa.
### Bước 6 — Thay sniffer bằng bridge: khởi động cùng ccw

Xoá cơ chế HTTP cũ và đổi cờ khởi động:

- `be/internal/httpapi/dia_sniff.go` — xoá toàn bộ `DiaSniffer`, `captureToken`, `diaDeviceStart`, `diaDevicePoll`, `diaLoginFinish`
- `be/internal/httpapi/dia_sniff_test.go` — xoá toàn bộ
- `be/internal/httpapi/oauth.go` — xoá 3 nhánh: `prov == "dia"` trong `loginStart` (342-345), `r.PathValue("provider") == "dia"` trong `loginFinish` (398-401), `devicePoll` (777-780). Bắt buộc vì bước 4 đổi `Setup` sang `"none"` — nếu giữ, `loginStart` sẽ trả `404 this provider has no sign-in` khi ai đó bấm nhầm.
- `be/cmd/ccw/main.go` — xoá flags `-sniff-dia`, `-sniff-dia-addr` và khối `if *sniffDia {...}` (60-67); thêm hai flag mới:
  ```go
  diaFront := flag.String("dia-bridge-front", "127.0.0.1:20139", "Dia bridge front address that Dia's claude calls (empty to disable)")
  diaBack := flag.String("dia-bridge-back", "127.0.0.1:20140", "Dia bridge back address that ccw calls")
  ```
  và khối tương ứng đặt sau `setupAuth`, trước `srv := newHTTPServer(...)`:
  ```go
  if *diaFront != "" {
      b := diabridge.New()
      if err := b.Start(*diaFront, *diaBack); err != nil {
          log.Fatalf("start dia bridge: %v", err)
      }
      defer b.Close()
      slog.Info("dia.bridge.listen", "front", b.Addr(), "back", b.BackAddr())
  }
  ```
  Import `"github.com/naniiluja/ccw/internal/diabridge"`. Bridge chạy trong tiến trình ccw nên không cần daemon riêng. Hai cờ tách bạch để `-dia-bridge-front=""` tắt bridge mà vẫn giữ được giá trị mặc định của `-dia-bridge-back` cho `BaseURL` trong registry.
- `be/internal/store/connection.go` — xoá `SetSecret`. Xác nhận trước khi xoá: `grep -rn "SetSecret" be/ --include='*.go' | grep -v _test` phải trả rỗng sau khi xoá `dia_sniff.go`
- Xoá `TestSetSecretUpdatesCredential` trong `be/internal/store/connection_test.go`

Dọn dữ liệu cũ trên máy đang chạy:
```bash
sqlite3 /Users/naniiluja/Developer/ccw-run/ccw.db \
  "DELETE FROM connections WHERE provider='dia';
   DELETE FROM settings WHERE key='dia_auth_token';"
```

Gỡ cờ khỏi `~/Library/LaunchAgents/dev.ccw.gateway.plist` (bỏ `<string>-sniff-dia</string>`), thay bằng:
```xml
<string>-dia-bridge-front</string>
<string>127.0.0.1:20139</string>
<string>-dia-bridge-back</string>
<string>127.0.0.1:20140</string>
```

### Bước 7 — Vá `handler` và ký lại Dia

Backup trước, rồi vá:
```bash
cp -R /Applications/Dia.app /Applications/Dia.app.backup-$(date +%Y%m%d-%H%M%S)
bash scripts/dia-patch.sh
```

`scripts/dia-patch.sh` (ngoài module Go, không bị `go vet`/`gocyclo` quét) làm hai việc, idempotent:
1. `sed` thay ``ANTHROPIC_VERTEX_BASE_URL = `http://127.0.0.1:${p.port}` `` → ``ANTHROPIC_VERTEX_BASE_URL = `http://127.0.0.1:20139` `` trong `dist/handler`, và chèn `writeFileSync("/tmp/dia-handler.port", String(c))` sau `let c = a.address().port;`. Bỏ `sed -i` nếu mẫu đã hết (đã vá rồi) để chạy lại không làm hỏng.
2. `codesign --force --deep --sign - /Applications/Dia.app`

Hệ quả phải biết: Sparkle từ chối cập nhật vì chữ ký ad-hoc; mỗi lần Dia tự cập nhật phải chạy lại `scripts/dia-patch.sh`.

Sau khi vá, khởi động lại ccw qua launchd. Bridge đã bật sẵn nhờ giá trị mặc định của `-dia-bridge-front`.

### Bước 8 — Đăng ký provider qua cơ chế sẵn có

Không viết code mới. Với `Setup: "none"` + `DefaultSecret: "bridge"`, `SeedKeylessConnections` (`be/internal/store/connection.go:64`) tạo connection ở lần khởi động đầu tiên sau khi bước 6 dọn DB. Failover, rotation, drift, quota dùng nguyên đường ống có sẵn.
## Critical files & anchors

- `be/internal/diabridge/bridge.go` — file mới; proxy hai đầu, nơi duy nhất đọc token từ `claude`
- `be/internal/provider/registry.go:270-287` — entry `dia`, sửa `BaseURL`/`Models`/`Setup`/`AuthHeader`/`Clean`
- `be/internal/httpapi/proxy.go:381-384` — `shapeFor`, path phải là `/v1/messages` của bridge chứ không phải path handler nhận
- `/Applications/Dia.app/Contents/Resources/agent-server-resources/dist/handler` — hai chỗ vá: dòng gán `ANTHROPIC_VERTEX_BASE_URL` và dòng `let c = a.address().port;`. File ngoài repo; vá qua `scripts/dia-patch.sh` để chạy lại được sau mỗi lần Dia update.

## Verification

**1. Bridge đúng logic, không cần Dia chạy (bước 3).**
```bash
cd be && go test ./internal/diabridge/ -v
# Kỳ vọng: 3 test PASS, gồm TestBridgeForwardsClaudeRequestAndKeepsToken
```

**2. `handler` đã vá, Dia vẫn chat bình thường (bước 1 + 7).**
```bash
grep -a -oE 'ANTHROPIC_VERTEX_BASE_URL = .http://127.0.0.1:\\\$\{[a-z]\.port\}' \
  /Applications/Dia.app/Contents/Resources/agent-server-resources/dist/handler | wc -l
# Kỳ vọng: 0
grep -ac 'ANTHROPIC_VERTEX_BASE_URL' \
  /Applications/Dia.app/Contents/Resources/agent-server-resources/dist/handler
# Kỳ vọng: 6 (không mất chỗ nào)
codesign -dv /Applications/Dia.app 2>&1 | grep -i signature
# Kỳ vọng: không còn "Developer ID Application: The Browser Company"
```
Mở app Dia, gửi một câu chat, phải nhận câu trả lời bình thường. Sau đó:
```bash
lsof -nP -iTCP -sTCP:LISTEN | grep 20139   # bridge của ccw
cat /tmp/dia-handler.port                 # cổng thật của handler
curl -s http://127.0.0.1:20140/health
# Kỳ vọng: {"ok":true,"handlerPort":<số khớp file ở trên>}
```

**3. Bridge đã học token (bước 2).**
```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:20140/v1/messages \
# Kỳ vọng: 200
curl -sN -X POST http://127.0.0.1:20140/v1/messages \
# Kỳ vọng: SSE có event: content_block_delta chứa text "OK"
```

**4. ccw định tuyến end-to-end — kỳ vọng chính của toàn bộ kế hoạch (bước 4 + 8).**
```bash
cd be && go test ./internal/httpapi -run TestDia -v   # tất cả PASS
bash scripts/ui-build.sh && go build -o /Users/naniiluja/Developer/ccw-run/ccw ./cmd/ccw
launchctl unload ~/Library/LaunchAgents/dev.ccw.gateway.plist
launchctl load ~/Library/LaunchAgents/dev.ccw.gateway.plist
curl -s -c /tmp/c -X POST http://127.0.0.1:20130/login -d "password=$CCW_PASSWORD"
curl -s -b /tmp/c -X POST http://127.0.0.1:20130/api/providers/dia/models/test \
  -d '{"model":"claude-opus-4-8"}'
# Kỳ vọng: {"ok":true,...} — thay cho {"ok":false,"status":401,"message":"Unauthorized"}
```

**5. Model sai đã gỡ (bước 4).**
```bash
curl -s -b /tmp/c http://127.0.0.1:20130/api/providers/dia/models
# Kỳ vọng: chỉ claude-opus-4-8 và claude-opus-4-6, không có "claude-opus-4-6[1m]"
```

**6. Cổng toàn bộ.**
```bash
bash scripts/check.sh
```
Phải xanh hết, gồm `go test -race ./...` với test kiến trúc trong `be/internal/httpapi/arch_test.go`.

**7. Qua đường Tailscale.**
```bash
curl -s -c /tmp/ts -X POST https://naniiluja.tail4bb1f7.ts.net/login -d "password=$CCW_PASSWORD"
curl -s -b /tmp/ts https://naniiluja.tail4bb1f7.ts.net/api/accounts | grep -o '"dia"'
# Kỳ vọng: xuất hiện provider "dia" do SeedKeylessConnections tạo
```
## Assumptions & contingencies

- **`ANTHROPIC_VERTEX_BASE_URL` chỉ có một chỗ gán** — grep xác minh 6 chỗ xuất hiện, chỉ một có dạng `` = `http://127.0.0.1:${p.port}` ``. Nếu bản Dia cập nhật thêm chỗ gán, Verification 2 phát hiện ngay (số dòng khớp mẫu > 0); khi đó vá thêm chỗ thứ hai, cùng cổng 20139.
- **Biến `c` trong `let c = a.address().port;` giữ nguyên tên sau khi vá** — xác minh mã tại offset 66779000. Nếu Dia đổi tên biến, `sed` trong `scripts/dia-patch.sh` không khớp và script phải báo lỗi thay vì vá hụt. Script kiểm tra cả hai mẫu, fail nếu thiếu bất kỳ mẫu nào.
- **Cổng 20139/20140 chưa bị chiếm** — kiểm tra `lsof -iTCP:20139 -sTCP:LISTEN` và `20140` trước khi vá. Nếu đã có, đổi cả ba chỗ cho khớp: giá trị vá trong `handler`, hai cờ `-dia-bridge-front`/`-dia-bridge-back` trong `~/Library/LaunchAgents/dev.ccw.gateway.plist`, và `BaseURL` trong registry.
- **`claude` gửi `Authorization` ở mọi request** — suy ra từ mã handler (`authorization: \`Bearer ${process.env.ANTHROPIC_AUTH_TOKEN}\``) và từ `claude` phải xác thực với handler. Nếu `claude` chỉ gửi token ở request đầu và bỏ ở request sau, bridge học token ở request đầu rồi tự thêm cho các request sau — vẫn đúng. Nếu ngược lại (chỉ gửi khi cần), bridge học ở lần đầu có token và Verification 3 sẽ cho `503` nếu chưa có; xử lý bằng cách gửi một chat trong Dia trước khi test.
- **Token đổi mỗi lượt chat** — handler sinh `sk-${random}` cho mỗi tiến trình con. Bridge học lại mỗi request nên không cần làm mới; nếu handler sinh token chỉ một lần cho cả phiên, bridge vẫn dùng token đã học cho tới khi `claude` gửi request mới.
- **Vá `handler` làm hỏng Sparkle updater** — chữ ký ad-hoc khiến updater từ chối bản cập nhật. Hệ quả đã chấp nhận; `scripts/dia-patch.sh` chạy lại sau mỗi lần Dia tự cập nhật.
- **Nếu `codesign` làm Dia không khởi động** — khôi phục: `rm -rf /Applications/Dia.app && cp -R /Applications/Dia.app.backup-<timestamp> /Applications/Dia.app`.
- **`require("fs")` có sẵn trong `handler`** — đã xác minh: `grep -a -c 'require("fs")'` trả `1`, nên CommonJS và cách vá dùng `require("fs")` là đúng. Nếu bản cập nhật đổi sang ESM, script báo lỗi thay vì vá hụt.
