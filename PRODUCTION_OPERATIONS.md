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

现有 `wz6688.app` 主机也可使用 `deploy/nginx/wz6688.split-hosts.conf`：会员端为 `https://wz6688.app`，管理端为 `https://www.wz6688.app`，适用于 DNS 和现有 SAN 证书仅覆盖 apex/www 的情况。此布局构建管理端时设置 `VITE_MEMBER_ASSET_BASE_URL=https://wz6688.app`；两端仍分别使用同源 `/api`，会员域名拒绝管理接口。不要与同域名的旧 vhost 同时启用，也不要为切换此站点修改其他域名的配置。

从旧系统重新安装时，使用新的应用数据库、独立角色和 Redis 实例，先加密备份旧数据库及原站点配置，校验发布包、迁移和就绪接口后再切换 Nginx。仅初始化默认目录与首位管理员，不导入本地测试余额、注单或开发账号。数据库切换成功不等于完整生产就绪：异机加密备份、PITR、恢复演练、告警接收端及主机访问加固仍须按下文完成；单机回滚备份不能替代这些要求，不得通过放宽正式发布门禁来宣称已完成。

以下流程假设一台专用 Ubuntu/Debian 主机，公网只开放 80/443，PostgreSQL 与 Redis 只监听回环或受控私网。不要把真实环境文件复制进代码目录或发布包。

主机本身先完成基础加固：SSH 仅允许密钥认证、禁止 root/密码远程登录；22 端口只对运维来源开放；启用自动安全更新和时间同步；PostgreSQL/Redis 不绑定公网地址；Redis 6.2+ 开启 AOF，远程连接使用 TLS；系统日志设置保留期并限制读取权限。云安全组与主机防火墙应只放行 80/443 和受限的运维 SSH，不能把 5432、6379 或 8080 暴露到公网。

1. 创建应用、备份、监控三个无登录系统用户，并准备最小权限目录。`wangzhe-restore` 只能创建在独立恢复主机，不得出现在生产应用主机。备份目录使用 setgid 的专用监控组：明文临时文件只允许进入专用 LUKS2 工作盘上的服务私有目录，只有验证后的 `.age`、SHA、异机凭证和 WAL 源凭证会变为组只读 `0640`，因此监控可以检查证据但不能读到明文或修改备份：

   ```bash
   sudo useradd --system --user-group --home /var/lib/wangzhe --shell /usr/sbin/nologin wangzhe
   sudo groupadd --system wangzhe-monitor
   sudo useradd --system --user-group --home /var/backups/wangzhe --shell /usr/sbin/nologin wangzhe-backup
   sudo useradd --system --gid wangzhe-monitor --home /var/lib/wangzhe-monitor --shell /usr/sbin/nologin wangzhe-monitor
   sudo usermod -a -G wangzhe wangzhe-backup
   sudo install -d -o root -g root -m 0755 /opt/wangzhe/releases
   sudo install -d -o wangzhe -g wangzhe -m 0750 /var/lib/wangzhe/uploads
   sudo install -d -o root -g root -m 0755 /var/backups/wangzhe
   sudo install -d -o wangzhe-backup -g wangzhe-monitor -m 2750 /var/backups/wangzhe/database /var/backups/wangzhe/uploads
   sudo install -d -o postgres -g wangzhe-monitor -m 2750 /var/backups/wangzhe/wal /var/backups/wangzhe/base
   sudo install -d -o wangzhe-monitor -g wangzhe-monitor -m 0700 /var/lib/wangzhe-monitor
   sudo install -d -o root -g root -m 0755 /etc/wangzhe /var/www/acme
   ```

   另外安装并锁定受支持版本的 `cryptsetup`、`util-linux`（提供 `findmnt`）、`age`、`rclone` 和 PostgreSQL client，再准备一块只用于短暂明文的独立块设备。下例的 `/dev/disk/by-id/CHANGE_ME_BACKUP_WORK_DEVICE` 必须替换为人工核验过的精确设备；`luksFormat` 会清空该设备，禁止复制示例后直接执行。使用 LUKS2 直接映射并把密钥托管到服务器的受控密钥系统，不能复用 AGE identity、数据库密码或登录密码。挂载必须启用 `nodev,nosuid,noexec`，并写入 `/etc/crypttab` 与 `/etc/fstab`，保证开机解锁失败时备份单元直接失败而不会落到根盘：

   ```bash
   sudo cryptsetup luksFormat --type luks2 /dev/disk/by-id/CHANGE_ME_BACKUP_WORK_DEVICE
   sudo cryptsetup open /dev/disk/by-id/CHANGE_ME_BACKUP_WORK_DEVICE wangzhe-backup-work
   sudo mkfs.ext4 -L wangzhe-backup-work /dev/mapper/wangzhe-backup-work
   sudo install -d -o root -g root -m 0711 /var/lib/wangzhe-backup-work
   sudo mount -o nodev,nosuid,noexec /dev/mapper/wangzhe-backup-work /var/lib/wangzhe-backup-work
   sudo chmod 0711 /var/lib/wangzhe-backup-work
   sudo install -d -o wangzhe-backup -g wangzhe-backup -m 0700 \
     /var/lib/wangzhe-backup-work/database /var/lib/wangzhe-backup-work/uploads
   sudo install -d -o postgres -g postgres -m 0700 /var/lib/wangzhe-backup-work/pitr
   findmnt --mountpoint /var/lib/wangzhe-backup-work -o SOURCE,FSTYPE,OPTIONS
   sudo cryptsetup status wangzhe-backup-work
   ```

   `/etc/crypttab` 应使用 `wangzhe-backup-work UUID=<cryptsetup luksUUID 输出> none luks`，把映射名固定为 `wangzhe-backup-work`；`/etc/fstab` 应使用 `blkid /dev/mapper/wangzhe-backup-work` 得到的文件系统 UUID，并写成 `UUID=<文件系统 UUID> /var/lib/wangzhe-backup-work ext4 nodev,nosuid,noexec 0 2`。两处 UUID 不是同一个概念，必须分别读取。脚本会同时核验：挂载源是 `/dev/mapper`/`dm-*`、设备 UUID 是 `CRYPT-LUKS1/2`、文件系统为 ext4/xfs、工作目录 owner/`0700`、整条路径无 symlink 且开工前为空。发现任何上次任务残留时会拒绝自动删除并让任务失败；先停用对应 timer、保留日志并调查退出原因，必要时重新建立该 LUKS 文件系统和密钥，再恢复定时任务。

