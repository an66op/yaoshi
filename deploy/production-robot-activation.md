# 生产机器人受控启用

生产环境的房间机器人采用两道独立门禁：

- `BACKEND_ROOM_ACTIVITY` 是进程开关。只有精确值 `1` 会在 release 模式启动调度；缺失、`0`、`true` 等值都会保持关闭。
- `BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES` 是允许启用的机器人工作区数量上限，范围为 `0-100`。调度开启时必须大于零；数据库中启用数量一旦超过它，本轮所有机器人调度都会暂停，而不是只运行其中一部分。

首次发布必须保持：

```dotenv
BACKEND_ROOM_ACTIVITY=0
BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=0
```

并确认 `workspace_robot_settings.enabled` 没有启用记录。正常生产门禁、开奖、结算、余额对账、Redis、备份与 WebSocket 检查全部通过之前，不得开启机器人。

机器人注单只用于房间活跃展示：后端在下注和结算两个边界强制它们的飞单、回水和代理分成为零，分成/回水聚合也会再次排除机器人身份。启用验收必须确认机器人流水没有生成任何真实代理收益或对外跟注金额。

## 首次受控开启

以下操作应安排维护窗口，并由有权修改 root-only 环境文件和机器人设置的人员共同执行：

1. 保持 `BACKEND_ROOM_ACTIVITY=0`，把 `BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES` 设置成这次批准的小范围数量，例如 `1`。
2. 在后台只启用已经核对彩种、限额、每日注单上限和最大待结注单数的工作区。不要超过环境文件中的上限。
3. 创建 `/etc/wangzhe/maintenance`，确认两个公网入口都进入带安全响应头的 503 维护状态。
4. 在调度仍关闭时执行完整门禁：

   ```bash
   sudo ALLOW_MAINTENANCE_503=1 BACKEND_URL=http://127.0.0.1:8080 \
     /usr/local/libexec/wangzhe/production-readiness.sh /etc/wangzhe/backend.env
   ```

5. 门禁通过后，将 `BACKEND_ROOM_ACTIVITY` 精确改为 `1`，先执行配置检查，再重启服务并再次执行同一完整门禁。
6. 检查机器人状态页、首个下注/开奖/结算、余额流水和异常对账均正常，且该注单的 `fly_cents`、`rebate_cents`、`agent_share_cents` 全部为 `0` 后，才移除维护标记。

后续发布允许保留显式开启状态。发布器仍会运行配置检查和完整门禁，数据库中启用的工作区数量必须不超过固定上限，因此不会因为开关为 `1` 永久失败。

后台机器人开关属于特权接口，修改记录由现有特权审计中间件保存；release 进程启动调度时还会把显式启用和工作区上限写入 `journalctl -u wangzhe-backend`。运维人员应把 root-only 环境文件的审批记录、两次门禁输出和对应后台审计记录放在同一变更单中，形成可追溯的启用凭证。

## 紧急停止与扩容

发现开奖源、结算、余额、Redis 或异常注单问题时，立即将 `BACKEND_ROOM_ACTIVITY=0` 并重启后端；数据库设置可以保留用于排查，但机器人不会继续调度。

扩容不能只提高数据库启用数量。先在调度关闭和维护模式下提高环境上限，重新执行完整门禁，再按“首次受控开启”的顺序恢复。不要用命令行临时覆盖上限绕过受控环境文件。
