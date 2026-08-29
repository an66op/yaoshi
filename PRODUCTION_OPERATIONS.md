# 王者生产运维手册

## 上线原则

- 用户端、管理端只通过 Nginx HTTPS 访问；Go 服务仅监听内网或本机 `8080`。
- 正式环境不得使用开发快捷账号、默认管理员密码或示例密钥。
- 数据库变更由 `backend/migrations` 的版本化迁移执行；上线前必须先备份。
- 机器人默认关闭。只有开奖、结算、余额对账和 WebSocket 健康检查全部通过后，才按房间逐个开启。
- 外部开奖源异常时停盘，不能用随机结果替代。

## 构建与发布包

Go 模块最低版本是 1.25，构建默认使用 `backend/go.mod` 已钉住的 Go 1.26.7 安全工具链；构建机还需要项目锁定依赖可用的 Node.js/npm。生产服务器运行预编译二进制，不需要安装 Go、Node.js、npm 或源码依赖。构建机执行：

```bash
make release
# ARM64 Linux 服务器改用：make release RELEASE_GOARCH=arm64
```

默认产物是 `linux/amd64`；仅支持显式改为 `linux/arm64`。`release/TARGET` 会记录目标平台，服务器架构不匹配时发布脚本会拒绝启动。该命令先执行后端和两个前端的测试、lint、构建及部署静态检查，再生成后端二进制、两套静态页面、部署配置、运维脚本和 `release/SHA256SUMS`，最后在终端打印清单文件本身的 SHA-256。把这 64 位摘要保存到密码管理器或受控发布记录，并通过不同于发布包的可信通道传给服务器操作者。

传输到服务器后，先把整个 `release/` 目录放在临时位置，再执行 `sudo chown -R root:root /tmp/wangzhe-release && sudo chmod -R a+rX,go-w /tmp/wangzhe-release`。服务器预装的可信发布器会先把 `SHA256SUMS` 与外部摘要比对，再检查目录内每一个条目的 owner 和写权限，拒绝 symlink/特殊文件/换行文件名，并在复制前后各验一次清单，避免“校验后、复制前”被替换。禁止直接以 root 执行 `/tmp/wangzhe-release/scripts/production-deploy.sh`：只验证包内清单无法防止攻击者同时替换载荷、清单和验证器。当前流程以服务器预装工具和外部摘要作为信任锚；正式接入 CI 后还应增加制品签名/attestation。

## 首次服务器准备

以下流程假设一台专用 Ubuntu/Debian 主机，公网只开放 80/443，PostgreSQL 与 Redis 只监听回环或受控私网。不要把真实环境文件复制进代码目录或发布包。

主机本身先完成基础加固：SSH 仅允许密钥认证、禁止 root/密码远程登录；22 端口只对运维来源开放；启用自动安全更新和时间同步；PostgreSQL/Redis 不绑定公网地址；Redis 6.2+ 开启 AOF，远程连接使用 TLS；系统日志设置保留期并限制读取权限。云安全组与主机防火墙应只放行 80/443 和受限的运维 SSH，不能把 5432、6379 或 8080 暴露到公网。

1. 创建两个无登录系统用户，并准备最小权限目录：

   ```bash
   sudo useradd --system --home /var/lib/wangzhe --shell /usr/sbin/nologin wangzhe
   sudo useradd --system --home /var/backups/wangzhe --shell /usr/sbin/nologin wangzhe-backup
   sudo install -d -o root -g root -m 0755 /opt/wangzhe/releases
   sudo install -d -o wangzhe -g wangzhe -m 0750 /var/lib/wangzhe/uploads
   sudo install -d -o wangzhe-backup -g wangzhe-backup -m 0700 /var/backups/wangzhe
   sudo install -d -o root -g root -m 0755 /etc/wangzhe /var/www/acme
   ```

