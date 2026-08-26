import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
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
import { lotteryGameLogo } from "../hooks/useLotteryGames";
import type { Game } from "../types";
import { PlanDetail, PlanLobby } from "./PlanGroup";

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

function useChatHistory(room: Room, gameId = room === "service" ? "service" : "lobby") {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const messagesRef = useRef<ChatMessage[]>([]);
  const conversationRef = useRef(`${room}:${gameId}`);
	conversationRef.current = `${room}:${gameId}`;

  const replaceMessages = useCallback((next: ChatMessage[]) => {
    const ordered = mergeMessages(next);
    messagesRef.current = ordered;
    setMessages(ordered);
  }, []);

  const loadInitial = useCallback(async () => {
	const requestConversation = `${room}:${gameId}`;
    try {
	  const page = await chatApi.messages(room, gameId, 20);
	  if (conversationRef.current !== requestConversation) return;
      // A send may complete while the initial history request is still in
      // flight. Merge the snapshot with the locally appended response instead
      // of replacing it and making the member's own message disappear.
      replaceMessages([...messagesRef.current, ...page.items]);
      setHasMore(page.has_more);
    } catch {
	  if (conversationRef.current === requestConversation && messagesRef.current.length === 0) setHasMore(false);
    } finally {
	  if (conversationRef.current === requestConversation) setInitialLoaded(true);
    }
  }, [gameId, replaceMessages, room]);

  const loadNew = useCallback(async () => {
	const requestConversation = `${room}:${gameId}`;
    const newest = messagesRef.current.at(-1);
    if (!newest) {
      await loadInitial();
      return;
    }
    try {
	  const page = await chatApi.messages(room, gameId, 50, { after_id: newest.id });
	  if (conversationRef.current === requestConversation && page.items.length) replaceMessages([...messagesRef.current, ...page.items]);
    } catch {
      // A later poll will retry without interrupting the current conversation.
    }
  }, [gameId, loadInitial, replaceMessages, room]);

  const loadOlder = useCallback(async () => {
	const requestConversation = `${room}:${gameId}`;
    const oldest = messagesRef.current[0];
    if (!oldest || historyLoading || !hasMore) return;
    setHistoryLoading(true);
    try {
	  const page = await chatApi.messages(room, gameId, 20, { before_id: oldest.id });
	  if (conversationRef.current !== requestConversation) return;
      replaceMessages([...page.items, ...messagesRef.current]);
      setHasMore(page.has_more);
    } finally {
      setHistoryLoading(false);
    }
  }, [gameId, hasMore, historyLoading, replaceMessages, room]);

  const appendMessage = useCallback((message: ChatMessage) => {
	if (message.room_type !== room || message.game_id !== gameId) return;
    replaceMessages([...messagesRef.current, message]);
  }, [gameId, replaceMessages, room]);

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
  games,
  planGameId,
  onOpenPlanGame,
}: {
  view: ChatView;
  unreadCount: number;
  onMarkAllRead: () => void;
  onNavigate: (view: ChatView) => void;
  onServiceBack?: () => void;
  onRefreshUnread?: () => void;
  games: Game[];
  planGameId?: string;
  onOpenPlanGame: (gameId: string) => void;
}) {
  const [preview, setPreview] = useState<ChatPreview | null>(null);
  const [notifications, setNotifications] = useState<MemberNotification[]>([]);
  const [promotionTitles, setPromotionTitles] = useState<string[]>([]);
  const [servicePreview, setServicePreview] = useState("客服小七：已为您接入专属客服");
  const [roomLogo, setRoomLogo] = useState("");
  const [pinnedRows, setPinnedRows] = useState<string[]>(["service", "group"]);
  const [hiddenRows, setHiddenRows] = useState<string[]>(["winning"]);

  useEffect(() => {
    void portalApi.roomSettings().then((settings) => {
      setRoomLogo(settings.room_logo || "");
      const pinned = settings.game?.message_pinned_rows;
      const hidden = settings.game?.message_hidden_rows;
      if (Array.isArray(pinned)) setPinnedRows(pinned.filter((item): item is string => typeof item === "string"));
      if (Array.isArray(hidden)) setHiddenRows(hidden.filter((item): item is string => typeof item === "string"));
    }).catch(() => undefined);
  }, []);

  useEffect(() => {
    const loadPreview = () => {
      void chatApi.preview().then(setPreview).catch(() => setPreview(null));
      void portalApi.notifications(20).then((page) => setNotifications(page.items)).catch(() => setNotifications([]));
      void portalApi.activities().then((items) => {
        setPromotionTitles(items.filter((item) => item.status === "active").map((item) => item.title));
      }).catch(() => setPromotionTitles([]));
	  void chatApi.messages("service", "service", 1).then((page) => {
        const last = page.items.at(-1);
        if (last) setServicePreview(`${last.mine ? "我" : "客服"}：${last.content}`);
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
    const items = category === "activity"
      ? promotionNotices
      : notifications.filter((item) => item.category === category && (category !== "system" || !resultNotificationTitles.has(item.title)));
    return items.find((item) => !item.read) ?? items[0] ?? null;
  };
  const unreadFor = (category: "system" | "activity" | "winning") => (category === "activity" ? promotionNotices : notifications.filter((item) => item.category === category && (category !== "system" || !resultNotificationTitles.has(item.title)))).filter((item) => !item.read).length;
  const systemNotice = noticeFor("system");
  const activityNotice = noticeFor("activity");
  const allRead = unreadFor("system") + unreadFor("activity") + unreadFor("winning") === 0;

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
  if (view === "plans")
    return <PlanLobby games={games} onBack={() => onNavigate("list")} onSelect={onOpenPlanGame} />;
  if (view === "plan")
    return <PlanDetail games={games} gameId={planGameId} onBack={() => onNavigate("plans")} />;
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

  const pinned = new Set(pinnedRows);
  const messageRows = [
    {
      key: "system", kind: "notice" as const, name: "系统通知",
      message: systemNotice?.content || systemNotice?.title || "暂无系统通知",
      time: systemNotice?.created_at ? new Date(systemNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "通知",
      badge: unreadFor("system") ? (unreadFor("system") > 9 ? "9+" : String(unreadFor("system"))) : undefined,
      onClick: () => openNoticeThread("system", systemNotice),
    },
    {
      key: "activity", kind: "activity" as const, name: "活动通知",
      message: activityNotice?.content || activityNotice?.title || "优惠活动与专属礼遇会在这里展示",
      time: activityNotice?.created_at ? new Date(activityNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "活动",
      badge: unreadFor("activity") ? (unreadFor("activity") > 9 ? "9+" : String(unreadFor("activity"))) : undefined,
      onClick: () => openNoticeThread("activity", activityNotice),
    },
    { key: "service", kind: "service" as const, name: "在线客服", message: servicePreview, time: "刚刚", badge: undefined, onClick: () => onNavigate("service") },
    { key: "group", kind: "group" as const, name: "聊天室", message: groupMessage, time: groupTime, badge: undefined, onClick: () => onNavigate("group") },
    { key: "plan", kind: "plan" as const, name: "计划群", message: "大师推荐 · 3 个彩票人工计划", time: "每期更新", badge: undefined, onClick: () => onNavigate("plans") },
  ].filter((row) => !hiddenRows.includes(row.key)).sort((left, right) => {
    const leftPinned = pinned.has(left.key);
    const rightPinned = pinned.has(right.key);
    if (leftPinned !== rightPinned) return leftPinned ? -1 : 1;
    if (leftPinned && rightPinned) return pinnedRows.indexOf(left.key) - pinnedRows.indexOf(right.key);
    return 0;
  });

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
      {messageRows.map((row) => <ChatRow key={row.key} kind={row.kind} image={row.key === "group" ? roomLogo : undefined} pinned={pinned.has(row.key)} name={row.name} message={row.message} time={row.time} badge={row.badge} onClick={row.onClick} />)}
    </section>
  );
}

function ChatRow({
  kind,
  name,
  message,
  time,
  badge,
  image,
  pinned,
  onClick,
}: {
  kind: Room | "notice" | "activity" | "winning" | "plan";
  name: string;
  message: string;
  time: string;
  badge?: string;
  image?: string;
  pinned?: boolean;
  onClick: () => void;
}) {
  return (
    <button aria-label={`${name}${badge ? `，${badge} 条未读消息` : ""}`} className={`chat-row ${pinned ? "chat-row-pinned" : ""}`} onClick={onClick}>
      <MessageLogo kind={kind} badge={badge} image={image} />
      <div>
        <b>{name}</b>
        <small>{message}</small>
      </div>
      <time>{time}</time>
    </button>
  );
}

function MessageLogo({ kind, badge, image }: { kind: Room | "notice" | "activity" | "winning" | "plan"; badge?: string; image?: string }) {
  const art = kind === "service" ? <><path d="M5 13a7 7 0 0 1 14 0v4" /><path d="M5 14H3v4h3m13-4h2v4h-3M16 20c-1 1-2.3 1.5-4 1.5" /><path d="M8 12h.01M16 12h.01" /></>
    : kind === "group" ? <><path d="M4 6.5h11v8H9l-4 3v-11Z" /><path d="M15 9.5h5v7l-3 2v-2h-2" /><path d="M7.5 10h4" /></>
      : kind === "notice" ? <><path d="m4 13 12-5v10L4 13Z" /><path d="M16 10.5 20 8v10l-4-2.5M7 15.5l1.5 3h3" /></>
        : kind === "winning" ? <><path d="M8 4h8v4a4 4 0 0 1-8 0V4Z" /><path d="M8 6H5v1a4 4 0 0 0 4 4m7-5h3v1a4 4 0 0 1-4 4M12 12v4m-3 3h6m-5-3h4" /></>
            : kind === "plan" ? <><path d="M4 18V9m5 9V5m5 13v-6m5 6V3" /><path d="m3 7 5-3 5 5 7-7" /></>
            : <><rect x="5" y="8" width="14" height="11" rx="2" /><path d="M3.5 8h17v4h-17zM12 8v11M12 8S8 7 8 4.8C8 3.4 10 3.6 12 8Zm0 0s4-1 4-3.2C16 3.4 14 3.6 12 8Z" /></>;
  return <span className={`message-logo message-logo-${kind} ${image ? "has-room-logo" : ""}`} aria-hidden="true">{image ? <img alt="" src={image} /> : <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{art}</svg>}{badge && <i>{badge}</i>}</span>;
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
  const [packet, setPacket] = useState<{ messageId: number; claimed: boolean; greeting: string; cover: string; minTurnover: number } | null>(null);
  const [claimedPacketIDs, setClaimedPacketIDs] = useState<Set<number>>(() => new Set());
  const [packetReward, setPacketReward] = useState<number | null>(null);
  const [packetOpening, setPacketOpening] = useState(false);
  const [packetError, setPacketError] = useState("");
  const [roomNotice, setRoomNotice] = useState("加载房间公告…");
  const [quickReplies, setQuickReplies] = useState<string[]>([]);
  const [groupDraft, setGroupDraft] = useState("");
  const [groupSending, setGroupSending] = useState(false);
  const groupHistoryRef = useRef<HTMLDivElement>(null);
  const groupInitialScrollDone = useRef(false);
  const [groupScrollReady, setGroupScrollReady] = useState(false);
  const { messages, hasMore, historyLoading, initialLoaded, loadOlder, loadNew, appendMessage } = useChatHistory(room);

  useEffect(() => {
    groupInitialScrollDone.current = false;
    setGroupScrollReady(false);
  }, [room]);

  useLayoutEffect(() => {
    if (room !== "group" || !initialLoaded || groupInitialScrollDone.current) return;
    const history = groupHistoryRef.current;
    if (!history) return;
    groupInitialScrollDone.current = true;
    history.scrollTop = history.scrollHeight;
    setGroupScrollReady(true);
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
  }, [room]);

  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
		if (detail?.type === "chat_message" && detail.data.room_type === room && detail.data.game_id === (room === "service" ? "service" : "lobby")) {
        void loadNew();
      }
    };
    window.addEventListener(WS_EVENT, onWs);
    return () => {
      window.removeEventListener(WS_EVENT, onWs);
    };
  }, [loadNew, room]);

  const claimRedPacket = async () => {
    if (!packet || packet.claimed || claimedPacketIDs.has(packet.messageId) || packetOpening) return;
    setPacketOpening(true);
    setPacketError("");
    try {
      const result = await chatApi.claimRedPacket(packet.messageId);
      setClaimedPacketIDs((current) => new Set(current).add(packet.messageId));
      setPacketReward(result.reward);
      onRefreshUnread?.();
      playNotificationSound("reward");
    } catch (reason) {
      setPacketError(reason instanceof Error ? reason.message : "红包暂时无法领取，请稍后重试");
    } finally {
      setPacketOpening(false);
    }
  };

  const sendGroupMessage = async () => {
    const text = groupDraft.trim();
    if (!text || groupSending) return;
    setGroupSending(true);
    try {
	  const created = await chatApi.send(text, "group", "lobby");
      setGroupDraft("");
      appendMessage(created);
      onRefreshUnread?.();
    } catch {
      setGroupDraft(text);
    } finally {
      setGroupSending(false);
    }
  };

  const groupMessageTimeline = messages.map((message) => message.message_type === "redpacket" ? ({
    kind: "packet" as const,
    at: new Date(message.created_at).getTime(),
    packetKind: "lucky" as const,
    packetKey: `message-${message.id}`,
    referenceId: message.reference_id,
    title: message.content || "房间福利红包",
    description: message.claimed || claimedPacketIDs.has(message.id) ? "红包已领取" : `共 ${message.red_packet_count || 1} 个红包 · 点击领取`,
    cover: message.red_packet_cover || "classic",
    minTurnover: Number(message.red_packet_min_turnover || 0),
    reward: message.red_packet_reward ?? null,
    claimed: Boolean(message.claimed || claimedPacketIDs.has(message.id)),
    messageId: message.id,
  }) : ({ kind: "message" as const, at: new Date(message.created_at).getTime(), message }));
  const groupTimeline = groupMessageTimeline.sort((left, right) => left.at - right.at);

  return (
    <section className="chat-room">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}>
          <Icon name="back" />
        </button>
        <b>{title}</b>
      </header>
      {room === "service" ? (
        <ServiceConversation quickReplies={quickReplies} onRefreshUnread={onRefreshUnread} />
      ) : (
        <>
          <div className="room-notice">{roomNotice}</div>
          <div aria-busy={!initialLoaded || !groupScrollReady} className={`chat-history ${!initialLoaded || !groupScrollReady ? "chat-history-positioning" : "chat-history-ready"}`} ref={groupHistoryRef}>
            {!initialLoaded ? <ChatInitialLoading /> : <>
              <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
              <p>今天</p>
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
              ) : <PacketBubble key={item.packetKey} title={item.title} description={item.minTurnover > 0 && !item.claimed ? `${item.description} · 流水满 ${item.minTurnover.toFixed(2)}` : item.description} cover={item.cover} claimed={item.claimed} time={new Date(item.at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })} onClick={() => { setPacketReward(item.reward); setPacketError(""); setPacketOpening(false); setPacket({ messageId: item.messageId, claimed: item.claimed, greeting: item.title, cover: item.cover, minTurnover: item.minTurnover }); }} />)}
            </>}
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
              type="lucky"
              claimed={packet.claimed || claimedPacketIDs.has(packet.messageId)}
              reward={packetReward}
              greeting={packet.greeting}
              cover={packet.cover}
              minTurnover={packet.minTurnover}
              opening={packetOpening}
              error={packetError}
              onOpen={() => void claimRedPacket()}
              onClose={() => { if (!packetOpening) setPacket(null); }}
            />
          )}
        </>
      )}
    </section>
  );
}

