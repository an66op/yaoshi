# 本地开发业务数据重置

本项目提供两种不同级别的本地重置，不能混用：

- `dev-reset-business-data.sh`：清空业务记录，保留账号、工作区、目录和配置。
- `dev-reset-database.sh`：完整重起系统，备份后重建 `public` schema，所有账号、数据、配置、迁移记录和孤立旧表全部清空；下次启动由后端 bootstrap 重建。

## 保留范围

- 登录账号、工作区、成员关系。
- 彩票目录、玩法限额、房间彩种开关、赔率与系统设置。
- 活动、收款渠道、特殊号码等运营配置。
- 全局及房间数据保留策略。
- 历史开发重置凭证。

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
6. 完整 `pg_dump` 备份及 SHA-256 校验成功。
7. 数据库已经应用到 `202608270012_reset_identity_receipts.sql`，且独立 `wangzhe_meta` schema 中的开发 sentinel 与当前物理 PostgreSQL 实例完全匹配。
8. 数据库中不存在未分类的新表；出现新表时必须先更新重置清单。
9. 明确设置 `BACKEND_SERVER_PORT`；备份前后该端口都不得监听，目标数据库也不得存在其他客户端会话。
10. 完整重建的备份必须成功恢复到随机命名的临时数据库，并对平用户数、余额总额和账务流水数；临时数据库随后精确删除。

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
export BACKEND_ALLOW_DEVELOPMENT_RESET=YES
export BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev
# 从密码管理器安全导出，不要写进 Git
export BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN='至少32字符的本机随机值'
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

成功后会同时生成数据库内不可变的 `development_reset_receipts` 记录和备份旁的 `.reset-receipt` 文件。先验证备份可读，再重新启动后端，让开奖期号重新同步。

不要把上述两个开发授权变量加入服务器的 release 环境文件，也不要通过修改脚本绕开远程数据库、活动连接、备份或确认口令检查。

## 完整重起系统

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

该模式不会删除 PostgreSQL 数据库本身，但会执行 `DROP SCHEMA public CASCADE`。数据库内的审计凭证也会随 schema 清空，因此凭证只写到已验证备份旁边。执行前先写入 `schema_reset_authorized_pending`，schema 事务成功后再原子更新为 `bootstrap_pending`；两个状态都不能被当成完成。

重启后端并等待 bootstrap 后，指定该次外部凭证运行只读验收：

```bash
bash scripts/dev-reset-complete-receipt.sh \
  --receipt /绝对路径/wangzhe-dev-backups/<备份>.dump.full-reset-receipt \
  /绝对路径/backend.dev.env
```

验收会检查 `/ready`、最新迁移、默认平台/租户/代理/会员、房间 8801、期初余额与不可变账务对平、应为空的申请/注单/红包/清理归档及 legacy 痕迹。所有检查通过后，才会把这一个仍为 `bootstrap_pending` 的凭证原子更新为 `complete`；失败时凭证保持 pending。