2. 创建应用数据库账号、数据库，以及只拥有目标数据库读取权限的独立备份账号。不要给备份账号授予集群级 `pg_read_all_data`，否则共享 PostgreSQL 集群中的其他数据库也可能被读取：

   ```sql
   CREATE ROLE wangzhe LOGIN PASSWORD '使用密码管理器生成的独立强密码';
   CREATE DATABASE wangzhe OWNER wangzhe;
   CREATE ROLE wangzhe_backup LOGIN PASSWORD '另一个独立强密码';
   GRANT CONNECT ON DATABASE wangzhe TO wangzhe_backup;
   \connect wangzhe
   GRANT USAGE ON SCHEMA public TO wangzhe_backup;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO wangzhe_backup;
   GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO wangzhe_backup;
   ALTER DEFAULT PRIVILEGES FOR ROLE wangzhe IN SCHEMA public
     GRANT SELECT ON TABLES TO wangzhe_backup;
   ALTER DEFAULT PRIVILEGES FOR ROLE wangzhe IN SCHEMA public
     GRANT SELECT ON SEQUENCES TO wangzhe_backup;
   ```

   如果同一集群还有其他数据库，DBA 还应检查这些数据库对 `PUBLIC` 的默认 `CONNECT` 权限，并按隔离要求撤销；上线前用备份账号实际执行一次 `pg_dump` 验证权限恰好够用。

3. 从已审计的 Git 提交安装服务器信任锚。该目录和文件必须属于 root，且非 root 不可写；以后更新发布工具也必须先代码审查，不能从尚未认证的上传包直接覆盖：

   ```bash
   sudo install -d -o root -g root -m 0755 /usr/local/libexec/wangzhe /usr/local/libexec/wangzhe/lib
   sudo install -o root -g root -m 0755 \
     scripts/production-deploy.sh scripts/production-rollback.sh \
     scripts/production-config-check.sh scripts/production-readiness.sh \
     scripts/postgres-backup.sh scripts/release-integrity.sh \
     /usr/local/libexec/wangzhe/
   sudo install -o root -g root -m 0644 \
     scripts/lib/backend-env.sh scripts/lib/safe-integer.sh scripts/lib/maintenance-edge.sh \
     /usr/local/libexec/wangzhe/lib/
   sudo ln -sfn /usr/local/libexec/wangzhe/production-deploy.sh /usr/local/sbin/wangzhe-production-deploy
   sudo ln -sfn /usr/local/libexec/wangzhe/production-rollback.sh /usr/local/sbin/wangzhe-production-rollback
   ```

4. 从示例分别创建应用和备份环境文件。应用文件仅由 root 读取；备份文件只包含 pg_dump 所需数据库变量，由 `wangzhe-backup` 读取：

   ```bash
   sudo install -o root -g root -m 0400 deploy/env/backend.env.example /etc/wangzhe/backend.env
   sudo install -o wangzhe-backup -g wangzhe-backup -m 0400 deploy/env/backup.env.example /etc/wangzhe/backup.env
   sudo editor /etc/wangzhe/backend.env
   sudo editor /etc/wangzhe/backup.env
   sudo /usr/local/libexec/wangzhe/production-config-check.sh /etc/wangzhe/backend.env
   ```

   `BACKEND_SERVER_BIND` 保持 `127.0.0.1`，JWT 与数据加密密钥必须独立随机生成。远程 PostgreSQL（包括备份账号）使用 `verify-ca`/`verify-full`；备份账号不得与应用账号相同。远程 Redis 必须启用 TLS 和密码，并为生产设置独立 Redis 前缀。真实 `.env`、证书私钥和数据库备份不得进入 Git。