2. 创建应用数据库账号、数据库、只读逻辑备份账号、监控账号及仅有 `REPLICATION` 的基础备份账号。不要给逻辑备份账号授予集群级 `pg_read_all_data`：

   ```sql
   CREATE ROLE wangzhe LOGIN PASSWORD '使用密码管理器生成的独立强密码';
   CREATE DATABASE wangzhe OWNER wangzhe;
   CREATE ROLE wangzhe_backup LOGIN PASSWORD '另一个独立强密码';
   CREATE ROLE wangzhe_monitor LOGIN PASSWORD '独立监控密码';
   CREATE ROLE wangzhe_replication LOGIN REPLICATION PASSWORD '独立复制密码';
   GRANT CONNECT ON DATABASE wangzhe TO wangzhe_backup;
   \connect wangzhe
   GRANT USAGE ON SCHEMA public TO wangzhe_backup;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO wangzhe_backup;
   GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO wangzhe_backup;
   ALTER DEFAULT PRIVILEGES FOR ROLE wangzhe IN SCHEMA public
     GRANT SELECT ON TABLES TO wangzhe_backup;
   ALTER DEFAULT PRIVILEGES FOR ROLE wangzhe IN SCHEMA public
     GRANT SELECT ON SEQUENCES TO wangzhe_backup;
   GRANT CONNECT ON DATABASE wangzhe TO wangzhe_monitor;
   GRANT USAGE ON SCHEMA public TO wangzhe_monitor;
   GRANT SELECT ON TABLE
     lottery_bets, "user", user_balance_transactions,
     user_balance_transaction_archives
     TO wangzhe_monitor;
   GRANT pg_monitor TO wangzhe_monitor;
   ```

   在 `pg_hba.conf` 中显式允许本机复制账号，并确保它不能从其他网段登录：

   ```text
   host replication wangzhe_replication 127.0.0.1/32 scram-sha-256
   host replication wangzhe_replication ::1/128      scram-sha-256
   ```

   修改后执行 `sudo -u postgres psql -c 'select pg_reload_conf()'`，再用复制账号实际运行一次 `pg_basebackup` 验证认证。如果同一集群还有其他数据库，DBA 还应检查这些数据库对 `PUBLIC` 的默认 `CONNECT` 权限，并按隔离要求撤销；上线前用逻辑备份账号实际执行一次 `pg_dump` 验证权限恰好够用。

3. 从已审计的 Git 提交安装服务器信任锚。该目录和文件必须属于 root，且非 root 不可写；以后更新发布工具也必须先代码审查，不能从尚未认证的上传包直接覆盖：

   ```bash
   sudo install -d -o root -g root -m 0755 /usr/local/libexec/wangzhe /usr/local/libexec/wangzhe/lib
   sudo install -o root -g root -m 0755 \
     scripts/production-deploy.sh scripts/production-rollback.sh \
     scripts/production-config-check.sh scripts/production-readiness.sh \
     scripts/postgres-backup.sh scripts/upload-backup.sh scripts/redis-production-check.sh \
     scripts/postgres-archive-wal.sh scripts/postgres-base-backup.sh \
     scripts/production-monitor.sh scripts/production-backup-integrity.sh \
     scripts/production-recovery-evidence-check.sh scripts/release-integrity.sh \
     /usr/local/libexec/wangzhe/
   sudo install -o root -g root -m 0644 \
     scripts/lib/backend-env.sh scripts/lib/safe-integer.sh scripts/lib/maintenance-edge.sh \
     scripts/lib/strict-env.sh scripts/lib/encrypted-backup.sh \
     /usr/local/libexec/wangzhe/lib/
   sudo ln -sfn /usr/local/libexec/wangzhe/production-deploy.sh /usr/local/sbin/wangzhe-production-deploy
   sudo ln -sfn /usr/local/libexec/wangzhe/production-rollback.sh /usr/local/sbin/wangzhe-production-rollback
   ```

