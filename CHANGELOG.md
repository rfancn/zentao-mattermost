# Changelog

## v0.3.0

- 从 Traefik WASM 插件改造为独立 Go 服务。
- 使用 cobra 提供 `run` 和 `version` 子命令。
- 使用 hd SDK 加载 TOML 配置与 zerolog 日志。
- 支持 `HD_` 环境变量覆盖敏感配置。
- 新增强制 webhook 共享密钥鉴权。
- 移除 `passThrough` 与 WASM 专用的 DNS 配置。
- 保留 token 缓存、401 刷新、去重、频道与成员并发容错。
- 新增单元测试与 httptest 集成测试。

## v0.2.0

- Cache ZenTao REST token.
- Refresh the dynamically acquired ZenTao token once after HTTP 401.
- Use `assignedTo.account` as the Mattermost username.
- Resolve the Mattermost bot with `botUser` or `/api/v4/users/me`.
- Ensure both bot and target user are channel members.
- Make channel creation race-tolerant.
- Make member insertion race-tolerant.
- Add bounded in-memory SHA-256 webhook deduplication with TTL.
- Failed notification attempts are removed from the dedupe cache so ZenTao can retry them.