5. 先用临时 HTTP 配置申请证书，再启用正式 TLS 配置。同一张 SAN 证书必须同时覆盖 `wz6688.app` 与 `admin.wz6688.app`，配置中的证书路径也由两个 HTTPS vhost 共用。正式配置会拒绝未知 Host、将固定域名从 HTTP 308 跳转到 HTTPS、添加 HSTS/CSP 等安全头、限制每 IP 连接/请求频率，并把普通请求体限制为 1MB；只有管理端活动图片上传允许 9MB。用户域名会直接拒绝 `/api/admin`、`/api/tenant` 和 `/api/agent`。当 `/etc/wangzhe/maintenance` 存在时，两个 HTTPS 站点统一返回 503；该标记跨重启保留，只有完整上线门禁成功后发布器才移除。Nginx 访问日志只记录 `$request_method` 与不含查询串的 `$uri`，不会把 `/api/ws?ticket=...` 的一次性票据写入磁盘；不要改回包含 `$request` 或 `$args` 的 combined 格式：

   ```bash
   sudo rm -f /etc/nginx/sites-enabled/default
   sudo install -m 0644 deploy/nginx/wangzhe-acme-bootstrap.conf /etc/nginx/sites-available/wangzhe-bootstrap.conf
   sudo ln -s /etc/nginx/sites-available/wangzhe-bootstrap.conf /etc/nginx/sites-enabled/wangzhe-bootstrap.conf
   sudo nginx -t && sudo systemctl reload nginx
   sudo certbot certonly --webroot -w /var/www/acme --cert-name wz6688.app \
     -d wz6688.app -d admin.wz6688.app
   sudo rm /etc/nginx/sites-enabled/wangzhe-bootstrap.conf
   sudo install -m 0644 deploy/nginx/snippets/*.conf /etc/nginx/snippets/
   sudo install -m 0644 deploy/nginx/wz6688.app.conf /etc/nginx/sites-available/wz6688.app.conf
   sudo ln -s /etc/nginx/sites-available/wz6688.app.conf /etc/nginx/sites-enabled/wz6688.app.conf
   sudo nginx -t && sudo systemctl reload nginx
   sudo certbot renew --dry-run
   ```

6. 安装 systemd 单元。后端以 `wangzhe` 运行，备份以独立的 `wangzhe-backup` 运行；两者均无额外 capability，并启用只读系统、私有临时目录、命名空间/设备/内核保护：

   ```bash
   sudo install -m 0644 deploy/systemd/wangzhe-*.service deploy/systemd/wangzhe-*.timer /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable wangzhe-backend.service wangzhe-backup.timer
   ```

## 版本化发布

发布脚本拒绝覆盖已有版本，并按固定顺序执行：用外部摘要认证包清单 → 校验发布目录整条父路径、目标架构与全部文件 → `nginx -t` → 用隔离环境和独立备份账号创建且验证数据库备份 → 复制到同文件系统临时目录并再次比对外部摘要/全部文件 → 创建持久维护标记并实际访问两个公网域名，确认当前运行中的 Nginx 已返回 503 与安全头 → 原子安装并切换 `/opt/wangzhe/current` → 启动并等待 `/ready`（包括迁移清单/校验和）→ 在外部仍只收到 503 时使用固定安全阈值执行完整生产门禁 → 重载 Nginx 并移除维护标记。不要直接把文件覆盖进 `current`：

```bash
sudo chown -R root:root /tmp/wangzhe-release
sudo chmod -R a+rX,go-w /tmp/wangzhe-release
sudo EXPECTED_MANIFEST_SHA256=构建机通过独立通道提供的64位摘要 \
  RELEASE_ID=20260829-1 \
  /usr/local/sbin/wangzhe-production-deploy /tmp/wangzhe-release
sudo systemctl enable --now wangzhe-backup.timer
```

每次后端启动会用 PostgreSQL advisory lock 顺序执行尚未应用的版本化迁移。迁移失败时服务不会就绪；禁止跳过迁移、修改已发布 SQL 或手改 `schema_migrations` 校验和。

