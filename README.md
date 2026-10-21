# ZenTao -> Mattermost

独立运行的 Go 服务：接收禅道 Bug webhook，查询 Bug 当前指派人，在 Mattermost 中获取或创建对应的私有频道，并发送通知。

不再依赖 Traefik WASM，直接以 HTTP 服务方式运行。

## 功能

- 接收禅道 Bug webhook（`POST /webhook`）
- 强制共享密钥鉴权（`Authorization: Bearer` 或 `X-Webhook-Token`）
- 从 webhook 文本或 `objectID` 提取 Bug ID
- 查询禅道 Bug 详情，使用 `assignedTo.account` 作为 Mattermost 用户名
- 按 `zentao-{username}` 获取或创建频道
- 确保机器人和被指派人都在频道中
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

然后修改其中的禅道、Mattermost 和 `server.token`。

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
| `HD_SERVER_TOKEN` | `server.token` |
| `HD_ZENTAO_TOKEN` | `zentao.token` |
| `HD_ZENTAO_PASSWORD` | `zentao.password` |
| `HD_MATTERMOST_TOKEN` | `mattermost.token` |
| `HD_ENV` | 选择 `<app>.<env>.toml` 配置文件 |

生产环境建议把密钥放在环境变量或密钥管理系统中，不要提交到仓库。

### 最小配置示例

```toml
[server]
listen = ":8080"
path = "/webhook"
token = "CHANGE_ME"

[zentao]
url = "http://192.168.2.189"
account = "admin"
password = "YOUR_ZENTAO_PASSWORD"

[mattermost]
url = "https://im.example.internal"
token = "YOUR_MATTERMOST_BOT_TOKEN"
team_id = "YOUR_TEAM_ID"
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

`[sdk.log]` 由 hd SDK 使用。`dir` 必须是运行用户可写的目录；省略时 Linux 默认尝试 `/var/log`。

## 运行参数

```text
zentao-mattermost run [--config <file>] [--app-address <addr>]
zentao-mattermost version
```

- `--config, -c`：配置文件路径
- `--app-address, -d`：监听地址，覆盖 `server.listen`
- `version`：打印版本、commit 和构建时间

## webhook 接口

```text
POST /webhook
Authorization: Bearer <server.token>
Content-Type: application/json
```

或：

```text
POST /webhook
X-Webhook-Token: <server.token>
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
| 请求体超过 `http.max_body_bytes` | 413 |
| 非 Bug、动作未订阅、无指派人、重复 webhook | 200 `{"ok":true}`（跳过） |
| 禅道或 Mattermost 调用失败 | 500，禅道会按自身策略重试 |
| 发送成功 | 200 `{"ok":true}` |

## 禅道配置

在禅道的 webhook 设置中：

- URL 指向 `http://<服务地址>:8080/webhook`
- 方法为 POST
- Content-Type 为 `application/json`
- 添加请求头 `Authorization: Bearer <server.token>` 或 `X-Webhook-Token: <server.token>`

服务会按 `notify.actions` 过滤动作，默认：

```toml
[notify]
actions = ["created", "edited", "activated", "resolved", "closed"]
```

## Mattermost 权限

Mattermost token 对应的机器人账号需要：

1. 访问配置的 team
2. 在频道不存在时创建频道
3. 把机器人和被指派人加入频道
4. 在频道中发帖

如果管理员预先创建好频道和成员，可以去掉创建频道的要求，但仍需要读取频道和发帖权限。

## 去重

去重键是完整 webhook 请求体的 SHA-256，保存在进程内：

```toml
[dedupe]
enabled = true
ttl_seconds = 600
max_entries = 2048
```

- 处理成功：记录保留到 TTL 过期
- 处理失败：立即移除记录，允许禅道重试
- 进程重启后记录丢失；多实例之间不共享

## 禅道 token

两种方式二选一，`token` 优先：

```toml
[zentao]
url = "http://192.168.2.189"
token = "YOUR_ZENTAO_TOKEN"
```

```toml
[zentao]
url = "http://192.168.2.189"
account = "admin"
password = "YOUR_PASSWORD"
```

使用账号密码时，服务会缓存 token；Bug 接口返回 401 时丢弃缓存并重新获取一次。

## Docker

```bash
docker build -t zentao-mattermost .
docker run --rm -p 8080:8080 \
  -v "$PWD/config:/app/config" \
  -e HD_SERVER_TOKEN=CHANGE_ME \
  zentao-mattermost
```

## 测试

```bash
go test -race ./...
go test -cover ./...
golangci-lint run
gosec ./...
```