function ServiceConversation({ quickReplies, onRefreshUnread }: { quickReplies: string[]; onRefreshUnread?: () => void }) {
  const [draft, setDraft] = useState("");
  // A room owner nickname belongs to the agent room, not to customer service.
  // Keep this identity stable so the heading can never become “群主在线”.
  const label = "客服小七";
  const welcomeTime = useRef(messageTime()).current;
  const historyRef = useRef<HTMLDivElement>(null);
  const initialScrollDone = useRef(false);
  const [scrollReady, setScrollReady] = useState(false);
  const { messages, hasMore, historyLoading, initialLoaded, loadOlder, loadNew, appendMessage } = useChatHistory("service", "service");

  useLayoutEffect(() => {
    if (!initialLoaded || initialScrollDone.current) return;
    const history = historyRef.current;
    if (!history) return;
    initialScrollDone.current = true;
    // Customer service is a ticket-style conversation: its greeting and the
    // first historical message must always start below the status bar. Do not
    // reuse the group-chat behaviour that opens on the latest message.
    history.scrollTop = 0;
    setScrollReady(true);
  }, [initialLoaded]);

  // The periodic request in useChatHistory is only a recovery path. Replies
  // from the admin console arrive through the authenticated WebSocket and are
  // pulled into the timeline immediately.
  useEffect(() => {
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
	  if (detail?.type === "chat_message" && detail.data.room_type === "service" && detail.data.game_id === "service") {
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
	  const created = await chatApi.send(text, "service", "service");
      setDraft("");
      appendMessage(created);
      onRefreshUnread?.();
    } catch {
      setDraft(text);
    }
  };
  return (
    <>
      <div className="room-notice service-room-status"><b>专属客服在线</b><span>随时为您服务</span></div>
      <div aria-busy={!initialLoaded || !scrollReady} className={`chat-history service-history ${messages.length === 0 ? "service-history-empty" : ""} ${!initialLoaded || !scrollReady ? "chat-history-positioning" : "chat-history-ready"}`} ref={historyRef}>
        {!initialLoaded ? <ChatInitialLoading /> : <>
          <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
          <p className="service-time">今天</p>
          <ServiceMessage text={`您好，我是${label}，很高兴为您服务。请问有什么可以帮您？`} time={welcomeTime} />
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
        </>}
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

function ChatInitialLoading() {
  return <div className="chat-initial-loading" role="status"><span aria-hidden="true" /><b>正在加载最新消息</b></div>;
}

function PacketBubble({
  title,
  description,
  cover,
  claimed,
  time,
  onClick,
}: {
  title: string;
  description: string;
  cover: string;
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
          className={`red-packet packet-cover-${cover} ${claimed ? "claimed" : ""}`}
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
  winning: { title: "开奖通知", preview: "开奖号码与投注结果", time: "最新", icon: "开" },
  activity: { title: "活动通知", preview: "优惠活动与专属礼遇", time: "最新", icon: "活" },
} as const;

const resultNotificationTitles = new Set(["开奖结果", "恭喜中奖", "未中奖", "开奖通知"]);

function visibleNotifications(kind: "system" | "winning", rows: MemberNotification[]) {
  if (kind === "winning") return rows.filter((item) => item.category === "winning");
  return rows.filter((item) => item.category === "system" && !resultNotificationTitles.has(item.title));
}

function money(value?: number) {
  return Number(value ?? 0).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

function DrawBalls({ numbers }: { numbers?: number[] }) {
  if (!numbers?.length) return null;
  return <div className="draw-notice-balls" aria-label={`开奖号码 ${numbers.join("、")}`}>{numbers.map((number, index) => <i key={`${number}-${index}`}>{number}</i>)}</div>;
}

function DrawNoticeSummary({ message, detail = false }: { message: MemberNotification; detail?: boolean }) {
  const hasStructuredResult = Boolean(message.game_name || message.issue || message.draw_numbers?.length);
  if (!hasStructuredResult) return <p>{message.content}</p>;
  const gameLogo = lotteryGameLogo(message.game_id);
  return (
    <div className={detail ? "draw-notice-detail" : "draw-notice-summary"}>
      <div className="draw-notice-heading">
        <div className="draw-notice-game">
          {gameLogo && <span><img alt={`${message.game_name || "彩票"} Logo`} src={gameLogo} /></span>}
          <b>{message.game_name || "开奖信息"}</b>
        </div>
        {message.issue && <small>第 {message.issue} 期</small>}
      </div>
      <DrawBalls numbers={message.draw_numbers} />
      <time>开奖时间 {new Date(message.draw_at || message.created_at).toLocaleString("zh-CN")}</time>
      <div className="draw-notice-stats">
        <span><small>投注</small><b>{message.bet_count ?? 0} 注</b><em>{money(message.stake_amount)} 元</em></span>
        <span className={(message.payout_amount ?? 0) > 0 ? "has-prize" : ""}><small>中奖</small><b>{message.won_count ?? 0} 注</b><em>{money(message.payout_amount)} 元</em></span>
      </div>
      {detail && Boolean(message.bet_details?.length) && (
        <div className="draw-bet-list">
          <h3>投注明细</h3>
          {message.bet_details?.map((bet, index) => (
            <section key={`${bet.play_name}-${bet.position}-${bet.selection}-${index}`}>
              <div><b>{bet.play_name}{bet.position ? ` · 第${bet.position}球` : ""}</b><small>{bet.result === "won" ? "已中奖" : "未中奖"}</small></div>
              <p>{bet.selection} · 投注 {money(bet.amount)} 元 · 赔率 {Number(bet.odds || 0).toFixed(3)}</p>
              {bet.result === "won" && <em>中奖 {money(bet.payout)} 元</em>}
            </section>
          ))}
        </div>
      )}
    </div>
  );
}

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
      setItems(visibleNotifications(kind, page.items));
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
      const visible = visibleNotifications(kind, page.items);
      setItems((current) => [...current, ...visible.filter((item) => !current.some((existing) => existing.id === item.id))]);
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
      <div className="system-notice-list">
        {items.length === 0 && <p className="empty-notice">{isWinning ? "暂无开奖通知" : "暂无系统公告"}</p>}
        {items.map((message) => (
          <button className={`system-notice-card ${isWinning ? "draw-notice-card" : ""} ${!message.read ? "is-unread" : ""}`} key={message.id} onClick={() => void openItem(message)}>
            <div className="system-notice-card-top">
              <span>
                {isWinning ? "开奖通知" : "系统公告"}
                {isWinning && !message.read && <i className="notification-unread-dot" aria-label="未读" />}
              </span>
              <time>{new Date(message.created_at).toLocaleString("zh-CN")}</time>
            </div>
            <div>
              {!isWinning && <b>{message.title}{!message.read && <i className="notification-unread-dot" aria-label="未读" />}</b>}
              {isWinning ? <DrawNoticeSummary message={message} /> : <p>{message.content}</p>}
              <em>查看详情 <Icon name="arrow" /></em>
            </div>
          </button>
        ))}
        <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
      </div>
      {selected && (
        <ActionDialog title={isWinning ? "开奖通知" : selected.title} description={isWinning && (selected.game_name || selected.issue) ? `${selected.game_name || "开奖信息"}${selected.issue ? ` · 第 ${selected.issue} 期` : ""}` : selected.content} onClose={() => setSelected(null)}>
          {isWinning && <DrawNoticeSummary message={selected} detail />}
        </ActionDialog>
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

function openActivityAction(activity?: ActivityItem) {
  const config = activity?.config;
  const actionType = typeof config?.action_type === "string" ? config.action_type : "none";
  const actionURL = typeof config?.action_url === "string" ? config.action_url.trim() : "";
  if (!actionURL || actionType === "none") return false;
  if (actionType === "internal" && actionURL.startsWith("/")) {
    window.history.pushState({}, "", actionURL);
    window.dispatchEvent(new PopStateEvent("popstate"));
    return true;
  }
  if (actionType === "external" && /^https:\/\//i.test(actionURL)) {
    window.location.assign(actionURL);
    return true;
  }
  return false;
}

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
      setActivities(activityRows.filter((row) => row.status === "active" && row.type === "promotion"));
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

  const openCard = (card: ActivityNotice) => {
    if (card.notification && !card.notification.read) {
      setNotices((current) => current.map((item) => item.id === card.notification?.id ? { ...item, read: true } : item));
      void portalApi.markRead(card.notification.id).catch(() => undefined);
      onRefreshUnread?.();
    }
    if (openActivityAction(card.activity)) return;
    setSelected(card);
  };

  return (
    <section className="notification-page activity-notice-page">
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button>
        <b>活动通知</b>
        <span aria-hidden="true" />
      </header>
      <div className="activity-notice-list">
        {cards.length === 0 && <p className="empty-notice">暂无进行中的活动</p>}
        {cards.map((card, index) => (
          <button aria-label={`${card.title}，${card.subtitle}`} className={`activity-notice-card card-tone-${index % 4} ${card.cover ? "has-cover" : ""}`} key={card.id} onClick={() => void openCard(card)}>
            {card.cover ? <>
              <img alt={card.title} src={card.cover} />
              {card.notification && !card.notification.read && <span className="activity-cover-badge">新活动</span>}
              <span className="activity-cover-action" aria-hidden="true"><Icon name="arrow" /></span>
            </> : <>
              <span className="activity-notice-art" aria-hidden="true"><i>{index % 2 ? "福利" : "好运"}</i><b>{index % 2 ? "专属礼遇" : "幸运相伴"}</b></span>
              <div className="activity-notice-shade" />
              <div className="activity-notice-copy">
                <small>{card.notification && !card.notification.read ? "新活动" : "正在进行"}</small>
                <b>{card.title}</b>
                <p>{card.subtitle}</p>
                <em>查看活动 <Icon name="arrow" /></em>
              </div>
            </>}
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
