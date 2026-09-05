# 本地开发业务数据重置

本项目提供两种不同级别的本地重置，不能混用：

- `dev-reset-business-data.sh`：清空业务记录，保留账号、工作区、目录和配置。
- `dev-reset-database.sh`：完整重起系统，备份后重建 `public` schema，所有账号、数据、配置、迁移记录和孤立旧表全部清空；下次启动先执行版本化 SQL 建立结构，再由后端 bootstrap 初始化数据。

项目尚未上线，不自动接管无版本旧库。完整重建仍须满足下方备份、sentinel 和确认口令要求；不满足条件时，先备份旧库并另建独立空开发库，不能通过删除迁移记录或绕过校验继续启动。

## 保留范围

- 登录账号、工作区、成员关系。
- 彩票目录、玩法限额、房间彩种开关、赔率与系统设置。
- 活动、收款渠道、特殊号码等运营配置。
- 全局及房间数据保留策略。
- 房间计划自动化配置（包括已有启用状态）。
- 历史开发重置凭证、不可变机器人重置凭证与待投递/已投递的会话撤销 outbox。

聊天已读游标随消息清空，避免重启后新消息编号被旧游标遮住；房间期号窗口随
开奖/期号清空。计划生成凭证、订阅流、周期和期次记录属于运行数据，与计划
推荐一起清空，保留房间的计划策略配置。不会用 `CASCADE` 隐式扩展清理范围；
后续迁移新增表未明确分类时仍会拒绝执行。

重置会把全部账号余额归零、递增会话版本以注销旧 JWT，并关闭房间机器人总开关。用户、租户、代理及房间本身不会删除。

## 安全条件

执行模式同时要求：

1. 两种重置都只允许精确的 `BACKEND_SERVER_MODE=debug`；`test` 与 `release` 都会被拒绝。
2. PostgreSQL 主机是 `127.0.0.1`、`localhost` 或 `::1`。
3. 环境文件或当前进程环境明确设置：

   ```text
   BACKEND_ALLOW_DEVELOPMENT_RESET=YES
   BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev
   ```

4. 数据库名与确认口令完全一致。
5. 前端、后台、后端和其他数据库客户端已经停止。
6. 完整 `pg_dump` 备份以及与它同名配对的会员收款二维码归档都完成 age 加密、解密回读及 SHA-256 校验。任一制品失败都不会进入数据库重置。
7. 数据库已经应用到 `202608270012_reset_identity_receipts.sql`，且独立 `wangzhe_meta` schema 中的开发 sentinel 与当前物理 PostgreSQL 实例完全匹配。
8. 数据库中不存在未分类的新表；出现新表时必须先更新重置清单。
9. 明确设置 `BACKEND_SERVER_PORT`；备份前后该端口都不得监听，目标数据库也不得存在其他客户端会话。
10. 完整重建的备份必须成功恢复到随机命名的临时数据库，并对平用户数、余额总额和账务流水数；临时数据库随后精确删除。
11. 两种重置的执行环境都必须显式设置非符号链接的绝对路径 `BACKEND_UPLOAD_DIR`。脚本先把 `.private/member-payment-qr/<workspace>/<user>/<32位随机名>.png` 精确写入数据库备份同目录的 `<数据库备份>.member-payment-qr.tar.age`，发布独立 SHA-256 后，才允许提交数据库事务并逐个删除同一批文件。归档、删除和恢复都拒绝符号链接、特殊文件、越界层级和非应用命名文件；不递归删除目录。备份目录不得位于该二维码子树中。

## 现有开发库一次性登记 sentinel

先正常启动一次后端，让迁移 012 应用成功，然后停止后端。用安全输入读取一个长期保存在本机密码管理器中的随机 token（不会写进数据库明文，也不会打印）：

```bash
read -r -s BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
export BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
export BACKEND_SERVER_MODE=debug BACKEND_SERVER_PORT=8080
export BACKEND_ALLOW_DEVELOPMENT_RESET=YES
export BACKEND_INITIALIZE_DEVELOPMENT_SENTINEL=YES
export BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev
bash scripts/dev-reset-init-sentinel.sh --execute \
  --confirm 'INIT:wangzhe_dev:DEVELOPMENT-SENTINEL'
unset BACKEND_INITIALIZE_DEVELOPMENT_SENTINEL
```

