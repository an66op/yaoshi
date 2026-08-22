import { useEffect, useState } from "react";
import { Icon } from "../components/Icon";
import { Avatar } from "../components/Avatar";
import { RedPacketDialog } from "../components/Dialogs";
import { ActionDialog } from "../components/Dialogs";
import type { ChatView } from "../router";
import { playNotificationSound } from "../utils/notificationAudio";
import { portalApi, type MemberNotification } from "../api/portal";
import { chatApi, type ChatMessage, type ChatPreview } from "../api/chat";
import { WS_EVENT, type WsEvent } from "../hooks/useWebSocket";

type Room = "group" | "service";

export function Chats({
  view,
  unreadCount,
  onMarkAllRead,
  onNavigate,
  onRefreshUnread,
}: {
  view: ChatView;
  unreadCount: number;
  onMarkAllRead: () => void;
  onNavigate: (view: ChatView) => void;
  onRefreshUnread?: () => void;
}) {
  const allRead = unreadCount === 0;
  const [preview, setPreview] = useState<ChatPreview | null>(null);
  const [latestNotice, setLatestNotice] = useState<MemberNotification | null>(null);
  const [servicePreview, setServicePreview] = useState("客服小七：已为您接入专属客服");

  useEffect(() => {
    const loadPreview = () => {
      void chatApi.preview().then(setPreview).catch(() => setPreview(null));
      void portalApi.notifications(1).then((rows) => setLatestNotice(rows[0] ?? null)).catch(() => setLatestNotice(null));
      void chatApi.messages("service", 1).then((rows) => {
        const last = rows.at(-1);
        if (last) setServicePreview(`${last.mine ? "我" : last.nickname}：${last.content}`);
      }).catch(() => undefined);
    };
    loadPreview();
    const timer = window.setInterval(loadPreview, 8000);
    return () => window.clearInterval(timer);
  }, [unreadCount]);

  const groupMessage = preview?.latest_message || latestNotice?.title || (allRead ? "暂无未读消息" : "[系统] 您有新的通知");
  const groupTime = preview?.latest_at
    ? new Date(preview.latest_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
    : latestNotice?.created_at
      ? new Date(latestNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
      : "刚刚";
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
        onRefreshUnread={onRefreshUnread}
      />
    );
  if (view === "group" || view === "service")
    return (
      <ChatRoom
        room={view}
        title={view === "group" ? "聊天室" : "在线客服"}
        onBack={() => onNavigate("list")}
        onOpenNotices={() => onNavigate("notices")}
        onRefreshUnread={onRefreshUnread}
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
        message={servicePreview}
        time="刚刚"
        onClick={() => onNavigate("service")}
      />
      <ChatRow
        kind="group"
        name="聊天室"
        message={groupMessage}
        time={groupTime}
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
  onRefreshUnread,
}: {
  room: Room;
  title: string;
  onBack: () => void;
  onOpenNotices: () => void;
  onRefreshUnread?: () => void;
}) {
  const [opened, setOpened] = useState(false);
  const [packet, setPacket] = useState<"daily" | "lucky" | null>(null);
  const [redPacketId, setRedPacketId] = useState<number | null>(null);
  const [roomNotice, setRoomNotice] = useState("加载房间公告…");
  const [quickReplies, setQuickReplies] = useState<string[]>([]);
  const [chatPreview, setChatPreview] = useState<ChatPreview | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [sendError, setSendError] = useState("");
  const [checkInDone, setCheckInDone] = useState(false);

  const loadMessages = () => {
    void chatApi.messages(room, 50).then(setMessages).catch(() => setMessages([]));
  };

  useEffect(() => {
    void portalApi.roomSettings().then((settings) => {
      if (settings.room_notice) setRoomNotice(settings.room_notice);
      const replies = settings.quick_replies;
      if (Array.isArray(replies)) {
        setQuickReplies(replies.map((item) => {
          if (typeof item === 'string') return item
          if (item && typeof item === 'object') {
            const row = item as { title?: string; content?: string }
            return row.content?.trim() || row.title?.trim() || ''
          }
          return ''
        }).filter(Boolean))
      }
    }).catch(() => undefined);
    void chatApi.preview().then(setChatPreview).catch(() => setChatPreview(null));
    void portalApi.activities("redpacket").then((items) => {
      const active = items.find((item) => item.status === "active");
      if (active) setRedPacketId(active.id);
    }).catch(() => undefined);
    void portalApi.activities("checkin").then(async (items) => {
      const active = items.find((item) => item.status === "active");
      if (!active) return;
      try {
        const status = await portalApi.activityStatus(active.id);
        setCheckInDone(status.checked_in);
      } catch {
        setCheckInDone(false);
      }
    }).catch(() => undefined);
  }, [room]);

  useEffect(() => {
    loadMessages();
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
      if (detail?.type === "chat_message" && detail.data.room_type === room) {
        loadMessages();
      }
    };
    window.addEventListener(WS_EVENT, onWs);
    const timer = window.setInterval(loadMessages, 8000);
    return () => {
      window.removeEventListener(WS_EVENT, onWs);
      window.clearInterval(timer);
    };
  }, [room]);

  const sendGroupMessage = async (value = draft) => {
    const text = value.trim();
    if (!text) return;
    setSendError("");
    try {
      await chatApi.send(text, room);
      setDraft("");
      loadMessages();
      playNotificationSound("message");
    } catch (reason) {
      setSendError(reason instanceof Error ? reason.message : "发送失败");
    }
  };

  const claimRedPacket = async () => {
    if (!redPacketId) return;
    try {
      await portalApi.claimRedPacket(redPacketId);
      setOpened(true);
      onRefreshUnread?.();
      playNotificationSound("message");
    } catch {
      setOpened(true);
    }
  };

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
        <ServiceConversation quickReplies={quickReplies} chatNickname={chatPreview?.chat_nickname} onRefreshUnread={onRefreshUnread} />
      ) : (
        <>
          <div className="room-notice">{roomNotice}</div>
          <div className="chat-history">
            <p>今天 · 房间消息来自后端</p>
            {messages.map((message) => (
              <div className={`service-message ${message.mine ? "outgoing" : ""}`} key={message.id}>
                {!message.mine && <Avatar className="service-avatar" index={Number(message.user_id) % 20} label={`${message.nickname}头像`} />}
                <div className="service-bubble"><small>{message.nickname}</small>{message.content}</div>
                {message.mine && <Avatar className="service-avatar user" index={-1} label="我的头像" />}
              </div>
            ))}
            <PacketBubble
              title="今日签到奖励"
              description={checkInDone ? "今日已签到" : "前往彩种页签到领取"}
              claimed={checkInDone}
              onClick={() => setPacket("daily")}
            />
            {redPacketId && (
              <PacketBubble
                title={opened ? "好运奖励包" : "打开好运奖励包"}
                description={opened ? "红包奖励已领取" : "点击领取随机红包"}
                claimed={opened}
                onClick={() => setPacket("lucky")}
              />
            )}
          </div>
          <div className="chat-input">
            <button aria-label="添加内容" disabled><Icon name="plus" /></button>
            {chatPreview?.can_chat ? (
              <input aria-label="输入聊天消息" onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void sendGroupMessage() }} placeholder="输入消息" value={draft} />
            ) : (
              <div>余额需达到 {chatPreview?.min_chat_score?.toFixed(0) ?? 0} 元才可发言</div>
            )}
            <button aria-label="发送消息" className="service-send" disabled={!chatPreview?.can_chat || !draft.trim()} onClick={() => void sendGroupMessage()}>
              <Icon name="arrow" />
            </button>
          </div>
          {sendError && <p className="room-notice">{sendError}</p>}
          {packet && (
            <RedPacketDialog
              type={packet}
              claimed={opened || packet === "daily"}
              onOpen={() => void claimRedPacket()}
              onClose={() => setPacket(null)}
            />
          )}
        </>
      )}
    </section>
  );
}

