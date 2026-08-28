import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { BRAND_NAME } from "../data/brand";
import { Icon } from "../components/Icon";
import { Avatar } from "../components/Avatar";
import { RedPacketDialog } from "../components/Dialogs";
import { ActionDialog } from "../components/Dialogs";
import type { ChatView } from "../router";
import { playNotificationSound } from "../utils/notificationAudio";
import { portalApi, type ActivityItem, type MemberNotification, type RoomSettings } from "../api/portal";
import { chatApi, type ChatMessage, type ChatPreview } from "../api/chat";
import { WS_EVENT, type WsEvent, useWebSocketConnected } from "../hooks/useWebSocket";
import { lotteryGameLogo } from "../hooks/useLotteryGames";
import type { Game } from "../types";
import { PlanDetail, PlanLobby } from "./PlanGroup";
import { mergeChatMessages } from "../utils/chatMessages";
import {
  activePromotionTitles,
  configuredHiddenMessageRows,
  visibleNotificationsForRow,
} from "../utils/notificationVisibility";

type Room = "group" | "service";

function friendlyChatError(reason: unknown, fallback: string) {
  const message = reason instanceof Error ? reason.message.trim() : "";
  if (!message) return fallback;
  if (/failed to fetch|network|timeout|load failed/i.test(message)) return "网络连接不稳定，消息暂时未更新";
  return message;
}

function useChatHistory(room: Room, gameId = room === "service" ? "service" : "lobby") {
  const websocketConnected = useWebSocketConnected();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const [historyError, setHistoryError] = useState("");
  const messagesRef = useRef<ChatMessage[]>([]);
  const newMessagesLoadingRef = useRef(false);
  const conversationRef = useRef(`${room}:${gameId}`);
	conversationRef.current = `${room}:${gameId}`;

  const replaceMessages = useCallback((next: ChatMessage[]) => {
    const ordered = mergeChatMessages(next);
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
      setHistoryError("");
    } catch (reason) {
	  if (conversationRef.current === requestConversation) {
        if (messagesRef.current.length === 0) setHasMore(false);
        setHistoryError(friendlyChatError(reason, "消息暂时未加载"));
      }
    } finally {
	  if (conversationRef.current === requestConversation) setInitialLoaded(true);
    }
  }, [gameId, replaceMessages, room]);

  const loadNew = useCallback(async () => {
	const requestConversation = `${room}:${gameId}`;
    if (newMessagesLoadingRef.current) return;
    const newest = messagesRef.current.at(-1);
    if (!newest) {
      await loadInitial();
      return;
    }
    newMessagesLoadingRef.current = true;
    try {
	  const page = await chatApi.messages(room, gameId, 50, { after_id: newest.id });
	  if (conversationRef.current === requestConversation) {
        if (page.items.length) replaceMessages([...messagesRef.current, ...page.items]);
        setHistoryError("");
      }
    } catch (reason) {
      if (conversationRef.current === requestConversation) setHistoryError(friendlyChatError(reason, "消息连接恢复中"));
    } finally { newMessagesLoadingRef.current = false; }
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
      setHistoryError("");
    } catch (reason) {
      if (conversationRef.current === requestConversation) setHistoryError(friendlyChatError(reason, "更早消息暂时无法加载"));
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
    setHistoryError("");
    void loadInitial();
  }, [loadInitial]);

  useEffect(() => {
    if (!initialLoaded) return;
    // A status change means either the live channel has recovered or just
    // dropped. In both cases pull once from the last known message ID. While
    // connected, WebSocket events are the only recurring update mechanism.
    void loadNew();
    if (websocketConnected) return;
    const timer = window.setInterval(() => { void loadNew(); }, 15_000);
    return () => window.clearInterval(timer);
  }, [initialLoaded, loadNew, websocketConnected]);

  const retry = useCallback(() => messagesRef.current.length ? loadNew() : loadInitial(), [loadInitial, loadNew]);
  return { messages, hasMore, historyLoading, initialLoaded, historyError, loadOlder, loadNew, appendMessage, retry };
}

