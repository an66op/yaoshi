# 多实例运行要求

正式环境必须配置 Redis。后端启动时会先执行连通性检查，Redis 不可用时拒绝启动；`debug`/`test` 环境可明确回退为单实例内存实现。

Redis 只保存短期协调数据，业务账务仍以 PostgreSQL 为准：

- 登录、WebSocket 票据和下注接口采用共享固定窗口限流。
- WebSocket 一次性票据使用带 TTL 的 Redis 键并通过 `GETDEL` 原子消费。
- 聊天、开奖和余额通知继续通过 Pub/Sub 跨实例转发；事件携带实例来源并忽略自身回环。
- 封号、改密、退出、换房和角色/层级变化由 PostgreSQL 触发器在同一事务中推进 `auth_version` 并写入 `ws_session_revocation_outbox`。持有 Redis 租约的 worker 将待办写入专用 Redis Stream，确认 `XADD` 后才标记 outbox 已投递；每个后端实例以独立游标读取，不与普通消息共用消费组。
- 开奖同步、平台自开彩、结算恢复、幂等请求恢复、机器人、红包过期、数据生命周期和审计补写均使用带随机持有令牌的 Redis 租约，同一任务同一时刻仅一个实例执行。
- 撤权事件携带不可变 `event_id` 和被撤销的 `revoked_auth_version`，重复/延迟回放只关闭旧代连接，不会踢掉撤权后重新登录的新连接。即使 Redis Stream 暂时不可读，每条连接仍每 30 秒向 PostgreSQL 校验身份版本、账号状态、工作区与上级房间状态。

生产环境需设置 `BACKEND_REDIS_ADDR`、`BACKEND_REDIS_PASSWORD`、`BACKEND_REDIS_DB`、`BACKEND_REDIS_TLS` 和独立的 `BACKEND_REDIS_PREFIX`。同一套系统的所有实例必须使用相同前缀；测试、预发布和生产必须使用不同前缀。

撤权流要求 Redis 6.2 或更高版本，并应启用 AOF（建议 `appendfsync everysec`）。Stream 保留 24 小时，远大于 30 秒数据库复核周期；确认投递的 PostgreSQL 回执保留 7 天，未投递回执永不由清理任务删除。日常巡检至少检查：

```sql
SELECT COUNT(*) AS pending,
       MIN(created_at) AS oldest_pending,
       MAX(attempt_count) AS max_attempts
FROM ws_session_revocation_outbox
WHERE delivered_at IS NULL;
```

`debug`/`test` 未配置 Redis 时，单实例仍会立即断开本机 socket，事务 outbox 会保留待办而不是静默丢弃。以后给同一数据库配置 Redis 时，这些旧事件会安全回放；由于消息精确绑定旧 `auth_version`，不会影响当时的新代连接。