4. 从示例创建应用、逻辑备份、备份加密、PITR、Redis 检查和监控环境文件。先读取当前 PostgreSQL `system_identifier`，把它写入 root 保护的单行文件，并替换 `pitr.env`、`pitr-restore.env`、`monitor.env` 中所有 `CHANGE_ME_POSTGRES_SYSTEM_IDENTIFIER`。本地与异机的 WAL/基础备份均按此标识分目录，避免重建集群后复用同名 WAL。`age` recipient 是公钥；解密 identity 只放在隔离恢复主机。rclone 配置必须使用独立最小权限 remote、启用对象版本/保留锁，并通过显式 `*_RCLONE_CONFIG` 路径读取，不能依赖被 systemd 隔离的 `~/.config`：

   ```bash
   sudo install -o root -g root -m 0400 deploy/env/backend.env.example /etc/wangzhe/backend.env
   sudo install -o wangzhe-backup -g wangzhe-backup -m 0400 deploy/env/backup.env.example /etc/wangzhe/backup.env
   sudo install -o wangzhe-backup -g wangzhe-backup -m 0400 deploy/env/backup-crypto.env.example /etc/wangzhe/backup-crypto.env
   sudo install -o postgres -g postgres -m 0400 deploy/env/pitr.env.example /etc/wangzhe/pitr.env
   sudo install -o wangzhe-monitor -g wangzhe-monitor -m 0400 deploy/env/redis-check.env.example /etc/wangzhe/redis-check.env
   sudo install -o wangzhe-monitor -g wangzhe-monitor -m 0400 deploy/env/monitor.env.example /etc/wangzhe/monitor.env
   sudo install -o wangzhe-monitor -g wangzhe-monitor -m 0400 deploy/env/ops-alert.env.example /etc/wangzhe/ops-alert.env
   sudo install -o root -g root -m 0400 deploy/env/recovery-evidence.env.example /etc/wangzhe/recovery-evidence.env
   sudo install -o wangzhe-backup -g wangzhe-backup -m 0400 /secure/path/backup-rclone.conf /etc/wangzhe/backup-rclone.conf
   sudo install -o postgres -g postgres -m 0400 /secure/path/pitr-rclone.conf /etc/wangzhe/pitr-rclone.conf
   sudo install -o wangzhe-monitor -g wangzhe-monitor -m 0400 /secure/path/monitor-rclone.conf /etc/wangzhe/monitor-rclone.conf
   sudo install -o root -g root -m 0400 /secure/path/recovery-evidence-read-rclone.conf /etc/wangzhe/recovery-evidence-read-rclone.conf
   # 恢复状态私钥只在隔离恢复机；生产机只保存对应公钥。
   sudo install -o root -g wangzhe-monitor -m 0440 /secure/path/logical-restore-status-ed25519-public.pem /etc/wangzhe/logical-restore-status-ed25519-public.pem
   sudo install -o root -g wangzhe-monitor -m 0440 /secure/path/pitr-restore-status-ed25519-public.pem /etc/wangzhe/pitr-restore-status-ed25519-public.pem
   # 另外生成两套互不相同的制品来源签名密钥：database/uploads 共用第一套，
   # basebackup/WAL 共用第二套。私钥只给对应写入服务；root-owned 公钥供监控和恢复验签。
   umask 077
   openssl genpkey -algorithm ED25519 -out /secure/path/backup-provenance-ed25519-private.pem
   openssl pkey -in /secure/path/backup-provenance-ed25519-private.pem -pubout -out /secure/path/backup-provenance-ed25519-public.pem
   openssl genpkey -algorithm ED25519 -out /secure/path/pitr-provenance-ed25519-private.pem
   openssl pkey -in /secure/path/pitr-provenance-ed25519-private.pem -pubout -out /secure/path/pitr-provenance-ed25519-public.pem
   sudo install -o wangzhe-backup -g wangzhe-backup -m 0400 /secure/path/backup-provenance-ed25519-private.pem /etc/wangzhe/backup-provenance-ed25519-private.pem
   sudo install -o postgres -g postgres -m 0400 /secure/path/pitr-provenance-ed25519-private.pem /etc/wangzhe/pitr-provenance-ed25519-private.pem
   sudo install -o root -g root -m 0444 /secure/path/backup-provenance-ed25519-public.pem /etc/wangzhe/backup-provenance-ed25519-public.pem
   sudo install -o root -g root -m 0444 /secure/path/pitr-provenance-ed25519-public.pem /etc/wangzhe/pitr-provenance-ed25519-public.pem
   PGDATA_PATH="$(sudo -u postgres psql -Atqc 'show data_directory')"
   PITR_CLUSTER_ID="$(sudo -u postgres pg_controldata "$PGDATA_PATH" | awk -F: '/Database system identifier/ {gsub(/ /,"",$2); print $2}')"
   test "$PITR_CLUSTER_ID" -gt 0
   printf '%s\n' "$PITR_CLUSTER_ID" | sudo tee /etc/wangzhe/pitr-cluster-id >/dev/null
   sudo chown root:root /etc/wangzhe/pitr-cluster-id && sudo chmod 0444 /etc/wangzhe/pitr-cluster-id
   sudo install -d -o postgres -g wangzhe-monitor -m 2750 \
     "/var/backups/wangzhe/wal/$PITR_CLUSTER_ID" "/var/backups/wangzhe/base/$PITR_CLUSTER_ID"
   sudo editor /etc/wangzhe/backend.env
   sudo editor /etc/wangzhe/backup.env
   sudo editor /etc/wangzhe/backup-crypto.env
   sudo editor /etc/wangzhe/pitr.env
   sudo editor /etc/wangzhe/redis-check.env
   sudo editor /etc/wangzhe/monitor.env
   sudo editor /etc/wangzhe/ops-alert.env
   sudo editor /etc/wangzhe/recovery-evidence.env
   sudo /usr/local/libexec/wangzhe/production-config-check.sh /etc/wangzhe/backend.env
   ```

   四份 rclone 配置必须是不同的最小权限凭据：备份账号只能向指定制品前缀写入/回读，PITR 账号只能访问当前 `system_identifier` 前缀，监控账号只能 read/list 数据库、uploads、当前 `system_identifier` 的 basebackup/WAL 及恢复状态前缀，明确禁止 write/delete；发布恢复证据账号只能读取两个精确状态名及其 `.sha256`、`.sig`，不能读取备份正文或写删对象。任何一份权限或 owner 不对都会被脚本拒绝。

   `BACKEND_SERVER_BIND` 保持 `127.0.0.1`，JWT、数据加密、PostgreSQL、Redis、Webhook、rclone、两套制品来源签名密钥与两套恢复状态签名密钥必须分别生成。来源私钥只进入对应生产写入服务，恢复状态私钥绝不能进入生产机；公钥必须 root 所有且不可被非 root 修改。当前实现使用主机上的 OpenSSL 3 Ed25519 私钥；后续更高保障应把签名操作迁入 KMS/HSM，并保持相同的字段绑定和独立密钥域。远程 PostgreSQL 使用 `verify-ca`/`verify-full`；远程 Redis 必须启用 TLS。真实 `.env`、rclone 配置、AGE identity、签名私钥、证书私钥和备份不得进入 Git。

   安装 Redis 6.2+ 配置时生成两套不同密码：应用使用 `wangzhe-app`（写入 `BACKEND_REDIS_USERNAME/PASSWORD`），监控使用 `wangzhe-monitor`（只把监控密码写入 `/etc/wangzhe/redis-check.env`），严禁复用。该检查文件另外只记录公开的 `REDIS_EXPECTED_APP_USERNAME`/`REDIS_EXPECTED_APP_PREFIX`，就绪门禁会将它们与后端配置精确比对，绝不写应用 Redis 明文密码。该 Redis 实例专用于本项目，ACL 必须且只能包含已 reset/off 的 `default`、应用和监控三个用户；`acl-pubsub-default` 固定为 `resetchannels`。应用 ACL 只允许 `wangzhe-production:*` 键/频道和代码实际使用的命令，监控用户没有任何键/频道权限，只额外允许只读 `ACL LIST/GETUSER`；AOF 使用 `everysec` 且内存策略为 `noeviction`：

   ```bash
   sudo install -o redis -g redis -m 0600 deploy/redis/wangzhe-users.acl.example /etc/redis/wangzhe-users.acl
   sudo editor /etc/redis/wangzhe-users.acl
   # 审核后把 deploy/redis/redis.conf.example 的生产项合入发行版 redis.conf
   sudo systemctl restart redis-server
   sudo -u wangzhe-monitor /usr/local/libexec/wangzhe/redis-production-check.sh /etc/wangzhe/redis-check.env
   ```

   PostgreSQL 加载 `deploy/postgresql/wangzhe-pitr.conf.example` 后重启。当前基础备份流程有意不兼容自定义 tablespace；上线门禁会查询 `pg_tablespace`，发现 `pg_default`、`pg_global` 以外的表空间就拒绝上线，避免生成看似成功但布局不完整的 PITR 制品。`archive_command` 只有在 WAL 已经 age 加密、SHA-256 校验且按策略完成异机回读时才返回成功；失败时 PostgreSQL 会保留 WAL 并重试。禁止手工删除 `pg_wal` 或尚未被较新基础备份覆盖的归档 WAL。

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

