# 王者生产运维手册

## 上线原则

- 用户端、管理端只通过 Nginx HTTPS 访问；Go 服务仅监听内网或本机 `8080`。
- 正式环境不得使用开发快捷账号、默认管理员密码或示例密钥。
- 数据库变更由 `backend/migrations` 的版本化迁移执行；上线前必须先备份。
- 机器人默认关闭。只有开奖、结算、余额对账和 WebSocket 健康检查全部通过后，才按房间逐个开启。
- 外部开奖源异常时停盘，不能用随机结果替代。

## 首次部署

1. 创建独立的 `wangzhe` 系统用户、PostgreSQL 用户和数据库。
2. 将代码放到 `/opt/wangzhe`，把用户端构建产物复制到 `/var/www/wangzhe/member`，管理端复制到 `/var/www/wangzhe/admin`。
3. 将 `deploy/env/backend.env.example` 复制到 `/etc/wangzhe/backend.env`，替换全部密钥并设置权限 `600`。
4. 安装 `deploy/systemd/wangzhe-backend.service`，创建 `/var/lib/wangzhe/uploads` 并交给 `wangzhe` 用户；同时预先创建仅 root 可访问的 `/var/backups/wangzhe`（建议权限 `700`）。
   同时安装 `wangzhe-backup.service` 与 `wangzhe-backup.timer`，执行
   `systemctl enable --now wangzhe-backup.timer`，并先手工运行一次备份服务确认成功。
5. 安装 Nginx 示例配置，先执行 `nginx -t`，再申请免费证书：`certbot --nginx -d wz6688.app -d admin.wz6688.app`。
6. 首次启动主服务前，使用下述 `wangzhe-bootstrap-admin` 命令创建唯一的初始平台管理员。
7. 启动后端并检查日志。首次迁移失败时不要跳过迁移或手改校验和，应先恢复备份并修正迁移。

正式库必须预先创建一个非默认密码的平台管理员。后端在 release 模式不会自动生成 `admin / 123456`，也不会把本地验收会员、租户、代理、模拟历史开奖或默认计划写入正式库。初始密码只能通过 owner-only 文件传入，不能放在命令行或环境变量：

```bash
sudo install -o wangzhe -g wangzhe -m 0600 /dev/null /run/wangzhe-admin-password
sudo -u wangzhe sh -c 'umask 077; openssl rand -base64 32 > /run/wangzhe-admin-password'
sudo systemd-run --wait --pipe --collect --uid=wangzhe \
  -p EnvironmentFile=/etc/wangzhe/backend.env \
  /opt/wangzhe/bin/wangzhe-bootstrap-admin \
  --username platform-owner --password-file /run/wangzhe-admin-password
sudo rm -f /run/wangzhe-admin-password
```

该命令只能在库内尚无管理员时成功，密码至少 14 位且必须包含大小写字母、数字和符号。创建后按团队密钥管理规范保存凭据并定期轮换。

`BACKEND_SERVER_BIND` 保持为 `127.0.0.1`，公网只开放 Nginx 的 80/443。JWT 密钥与数据加密密钥必须分别随机生成；远程 PostgreSQL 必须使用 `verify-ca` 或 `verify-full`，仅本机数据库可关闭 TLS。

生产构建：

```bash
make release
```

该命令会先清空旧的 `release/`，执行后端测试和两个前端的测试、lint、构建，再生成后端二进制、两套静态页面、部署配置、检查/备份脚本和本手册，避免上一个版本的静态文件混入新包。部署后一次启动：

```bash
sudo systemctl enable --now wangzhe-backend.service wangzhe-backup.timer
```

上线前检查：

```bash
sudo BACKEND_URL=http://127.0.0.1:8080 scripts/production-readiness.sh /etc/wangzhe/backend.env
```

就绪检查会核验动态迁移清单及校验和、22 个开放彩种、开奖源、超时/异常/无归属注单、无归属账号、机器人、清理任务和最近 26 小时内备份。任何已知历史异常默认阻止上线；只能在完成对账后重新检查，不能为了通过检查擅自提高阈值。发布配置自身可先离线检查：

```bash
make readiness-test
```

## 备份与恢复演练

每天至少执行一次：

```bash
sudo BACKUP_RETENTION_DAYS=14 scripts/postgres-backup.sh /etc/wangzhe/backend.env
```

备份脚本先写入临时文件，使用 `pg_restore --list` 验证后再原子改名，并保存 SHA-256。至少保留一份异机备份。每月在独立测试数据库执行恢复演练；禁止直接覆盖生产库：

```bash
createdb wangzhe_restore_test
pg_restore --clean --if-exists --no-owner --dbname wangzhe_restore_test /var/backups/wangzhe/FILE.dump
```

## 数据删除与归档

- 所有清理必须先预览，再使用同一个 `request_id` 执行；重复请求不会重复处理。
- 会员真实注单、余额流水、异常待处理数据不能直接删除。
- 聊天和通知先软删除；审计日志先归档并校验数量，再从热表移除，归档仍可在数据维护中检索。
- 机器人已结算测试数据可以进入冷归档；未结、异常、无工作区数据只能进入对账队列。
- 数据维护任务及其操作者、范围、条件、数量和失败原因永久留痕。
- 启用的保留策略每天北京时间 03:30 自动运行；策略默认关闭，首次启用前必须在后台预览候选量。
- `scripts/dev-reset-business-data.sh` 只允许本机非 release 数据库；生产环境不得配置开发重置授权变量，也不得复制该脚本作为清库手段。

## 日常巡检

- `/health`、用户端、管理端和 WebSocket 均可用。
- 开奖源最后成功时间、当前期状态、待结算及异常对账数量正常。
- 余额流水末值等于会员余额，无负余额，无跨工作区数据。
- 申请、红包、回水、返佣、机器人和通知的任务无重复执行。
- 下注幂等请求不会长期停留在 `processing`；自动巡检只凭不可变扣分流水恢复成功回执，无扣分证据的请求转为明确失败，账务证据异常时必须人工对账。
- `ws_session_revocation_outbox` 没有长期未投递记录；若 `pending` 或 `attempt_count` 持续增长，先恢复 Redis 6.2+ 与 AOF 持久化，禁止手工删除待办。撤权 Redis Stream 保留 24 小时、已投递数据库回执保留 7 天，连接本身每 30 秒再向 PostgreSQL 复核授权状态。
- 备份文件更新时间不超过 24 小时，磁盘和 PostgreSQL 连接数有余量。

发生异常时先停对应彩种和机器人，保留现场与日志；不要伪造开奖结果，也不要直接修改余额。通过后台对账/恢复操作处理，并保留审计记录。
