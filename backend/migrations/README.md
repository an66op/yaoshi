# 数据库迁移

后端启动只执行本目录内按文件名排序的 SQL 迁移。每个文件的 SHA-256
校验和记录在 `public.schema_migrations`，同一数据库上的并发实例通过
PostgreSQL advisory lock 串行执行迁移。正式运行路径不会调用 GORM
`AutoMigrate`。

## 全新数据库

直接启动后端即可。`202608260000_core_schema.sql` 创建完整核心结构，后续
迁移依次追加约束、数据修复和功能字段。相同版本再次启动不会重复执行；已
应用文件被修改时，启动会因校验和不一致而失败。

## 现有数据库

首次升级时，如果旧库已有完整的核心表，迁移器会核对基线表清单并登记基线
校验和，不会重建或清空任何表。所有后续版本执行完后还会核对基线列清单；
结构不完整时进程会停止，而不是偷偷用当前 Go 模型修改生产库。

非常旧且结构不完整的开发库可使用一次性兼容命令：

```sh
BACKEND_DATABASE_LEGACY_BOOTSTRAP_CONFIRM='legacy-bootstrap:backend' \
  go run ./cmd/db-bootstrap
```

该命令默认关闭、只允许 `debug`/`test` 环境、只接受已有 `user` 表的旧库，
不能用于 release。全新数据库不需要也不能使用它。

## 新增迁移

1. 新建一个大于当前版本的 `YYYYMMDDNNNN_description.sql` 文件。
2. SQL 必须可在事务中执行，数据修复需幂等；禁止修改已经发布的文件。
3. 不要向 `config.BootstrapLegacySchema` 或模型标签中加入发布时依赖的隐式
   变更。
4. 运行 `go test ./...`，并至少在一个空 PostgreSQL 数据库验证首次迁移和
   二次幂等启动。

若必须修正已发布迁移，新增补丁版本，不能改动旧文件的校验和。