6. 安装生产主机 systemd 单元。后端、逻辑/上传备份、PITR 基础备份和主动监控使用不同账号；均无 capability，并启用只读系统、私有临时目录和内核保护。这里显式列出生产单元，绝不使用会把恢复单元一起装入生产机的通配符：

   ```bash
   sudo install -m 0644 \
     deploy/systemd/wangzhe-backend.service \
     deploy/systemd/wangzhe-backup.service deploy/systemd/wangzhe-backup.timer \
     deploy/systemd/wangzhe-upload-backup.service deploy/systemd/wangzhe-upload-backup.timer \
     deploy/systemd/wangzhe-base-backup.service deploy/systemd/wangzhe-base-backup.timer \
     deploy/systemd/wangzhe-monitor.service deploy/systemd/wangzhe-monitor.timer \
     deploy/systemd/wangzhe-backup-integrity.service deploy/systemd/wangzhe-backup-integrity.timer \
     deploy/systemd/wangzhe-ops-failure-alert@.service \
     /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo install -m 0644 deploy/logrotate/wangzhe-nginx /etc/logrotate.d/wangzhe-nginx
   sudo touch /var/log/nginx/wangzhe-access.log
   sudo chown www-data:wangzhe-monitor /var/log/nginx/wangzhe-access.log
   sudo chmod 0640 /var/log/nginx/wangzhe-access.log
   sudo systemctl enable wangzhe-backend.service wangzhe-backup.timer wangzhe-upload-backup.timer wangzhe-base-backup.timer wangzhe-monitor.timer wangzhe-backup-integrity.timer
   ```

## 版本化发布

发布脚本拒绝覆盖已有版本，并按固定顺序执行：用外部摘要认证包清单 → 校验发布目录整条父路径、目标架构与全部文件 → `nginx -t` → 用隔离环境和独立备份账号创建且验证数据库及 uploads 备份 → 以 `wangzhe-monitor` 和 `/etc/wangzhe/monitor-rclone.conf` 的只读/list 最小权限身份实时回读数据库、uploads、PITR 基础备份及全部保留 WAL，逐个核对完整 SHA-256、远端证据和 Ed25519 来源绑定 → 使用另一份 status-only 凭据回读并验签近期逻辑恢复和 PITR 演练证据，精确绑定生产数据库、对象前缀及 `system_identifier` → 复制到同文件系统临时目录并再次比对外部摘要/全部文件 → 创建持久维护标记并实际访问两个公网域名，确认当前运行中的 Nginx 已返回 503 与安全头 → 原子安装并切换 `/opt/wangzhe/current` → 启动并等待 `/ready`（包括迁移清单/校验和）→ 在外部仍只收到 503 时使用固定安全阈值执行完整生产门禁 → 重载 Nginx 并移除维护标记。两类远端校验都发生在任何 release 目录创建或链接切换之前；网络不可达、只读 IAM 越权/缺权、对象缺失/多余、摘要或来源签名不一致、演练过期或签名域复用都会 fail closed，本地历史 `.offsite-ok` 不能单独放行。不要直接把文件覆盖进 `current`：

```bash
sudo chown -R root:root /tmp/wangzhe-release
sudo chmod -R a+rX,go-w /tmp/wangzhe-release
sudo EXPECTED_MANIFEST_SHA256=构建机通过独立通道提供的64位摘要 \
  RELEASE_ID=20260829-1 \
  /usr/local/sbin/wangzhe-production-deploy /tmp/wangzhe-release
sudo systemctl enable --now wangzhe-backup.timer wangzhe-upload-backup.timer wangzhe-base-backup.timer wangzhe-monitor.timer wangzhe-backup-integrity.timer
```

每次后端启动会用 PostgreSQL advisory lock 顺序执行尚未应用的版本化迁移。迁移失败时服务不会就绪；禁止跳过迁移、修改已发布 SQL 或手改 `schema_migrations` 校验和。

首次发布完成后必须立即创建一个非默认密码的平台管理员。后端在 release 模式不会自动生成任何本地体验管理员，也不会把本地验收会员、租户、代理、模拟历史开奖或默认计划写入正式库。先在密码管理器生成并保存满足复杂度要求的密码，再通过交互输入写入 owner-only 文件；密码不能放在命令行、Shell 历史或环境变量：

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

如需正式启用机器人，必须按 `deploy/production-robot-activation.md` 在维护窗口执行“环境上限 + 按工作区启用”双门禁，不得用命令行临时变量绕过。

就绪检查会验证 Redis 认证/6.2+/AOF/noeviction 及运行时 ACL 无漂移、两个公网域名的证书和安全头，并核验动态迁移、彩种、开奖源、异常数据、机器人、清理任务，以及数据库、uploads、WAL、基础备份的加密校验和异机回读凭证。任何已知异常默认阻止上线；只能修复后重新检查，不能为了通过检查擅自提高阈值。发布配置自身可先离线检查：

```bash
make readiness-test
```

## 加密备份、PITR 与恢复演练

