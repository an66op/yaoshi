import { useState } from "react";
import { Icon } from "../components/Icon";
import { Avatar } from "../components/Avatar";
import { RedPacketDialog } from "../components/Dialogs";
import { ActionDialog } from "../components/Dialogs";
import type { ChatView } from "../router";
import { playNotificationSound } from "../utils/notificationAudio";

type Room = "group" | "service";

export function Chats({
  view,
  unreadCount,
  onMarkAllRead,
  onNavigate,
}: {
  view: ChatView;
  unreadCount: number;
  onMarkAllRead: () => void;
  onNavigate: (view: ChatView) => void;
}) {
  const allRead = unreadCount === 0;
  if (view === "notices")
    return (
      <NotificationList
        onBack={() => onNavigate("list")}
        onOpen={(kind) =>
          onNavigate(kind === "system" ? "notices-system" : "notices-activity")
        }
      />
    );
  if (view === "notices-system" || view === "notices-activity")
    return (
      <NotificationThread
        kind={view === "notices-system" ? "system" : "activity"}
        onBack={() => onNavigate("notices")}
      />
    );
  if (view === "group" || view === "service")
    return (
      <ChatRoom
        room={view}
        title={view === "group" ? "聊天室" : "在线客服"}
        onBack={() => onNavigate("list")}
        onOpenNotices={() => onNavigate("notices")}
      />
    );
  return (
    <section className="chat-list">
      <header className="blue-header">
        <b>聊天</b>
        <button aria-label="查看通知" onClick={() => onNavigate("notices")}>
          <Icon name="bell" />
        </button>
      </header>
      <div className="chat-subhead">
        <span>消息</span>
        <button onClick={onMarkAllRead}>
          {allRead ? "已全部读" : "全部已读"}
        </button>
      </div>
      <ChatRow
        kind="service"
        name="在线客服"
        message="客服小七：已为您接入专属客服"
        time="12:18"
        onClick={() => onNavigate("service")}
      />
      <ChatRow
        kind="group"
        name="聊天室"
        message={
          allRead ? "暂无未读消息" : "[系统] 第 084 期即将截止，请合理安排时间"
        }
        time="昨天"
        badge={allRead ? undefined : String(unreadCount)}
        onClick={() => onNavigate("group")}
      />
    </section>
  );
}

function ChatRow({
  kind,
  name,
  message,
  time,
  badge,
  onClick,
}: {
  kind: Room;
  name: string;
  message: string;
  time: string;
  badge?: string;
  onClick: () => void;
}) {
  return (
    <button className="chat-row" onClick={onClick}>
      <span className={kind === "service" ? "service-logo" : "group-logo"}>
        {kind === "service" ? "七" : "聊"}
        {badge && <i>{badge}</i>}
      </span>
      <div>
        <b>{name}</b>
        <small>{message}</small>
      </div>
      <time>{time}</time>
    </button>
  );
}

function ChatRoom({
  room,
  title,
  onBack,
  onOpenNotices,
}: {
  room: Room;
  title: string;
  onBack: () => void;
  onOpenNotices: () => void;
}) {
  const [opened, setOpened] = useState(false);
  const [packet, setPacket] = useState<"daily" | "lucky" | null>(null);
  return (
    <section className="chat-room">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}>
          <Icon name="back" />
        </button>
        <b>{title}</b>
        <button aria-label="查看通知" onClick={onOpenNotices}>
          <Icon name="bell" />
        </button>
      </header>
      {room === "service" ? (
        <ServiceConversation />
      ) : (
        <>
          <div className="room-notice">
            为保障社区体验，当前聊天室仅支持查看消息
          </div>
          <div className="chat-history">
            <p>
              今天 11:08
              <br />
              系统已为您同步最新活动
            </p>
            <div className="system-pill">第 20260816-084 期还有 01:42 截止</div>
            <PacketBubble
              title="今日签到奖励"
              description="18 积分已存入账户"
              claimed
              onClick={() => setPacket("daily")}
            />
            <PacketBubble
              title={opened ? "好运奖励包" : "打开好运奖励包"}
              description={
                opened ? "获得 8 积分，已存入账户" : "本期专属福利 · 点击领取"
              }
              claimed={opened}
              onClick={() => setPacket("lucky")}
            />
          </div>
          <div className="chat-input">
            <button aria-label="添加内容" disabled>
              <Icon name="plus" />
            </button>
            <div>聊天室暂时禁言</div>
            <button aria-label="更多操作" disabled>
              <Icon name="more" />
            </button>
          </div>
          {packet && (
            <RedPacketDialog
              type={packet}
              claimed={opened || packet === "daily"}
              onOpen={() => setOpened(true)}
              onClose={() => setPacket(null)}
            />
          )}
        </>
      )}
    </section>
  );
}