function HistoryLoadButton({ hasMore, loading, onLoad }: { hasMore: boolean; loading: boolean; onLoad: () => void }) {
  if (!hasMore) return null;
  return <button type="button" className="chat-load-history" disabled={loading} onClick={onLoad}>{loading ? "正在加载…" : "查看更早消息"}</button>;
}

async function markNotificationRowsRead(rows: MemberNotification[]) {
  const unread = rows.filter((item) => !item.read);
  if (!unread.length) return new Set<number>();
  const results = await Promise.allSettled(unread.map((item) => portalApi.markRead(item.id)));
  return new Set(unread.filter((_, index) => results[index].status === "rejected").map((item) => item.id));
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
  const websocketConnected = useWebSocketConnected();
  const [preview, setPreview] = useState<ChatPreview | null>(null);
  const [notifications, setNotifications] = useState<MemberNotification[]>([]);
  const [promotionTitles, setPromotionTitles] = useState<string[]>([]);
  const [servicePreview, setServicePreview] = useState("暂无客服消息");
  const [servicePreviewTime, setServicePreviewTime] = useState("暂无");
  const [roomName, setRoomName] = useState("");
  const [roomLogo, setRoomLogo] = useState("");
  const [pinnedRows, setPinnedRows] = useState<string[]>(["service", "group"]);
  const [hiddenRows, setHiddenRows] = useState<string[]>(["winning"]);
  const [listError, setListError] = useState("");

  useEffect(() => {
    void portalApi.roomSettings().then((settings) => {
      setRoomName(settings.room_name?.trim() || "");
      setRoomLogo(settings.room_logo || "");
      const pinned = settings.game?.message_pinned_rows;
      if (Array.isArray(pinned)) setPinnedRows(pinned.filter((item): item is string => typeof item === "string"));
      setHiddenRows(configuredHiddenMessageRows(settings.game));
    }).catch((reason) => setListError(friendlyChatError(reason, "房间消息设置暂时未加载")));
  }, []);

  const loadPreview = useCallback(async () => {
    const results = await Promise.allSettled([
      chatApi.preview().then(setPreview),
      portalApi.notifications(50).then((page) => setNotifications(page.items)),
      portalApi.activities().then((items) => {
        setPromotionTitles([...activePromotionTitles(items)]);
      }),
	  chatApi.messages("service", "service", 1).then((page) => {
        const last = page.items.at(-1);
        if (last) {
          setServicePreview(`${last.mine ? "我" : "客服"}：${last.content}`);
          setServicePreviewTime(messageTime(last.created_at));
        } else {
          setServicePreview("暂无客服消息");
          setServicePreviewTime("暂无");
        }
      }),
    ]);
    const failure = results.find((result): result is PromiseRejectedResult => result.status === "rejected");
    setListError(failure ? friendlyChatError(failure.reason, "部分消息暂时未更新") : "");
  }, []);

  useEffect(() => {
    void loadPreview();
    const timer = websocketConnected ? 0 : window.setInterval(() => { void loadPreview(); }, 15_000);
    const onWs = (event: Event) => {
      const detail = (event as CustomEvent<WsEvent>).detail;
      if (detail?.type === "chat_message" || detail?.type === "notification") void loadPreview();
    };
    window.addEventListener(WS_EVENT, onWs);
    return () => {
      if (timer) window.clearInterval(timer);
      window.removeEventListener(WS_EVENT, onWs);
    };
  }, [loadPreview, unreadCount, websocketConnected]);

  const groupMessage = preview?.latest_message || "暂无群聊消息";
  const groupTime = preview?.latest_at
    ? new Date(preview.latest_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
    : "暂无";
  const promotionTitleSet = new Set(promotionTitles);
  const noticeFor = (category: "system" | "activity" | "winning") => {
    const items = visibleNotificationsForRow(category, notifications, promotionTitleSet);
    return items.find((item) => !item.read) ?? items[0] ?? null;
  };
  const unreadFor = (category: "system" | "activity" | "winning") => visibleNotificationsForRow(category, notifications, promotionTitleSet).filter((item) => !item.read).length;
  const systemNotice = noticeFor("system");
  const activityNotice = noticeFor("activity");
  const winningNotice = noticeFor("winning");
  const allRead = (["system", "activity", "winning"] as const)
    .filter((category) => !hiddenRows.includes(category))
    .every((category) => unreadFor(category) === 0);

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
        groupChatEnabled={preview?.can_chat ?? false}
        onBack={view === "service" && onServiceBack ? onServiceBack : () => onNavigate("list")}
        onRefreshUnread={onRefreshUnread}
      />
    );
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
      onClick: () => onNavigate("system"),
    },
    {
      key: "activity", kind: "activity" as const, name: "活动通知",
      message: activityNotice?.content || activityNotice?.title || "优惠活动与专属礼遇会在这里展示",
      time: activityNotice?.created_at ? new Date(activityNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "活动",
      badge: unreadFor("activity") ? (unreadFor("activity") > 9 ? "9+" : String(unreadFor("activity"))) : undefined,
      onClick: () => onNavigate("activity"),
    },
    {
      key: "winning", kind: "winning" as const, name: "开奖通知",
      message: winningNotice?.content || winningNotice?.title || "开奖号码与投注结果会在这里展示",
      time: winningNotice?.created_at ? new Date(winningNotice.created_at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) : "开奖",
      badge: unreadFor("winning") ? (unreadFor("winning") > 9 ? "9+" : String(unreadFor("winning"))) : undefined,
      onClick: () => onNavigate("winning"),
    },
    { key: "service", kind: "service" as const, name: "在线客服", message: servicePreview, time: servicePreviewTime, badge: undefined, onClick: () => onNavigate("service") },
    { key: "group", kind: "group" as const, name: roomName || "聊天室", message: groupMessage, time: groupTime, badge: undefined, onClick: () => onNavigate("group") },
    { key: "plan", kind: "plan" as const, name: "计划群", message: "查看房间已发布计划", time: "计划", badge: undefined, onClick: () => onNavigate("plans") },
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
      {listError && <button type="button" className="chat-inline-retry" onClick={() => void loadPreview()}>消息更新失败，点击重试</button>}
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

function selectedRoomAnnouncement(settings: RoomSettings) {
  const current = [...(settings.announcements ?? [])]
    .filter((item) => item.enabled && item.content.trim())
    .sort((left, right) => left.sort_order - right.sort_order)[0];
  return current?.content.trim() || settings.room_notice?.trim() || "";
}

function chatMemberTitle(message: ChatMessage, roomTitle: string) {
  const title = message.title?.trim() || message.user_title?.trim();
  if (title) return title;
  return message.user_id === 0 ? roomTitle.trim() : "";
}

function ChatMemberAvatar({ message, staffAvatar }: { message: ChatMessage; staffAvatar: string }) {
  if (message.mine) return <Avatar className="service-avatar user" index={-1} src={message.avatar?.trim()} label="我的头像" />;
  const image = message.avatar?.trim() || (message.user_id === 0 ? staffAvatar.trim() : "");
  if (image) return <img alt={`${message.nickname}头像`} className="app-avatar service-avatar room-member-avatar" src={image} />;
  return <Avatar className="service-avatar" index={Number(message.user_id) % 32} label={`${message.nickname}头像`} />;
}

function ChatRoom({
  room,
  title,
  groupChatEnabled,
  onBack,
  onRefreshUnread,
}: {
  room: Room;
  title: string;
  groupChatEnabled: boolean;
  onBack: () => void;
  onRefreshUnread?: () => void;
}) {
  const [packet, setPacket] = useState<{ messageId: number; claimed: boolean; greeting: string; cover: string; minTurnover: number } | null>(null);
  const [claimedPacketIDs, setClaimedPacketIDs] = useState<Set<number>>(() => new Set());
  const [packetReward, setPacketReward] = useState<number | null>(null);
  const [packetOpening, setPacketOpening] = useState(false);
  const [packetError, setPacketError] = useState("");
  const [roomNotice, setRoomNotice] = useState("");
  const [roomName, setRoomName] = useState("");
  const [roomLogo, setRoomLogo] = useState("");
  const [roomChatTitle, setRoomChatTitle] = useState("");
  const [roomChatAvatar, setRoomChatAvatar] = useState("");
  const [quickReplies, setQuickReplies] = useState<string[]>([]);
  const [roomSettingsError, setRoomSettingsError] = useState("");
  const [groupDraft, setGroupDraft] = useState("");
  const [groupSending, setGroupSending] = useState(false);
  const [sendError, setSendError] = useState("");
  const groupHistoryRef = useRef<HTMLDivElement>(null);
  const groupInitialScrollDone = useRef(false);
  const [groupScrollReady, setGroupScrollReady] = useState(false);
  const { messages, hasMore, historyLoading, initialLoaded, historyError, loadOlder, loadNew, appendMessage, retry } = useChatHistory(room);

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

  const loadRoomSettings = useCallback(async () => {
    try {
      const settings = await portalApi.roomSettings();
      setRoomNotice(selectedRoomAnnouncement(settings));
      setRoomName(settings.room_name?.trim() || "");
      setRoomLogo(settings.room_logo?.trim() || "");
      setRoomChatTitle(settings.chat_nickname?.trim() || "");
      setRoomChatAvatar(settings.chat_avatar?.trim() || "");
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
      setRoomSettingsError("");
    } catch (reason) {
      setRoomSettingsError(friendlyChatError(reason, "房间设置暂时未加载"));
    }
  }, []);

  useEffect(() => { void loadRoomSettings(); }, [loadRoomSettings, room]);

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
    if (!text || groupSending || !groupChatEnabled) return;
    setGroupSending(true);
    setSendError("");
    try {
	  const created = await chatApi.send(text, "group", "lobby");
      setGroupDraft("");
      appendMessage(created);
      onRefreshUnread?.();
    } catch (reason) {
      setGroupDraft(text);
      setSendError(friendlyChatError(reason, "消息发送失败，请重试"));
    } finally {
      setGroupSending(false);
    }
  };

  const groupMessageTimeline = messages.map((message) => {
    if (message.message_type !== "redpacket") return { kind: "message" as const, at: new Date(message.created_at).getTime(), message };
    const claimed = Boolean(message.claimed || claimedPacketIDs.has(message.id));
    const statusLabel = claimed ? "" : message.red_packet_status === "empty" ? "红包已领完" : message.red_packet_status === "expired" ? "红包已过期" : message.red_packet_status === "closed" ? "红包已关闭" : "";
    return {
      kind: "packet" as const,
      at: new Date(message.created_at).getTime(),
      packetKind: "lucky" as const,
      packetKey: `message-${message.id}`,
      referenceId: message.reference_id,
      title: message.content || "房间福利红包",
      description: claimed ? "红包已领取" : statusLabel || `已领取 ${message.red_packet_claimed_count || 0}/${message.red_packet_count || 1} · 点击领取`,
      cover: message.red_packet_cover || "classic",
      minTurnover: Number(message.red_packet_min_turnover || 0),
      reward: message.red_packet_reward ?? null,
      claimed,
      statusLabel: statusLabel || (message.red_packet_close_reason ?? ""),
      messageId: message.id,
    };
  });
  const groupTimeline = groupMessageTimeline.sort((left, right) => left.at - right.at);
  const displayTitle = room === "group" ? roomName || title : title;

  return (
    <section className={`chat-room ${room === "group" && !roomNotice ? "no-room-notice" : ""}`}>
      <header className={`blue-header ${room === "group" ? "chat-room-header" : ""}`}>
        <button aria-label="返回消息列表" onClick={onBack}>
          <Icon name="back" />
        </button>
        {room === "group" ? (
          <b className="chat-room-heading">
            <span className={`chat-room-avatar ${roomLogo ? "has-image" : ""}`} aria-hidden="true">
              {roomLogo ? <img alt="" src={roomLogo} /> : displayTitle.slice(0, 1)}
            </span>
            <span><strong>{displayTitle}</strong>{roomChatTitle && <small>{roomChatTitle}</small>}</span>
          </b>
        ) : <b>{displayTitle}</b>}
      </header>
      {roomSettingsError && <div className="chat-room-settings-error"><ChatRetryState compact message={roomSettingsError} onRetry={() => void loadRoomSettings()} /></div>}
      {room === "service" ? (
        <ServiceConversation quickReplies={quickReplies} serviceName={roomChatTitle || "在线客服"} serviceAvatar={roomChatAvatar || roomLogo} onRefreshUnread={onRefreshUnread} />
      ) : (
        <>
          {roomNotice && <div className="room-notice">{roomNotice}</div>}
          <div aria-busy={!initialLoaded || !groupScrollReady} className={`chat-history ${!initialLoaded || !groupScrollReady ? "chat-history-positioning" : "chat-history-ready"}`} ref={groupHistoryRef}>
            {!initialLoaded ? <ChatInitialLoading /> : <>
              {historyError && <ChatRetryState message={historyError} onRetry={() => void retry()} />}
              <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
              <p>今天</p>
              {groupTimeline.map((item) => {
                if (item.kind !== "message") return <PacketBubble key={item.packetKey} title={item.title} description={item.minTurnover > 0 && !item.claimed && !item.statusLabel ? `${item.description} · 流水满 ${item.minTurnover.toFixed(2)}` : item.description} cover={item.cover} claimed={item.claimed} statusLabel={item.statusLabel} time={new Date(item.at).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })} onClick={() => { if (item.statusLabel) return; setPacketReward(item.reward); setPacketError(""); setPacketOpening(false); setPacket({ messageId: item.messageId, claimed: item.claimed, greeting: item.title, cover: item.cover, minTurnover: item.minTurnover }); }} />;
                const memberTitle = chatMemberTitle(item.message, roomChatTitle);
                const memberBadge = item.message.badge?.trim() || "";
                return (
                  <div className={`service-message chat-message-row ${item.message.mine ? "outgoing" : ""}`} key={`message-${item.message.id}`}>
                    {!item.message.mine && <ChatMemberAvatar message={item.message} staffAvatar={roomChatAvatar || roomLogo} />}
                    <div className="service-bubble">
                      {(!item.message.mine || memberTitle || memberBadge) && <small className="chat-member-name"><span>{item.message.nickname}</span>{memberTitle && <i>{memberTitle}</i>}{memberBadge && <em>{memberBadge}</em>}</small>}
                      <span>{item.message.content}</span>
                      <time className="message-bubble-time">{messageTime(item.message.created_at)}</time>
                    </div>
                    {item.message.mine && <ChatMemberAvatar message={item.message} staffAvatar={roomChatAvatar || roomLogo} />}
                  </div>
                );
              })}
            </>}
          </div>
          {sendError && <ChatRetryState compact message={sendError} onRetry={() => void sendGroupMessage()} />}
          <div className="chat-input">
            <button aria-label="添加内容" disabled><Icon name="plus" /></button>
            <input aria-label="输入聊天室消息" disabled={!groupChatEnabled} onChange={(event) => setGroupDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void sendGroupMessage() }} placeholder={groupChatEnabled ? "输入消息" : "群聊已禁言"} value={groupDraft} />
            <button aria-label="发送聊天室消息" className="service-send" disabled={!groupChatEnabled || !groupDraft.trim() || groupSending} onClick={() => void sendGroupMessage()}>
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

function ServiceConversation({ quickReplies, serviceName, serviceAvatar, onRefreshUnread }: { quickReplies: string[]; serviceName: string; serviceAvatar: string; onRefreshUnread?: () => void }) {
  const [draft, setDraft] = useState("");
  const [sendError, setSendError] = useState("");
  const historyRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const initialScrollDone = useRef(false);
  const [scrollReady, setScrollReady] = useState(false);
  const { messages, hasMore, historyLoading, initialLoaded, historyError, loadOlder, loadNew, appendMessage, retry } = useChatHistory("service", "service");

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
    setSendError("");
    try {
	  const created = await chatApi.send(text, "service", "service");
      setDraft("");
      appendMessage(created);
      onRefreshUnread?.();
    } catch (reason) {
      setDraft(text);
      setSendError(friendlyChatError(reason, "消息发送失败，请重试"));
    } finally {
      // Keep the composer active after sending. Preventing the send button
      // from taking focus is what keeps the iOS/Safari keyboard visible;
      // this focus call also covers quick replies and failed requests.
      window.requestAnimationFrame(() => inputRef.current?.focus({ preventScroll: true }));
    }
  };
  return (
    <>
      <div className="room-notice service-room-status"><b>{serviceName}在线</b><span>随时为您服务</span></div>
      <div aria-busy={!initialLoaded || !scrollReady} className={`chat-history service-history ${messages.length === 0 ? "service-history-empty" : ""} ${!initialLoaded || !scrollReady ? "chat-history-positioning" : "chat-history-ready"}`} ref={historyRef}>
        {!initialLoaded ? <ChatInitialLoading /> : <>
          {historyError && <ChatRetryState message={historyError} onRetry={() => void retry()} />}
          <HistoryLoadButton hasMore={hasMore} loading={historyLoading} onLoad={() => void loadOlder()} />
          {messages.length > 0 && <p className="service-time">今天</p>}
          {messages.map((message) => <ServiceMessage key={message.id} message={message} serviceName={serviceName} serviceAvatar={serviceAvatar} />)}
          {messages.length === 0 && quickReplies.length > 0 && (
            <div className="service-replies">
              {quickReplies.slice(0, 3).map((text) => (
                <button key={text} onClick={() => void sendMessage(text)}>{text}</button>
              ))}
            </div>
          )}
        </>}
      </div>
      {sendError && <ChatRetryState compact message={sendError} onRetry={() => void sendMessage()} />}
      <div className="chat-input">
        <button aria-label="添加内容" disabled><Icon name="plus" /></button>
        <input ref={inputRef} aria-label="输入客服消息" enterKeyHint="send" onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void sendMessage(); } }} placeholder="输入消息" value={draft} />
        <button aria-label="发送消息" className="service-send" disabled={!draft.trim()} onPointerDown={(event) => event.preventDefault()} onClick={() => void sendMessage()}>
          <Icon name="arrow" />
        </button>
      </div>
    </>
  );
}