数据库逻辑备份与上传目录每天分别执行；PostgreSQL 基础备份每周执行，WAL 由 `archive_command` 连续归档。数据库 dump、uploads tar/校验解包和 `pg_basebackup` 数据目录只在 `/var/lib/wangzhe-backup-work/{database,uploads,pitr}` 的 LUKS 工作盘中以 `0600`/`0700` 生成，加密后删除；持久备份盘上从不创建明文 partial。只有验证后的 AGE 密文和证据对监控组只读。每个密文都有 `.sha256`、`.provenance` 和 `.provenance.sig`；原始 Ed25519 签名凭证精确绑定 schema、制品类别/名称、完整 remote 对象、密文 SHA-256、数据库名或 PostgreSQL `system_identifier`、创建 epoch。制品与三份证据全部上传并完整回读后才生成本地 `.offsite-ok`。`BACKUP_REQUIRE_OFFSITE=1` 与 `PITR_REQUIRE_OFFSITE=1` 是安全默认；异机失败会让任务失败且不生成 `.offsite-ok`。每个 WAL 另保存源明文 SHA、WAL 名和 PostgreSQL `system_identifier`，恢复时解密后再次比对：

```bash
sudo systemctl start wangzhe-backup.service
sudo systemctl start wangzhe-upload-backup.service
sudo systemctl start wangzhe-base-backup.service
sudo systemctl start wangzhe-backup-integrity.service
sudo systemctl status wangzhe-backup.service
sudo systemctl status wangzhe-upload-backup.service wangzhe-base-backup.service wangzhe-backup-integrity.service
```

上传源必须精确为 `/var/lib/wangzhe/uploads`，且从 `/` 到源目录的每一级都不能是 symlink。上传备份在加密前会隔离解包，从归档重新生成完整文件 SHA-256 清单，与归档前清单逐字节比对后再逐文件验签；恢复端在解包前再次拒绝非普通文件/目录、重复规范路径和多个清单，归档内容只要与预清单存在任何差异（包括已经进入归档、却未进入预清单的并发新增文件）就会 fail closed，空上传目录也会验证“空清单对应零文件”。逻辑 dump 在加密前通过 `pg_restore --list`；基础备份通过 `pg_verifybackup`。可捕获的异常退出会清理本次明文 partial；断电或 `SIGKILL` 产生的残留会让下一次任务 fail closed，不能静默覆盖。

WAL 不能按“固定天数”孤立删除。对象存储必须启用版本、不可变保留和生命周期；本地清理只能由 DBA 在确认“最老保留基础备份 + 目标 RPO 时间后的完整连续 WAL + 异机对象回读 + 最近 PITR 演练成功”后执行。监控会在 WAL/备份盘容量或 inode 达到 80% 时主动告警；收到告警先扩容或完成一份新的、已演练基础备份，禁止删除 `pg_wal`。基础备份保留 35 天，逻辑与上传备份默认保留 14 天；对象存储保留期应覆盖更长的业务/合规窗口。

恢复演练不能在生产主机运行。准备独立恢复主机、独立 PostgreSQL 集群和最小权限对象存储凭据。恢复机必须安装与生产相同、已通过外部 SHA-256 摘要认证的不可变 release，并让 `/opt/wangzhe/current` 指向该版本；不要把单个脚本另行复制到 `/usr/local`，否则既绕过发布清单，也与 systemd 的固定执行路径不一致。然后创建专用用户与 root 标记：