function ServiceConversation({ quickReplies, chatNickname, onRefreshUnread }: { quickReplies: string[]; chatNickname?: string; onRefreshUnread?: () => void }) {
  const [draft, setDraft] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const label = chatNickname || "客服小七";

  const loadMessages = () => {
    void chatApi.messages("service", 50).then(setMessages).catch(() => setMessages([]));
  };

  useEffect(() => {
    loadMessages();
    const timer = window.setInterval(loadMessages, 8000);
    return () => window.clearInterval(timer);
  }, []);

  const sendMessage = async (value = draft) => {
    const text = value.trim();
    if (!text) return;
    try {
      await chatApi.send(text, "service");
      setDraft("");
      loadMessages();
      onRefreshUnread?.();
      playNotificationSound("message");
    } catch {
      setDraft(text);
    }
  };
  return (
    <>
      <div className="room-notice">{label}在线 · 消息已同步后端</div>
      <div className="chat-history service-history">
        <p className="service-time">今天</p>
        <ServiceMessage text={`您好，我是${label}，很高兴为您服务。请问有什么可以帮您？`} />
        {messages.map((message) => (
          <div key={message.id}>
            {message.mine ? <ServiceMessage outgoing text={message.content} /> : <ServiceMessage text={message.content} />}
          </div>
        ))}
        {messages.length === 0 && quickReplies.length > 0 && (
          <div className="service-replies">
            {quickReplies.slice(0, 3).map((text) => (
              <button key={text} onClick={() => void sendMessage(text)}>{text}</button>
            ))}
          </div>
        )}
      </div>
      <div className="chat-input">
        <button aria-label="添加内容" disabled><Icon name="plus" /></button>
        <input aria-label="输入客服消息" onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void sendMessage() }} placeholder="输入消息" value={draft} />
        <button aria-label="发送消息" className="service-send" disabled={!draft.trim()} onClick={() => void sendMessage()}>
          <Icon name="arrow" />
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
  system: { title: "系统通知", preview: "维护安排与重要服务提醒", time: "最新", icon: "系" },
  activity: { title: "活动通知", preview: "签到奖励与红包活动", time: "最新", icon: "活" },
} as const;