function ServiceMessage({
  message,
  serviceName,
  serviceAvatar,
}: {
  message: ChatMessage;
  serviceName: string;
  serviceAvatar: string;
}) {
  const outgoing = message.mine;
  const title = message.title?.trim() || message.user_title?.trim() || "";
  const badge = message.badge?.trim() || "";
  const inboundName = serviceName || message.nickname;
  const inboundAvatar = message.avatar?.trim() || serviceAvatar.trim();
  return (
    <div className={`service-message chat-message-row ${outgoing ? "outgoing" : ""}`}>
      {!outgoing && (
        inboundAvatar
          ? <img alt={`${inboundName}头像`} className="app-avatar service-avatar room-member-avatar" src={inboundAvatar} />
          : <Avatar className="service-avatar" index={7} label={`${inboundName}头像`} />
      )}
      <div className="service-bubble">
        <small className="chat-member-name"><span>{outgoing ? message.nickname : inboundName}</span>{title && <i>{title}</i>}{badge && <em>{badge}</em>}</small>
        <span>{message.content}</span><time className="message-bubble-time">{messageTime(message.created_at)}</time>
      </div>
      {outgoing && (
        <Avatar className="service-avatar user" index={-1} src={message.avatar?.trim()} label="我的头像" />
      )}
    </div>
  );
}