```bash
sudo useradd --system --home /var/lib/wangzhe-restore --shell /usr/sbin/nologin wangzhe-restore
sudo groupadd --system wangzhe-recovery-storage
sudo usermod -a -G wangzhe-recovery-storage wangzhe-restore
sudo usermod -a -G wangzhe-recovery-storage postgres
sudo groupadd --system wangzhe-monitor
sudo useradd --system --gid wangzhe-monitor --home /var/lib/wangzhe-monitor --shell /usr/sbin/nologin wangzhe-monitor
sudo install -d -o root -g root -m 0755 /etc/wangzhe /opt/wangzhe/releases
sudo install -d -o root -g root -m 0755 \
  /var/lib/wangzhe-restore /var/lib/wangzhe-pitr-drill /var/lib/wangzhe-recovery-postgresql
sudo install -d -o postgres -g postgres -m 0700 /var/lib/wangzhe-pitr-source
printf '%s\n' WANGZHE_ISOLATED_RECOVERY_HOST | sudo tee /etc/wangzhe/recovery-host >/dev/null
sudo chown root:root /etc/wangzhe/recovery-host
sudo chmod 0444 /etc/wangzhe/recovery-host
sudo install -o wangzhe-restore -g wangzhe-restore -m 0400 deploy/env/restore-drill.env.example /etc/wangzhe/restore-drill.env
sudo install -o postgres -g postgres -m 0400 deploy/env/pitr-source-sync.env.example /etc/wangzhe/pitr-source-sync.env
sudo install -o postgres -g postgres -m 0400 deploy/env/pitr-drill.env.example /etc/wangzhe/pitr-drill.env
sudo install -o postgres -g postgres -m 0400 deploy/env/pitr-restore.env.example /etc/wangzhe/pitr-restore.env
sudo install -o postgres -g postgres -m 0400 deploy/env/pitr-status.env.example /etc/wangzhe/pitr-status.env
sudo install -o wangzhe-monitor -g wangzhe-monitor -m 0400 deploy/env/ops-alert.env.example /etc/wangzhe/ops-alert.env
sudo install -m 0644 \
  deploy/systemd/wangzhe-restore-drill.service deploy/systemd/wangzhe-restore-drill.timer \
  deploy/systemd/wangzhe-pitr-source-sync.service \
  deploy/systemd/wangzhe-pitr-restore-drill.service deploy/systemd/wangzhe-pitr-restore-drill.timer \
  deploy/systemd/wangzhe-pitr-status-publish.service \
  deploy/systemd/wangzhe-ops-failure-alert@.service \
  /etc/systemd/system/
sudo systemctl daemon-reload
# 在继续前，为恢复机准备三块人工核验过的独立设备，分别建立 LUKS2，
# 并将 /dev/mapper 下三个不同映射直接格式化为 ext4/xfs 后挂载到：
#   /var/lib/wangzhe-restore
#   /var/lib/wangzhe-pitr-drill
#   /var/lib/wangzhe-recovery-postgresql
# `cryptsetup luksFormat` 会清空设备；必须逐块核对 by-id，禁止照抄设备名。
# 三个 /etc/fstab 条目都必须包含 nodev,nosuid,noexec，/etc/crypttab 必须
# 使用各自的 LUKS UUID；不得把 LVM、系统根盘或 bind mount 当成替代品。
mountpoint -q /var/lib/wangzhe-restore && \
  mountpoint -q /var/lib/wangzhe-pitr-drill && \
  mountpoint -q /var/lib/wangzhe-recovery-postgresql || exit 1
sudo chown root:wangzhe-recovery-storage \
  /var/lib/wangzhe-restore /var/lib/wangzhe-pitr-drill /var/lib/wangzhe-recovery-postgresql
sudo chmod 0750 \
  /var/lib/wangzhe-restore /var/lib/wangzhe-pitr-drill /var/lib/wangzhe-recovery-postgresql
sudo install -d -o wangzhe-restore -g wangzhe-restore -m 0700 /var/lib/wangzhe-restore/work
sudo install -d -o postgres -g postgres -m 0700 \
  /var/lib/wangzhe-pitr-drill/work /var/lib/wangzhe-recovery-postgresql/data
findmnt --mountpoint /var/lib/wangzhe-restore -o SOURCE,FSTYPE,OPTIONS
findmnt --mountpoint /var/lib/wangzhe-pitr-drill -o SOURCE,FSTYPE,OPTIONS
findmnt --mountpoint /var/lib/wangzhe-recovery-postgresql -o SOURCE,FSTYPE,OPTIONS
# 将隔离逻辑恢复实例的 PostgreSQL data_directory 配置为
# /var/lib/wangzhe-recovery-postgresql/data；脚本会从 SHOW data_directory
# 读取实际值并再次检查对应挂载源、dm UUID、文件系统与安全挂载选项。
# 将同一 AGE identity 分别复制为两个 0400 文件：logical 文件属于
# wangzhe-restore，pitr 文件属于 postgres；不要放宽为共享可读权限。
# 仅在恢复机的独立 PostgreSQL 集群创建专用角色：
# CREATE ROLE wangzhe_restore LOGIN CREATEDB PASSWORD '密码管理器生成的独立密码';
# GRANT pg_read_all_settings TO wangzhe_restore; -- 仅用于读取实际 data_directory 并校验 LUKS
# 它不得连接生产集群。
# 把同一 AGE identity 复制成两个独立文件：
sudo install -o wangzhe-restore -g wangzhe-restore -m 0400 /secure/path/recovery-age-identity.txt /etc/wangzhe/logical-recovery-age-identity.txt
sudo install -o postgres -g postgres -m 0400 /secure/path/recovery-age-identity.txt /etc/wangzhe/pitr-recovery-age-identity.txt
# 再安全传入四份权限独立的 rclone 配置，均为 owner-only 0400。
# logical source 凭据只能对环境文件中的数据库/uploads 精确前缀执行 list/read，
# 不能写、删或读取其他前缀；logical status 凭据只能写入并回读指定的
# last-success.status、.sha256、.sig，不能读取备份。两者不得复用凭据、文件、
# 硬链接或相同配置内容。PITR 的 read/status-write 凭据也遵循相同隔离原则。
sudo install -o wangzhe-restore -g wangzhe-restore -m 0400 /secure/path/logical-restore-source-read-rclone.conf /etc/wangzhe/logical-restore-source-read-rclone.conf
sudo install -o wangzhe-restore -g wangzhe-restore -m 0400 /secure/path/logical-restore-status-write-rclone.conf /etc/wangzhe/logical-restore-status-write-rclone.conf
sudo install -o postgres -g postgres -m 0400 /secure/path/pitr-wal-read-rclone.conf /etc/wangzhe/pitr-wal-read-rclone.conf
sudo install -o postgres -g postgres -m 0400 /secure/path/pitr-status-write-rclone.conf /etc/wangzhe/pitr-status-write-rclone.conf
# 只传入制品来源公钥，不得把生产 provenance 私钥复制到恢复机。
sudo install -o root -g root -m 0444 /secure/path/backup-provenance-ed25519-public.pem /etc/wangzhe/backup-provenance-ed25519-public.pem
sudo install -o root -g root -m 0444 /secure/path/pitr-provenance-ed25519-public.pem /etc/wangzhe/pitr-provenance-ed25519-public.pem
# 使用 OpenSSL 3.0+ 生成两套互不相同的 Ed25519 密钥。以下私钥只留在隔离恢复机；
# 公钥通过独立可信通道送到生产机，并按上文安装为 root:wangzhe-monitor 0440。
umask 077
openssl genpkey -algorithm ED25519 -out /secure/path/logical-restore-status-ed25519-private.pem
openssl pkey -in /secure/path/logical-restore-status-ed25519-private.pem -pubout -out /secure/path/logical-restore-status-ed25519-public.pem
openssl genpkey -algorithm ED25519 -out /secure/path/pitr-restore-status-ed25519-private.pem
openssl pkey -in /secure/path/pitr-restore-status-ed25519-private.pem -pubout -out /secure/path/pitr-restore-status-ed25519-public.pem
sudo install -o wangzhe-restore -g wangzhe-restore -m 0400 /secure/path/logical-restore-status-ed25519-private.pem /etc/wangzhe/logical-restore-status-ed25519-private.pem
sudo install -o postgres -g postgres -m 0400 /secure/path/pitr-restore-status-ed25519-private.pem /etc/wangzhe/pitr-restore-status-ed25519-private.pem
# 不得用一个共享文件放宽权限。逻辑恢复直接从只读异机源下载；PITR
# 演练由联网的 source-sync 单元直接从异机对象存储取得完整 base/WAL 集合。
# 确认 /opt/wangzhe/current 指向已认证的不可变 release。
sudo test -x /opt/wangzhe/current/scripts/production-restore-drill.sh
sudo test -x /opt/wangzhe/current/scripts/pitr-recovery-source-sync.sh
sudo test -x /opt/wangzhe/current/scripts/production-pitr-restore-drill.sh
sudo systemctl enable --now wangzhe-restore-drill.timer
sudo systemctl enable --now wangzhe-pitr-restore-drill.timer
sudo systemctl start wangzhe-restore-drill.service
sudo systemctl start wangzhe-pitr-source-sync.service
sudo systemctl start wangzhe-pitr-restore-drill.service
sudo systemctl status wangzhe-pitr-source-sync.service wangzhe-pitr-restore-drill.service wangzhe-pitr-status-publish.service
```