function NotificationList({
  onBack,
  onOpen,
}: {
  onBack: () => void;
  onOpen: (kind: "system" | "activity") => void;
}) {
  const [items, setItems] = useState<MemberNotification[]>([]);

  useEffect(() => {
    const load = () => {
      void portalApi.notifications(50).then(setItems).catch(() => setItems([]));
    };
    load();
    const timer = window.setInterval(load, 12_000);
    return () => window.clearInterval(timer);
  }, []);

  const previewFor = (kind: "system" | "activity") => {
    const rows = items.filter((row) => row.category === kind);
    return {
      count: rows.filter((row) => !row.read).length,
      preview: rows[0]?.title ?? notificationThreads[kind].preview,
      time: rows[0]?.created_at
        ? new Date(rows[0].created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
        : notificationThreads[kind].time,
    };
  };

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
      <p className="notification-pinned-title">通知分类</p>
      <div className="notification-thread-list">
        {(Object.entries(notificationThreads) as Array<["system" | "activity", typeof notificationThreads.system]>).map(([kind, thread]) => {
          const meta = previewFor(kind);
          return (
          <button key={kind} className="notification-thread" onClick={() => onOpen(kind)}>
            <span className={kind}><i>分类</i>{thread.icon}{meta.count > 0 && <b>{meta.count}</b>}</span>
            <div><b>{thread.title}</b><p>{meta.preview}</p></div>
            <time>{meta.time}</time>
            <Icon name="arrow" />
          </button>
        )})}
      </div>
    </section>
  );
}

function NotificationThread({
  kind,
  onBack,
  onRefreshUnread,
}: {
  kind: "system" | "activity";
  onBack: () => void;
  onRefreshUnread?: () => void;
}) {
  const thread = notificationThreads[kind];
  const [items, setItems] = useState<MemberNotification[]>([]);
  const [selected, setSelected] = useState<MemberNotification | null>(null);

  useEffect(() => {
    void portalApi.notifications(50).then((rows) => {
      setItems(rows.filter((row) => row.category === kind));
    }).catch(() => setItems([]));
  }, [kind]);

  const openItem = async (item: MemberNotification) => {
    setSelected(item);
    if (!item.read) {
      await portalApi.markRead(item.id).catch(() => undefined);
      onRefreshUnread?.();
      setItems((current) => current.map((row) => row.id === item.id ? { ...row, read: true } : row));
    }
  };

  return (
    <section className="notification-page notification-thread-page">
      <header className="blue-header">
        <button aria-label="返回通知列表" onClick={onBack}><Icon name="back" /></button>
        <b>{thread.title}</b>
        <button aria-label="更多通知操作"><Icon name="more" /></button>
      </header>
      <div className="notification-messages">
        <p>{items.length ? `共 ${items.length} 条${thread.title}` : "暂无通知"}</p>
        {items.map((message) => (
          <button key={message.id} onClick={() => void openItem(message)}>
            <span className={kind}>{thread.icon}</span>
            <div>
              <time>{new Date(message.created_at).toLocaleString("zh-CN")}</time>
              <b>{message.title}{!message.read && " · 未读"}</b>
              <p>{message.content}</p>
              <em>查看详情 <Icon name="arrow" /></em>
            </div>
          </button>
        ))}
      </div>
      {selected && (
        <ActionDialog title={selected.title} description={selected.content} onClose={() => setSelected(null)} />
      )}
    </section>
  );
}