数据库连接变量可来自同一个环境文件或当前 shell。脚本要求后端端口无监听、无其他数据库会话，并把数据库名、PostgreSQL `system_identifier`、服务地址/端口和 token 摘要写入不可变 sentinel。完整重建只删除 `public`，因此 sentinel 会保留并继续约束新系统。

## 使用

先只查看范围，不连接数据库：

```bash
bash scripts/dev-reset-business-data.sh --dry-run /绝对路径/backend.dev.env
```

也可以不创建密码文件，直接使用当前 shell 中已经显式导出的 `BACKEND_*` 变量：

```bash
export BACKEND_SERVER_MODE=debug
export BACKEND_DATABASE_HOST=127.0.0.1
export BACKEND_DATABASE_PORT=5432
export BACKEND_DATABASE_USER=developer
export BACKEND_DATABASE_PASSWORD='本地密码'
export BACKEND_DATABASE_DBNAME=wangzhe_dev
export BACKEND_DATABASE_SSLMODE=disable
export BACKEND_SERVER_PORT=8080
export BACKEND_UPLOAD_DIR='/绝对路径/yaoshi/backend/uploads'
export BACKEND_ALLOW_DEVELOPMENT_RESET=YES
export BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev
# 从密码管理器安全导出，不要写进 Git
export BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN='至少32字符的本机随机值'
# 与 sentinel 分开的 age identity；同样只从密码管理器读取
export BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY='AGE-SECRET-KEY-...'
bash scripts/dev-reset-business-data.sh --dry-run
```

`make dev-reset-plan` 与 `make dev-full-reset-plan` 同样默认读取当前环境；需要环境文件时传入 `ENV_FILE=/绝对路径/backend.dev.env`。dry-run 不会连接数据库。

确认前后端全部停止后执行：

```bash
bash scripts/dev-reset-business-data.sh \
  --execute \
  --confirm 'RESET:wangzhe_dev:BUSINESS-DATA' \
  --backup-dir /绝对路径/wangzhe-dev-backups \
  /绝对路径/backend.dev.env
```

使用当前环境执行时，省略命令末尾的环境文件即可；其他备份、确认口令和本机保护完全不变。

两种开发重置都使用专用的 `dev-postgres-backup.sh`：它只接受本机 debug PostgreSQL，
把 `pg_dump` 临时明文放在 `mktemp` 创建的 owner-only 目录中，使用 age 收件人加密，
再解密回读并通过 `pg_restore --list` 后才原子发布备份和 SHA-256 文件。它不会自动删除
旧备份。随后脚本用同一 age 收件人生成配套的二维码 `tar.age`，逐项解密回读并核对
归档路径、普通文件类型和内容摘要；不会留下明文 tar。`BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY` 必须与 sentinel token 分开保存在本机
密码管理器中，不得写入仓库、环境文件或数据库。

脚本会先读取 PostgreSQL 服务端主版本，并只使用同主版本的 `pg_dump`/`pg_restore`；
它会自动检查常见 Homebrew、EnterpriseDB 路径。非标准安装可显式设置绝对路径
`BACKEND_DEVELOPMENT_PG_BIN_DIR`，旧版本客户端不会被用于备份新版本数据库。

成功后会同时生成数据库内不可变的 `development_reset_receipts` 记录和备份旁的 `.reset-receipt` 文件。先验证备份可读，再重新启动后端，让开奖期号重新同步。

不要把上述两个开发授权变量加入服务器的 release 环境文件，也不要通过修改脚本绕开远程数据库、活动连接、备份或确认口令检查。

## 完整重起系统

完整重建的验收合同包含全新体验账号和 `88001` 房间，因此该流程使用的专用环境必须额外包含：

```dotenv
BACKEND_SEED_EXPERIENCE_ACCOUNTS=true
```

该变量不用于业务数据重置，也不应写入普通本地启动或 release/test 环境。完整重建脚本和只读验收会拒绝缺失或不为 `true` 的值。

先预览：