首次发布完成后必须立即创建一个非默认密码的平台管理员。后端在 release 模式不会自动生成 `admin / 123456`，也不会把本地验收会员、租户、代理、模拟历史开奖或默认计划写入正式库。先在密码管理器生成并保存满足复杂度要求的密码，再通过交互输入写入 owner-only 文件；密码不能放在命令行、Shell 历史或环境变量：

```bash
sudo install -o wangzhe -g wangzhe -m 0600 /dev/null /run/wangzhe-admin-password
sudo -u wangzhe bash -c 'read -rsp "Initial admin password: " secret; echo; printf "%s\n" "$secret" > /run/wangzhe-admin-password'
sudo systemd-run --wait --pipe --collect --uid=wangzhe \
  -p EnvironmentFile=/etc/wangzhe/backend.env \
  /opt/wangzhe/current/bin/wangzhe-bootstrap-admin \
  --username platform-owner --password-file /run/wangzhe-admin-password
sudo rm -f /run/wangzhe-admin-password
```

该命令只能在库内尚无管理员时成功，密码至少 14 位且必须包含大小写字母、数字和符号。创建后按团队密钥管理规范保存凭据并定期轮换。

上线前检查：

```bash
sudo BACKEND_URL=http://127.0.0.1:8080 /usr/local/libexec/wangzhe/production-readiness.sh /etc/wangzhe/backend.env
```

就绪检查会验证两个公网域名的证书、HTTP→HTTPS 固定跳转和安全头，并核验动态迁移清单及校验和、22 个开放彩种、开奖源、超时/异常/无归属注单、无归属账号、机器人、清理任务和最近 26 小时内备份。任何已知历史异常默认阻止上线；只能在完成对账后重新检查，不能为了通过检查擅自提高阈值。发布配置自身可先离线检查：

```bash
make readiness-test
```

## 备份与恢复演练

每天至少执行一次；正常情况由 timer 调用独立备份账号：

```bash
sudo systemctl start wangzhe-backup.service
sudo systemctl status wangzhe-backup.service
```

备份脚本先写入临时文件，使用 `pg_restore --list` 验证后再原子改名，并保存 SHA-256。至少保留一份异机备份。每月在独立测试数据库执行恢复演练；禁止直接覆盖生产库：

```bash
createdb wangzhe_restore_test
pg_restore --clean --if-exists --no-owner --dbname wangzhe_restore_test /var/backups/wangzhe/FILE.dump
```

## 回滚

后端会在监听 `/ready` 之前提交迁移，因此 `production-deploy.sh` 一旦尝试启动新进程，就绝不会自动启动旧代码：启动、就绪或完整门禁失败时会保留持久维护标记、停止服务并保持新版本链接，机器重启也不会意外对外放行。上一版仍保留在受控链接中，但只有人工确认新增迁移与旧代码兼容后才能回退。脚本绝不会自动逆向执行数据库迁移。上线后需要人工回退代码时：

```bash
sudo CONFIRM_SCHEMA_COMPATIBLE=YES /usr/local/sbin/wangzhe-production-rollback
```

脚本只接受受控的 `/opt/wangzhe/previous`，原子交换链接并等待旧版 `/ready`；旧版若与当前数据库不兼容，会立即切回回滚前版本。人工回滚成功后维护模式仍然保留，使用 `ALLOW_MAINTENANCE_503=1` 重新运行完整就绪检查，确认两个外部站点确实处于受保护的 503 状态且其他门禁均通过后，才能由运维人员删除标记：

```bash
sudo ALLOW_MAINTENANCE_503=1 BACKEND_URL=http://127.0.0.1:8080 \
  /usr/local/libexec/wangzhe/production-readiness.sh /etc/wangzhe/backend.env
sudo rm /etc/wangzhe/maintenance
```

数据库恢复属于单独的高风险维护操作：先停写、保留故障现场、再次备份当前库，在独立数据库验证目标 dump 和应用兼容性后，再由 DBA 执行。不要让发布脚本自动 `pg_restore --clean` 生产库。至少保留当前版、上一版和一份已验证的异机备份。

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