逻辑恢复不挂载、读取或信任生产机 `/var/backups` 副本和 `.offsite-ok`。脚本先验证 `/var/lib/wangzhe-restore` 是 root-owned、不可被非 root 修改、带 `nodev,nosuid,noexec` 的独立 ext4/xfs LUKS dm-crypt 直接挂载，再使用其 owner-only `/var/lib/wangzhe-restore/work` 子目录。它还会从隔离 PostgreSQL 实例读取实际 `SHOW data_directory`，并要求该目录所在文件系统通过同样的直接 LUKS、dm UUID 与 root 挂载点校验；普通根盘、LVM 外层、bind mount 或未解锁目录都会在解密/覆盖目标库前失败。脚本用 source-only 凭据分别对环境文件中的两个精确 remote 目录执行浅层 `lsf`；数据库对象必须以 `RESTORE_DRILL_SOURCE_DATABASE_NAME` 的安全名称开头并匹配严格时间格式，uploads 只接受 `uploads-*.tar.age`，然后选择最新对象，把密文、`.sha256`、`.provenance`、`.provenance.sig` 下载到 LUKS 工作目录下的本次随机临时目录。两个下载副本都必须完成全文件 SHA-256、root-owned 公钥签名及精确 remote/source/class 绑定校验后才允许解密和恢复，成功状态同时记录 provenance 摘要、三个 LUKS 挂载证据和实际下载的两个精确远端对象，并写入 `isolation=offsite_download_loopback_database_and_fixed_targets`，因此旧的本机副本演练状态不会被生产 monitor 接受。当前生产写链把两类对象放在同一个 `wangzhe-production` backup 前缀，因此两个 source 变量可以明确填写为同一前缀并依靠互斥文件名筛选；对象存储 IAM 允许时可进一步拆成数据库和 uploads 两个只读前缀，无需改恢复逻辑。

脚本只允许数字回环 PostgreSQL、固定目标库前缀、精确的 `/var/lib/wangzhe-restore/work/uploads` 与状态路径；上传恢复目标也会在原子替换后再次验证仍位于固定 LUKS 挂载。脚本验证迁移数大于 0、负余额/孤儿注单为 0，并重新生成完整上传清单。每套恢复主机用自己的 owner-only Ed25519 私钥对原始 status 做 detached signature，并通过与 source 凭据独立的 status-only rclone 凭据发布 `status`、`.sha256`、`.sig` 三个对象。生产 monitor 用只读凭据回读，在接受状态前先用对应的 root-owned 公钥执行 `openssl pkeyutl -verify -pubin -rawin`，再核验制品 SHA、业务一致性、隔离方式和时间；并把状态中的源数据库名、数据库 remote 前缀和 uploads remote 前缀与 `/etc/wangzhe/monitor.env` 的生产期望值做精确比较，避免误用测试库的有效签名演练证据。一个恢复域的签名不能冒充另一个域。该状态明确写入 `pitr_restore=not_in_scope`，不会冒充 PITR 演练。签名命令要求 OpenSSL 3.0+；系统自带、不支持 `-rawin` 或 ED25519 的旧版 LibreSSL 不可用于这些单元。

PITR 必须作为独立演练执行。每次演练前，`wangzhe-pitr-source-sync.service` 用 `/etc/wangzhe/pitr-wal-read-rclone.conf` 两次列举精确的远端集群目录，并把全部加密 base/WAL、`.sha256`、`.provenance`、`.provenance.sig` 和 WAL `.source.sha256` 下载到固定 `/var/lib/wangzhe-pitr-source`。该对象存储 IAM 身份只允许对 `wangzhe-production/pitr/<system_identifier>` 这一精确 PITR 前缀执行 list/read，必须显式拒绝写入、删除、其他集群前缀和 `restore-status`。只有前后远端快照一致、文件集合完全一致、所有密文 SHA-256、来源签名的 exact remote/class/source 绑定和 WAL 源凭证均匹配 `system_identifier` 后，才生成带精确远端对象路径的 `.offsite-ok`，并通过原子 generation 指针发布；意外文件、链接、特殊文件、陈旧源或任一校验失败都会中止。同步单元可联网，实际 drill 仍启用 `PrivateNetwork=true`，且用共享锁固定消费同一 generation。

PITR 成功状态只能由 `/etc/wangzhe/pitr-status-write-rclone.conf` 发布；其 IAM 身份只允许写入并按精确对象名回读 `wangzhe-production/restore-status/last-pitr-success.status`、`.sha256`、`.sig` 三个对象，必须拒绝列举/读取 PITR 备份、写入其他状态名和删除。读凭据与状态写凭据绝不能共用 IAM access key、rclone remote token 或配置内容。离线 drill 会在 `PrivateNetwork=true` 沙箱内 fail closed 地核验两份配置的固定规范路径、device/inode 文件身份和 SHA-256 均不同，再产生最终 v2 签名状态；发布器/生产 monitor 还会验证 source generation、精确 remote、远端快照 SHA-256、同步 epoch 及 base/WAL/WAL-segment 数量，不能用另一个前缀的有效演练状态冒充生产证据。

演练从与同一个 `system_identifier` 绑定且已校验的基础备份开始。所有解密后的 basebackup、WAL 暂存和演练 `data_directory` 只能进入 root 保护的独立 `/var/lib/wangzhe-pitr-drill` LUKS 直接挂载下的 postgres-owned `work` 子目录；systemd 通过 `RequiresMountsFor` 阻止缺盘降级，脚本再核验 dm UUID、ext4/xfs、`nodev,nosuid,noexec` 与实际数据目录设备。AGE identity 与 `deploy/env/pitr-restore.env.example` 仅放在恢复主机，通过 `restore_command` 回放带源凭证的 WAL，到明确的 `recovery_target_time`。自动演练使用隔离端口并在查询迁移、账务和开奖关键表后停止；事故人工恢复仍使用 `recovery_target_action='pause'`，人工核验后才能 promote。不得直接对生产集群执行恢复，也不能用逻辑 dump 演练替代 PITR 演练。

## 主动监控与告警

