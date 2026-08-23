import { useCallback, useEffect, useRef, useState } from "react";
import { BRAND_NAME } from "../data/brand";
import { Icon } from "../components/Icon";
import { Avatar } from "../components/Avatar";
import { RedPacketDialog } from "../components/Dialogs";
import { ActionDialog } from "../components/Dialogs";
import type { ChatView } from "../router";
import { playNotificationSound } from "../utils/notificationAudio";
import { portalApi, type ActivityItem, type MemberNotification } from "../api/portal";
import { chatApi, type ChatMessage, type ChatPreview } from "../api/chat";
import { WS_EVENT, type WsEvent } from "../hooks/useWebSocket";

type Room = "group" | "service";

function chronologicalMessages(rows: ChatMessage[]) {
  return [...rows].sort((left, right) => {
    const leftTime = new Date(left.created_at).getTime();
    const rightTime = new Date(right.created_at).getTime();
    if (!Number.isFinite(leftTime) || !Number.isFinite(rightTime)) return left.id - right.id;
    return leftTime - rightTime || left.id - right.id;
  });
}

function mergeMessages(...groups: ChatMessage[][]) {
  const seen = new Set<number>();
  return chronologicalMessages(groups.flat().filter((item) => {
    if (seen.has(item.id)) return false;
    seen.add(item.id);
    return true;
  }));
}

function useChatHistory(room: Room) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const messagesRef = useRef<ChatMessage[]>([]);
  const roomRef = useRef(room);
  roomRef.current = room;

  const replaceMessages = useCallback((next: ChatMessage[]) => {
    const ordered = mergeMessages(next);
    messagesRef.current = ordered;
    setMessages(ordered);
  }, []);

  const loadInitial = useCallback(async () => {
    const requestRoom = room;
    try {
      const page = await chatApi.messages(room, 20);
      if (roomRef.current !== requestRoom) return;
      // A send may complete while the initial history request is still in
      // flight. Merge the snapshot with the locally appended response instead
      // of replacing it and making the member's own message disappear.
      replaceMessages([...messagesRef.current, ...page.items]);
      setHasMore(page.has_more);
    } catch {
      if (roomRef.current === requestRoom && messagesRef.current.length === 0) setHasMore(false);
    } finally {
      if (roomRef.current === requestRoom) setInitialLoaded(true);
    }
  }, [replaceMessages, room]);

  const loadNew = useCallback(async () => {
    const requestRoom = room;
    const newest = messagesRef.current.at(-1);
    if (!newest) {
      await loadInitial();
      return;
    }
    try {
      const page = await chatApi.messages(room, 50, { after_id: newest.id });
      if (roomRef.current === requestRoom && page.items.length) replaceMessages([...messagesRef.current, ...page.items]);
    } catch {
      // A later poll will retry without interrupting the current conversation.
    }
  }, [loadInitial, replaceMessages, room]);

  const loadOlder = useCallback(async () => {
    const requestRoom = room;
    const oldest = messagesRef.current[0];
    if (!oldest || historyLoading || !hasMore) return;
    setHistoryLoading(true);
    try {
      const page = await chatApi.messages(room, 20, { before_id: oldest.id });
      if (roomRef.current !== requestRoom) return;
      replaceMessages([...page.items, ...messagesRef.current]);
      setHasMore(page.has_more);
    } finally {
      setHistoryLoading(false);
    }
  }, [hasMore, historyLoading, replaceMessages, room]);

  const appendMessage = useCallback((message: ChatMessage) => {
    if (message.room_type !== room) return;
    replaceMessages([...messagesRef.current, message]);
  }, [replaceMessages, room]);

  useEffect(() => {
    // Group and service conversations are separate timelines. Clear the old
    // room immediately; late responses are ignored by the roomRef guards.
    messagesRef.current = [];
    setMessages([]);
    setHasMore(false);
    setHistoryLoading(false);
    setInitialLoaded(false);
    void loadInitial();
    const timer = window.setInterval(() => { void loadNew(); }, 15_000);
    return () => window.clearInterval(timer);
  }, [loadInitial, loadNew]);

  return { messages, hasMore, historyLoading, initialLoaded, loadOlder, loadNew, appendMessage };
}

