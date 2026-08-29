# 生产恢复证据发布门禁

`scripts/production-recovery-evidence-check.sh` 是独立的发布前门禁。它只接受固定的 `/etc/wangzhe/recovery-evidence.env`，并用专用只读 `/etc/wangzhe/recovery-evidence-read-rclone.conf` 下载逻辑恢复与 PITR 的两组三件套状态。任何对象缺失、摘要或 Ed25519 签名不匹配、状态版本不对、演练过期、来源不是配置的生产数据库/上传/PITR 前缀、PITR 集群标识不符，都会返回非零并阻止发布。

逻辑恢复和 PITR 必须使用不同的 Ed25519 密钥；脚本会比较两把公钥的 DER SHA-256 指纹。专用 rclone IAM 只能读取两个精确状态名及其 `.sha256`、`.sig`，不得读取备份正文、写入或删除对象。环境文件示例见 `deploy/env/recovery-evidence.env.example`。

接入发布流程时，把脚本与两个依赖库作为服务器预装、root 保护的部署信任锚；在切换 `current` 软链接和重启应用之前，以 root 运行信任锚副本（不要执行上传包内的同名脚本）：

```bash
/usr/local/libexec/wangzhe/production-recovery-evidence-check.sh
```

如果组织规定“首次上线尚无演练证据”，必须走一次有审批、可审计且不可长期保留的 break-glass 流程；不要把跳过开关写进脚本或环境文件。正常发布只接受最长 35 天内的两类生产演练证据。