```bash
bash scripts/dev-reset-database.sh --dry-run /绝对路径/backend.dev.env
```

若变量已经在当前 shell 中显式导出，也可以直接运行：

```bash
bash scripts/dev-reset-database.sh --dry-run
```

停止前端、后台、后端和其他数据库客户端后执行：

```bash
bash scripts/dev-reset-database.sh \
  --execute \
  --confirm 'DROP:wangzhe_dev:REBUILD-PUBLIC-SCHEMA' \
  --backup-dir /绝对路径/wangzhe-dev-backups \
  /绝对路径/backend.dev.env
```

当前环境模式同样只需省略最后的环境文件参数，不会把数据库密码写入临时文件。

该模式不会删除 PostgreSQL 数据库本身，但会执行 `DROP SCHEMA public CASCADE`。数据库内的审计凭证也会随 schema 清空，因此凭证只写到已验证备份旁边。数据库 `.dump.age` 会先解密到 owner-only 的临时文件，再由 `pg_restore` 恢复到随机临时数据库进行账务快照核对；无论成功、失败或中断，明文 dump 和 identity 临时文件都会被精确清除。执行前先写入 `schema_reset_authorized_pending`；schema 事务成功后立即原子更新为 `schema_rebuilt_qr_cleanup_pending`，再重新校验并逐个清理收款二维码；只有文件清理成功后才会更新为 `bootstrap_pending`。因此，如果 schema 已重建但文件校验或删除失败，外部凭证会明确停在 `schema_rebuilt_qr_cleanup_pending`，不会误报 bootstrap 就绪。三个 pending 状态都不能被当成完成。

完整重建后的第一次后端启动必须沿用上述专用环境。使用当前 shell 时可以明确执行 `BACKEND_SEED_EXPERIENCE_ACCOUNTS=true make dev`；使用环境文件时，由后端进程管理方式安全加载同一文件。等待 bootstrap 后，指定该次外部凭证运行只读验收：

```bash
bash scripts/dev-reset-complete-receipt.sh \
  --receipt /绝对路径/wangzhe-dev-backups/<备份>.dump.full-reset-receipt \
  /绝对路径/backend.dev.env
```

验收会重新验证数据库备份和配套二维码归档的摘要，并使用同一 age identity 解密检查归档边界；随后检查 `/ready`、当前 SQL 文件的完整版本/校验和/表清单、并发唯一索引与 TRUNCATE 保护、默认平台/租户/代理/会员、体验房间 88001、机器人隔离、期初余额与不可变账务逐账号对平。新房间彩种和机器人总开关必须关闭；申请、注单、红包、清理归档、赔率覆盖及演示计划必须为空。验收不再使用旧的固定迁移数、机器人总数或默认赔率基线。

只读脚本不会修改 schema 或业务数据。所有检查通过后，才会把这一个仍为 `bootstrap_pending` 的凭证原子更新为 `complete`；失败时凭证保持 pending。

## 从成对备份恢复

数据库备份、二维码归档、两个 `.sha256` 和对应的 `.reset-receipt` / `.full-reset-receipt` 必须作为一组保存，不能只恢复其中一个。先停止后端，在权限受限的临时目录中用 `BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY` 解密凭证指向的数据库 `.dump.age`，用匹配版本的 `pg_restore` 恢复数据库并完成数据库侧校验。确认目标 `BACKEND_UPLOAD_DIR` 是正确且不含旧二维码的绝对目录后，再运行：

```bash
export BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY='从密码管理器读取的 AGE-SECRET-KEY-...'
bash scripts/dev-reset-restore-payment-qr.sh \
  --receipt /绝对路径/wangzhe-dev-backups/<数据库备份>.reset-receipt \
  --upload-dir /绝对路径/yaoshi/backend/uploads
unset BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY
```

完整重建使用 `.full-reset-receipt`，命令相同。恢复工具先确认数据库备份仍存在且摘要匹配，再对二维码归档做 age 解密回读、SHA-256、条目类型和路径边界校验；目标已有任何二维码时会拒绝合并或覆盖。只有数据库与二维码两部分都成功恢复后才能启动后端，避免数据库中的 `qr_code_file` 指向不存在的文件。