function ChatInitialLoading() {
  return <div className="chat-initial-loading" role="status"><span aria-hidden="true" /><b>正在加载最新消息</b></div>;
}

function ChatRetryState({ message, onRetry, compact = false }: { message: string; onRetry: () => void; compact?: boolean }) {
  return <div className={`chat-retry-state ${compact ? "compact" : ""}`} role="status"><span>{message}</span><button type="button" onClick={onRetry}>重试</button></div>;
}

function PacketBubble({
  title,
  description,
  cover,
  claimed,
  statusLabel,
  time,
  onClick,
}: {
  title: string;
  description: string;
  cover: string;
  claimed: boolean;
  statusLabel?: string;
  time: string;
  onClick: () => void;
}) {
  return (
    <div className="packet-line">
      <span className="service-logo">曜</span>
      <div>
        <small>{BRAND_NAME} · {time}</small>
        <button
          className={`red-packet packet-cover-${cover} ${claimed || statusLabel ? "claimed" : ""}`}
          disabled={Boolean(statusLabel)}
          onClick={onClick}
        >
          <span>
            <Icon name="gift" />
          </span>
          <b>{title}</b>
          <em>{description}</em>
          <footer>{statusLabel || (claimed ? "已领取 · 查看详情" : `${BRAND_NAME}奖励`)}</footer>
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
  const [loadError, setLoadError] = useState("");

  const loadInitial = useCallback(async () => {
    try {
      const page = await portalApi.notifications(20, { category: kind });
      const visible = visibleNotifications(kind, page.items);
      const unreadIDs = new Set(visible.filter((item) => !item.read).map((item) => item.id));
      setItems(visible.map((item) => unreadIDs.has(item.id) ? { ...item, read: true } : item));
      setHasMore(page.has_more);
      setLoadError("");
      if (unreadIDs.size) {
        const failed = await markNotificationRowsRead(visible);
        if (failed.size) {
          setItems((current) => current.map((item) => failed.has(item.id) ? { ...item, read: false } : item));
          setLoadError("部分通知状态暂时未保存");
        }
        onRefreshUnread?.();
      }
    } catch (reason) { setLoadError(friendlyChatError(reason, "通知暂时未加载")); }
  }, [kind, onRefreshUnread]);

  useEffect(() => { void loadInitial(); }, [loadInitial]);

  const loadOlder = async () => {
    const beforeID = items.at(-1)?.id;
    if (!beforeID || !hasMore || historyLoading) return;
    setHistoryLoading(true);
    try {
      const page = await portalApi.notifications(20, { category: kind, before_id: beforeID });
      const visible = visibleNotifications(kind, page.items);
      const unreadIDs = new Set(visible.filter((item) => !item.read).map((item) => item.id));
      setItems((current) => [...current, ...visible.filter((item) => !current.some((existing) => existing.id === item.id)).map((item) => unreadIDs.has(item.id) ? { ...item, read: true } : item)]);
      setHasMore(page.has_more);
      setLoadError("");
      if (unreadIDs.size) {
        const failed = await markNotificationRowsRead(visible);
        if (failed.size) {
          setItems((current) => current.map((item) => failed.has(item.id) ? { ...item, read: false } : item));
          setLoadError("部分通知状态暂时未保存");
        }
        onRefreshUnread?.();
      }
    } catch (reason) {
      setLoadError(friendlyChatError(reason, "更早通知暂时无法加载"));
    } finally {
      setHistoryLoading(false);
    }
  };

  const openItem = async (item: MemberNotification) => {
    setSelected(item);
    if (!item.read) {
      // Optimistically clear the marker as soon as the user opens it.
      setItems((current) => current.map((row) => row.id === item.id ? { ...row, read: true } : row));
      try { await portalApi.markRead(item.id); setLoadError(""); }
      catch (reason) {
        setItems((current) => current.map((row) => row.id === item.id ? { ...row, read: false } : row));
        setLoadError(friendlyChatError(reason, "通知状态暂时未保存"));
      }
      onRefreshUnread?.();
    }
  };

  return (
    <section className={`notification-page system-notice-page ${isWinning ? "winning-notice-page" : "system-announcement-page"}`}>
      <header className="blue-header">
        <button aria-label="返回消息列表" onClick={onBack}><Icon name="back" /></button>
        <b>{thread.title}</b>
        <span aria-hidden="true" />
      </header>
      <div className="system-notice-list">
        {loadError && <ChatRetryState message={loadError} onRetry={() => void loadInitial()} />}
        {items.length === 0 && <p className="empty-notice">{isWinning ? "暂无开奖通知" : "暂无系统公告"}</p>}
        {items.map((message) => (
          <button className={`system-notice-card ${isWinning ? "draw-notice-card" : "system-announcement-card"} ${!message.read ? "is-unread" : ""}`} key={message.id} onClick={() => void openItem(message)}>
            <div className="system-notice-card-top">
              <span>
                {isWinning ? "开奖通知" : "SYSTEM NOTICE"}
                {isWinning && !message.read && <i className="notification-unread-dot" aria-label="未读" />}
              </span>
              <time>{new Date(message.created_at).toLocaleString("zh-CN")}</time>
            </div>
            <div>
              {!isWinning && <b>{message.title}{!message.read && <i className="notification-unread-dot" aria-label="未读" />}</b>}
              {isWinning ? <DrawNoticeSummary message={message} /> : <p>{message.content}</p>}
              <em aria-label="查看详情">{isWinning && "查看详情 "}<Icon name="arrow" /></em>
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
  const [loadError, setLoadError] = useState("");

  const loadInitial = useCallback(async () => {
    try {
      const [notificationPage, activityRows] = await Promise.all([portalApi.notifications(20, { category: "activity" }), portalApi.activities()]);
      const activeRows = activityRows.filter((row) => row.status === "active" && row.type === "promotion");
      const visible = visibleNotificationsForRow("activity", notificationPage.items, activePromotionTitles(activeRows));
      const unreadIDs = new Set(visible.filter((item) => !item.read).map((item) => item.id));
      setNotices(notificationPage.items.map((item) => unreadIDs.has(item.id) ? { ...item, read: true } : item));
      setHasMore(notificationPage.has_more);
      setActivities(activeRows);
      setLoadError("");
      if (unreadIDs.size) {
        const failed = await markNotificationRowsRead(visible);
        if (failed.size) {
          setNotices((current) => current.map((item) => failed.has(item.id) ? { ...item, read: false } : item));
          setLoadError("部分活动通知状态暂时未保存");
        }
        onRefreshUnread?.();
      }
    } catch (reason) { setLoadError(friendlyChatError(reason, "活动通知暂时未加载")); }
  }, [onRefreshUnread]);

  useEffect(() => { void loadInitial(); }, [loadInitial]);

  const loadOlder = async () => {
    const beforeID = notices.at(-1)?.id;
    if (!beforeID || !hasMore || historyLoading) return;
    setHistoryLoading(true);
    try {
      const page = await portalApi.notifications(20, { category: "activity", before_id: beforeID });
      const visible = visibleNotificationsForRow("activity", page.items, activePromotionTitles(activities));
      const unreadIDs = new Set(visible.filter((item) => !item.read).map((item) => item.id));
      setNotices((current) => [...current, ...page.items.filter((item) => !current.some((existing) => existing.id === item.id)).map((item) => unreadIDs.has(item.id) ? { ...item, read: true } : item)]);
      setHasMore(page.has_more);
      setLoadError("");
      if (unreadIDs.size) {
        const failed = await markNotificationRowsRead(visible);
        if (failed.size) {
          setNotices((current) => current.map((item) => failed.has(item.id) ? { ...item, read: false } : item));
          setLoadError("部分活动通知状态暂时未保存");
        }
        onRefreshUnread?.();
      }
    } catch (reason) {
      setLoadError(friendlyChatError(reason, "更早活动暂时无法加载"));
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
      void portalApi.markRead(card.notification.id).catch((reason) => {
        setNotices((current) => current.map((item) => item.id === card.notification?.id ? { ...item, read: false } : item));
        setLoadError(friendlyChatError(reason, "通知状态暂时未保存"));
      });
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
        {loadError && <ChatRetryState message={loadError} onRetry={() => void loadInitial()} />}
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