`wangzhe-monitor.timer` 每分钟执行在线检查；对四类最新备份同时核验来源签名、精确 remote/source/class 绑定、新鲜度、权限、大小和清单元数据。`wangzhe-backup-integrity.timer` 每天低频下载并完整计算最新 database/uploads/base 密文以及本地保留的**全部** WAL 密文，逐一核对本地/远端 `.sha256`、`.provenance`、`.provenance.sig`，并核对 WAL 的远端 `.source.sha256` 与文件名/`system_identifier` 绑定。远端 WAL 五件套名称集合必须与本地保留 WAL 精确相等；多余或缺少对象都失败。只有全部成功才原子更新 v2 心跳，记录 WAL inventory SHA-256/count/first/last；每分钟 monitor 只接受 v2 并做 30 小时新鲜度检查，因此 timer 被 disable/mask、只检查最后一条 WAL 或对象集合漂移都会主动告警：

- 开奖源错误、错误/超时期号、不可恢复及异常注单；
- 负余额、余额流水末值不一致，以及账本算术错误、孤儿流水、重复 reference、账本链断裂；深度账务巡检默认每 60 分钟运行一次并受 20 秒查询超时保护，最近一次成功结果会持久保留在每分钟告警中，超过两个巡检周期未成功也会告警；
- PostgreSQL 连接使用率和 WAL archiver；
- Redis 6.2+、认证、AOF 写状态及 `noeviction`，并从运行时 `ACL LIST/GETUSER` 验证应用只能访问预期 production 键/频道和命令；`~*`、`&*`、命令分类、`CONFIG`、`ACL`、`FLUSH*` 或任何额外授权都会立即告警并阻止就绪；
- 数据库、uploads、WAL、基础备份的新鲜度和元数据、每日完整本地/异机 SHA-256，以及备份盘容量/inode；
- 逻辑数据库与上传恢复、PITR 恢复两套独立的异机成功状态，并用不同 Ed25519 公钥强制验签；
- 实际公网 TLS 握手、证书链、SAN/主机名及 21 天到期预警；
- 从不含 query string 的 Nginx JSON 日志增量统计 5xx。

告警通过带独立 Bearer token 的 HTTPS Webhook 发送。问题变化与恢复会立即通知；未变化的 firing 会按 `MONITOR_ALERT_REPEAT_MINUTES`（默认 60 分钟）重发，避免长时间故障只提醒一次。Webhook 失败不更新状态，下次重试。后端、监控、备份和恢复单元还配置了 `OnFailure`，任务一旦失败就用独立 `/etc/wangzhe/ops-alert.env` 立即通知，避免等到下一次新鲜度检查。关键 unit 不使用会把缺少前置条件静默标记为“跳过成功”的 `ConditionPath*`；缺少脚本、配置、密钥或目录会直接失败并触发告警。安装后先把 Webhook 指向测试接收器，主动制造一项无害失败并确认 firing/recovery 两条消息：

```bash
sudo systemctl start wangzhe-monitor.service
sudo systemctl start wangzhe-backup-integrity.service
sudo journalctl -u wangzhe-monitor.service --since today
sudo journalctl -u wangzhe-backup-integrity.service --since today
sudo systemctl list-timers 'wangzhe-*'
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

### 独立测试站的账号填充

现有双域名 Nginx 模板支持可关闭的测试站体验账号，不影响正常 release 登录校验。默认不存在 `/etc/wangzhe/test-login.enabled`，两个域名的 `/test-login.json` 都返回 404。只有明确用于测试且可接受访客修改测试数据时才启用；它不属于正式上线配置。

- 使用 `release/bin/wangzhe-test-site-accounts --confirm-test-site --config-file /etc/wangzhe/test-site-accounts.json` 显式创建独立管理员、租户、代理和会员。JSON 必须为执行用户所有的 0600 普通文件，包含 `site` 和四类 `username/password`，租户/代理另有 `room_code/room_name`，会员的 `room_code` 与代理一致。四份密码分别随机生成，不得使用原管理员的私有密码。
- 命令需以应用用户运行，读取受保护的正式数据库环境配置；不会被启动流程自动调用，没有公开初始化 API。既有同名账号、已停用账号或房间冲突会拒绝，不能重置已有密码。四个人类账号余额为零，不伪造下注、开奖或计划记录；新房间游戏仍默认关闭。
- 会员域名只读取 `/etc/wangzhe/test-login/member.json`；管理域名只读取同目录的 `admin.json`。格式为 `{"enabled":true,"profiles":{...}}`，会员键为 `member`，管理键为 `platform/tenant/agent`。每项只含账号、密码、必要的 `workspace` 或 `room_code`，严禁放入服务端密钥。目录 root:www-data 0750，文件 root:www-data 0640，不放入源码、public 目录或构建包。
- 确认四类账号实际登录成功后创建 root 保护的 `test-login.enabled`，校验 Nginx 并重载。响应禁止缓存和索引；前端只填充，不自动登录，也不覆盖手输。DEV 模式保留独立的本地体验配置。
- 结束测试时先撤下 `test-login.enabled`，再用原私有管理员在后台停用四个带 `test-site-accounts:v1:<site>:<role>` 标记的账号，确认旧会话已失效，最后移走两份公开 JSON。只隐藏填充不等于撤销公开密码。`production-readiness.sh` 会拒绝测试开关或仍启用的受控测试账号；不得绕过此检查宣称正式验收通过。

测试站更新也应先备份数据库、上传文件和配置。单机加密备份仅用于本次回滚，不能替代前述异机备份、PITR 和恢复演练。

### 常规检查

- `/health`、用户端、管理端和 WebSocket 均可用。
- 开奖源最后成功时间、当前期状态、待结算及异常对账数量正常。
- 余额流水末值等于会员余额，无负余额，无跨工作区数据。
- 申请、红包、回水、返佣、机器人和通知的任务无重复执行。
- 下注幂等请求不会长期停留在 `processing`；自动巡检只凭不可变扣分流水恢复成功回执，无扣分证据的请求转为明确失败，账务证据异常时必须人工对账。
- `ws_session_revocation_outbox` 没有长期未投递记录；若 `pending` 或 `attempt_count` 持续增长，先恢复 Redis 6.2+ 与 AOF 持久化，禁止手工删除待办。撤权 Redis Stream 保留 24 小时、已投递数据库回执保留 7 天，连接本身每 30 秒再向 PostgreSQL 复核授权状态。
- 备份文件更新时间不超过 24 小时，磁盘和 PostgreSQL 连接数有余量。

发生异常时先停对应彩种和机器人，保留现场与日志；不要伪造开奖结果，也不要直接修改余额。通过后台对账/恢复操作处理，并保留审计记录。
