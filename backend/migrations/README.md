# 数据库迁移

后端启动只执行本目录内按文件名排序的 SQL 迁移。每个文件的 SHA-256
校验和记录在 `public.schema_migrations`，同一数据库上的并发实例通过
PostgreSQL advisory lock 串行执行迁移。应用启动不会调用 GORM `AutoMigrate`，
工作区 bootstrap 也不再建表、重建索引或自动合并重复业务记录。迁移器仅管理
自己的 `schema_migrations` 表，其余结构变更全部来自版本化 SQL。

## 全新数据库

直接启动后端即可。`202608260000_core_schema.sql` 创建完整核心结构，后续
迁移依次追加约束、数据修复和功能字段。相同版本再次启动不会重复执行；已
应用文件被修改时，启动会因校验和不一致而失败。

## 本地数据库

项目尚未上线，不支持 AutoMigrate 旧库、无版本旧库或双 schema 兼容。若本地
开发库没有当前核心基线，先备份，再显式重建本地库并由后端完整执行本目录
迁移；迁移器会拒绝接管已有 public 表、视图或序列的无版本库，也会拒绝没有
核心基线记录的旧迁移账本，不会根据 Go 模型偷偷补表或补列。

已有 `schema_migrations` 的本地库可继续按校验和增量迁移。迁移文件一旦登记
便不可修改；校验和冲突或基线结构缺失都会令进程停止。

## 新增迁移

1. 新建一个大于当前版本的 `YYYYMMDDNNNN_description.sql` 文件。
2. SQL 必须可在事务中执行，数据修复需幂等；禁止修改已经发布的文件。
3. 不要使用 `AutoMigrate` 或模型标签承载发布时依赖的隐式结构变更。
4. 新增应用表时，在同一条迁移末尾调用
   `SELECT public.install_application_truncate_guards();`；触发器安装也是
   版本化 schema 变更，不依赖每次启动时执行无版本补丁。
5. 运行 `go test ./...`，并在一个独立空 PostgreSQL 数据库验证首次迁移、
   二次幂等启动和拒绝无版本库：

   ```bash
   BACKEND_MIGRATIONS_TEST_DSN='postgres://测试用户@127.0.0.1:测试端口/wangzhe_migrations_test?sslmode=disable' make migration-integration-test
   ```

   从仓库根目录执行；只允许回环地址和精确的 `wangzhe_migrations_test` 空库，
   所有结构和 fixture 都在测试事务结束时回滚，不读取开发配置文件。

完整开发重建后的只读验收脚本也从本目录即时计算迁移文件、校验和与表清单，
不维护第二份固定版本列表。

若必须修正已发布迁移，新增补丁版本，不能改动旧文件的校验和。
