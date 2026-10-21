# ZenTao -> Mattermost

独立运行的 Go 服务：接收禅道 Bug webhook，查询 Bug 当前指派人，在 Mattermost 中与指派人建立私聊并发送通知。

不再依赖 Traefik WASM，直接以 HTTP 服务方式运行。

## 功能

- 接收禅道 Bug webhook（`POST /<route.name>`）
- 强制共享密钥鉴权（`Authorization: Bearer` 或 `X-Webhook-Token`）
- 从 webhook 文本或 `objectID` 提取 Bug ID
- 查询禅道 Bug 详情，使用 `assignedTo.account` 作为 Mattermost 用户名
- 与指派人建立 Mattermost 私聊（Direct Message）
- 按 `route + 用户` 缓存私聊频道
- 进程内 SHA-256 去重（带 TTL 和容量上限）
- 禅道 token 缓存，401 时自动刷新一次
- TOML 配置，支持 `HD_` 环境变量覆盖
- 优雅退出

## 构建

要求 Go 1.24+。

```bash
./build.sh
```

产物为 `./zentao-mattermost`。

也可以直接：

```bash
go build -o zentao-mattermost .
```

## 配置

复制示例配置：

```bash
cp config/app/zentao-mattermost/zentao-mattermost.toml.example \
   config/app/zentao-mattermost/zentao-mattermost.toml
```

然后修改 `app.token` 以及各路由的 `source` / `target` 配置。

启动：

```bash
./zentao-mattermost run
```

也可以显式指定配置文件：

```bash
./zentao-mattermost run --config /etc/zentao-mattermost/zentao-mattermost.toml
```

### 配置查找顺序

使用 hd SDK 的配置加载规则：

1. `--config` 指定的文件（优先）
2. 未指定时从当前目录向上搜索：
   - `./<app>.toml`
   - `./config/app/<app>/<app>.<env>.toml`
   - `./common/config/app/<app>/<app>.<env>.toml`
   - 父目录中的同名路径
3. 环境变量覆盖

`<app>` 为 `zentao-mattermost`，`<env>` 来自 `HD_ENV`；未设置 `HD_ENV` 时使用 `<app>.toml`。

### 环境变量覆盖

hd SDK 会把 `HD_` 前缀的环境变量转换为配置项（下划线转点号）。常用：

| 环境变量 | 配置项 |
|---|---|
| `HD_APP_TOKEN` | `app.token` |
| `HD_ENV` | 选择 `<app>.<env>.toml` 配置文件 |

生产环境建议把密钥放在环境变量或密钥管理系统中，不要提交到仓库。

### 最小配置示例

```toml
[app]
listen = ":8080"
token = "CHANGE_ME"

[app.routes.zentao-mattermost-bug]

[app.routes.zentao-mattermost-bug.source]
type = "zentao"
url = "http://192.168.2.189"
account = "admin"
password = "YOUR_ZENTAO_PASSWORD"

[app.routes.zentao-mattermost-bug.target]
type = "mattermost"
url = "https://im.example.internal"
token = "YOUR_MATTERMOST_BOT_TOKEN"
bot_user = "zentao-bot"

[sdk.log]
level = "info"
filename = "zentao-mattermost.log"
dir = "logs"
[sdk.log.rotate]
max_age = 168
max_backup = 5
max_size = 100
compress = false
```

`app.routes` 下的每一个 key 都会注册为一个 webhook 路径：`POST /<key>`，例如 `[app.routes.zentao-mattermost-bug]` 对应 `POST /zentao-mattermost-bug`。`source.type` 决定消息解析方式，`target.type` 决定通知发送方式。当前支持 `zentao` → `mattermost`。

路由 key 会去除首尾空白，且只允许字母、数字、`-` 和 `_`。

`[sdk.log]` 由 hd SDK 使用。`dir` 必须是运行用户可写的目录；省略时 Linux 默认尝试 `/var/log`。

## 运行参数

```text
zentao-mattermost run [--config <file>] [--app-address <addr>]
zentao-mattermost version
```

- `--config, -c`：配置文件路径
- `--app-address, -d`：监听地址，覆盖 `app.listen`
- `version`：打印版本、commit 和构建时间

## webhook 接口

支持以下任意一种方式携带 token：

```text
POST /zentao-mattermost-bug
Authorization: Bearer <app.token>
Content-Type: application/json
```

```text
POST /zentao-mattermost-bug
X-Webhook-Token: <app.token>
Content-Type: application/json
```

```text
POST /zentao-mattermost-bug?token=<app.token>
Content-Type: application/json
```

```text
POST /zentao-mattermost-bug/<app.token>
Content-Type: application/json
```

请求体示例：

```json
{
  "objectType": "bug",
  "objectID": 3,
  "action": "edited",
  "actor": "admin",
  "date": "2026-10-08 17:43:58",
  "text": "admin编辑了Bug [#3::test_bug3](http://192.168.2.189/index.php?m=bug&f=view&id=3)"
}
```

`objectID` 支持字符串或数字。

### 响应语义

| 情况 | 状态码 |
|---|---|
| 鉴权失败 | 401 |
| 非 POST | 405 |
| Content-Type 非 `application/json` | 415 |
| JSON 解析失败、缺少 Bug ID | 400 |
| 请求体超过 固定 1MB | 413 |
| 非 Bug、动作未订阅、无指派人、重复 webhook | 200 `{"ok":true}`（跳过） |
| 禅道或 Mattermost 调用失败 | 500，禅道会按自身策略重试 |
| 发送成功 | 200 `{"ok":true}` |

## 禅道配置

在禅道的 webhook 设置中：

- URL 指向 `http://<服务地址>:8080/zentao-mattermost-bug`
- 方法为 POST
- Content-Type 为 `application/json`
- 添加请求头 `Authorization: Bearer <app.token>` 或 `X-Webhook-Token: <app.token>`
- 若禅道无法自定义请求头，可将 token 放在 URL 中：
  - `http://<服务地址>:8080/zentao-mattermost-bug?token=<app.token>`
  - `http://<服务地址>:8080/zentao-mattermost-bug/<app.token>`

解析出的消息会直接通知到对应 target，不再按动作过滤。

## Mattermost 权限

Mattermost token 对应的机器人账号需要：

1. 按用户名查询用户
2. 创建/获取与指定用户的私聊频道
3. 在私聊频道中发帖

不需要 Team 信息，也不再创建或管理普通频道。

## 去重

去重始终启用，无需配置。去重键是完整 webhook 请求体的 SHA-256，保存在进程内，默认 TTL 10 分钟、最多 2048 条。

- 处理成功：记录保留到 TTL 过期
- 处理失败：立即移除记录，允许禅道重试
- 进程重启后记录丢失；多实例之间不共享

## 禅道 token

服务使用 `account` + `password` 调用 `/api.php/v1/tokens` 获取 token，并在进程内缓存。token 失效（接口返回 401）时会自动清除缓存并重新获取一次。

```toml
[app.routes.zentao-mattermost-bug.source]
type = "zentao"
url = "http://192.168.2.189"
account = "admin"
password = "YOUR_PASSWORD"
```

## Docker

```bash
docker build -t zentao-mattermost .
docker run --rm -p 8080:8080 \
  -v "$PWD/config:/app/config" \
  -e HD_APP_TOKEN=CHANGE_ME \
  zentao-mattermost
```

## 测试

```bash
go test -race ./...
go test -cover ./...
golangci-lint run
gosec ./...
```