function HistoryLoadButton({ hasMore, loading, onLoad }: { hasMore: boolean; loading: boolean; onLoad: () => void }) {
  if (!hasMore) return null;
  return <button type="button" className="chat-load-history" disabled={loading} onClick={onLoad}>{loading ? "正在加载…" : "查看更早消息"}</button>;
}

function todayAt(hour: number, minute: number) {
  const value = new Date();
  value.setHours(hour, minute, 0, 0);
  return value;
}

function messageTime(value?: string | Date) {
  const date = value instanceof Date ? value : value ? new Date(value) : new Date();
  if (Number.isNaN(date.getTime())) return "刚刚";
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function Chats({
  view,
  unreadCount,
  onMarkAllRead,
  onNavigate,
  onServiceBack,
  onRefreshUnread,
}: {
  view: ChatView;
  unreadCount: number;
  onMarkAllRead: () => void;
  onNavigate: (view: ChatView) => void;
  onServiceBack?: () => void;
  onRefreshUnread?: () => void;
}) {
  const allRead = unreadCount === 0;
  const [preview, setPreview] = useState<ChatPreview | null>(null);
  const [notifications, setNotifications] = useState<MemberNotification[]>([]);
  const [promotionTitles, setPromotionTitles] = useState<string[]>([]);
  const [servicePreview, setServicePreview] = useState("客服小七：已为您接入专属客服");

  useEffect(() => {
    const loadPreview = () => {
      void chatApi.preview().then(setPreview).catch(() => setPreview(null));
      void portalApi.notifications(20).then((page) => setNotifications(page.items)).catch(() => setNotifications([]));
      void portalApi.activities().then((items) => {
        setPromotionTitles(items.filter((item) => item.status === "active").map((item) => item.title));
      }).catch(() => setPromotionTitles([]));
      void chatApi.messages("service", 1).then((page) => {
        const last = page.items.at(-1);
        if (last) setServicePreview(`${last.mine ? "我" : last.nickname}：${last.content}`);
      }).catch(() => undefined);
    };
    loadPreview();
    const timer = window.setInterval(loadPreview, 8000);
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
      if (detail?.type === "chat_message") loadPreview();
    };
    window.addEventListener(WS_EVENT, onWs);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener(WS_EVENT, onWs);
    };
  }, [unreadCount]);

  const groupMessage = preview?.latest_message || "暂无群聊消息";
  const groupTime = preview?.latest_at
    ? new Date(preview.latest_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
    : "刚刚";
  const promotionNotices = notifications.filter((item) => item.category === "activity" && promotionTitles.includes(item.title));
  const noticeFor = (category: "system" | "activity" | "winning") => {
    const items = category === "activity" ? promotionNotices : notifications.filter((item) => item.category === category);
    return items.find((item) => !item.read) ?? items[0] ?? null;
  };
  const unreadFor = (category: "system" | "activity" | "winning") => (category === "activity" ? promotionNotices : notifications.filter((item) => item.category === category)).filter((item) => !item.read).length;
  const systemNotice = noticeFor("system");
  const winningNotice = noticeFor("winning");
  const activityNotice = noticeFor("activity");

  if (view === "system")
    return (
      <NotificationThread
        kind="system"
        onBack={() => onNavigate("list")}
        onRefreshUnread={onRefreshUnread}
      />
    );
  if (view === "winning")
    return (
      <NotificationThread
        kind="winning"
        onBack={() => onNavigate("list")}
        onRefreshUnread={onRefreshUnread}
      />
    );
  if (view === "activity")
    return <ActivityNoticePage onBack={() => onNavigate("list")} onRefreshUnread={onRefreshUnread} />;
  if (view === "group" || view === "service")
    return (
      <ChatRoom
        room={view}
        title={view === "group" ? "聊天室" : "在线客服"}
        onBack={view === "service" && onServiceBack ? onServiceBack : () => onNavigate("list")}
        onRefreshUnread={onRefreshUnread}
      />
    );
  const markNoticeRead = (notice: MemberNotification) => {
    if (notice.read) return;
    setNotifications((current) => current.map((item) => item.id === notice.id ? { ...item, read: true } : item));
    void portalApi.markRead(notice.id).finally(() => onRefreshUnread?.());
  };

  const openNoticeThread = (category: "system" | "activity" | "winning", notice: MemberNotification | null) => {
    if (notice) markNoticeRead(notice);
    onNavigate(category);
  };

  const markAll = () => {
    setNotifications((current) => current.map((item) => ({ ...item, read: true })));
    void Promise.resolve(onMarkAllRead()).finally(() => onRefreshUnread?.());
  };

  return (
    <section className="chat-list">
      <header className="blue-header">
        <b>聊天</b>
      </header>
      <div className="chat-subhead">
        <span>消息</span>
        <button onClick={markAll}>
          {allRead ? "已全部读" : "全部已读"}
        </button>
      </div>
      <ChatRow
        kind="notice"
        pinned
        name="系统通知"
        message={systemNotice?.content || systemNotice?.title || "暂无系统通知"}
        time={systemNotice?.created_at ? new Date(systemNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "通知"}
        badge={unreadFor("system") ? (unreadFor("system") > 9 ? "9+" : String(unreadFor("system"))) : undefined}
        onClick={() => openNoticeThread("system", systemNotice)}
      />
      <ChatRow
        kind="activity"
        name="活动通知"
        message={activityNotice?.content || activityNotice?.title || "优惠活动与专属礼遇会在这里展示"}
        time={activityNotice?.created_at ? new Date(activityNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "活动"}
        badge={unreadFor("activity") ? (unreadFor("activity") > 9 ? "9+" : String(unreadFor("activity"))) : undefined}
        onClick={() => openNoticeThread("activity", activityNotice)}
      />
      <ChatRow
        kind="winning"
        name="中奖通知"
        message={winningNotice?.content || winningNotice?.title || "开奖与派奖结果会在这里单独展示"}
        time={winningNotice?.created_at ? new Date(winningNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "开奖"}
        badge={unreadFor("winning") ? (unreadFor("winning") > 9 ? "9+" : String(unreadFor("winning"))) : undefined}
        onClick={() => openNoticeThread("winning", winningNotice)}
      />
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
  pinned,
  onClick,
}: {
  kind: Room | "notice" | "activity" | "winning";
  name: string;
  message: string;
  time: string;
  badge?: string;
  pinned?: boolean;
  onClick: () => void;
}) {
  return (
    <button aria-label={`${name}${badge ? `，${badge} 条未读消息` : ""}`} className={`chat-row ${pinned ? "chat-row-pinned" : ""}`} onClick={onClick}>
      <MessageLogo kind={kind} badge={badge} />
      <div>
        <b>{name}</b>
        <small>{message}</small>
      </div>
      <time>{time}</time>
    </button>
  );
}

function MessageLogo({ kind, badge }: { kind: Room | "notice" | "activity" | "winning"; badge?: string }) {
  const art = kind === "service" ? <><path d="M5 13a7 7 0 0 1 14 0v4" /><path d="M5 14H3v4h3m13-4h2v4h-3M16 20c-1 1-2.3 1.5-4 1.5" /><path d="M8 12h.01M16 12h.01" /></>
    : kind === "group" ? <><path d="M4 6.5h11v8H9l-4 3v-11Z" /><path d="M15 9.5h5v7l-3 2v-2h-2" /><path d="M7.5 10h4" /></>
      : kind === "notice" ? <><path d="m4 13 12-5v10L4 13Z" /><path d="M16 10.5 20 8v10l-4-2.5M7 15.5l1.5 3h3" /></>
        : kind === "winning" ? <><path d="M8 4h8v4a4 4 0 0 1-8 0V4Z" /><path d="M8 6H5v1a4 4 0 0 0 4 4m7-5h3v1a4 4 0 0 1-4 4M12 12v4m-3 3h6m-5-3h4" /></>
          : <><rect x="5" y="8" width="14" height="11" rx="2" /><path d="M3.5 8h17v4h-17zM12 8v11M12 8S8 7 8 4.8C8 3.4 10 3.6 12 8Zm0 0s4-1 4-3.2C16 3.4 14 3.6 12 8Z" /></>;
  return <span className={`message-logo message-logo-${kind}`} aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{art}</svg>{badge && <i>{badge}</i>}</span>;
}

function ChatRoom({
  room,
  title,
  onBack,
  onRefreshUnread,
}: {
  room: Room;
  title: string;
  onBack: () => void;
  onRefreshUnread?: () => void;
}) {
  const [opened, setOpened] = useState(false);
  const [packet, setPacket] = useState<"daily" | "lucky" | null>(null);
  const [redPacketId, setRedPacketId] = useState<number | null>(null);
  const [roomNotice, setRoomNotice] = useState("加载房间公告…");
  const [quickReplies, setQuickReplies] = useState<string[]>([]);
  const [chatPreview, setChatPreview] = useState<ChatPreview | null>(null);
  const [checkInDone, setCheckInDone] = useState(false);
  const [groupDraft, setGroupDraft] = useState("");
  const [groupSending, setGroupSending] = useState(false);
  const groupHistoryRef = useRef<HTMLDivElement>(null);
  const groupInitialScrollDone = useRef(false);
  const { messages, hasMore, historyLoading, initialLoaded, loadOlder, loadNew, appendMessage } = useChatHistory(room);

  useEffect(() => {
    groupInitialScrollDone.current = false;
  }, [room]);

  useEffect(() => {
    if (room !== "group" || !initialLoaded || groupInitialScrollDone.current) return;
    groupInitialScrollDone.current = true;
    const scrollToLatest = () => groupHistoryRef.current?.scrollTo({ top: groupHistoryRef.current.scrollHeight, behavior: "auto" });
    const frame = window.requestAnimationFrame(scrollToLatest);
    const timer = window.setTimeout(scrollToLatest, 120);
    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(timer);
    };
  }, [initialLoaded, room]);

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
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
      if (detail?.type === "chat_message" && detail.data.room_type === room) {
        void loadNew();
      }
    };
    window.addEventListener(WS_EVENT, onWs);
    return () => {
      window.removeEventListener(WS_EVENT, onWs);
    };
  }, [loadNew, room]);

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

  const sendGroupMessage = async () => {
    const text = groupDraft.trim();
    if (!text || groupSending) return;
    setGroupSending(true);
    try {
      const created = await chatApi.send(text, "group");
      setGroupDraft("");
      appendMessage(created);
      onRefreshUnread?.();
      playNotificationSound("message");
    } catch {
      setGroupDraft(text);
    } finally {
      setGroupSending(false);
    }
  };

  const groupTimeline = [
    ...messages.map((message) => ({ kind: "message" as const, at: new Date(message.created_at).getTime(), message })),
    {
      kind: "packet" as const,
      at: todayAt(11, 26).getTime(),
      packetKind: "daily" as const,
      title: "每日福利红包",
      description: checkInDone ? "今日福利已领取" : "点击查看每日福利",
      claimed: checkInDone,
    },
    ...(redPacketId ? [{
      kind: "packet" as const,
      at: todayAt(11, 28).getTime(),
      packetKind: "lucky" as const,
      title: opened ? "好运奖励包" : "打开好运奖励包",
      description: opened ? "红包奖励已领取" : "点击领取随机红包",
      claimed: opened,
    }] : []),
  ].sort((left, right) => left.at - right.at);

  return (
    <section className="chat-room">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}>
          <Icon name="back" />
        </button>
        <b>{title}</b>
      </header>
      {room === "service" ? (
        <ServiceConversation quickReplies={quickReplies} chatNickname={chatPreview?.chat_nickname} onRefreshUnread={onRefreshUnread} />
      ) : (
        <>
          <div className="room-notice">{roomNotice}</div>
          <div className="chat-history" ref={groupHistoryRef}>
            <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
            <p>今天 · 房间消息来自后端</p>
            {groupTimeline.map((item) => item.kind === "message" ? (
              <div className={`service-message chat-message-row ${item.message.mine ? "outgoing" : ""}`} key={`message-${item.message.id}`}>
                {!item.message.mine && <Avatar className="service-avatar" index={Number(item.message.user_id) % 20} label={`${item.message.nickname}头像`} />}
                <div className="service-bubble">
                  {!item.message.mine && <small>{item.message.nickname}</small>}
                  <span>{item.message.content}</span>
                  <time className="message-bubble-time">{messageTime(item.message.created_at)}</time>
                </div>
                {item.message.mine && <Avatar className="service-avatar user" index={-1} label="我的头像" />}
              </div>
            ) : <PacketBubble key={item.packetKind} title={item.title} description={item.description} claimed={item.claimed} time={new Date(item.at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })} onClick={() => setPacket(item.packetKind)} />)}
          </div>
          <div className="chat-input">
            <button aria-label="添加内容" disabled><Icon name="plus" /></button>
            <input aria-label="输入聊天室消息" onChange={(event) => setGroupDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void sendGroupMessage() }} placeholder="输入消息" value={groupDraft} />
            <button aria-label="发送聊天室消息" className="service-send" disabled={!groupDraft.trim() || groupSending} onClick={() => void sendGroupMessage()}>
              <Icon name="arrow" />
            </button>
          </div>
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
  const label = chatNickname || "客服小七";
  const historyRef = useRef<HTMLDivElement>(null);
  const initialScrollDone = useRef(false);
  const { messages, hasMore, historyLoading, initialLoaded, loadOlder, loadNew, appendMessage } = useChatHistory("service");

  useEffect(() => {
    if (!initialLoaded || initialScrollDone.current) return;
    initialScrollDone.current = true;
    const scrollToLatest = () => historyRef.current?.scrollTo({ top: historyRef.current.scrollHeight, behavior: "auto" });
    const frame = window.requestAnimationFrame(scrollToLatest);
    const timer = window.setTimeout(scrollToLatest, 120);
    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(timer);
    };
  }, [initialLoaded]);

  // The periodic request in useChatHistory is only a recovery path. Replies
  // from the admin console arrive through the authenticated WebSocket and are
  // pulled into the timeline immediately.
  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
      if (detail?.type === "chat_message" && detail.data.room_type === "service") {
        void loadNew();
      }
    };
    window.addEventListener(WS_EVENT, onWs);
    return () => window.removeEventListener(WS_EVENT, onWs);
  }, [loadNew]);

  const sendMessage = async (value = draft) => {
    const text = value.trim();
    if (!text) return;
    try {
      const created = await chatApi.send(text, "service");
      setDraft("");
      appendMessage(created);
      onRefreshUnread?.();
      playNotificationSound("message");
    } catch {
      setDraft(text);
    }
  };
  return (
    <>
      <div className="room-notice">{label}在线 · 消息已同步后端</div>
      <div className="chat-history service-history" ref={historyRef}>
        <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
        <p className="service-time">今天</p>
        <ServiceMessage text={`您好，我是${label}，很高兴为您服务。请问有什么可以帮您？`} time={messageTime()} />
        {messages.map((message) => message.mine
          ? <ServiceMessage key={message.id} outgoing text={message.content} time={messageTime(message.created_at)} />
          : <ServiceMessage key={message.id} text={message.content} time={messageTime(message.created_at)} />)}
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
  time,
  outgoing = false,
}: {
  text: string;
  time: string;
  outgoing?: boolean;
}) {
  return (
    <div className={`service-message chat-message-row ${outgoing ? "outgoing" : ""}`}>
      {!outgoing && (
        <Avatar className="service-avatar" index={7} label="客服小七头像" />
      )}
      <div className="service-bubble"><span>{text}</span><time className="message-bubble-time">{time}</time></div>
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
  time,
  onClick,
}: {
  title: string;
  description: string;
  claimed: boolean;
  time: string;
  onClick: () => void;
}) {
  return (
    <div className="packet-line">
      <span className="service-logo">曜</span>
      <div>
        <small>{BRAND_NAME} · {time}</small>
        <button
          className={`red-packet ${claimed ? "claimed" : ""}`}
          onClick={onClick}
        >
          <span>
            <Icon name="gift" />
          </span>
          <b>{title}</b>
          <em>{description}</em>
          <footer>{claimed ? "已领取 · 查看详情" : `${BRAND_NAME}奖励`}</footer>
        </button>
      </div>
    </div>
  );
}