function ServiceConversation() {
  const [asked, setAsked] = useState(false);
  const sendQuickReply = () => {
    setAsked(true);
    playNotificationSound("message");
  };
  return (
    <>
      <div className="room-notice">客服小七在线 · 平均响应时间约 2 分钟</div>
      <div className="chat-history service-history">
        <p className="service-time">今天 12:18</p>
        <ServiceMessage text="您好，我是客服小七，很高兴为您服务。请问有什么可以帮您？" />
        <ServiceMessage outgoing text="我想查看今天的积分奖励。" />
        <ServiceMessage text="您今日已连续签到 7 天，18 积分已到账；完成商城任务还可以再领取 48 积分。" />
        {asked && (
          <>
            <ServiceMessage outgoing text="好的，我知道了，谢谢。" />
            <ServiceMessage text="不客气～ 已为您保留当前奖励记录。如需查看明细，可前往「我的 - 积分明细」。" />
          </>
        )}
        {!asked && (
          <div className="service-replies">
            <button onClick={sendQuickReply}>我想了解积分明细</button>
          </div>
        )}
      </div>
      <div className="chat-input">
        <button aria-label="添加内容">
          <Icon name="plus" />
        </button>
        <div>输入消息</div>
        <button aria-label="发送快捷消息" onClick={sendQuickReply}>
          <Icon name="more" />
        </button>
      </div>
    </>
  );
}

function ServiceMessage({
  text,
  outgoing = false,
}: {
  text: string;
  outgoing?: boolean;
}) {
  return (
    <div className={`service-message ${outgoing ? "outgoing" : ""}`}>
      {!outgoing && (
        <Avatar className="service-avatar" index={7} label="客服小七头像" />
      )}
      <div className="service-bubble">{text}</div>
      {outgoing && (
        <Avatar className="service-avatar user" index={-1} label="我的头像" />
      )}
    </div>
  );
}

function PacketBubble({
  title,
  description,
  claimed,
  onClick,
}: {
  title: string;
  description: string;
  claimed: boolean;
  onClick: () => void;
}) {
  return (
    <div className="packet-line">
      <span className="service-logo">曜</span>
      <div>
        <small>曜图 · 11:26</small>
        <button
          className={`red-packet ${claimed ? "claimed" : ""}`}
          onClick={onClick}
        >
          <span>
            <Icon name="gift" />
          </span>
          <b>{title}</b>
          <em>{description}</em>
          <footer>{claimed ? "已领取 · 查看详情" : "曜图奖励"}</footer>
        </button>
      </div>
    </div>
  );
}

const notificationThreads = {
  system: {
    title: "系统通知",
    preview: "维护安排与重要服务提醒",
    time: "今天 10:00",
    icon: "系",
    messages: [
      {
        time: "今天 10:00",
        title: "系统维护通知",
        content:
          "本周日 02:00 至 03:30 进行例行维护。维护期间，部分页面可能暂时无法访问。",
      },
      {
        time: "昨天 18:30",
        title: "积分到账提醒",
        content: "连续签到第 7 天奖励已到账，18 积分已存入你的积分账户。",
      },
    ],
  },
  activity: {
    title: "活动通知",
    preview: "周末任务与奖励活动已开启",
    time: "昨天 16:20",
    icon: "活",
    messages: [
      {
        time: "昨天 16:20",
        title: "周末活动开启",
        content: "完成指定任务可获得积分奖励，活动截止至周日 24:00。",
      },
      {
        time: "08-15 09:00",
        title: "好运奖励包",
        content: "本期专属福利已发放，前往聊天室即可查看与领取。",
      },
    ],
  },
} as const;

function NotificationList({
  onBack,
  onOpen,
}: {
  onBack: () => void;
  onOpen: (kind: "system" | "activity") => void;
}) {
  return (
    <section className="notification-page">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}>
          <Icon name="back" />
        </button>
        <b>通知</b>
        <button aria-label="更多通知操作">
          <Icon name="more" />
        </button>
      </header>
      <p className="notification-pinned-title">置顶通知</p>
      <div className="notification-thread-list">
        {(
          Object.entries(notificationThreads) as Array<
            ["system" | "activity", typeof notificationThreads.system]
          >
        ).map(([kind, thread]) => (
          <button
            key={kind}
            className="notification-thread"
            onClick={() => onOpen(kind)}
          >
            <span className={kind}>
              <i>置顶</i>
              {thread.icon}
            </span>
            <div>
              <b>{thread.title}</b>
              <p>{thread.preview}</p>
            </div>
            <time>{thread.time}</time>
            <Icon name="arrow" />
          </button>
        ))}
      </div>
    </section>
  );
}

function NotificationThread({
  kind,
  onBack,
}: {
  kind: "system" | "activity";
  onBack: () => void;
}) {
  const thread = notificationThreads[kind];
  const [selected, setSelected] = useState<
    (typeof thread.messages)[number] | null
  >(null);
  return (
    <section className="notification-page notification-thread-page">
      <header className="blue-header">
        <button aria-label="返回通知列表" onClick={onBack}>
          <Icon name="back" />
        </button>
        <b>{thread.title}</b>
        <button aria-label="更多通知操作">
          <Icon name="more" />
        </button>
      </header>
      <div className="notification-messages">
        <p>以下为近期开启的{thread.title}</p>
        {thread.messages.map((message) => (
          <button key={message.title} onClick={() => setSelected(message)}>
            <span className={kind}>{thread.icon}</span>
            <div>
              <time>{message.time}</time>
              <b>{message.title}</b>
              <p>{message.content}</p>
              <em>
                查看详情 <Icon name="arrow" />
              </em>
            </div>
          </button>
        ))}
      </div>
      {selected && (
        <ActionDialog
          title={selected.title}
          description={selected.content}
          onClose={() => setSelected(null)}
        />
      )}
    </section>
  );
}