const notificationThreads = {
  system: { title: "系统通知", preview: "维护安排与重要服务提醒", time: "最新", icon: "系" },
  winning: { title: "中奖通知", preview: "开奖与派奖结果", time: "最新", icon: "奖" },
  activity: { title: "活动通知", preview: "签到奖励与红包活动", time: "最新", icon: "活" },
} as const;

function NotificationThread({
  kind,
  onBack,
  onRefreshUnread,
}: {
  kind: "system" | "winning";
  onBack: () => void;
  onRefreshUnread?: () => void;
}) {
  const thread = notificationThreads[kind];
  const isWinning = kind === "winning";
  const [items, setItems] = useState<MemberNotification[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [selected, setSelected] = useState<MemberNotification | null>(null);

  useEffect(() => {
    void portalApi.notifications(20, { category: kind }).then((page) => {
      setItems(page.items);
      setHasMore(page.has_more);
    }).catch(() => {
      setItems([]);
      setHasMore(false);
    });
  }, [kind]);

  const loadOlder = async () => {
    const beforeID = items.at(-1)?.id;
    if (!beforeID || !hasMore || historyLoading) return;
    setHistoryLoading(true);
    try {
      const page = await portalApi.notifications(20, { category: kind, before_id: beforeID });
      setItems((current) => [...current, ...page.items.filter((item) => !current.some((existing) => existing.id === item.id))]);
      setHasMore(page.has_more);
    } finally {
      setHistoryLoading(false);
    }
  };

  const openItem = async (item: MemberNotification) => {
    setSelected(item);
    if (!item.read) {
      // Optimistically clear the marker as soon as the user opens it.
      setItems((current) => current.map((row) => row.id === item.id ? { ...row, read: true } : row));
      await portalApi.markRead(item.id).catch(() => undefined);
      onRefreshUnread?.();
    }
  };

  return (
    <section className="notification-page system-notice-page">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button>
        <b>{thread.title}</b>
        <span aria-hidden="true" />
      </header>
      <div className="system-notice-intro">
        <span>{isWinning ? "开奖中心" : "系统公告"}</span>
        <h1>{isWinning ? "开奖与派奖结果" : "重要信息与服务提醒"}</h1>
        <p>{isWinning ? "您的中奖、未中奖及派奖结果将在这里单独展示。" : "重要公告将以独立卡片展示，请留意最新内容。"}</p>
      </div>
      <div className="system-notice-list">
        {items.length === 0 && <p className="empty-notice">{isWinning ? "暂无中奖通知" : "暂无系统公告"}</p>}
        {items.map((message) => (
          <button className={`system-notice-card ${!message.read ? "is-unread" : ""}`} key={message.id} onClick={() => void openItem(message)}>
            <div className="system-notice-card-top">
              <span>{isWinning ? "开奖结果" : "系统公告"}</span>
              <time>{new Date(message.created_at).toLocaleString("zh-CN")}</time>
            </div>
            <div>
              <b>{message.title}{!message.read && <i className="notification-unread-dot" aria-label="未读" />}</b>
              <p>{message.content}</p>
              <em>阅读全文 <Icon name="arrow" /></em>
            </div>
          </button>
        ))}
        <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
      </div>
      {selected && (
        <ActionDialog title={selected.title} description={selected.content} onClose={() => setSelected(null)} />
      )}
    </section>
  );
}

type ActivityNotice = {
  id: string;
  title: string;
  subtitle: string;
  cover?: string;
  notification?: MemberNotification;
  activity?: ActivityItem;
};

function ActivityNoticePage({ onBack, onRefreshUnread }: { onBack: () => void; onRefreshUnread?: () => void }) {
  const [notices, setNotices] = useState<MemberNotification[]>([]);
  const [activities, setActivities] = useState<ActivityItem[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [selected, setSelected] = useState<ActivityNotice | null>(null);

  useEffect(() => {
    void Promise.all([portalApi.notifications(20, { category: "activity" }), portalApi.activities()]).then(([notificationPage, activityRows]) => {
      setNotices(notificationPage.items);
      setHasMore(notificationPage.has_more);
      setActivities(activityRows.filter((row) => row.status === "active"));
    }).catch(() => {
      setNotices([]);
      setHasMore(false);
      setActivities([]);
    });
  }, []);

  const loadOlder = async () => {
    const beforeID = notices.at(-1)?.id;
    if (!beforeID || !hasMore || historyLoading) return;
    setHistoryLoading(true);
    try {
      const page = await portalApi.notifications(20, { category: "activity", before_id: beforeID });
      setNotices((current) => [...current, ...page.items.filter((item) => !current.some((existing) => existing.id === item.id))]);
      setHasMore(page.has_more);
    } finally {
      setHistoryLoading(false);
    }
  };

  const cards: ActivityNotice[] = [
    ...activities.map((activity) => ({
      id: `activity-${activity.id}`,
      title: activity.title,
      subtitle: activity.subtitle || "参与活动，领取专属福利",
      cover: activity.cover,
      activity,
      notification: notices.find((notice) => notice.title === activity.title),
    })),
  ];

  const openCard = async (card: ActivityNotice) => {
    setSelected(card);
    if (card.notification && !card.notification.read) {
      setNotices((current) => current.map((item) => item.id === card.notification?.id ? { ...item, read: true } : item));
      await portalApi.markRead(card.notification.id).catch(() => undefined);
      onRefreshUnread?.();
    }
  };

  return (
    <section className="notification-page activity-notice-page">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button>
        <b>优惠活动</b>
        <span aria-hidden="true" />
      </header>
      <div className="activity-notice-list">
        {cards.length === 0 && <p className="empty-notice">暂无进行中的活动</p>}
        {cards.map((card, index) => (
          <button className={`activity-notice-card card-tone-${index % 4}`} key={card.id} onClick={() => void openCard(card)}>
            {card.cover ? <img alt="" src={card.cover} /> : <span className="activity-notice-art" aria-hidden="true"><i>{index % 2 ? "福利" : "好运"}</i><b>{index % 2 ? "专属礼遇" : "幸运相伴"}</b></span>}
            <div className="activity-notice-shade" />
            <div className="activity-notice-copy">
              <small>{card.notification && !card.notification.read ? "新活动" : "正在进行"}</small>
              <b>{card.title}</b>
              <p>{card.subtitle}</p>
              <em>查看活动 <Icon name="arrow" /></em>
            </div>
          </button>
        ))}
        <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
      </div>
      {selected && (
        <ActionDialog
          title={selected.title}
          description={selected.notification?.content || selected.activity?.subtitle || selected.subtitle}
          onClose={() => setSelected(null)}
        />
      )}
    </section>
  );
}
