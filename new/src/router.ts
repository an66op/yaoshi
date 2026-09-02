import { useCallback, useEffect, useState } from "react";
import type { Tab } from "./types";

export type ChatView =
  | "list"
  | "system"
  | "winning"
  | "activity"
  | "group"
  | "plans"
  | "plan"
  | "service";

export type WalletActionSlug =
  | "credit"
  | "debit"
  | "channels"
  | "applications"
  | "quota"
  | "bets"
  | "pending-bets"
  | "ledger"
  | "rebate"
  | "welfare"
  | "redpacket"
  | "invite";

export type AppRoute =
  | { kind: "login" }
  | { kind: "register" }
  | { kind: "room" }
  | { kind: "tab"; tab: Exclude<Tab, "chat">; walletAction?: WalletActionSlug; returnGameId?: string; lobbyFilter?: string; returnLobbyFilter?: string }
  | { kind: "chat"; tab: "chat"; view: ChatView; returnGameId?: string; planGameId?: string; returnLobbyFilter?: string }
  | { kind: "results"; gameId?: string; returnGameId?: string; returnLobbyFilter?: string }
  | { kind: "game-guide"; tab: "rules" | "odds" }
  | { kind: "game"; gameId: string; quickMenu?: boolean; returnLobbyFilter?: string };

const tabPaths: Record<Tab, string> = {
  lobby: "/lobby",
  shop: "/wallet",
  chat: "/messages",
  profile: "/profile",
};

function currentPath() {
  const pathname = window.location.pathname.replace(/\/+$/, "") || "/";
  return `${pathname}${window.location.search}`;
}

export function parseRoute(pathname: string): AppRoute {
  const [rawPath, rawQuery = ""] = pathname.split("?", 2);
  const path = rawPath.replace(/\/+$/, "") || "/";
  const query = new URLSearchParams(rawQuery);
  if (path === "/" || path === "/login") return { kind: "login" };
  if (path === "/register") return { kind: "register" };
  if (path === "/room") return { kind: "room" };
  if (path === "/lobby") {
    const lobbyFilter = boundedQueryValue(query, "category");
    return lobbyFilter ? { kind: "tab", tab: "lobby", lobbyFilter } : { kind: "tab", tab: "lobby" };
  }
  const returnGameId = query.get("from_game")?.trim() || undefined;
  const returnLobbyFilter = boundedQueryValue(query, "from_lobby");
  if (path === "/wallet" || path === "/shop")
    return { kind: "tab", tab: "shop", returnGameId, ...(returnLobbyFilter ? { returnLobbyFilter } : {}) };
  if (path.startsWith("/wallet/")) {
    const slug = path.slice("/wallet/".length) as WalletActionSlug;
    return { kind: "tab", tab: "shop", walletAction: slug, returnGameId, ...(returnLobbyFilter ? { returnLobbyFilter } : {}) };
  }
  if (path === "/profile/game-guide") return { kind: "game-guide", tab: query.get("tab") === "odds" ? "odds" : "rules" };
  if (path === "/profile") return { kind: "tab", tab: "profile" };
  if (path === "/results") return { kind: "results", gameId: query.get("game")?.trim() || undefined, returnGameId, ...(returnLobbyFilter ? { returnLobbyFilter } : {}) };
  if (path === "/messages/system")
    return { kind: "chat", tab: "chat", view: "system" };
  if (path === "/messages/account")
    return { kind: "chat", tab: "chat", view: "list" };
  if (path === "/messages/winning")
    return { kind: "chat", tab: "chat", view: "winning" };
  if (path === "/messages/activity")
    return { kind: "chat", tab: "chat", view: "activity" };
  if (path === "/messages/group")
    return { kind: "chat", tab: "chat", view: "group" };
  if (path.startsWith("/messages/plans/"))
    return { kind: "chat", tab: "chat", view: "plan", planGameId: decodeURIComponent(path.slice("/messages/plans/".length)) };
  if (path === "/messages/plans")
    return { kind: "chat", tab: "chat", view: "plans" };
  if (path === "/messages/service") {
    const returnGameId = query.get("from_game")?.trim();
    return { kind: "chat", tab: "chat", view: "service", returnGameId: returnGameId || undefined, ...(returnLobbyFilter ? { returnLobbyFilter } : {}) };
  }
  if (path === "/messages") return { kind: "chat", tab: "chat", view: "list" };
  if (path.startsWith("/games/")) {
    const returnLobbyFilter = boundedQueryValue(query, "from_lobby");
    return {
      kind: "game",
      gameId: decodeURIComponent(path.slice("/games/".length)),
      quickMenu: query.get("quick_menu") === "1",
      ...(returnLobbyFilter ? { returnLobbyFilter } : {}),
    };
  }
  return { kind: "login" };
}

export const pathForLogin = () => "/login";
export const pathForRegister = () => "/register";
export const pathForRoom = () => "/room";

export function pathForTab(tab: Tab) {
  return tabPaths[tab];
}

export function pathForLobby(filter?: string) {
  const value = filter?.trim();
  return value ? `${tabPaths.lobby}?category=${encodeURIComponent(value)}` : tabPaths.lobby;
}

export function pathForWallet(action?: WalletActionSlug, fromGameId?: string, fromLobbyFilter?: string) {
  const path = action ? `/wallet/${action}` : tabPaths.shop;
  return pathWithReturnContext(path, fromGameId, fromLobbyFilter);
}

export function pathForChat(view: ChatView, fromGameId?: string, fromLobbyFilter?: string) {
  const routes: Record<ChatView, string> = {
    list: tabPaths.chat,
    system: "/messages/system",
    winning: "/messages/winning",
    activity: "/messages/activity",
    group: "/messages/group",
    plans: "/messages/plans",
    plan: "/messages/plans",
    service: "/messages/service",
  };
  const path = routes[view];
  return view === "service" ? pathWithReturnContext(path, fromGameId, fromLobbyFilter) : path;
}

export function pathForPlanGame(gameId: string) {
  return `/messages/plans/${encodeURIComponent(gameId)}`;
}

export function pathForGame(gameId: string, quickMenu = false, fromLobbyFilter?: string) {
  const path = `/games/${encodeURIComponent(gameId)}`;
  const query = new URLSearchParams();
  if (quickMenu) query.set("quick_menu", "1");
  if (fromLobbyFilter?.trim()) query.set("from_lobby", fromLobbyFilter.trim());
  const suffix = query.toString();
  return suffix ? `${path}?${suffix}` : path;
}

export function pathForGameGuide(tab: "rules" | "odds" = "rules") {
  return `/profile/game-guide?tab=${tab}`;
}

export function pathForResults(gameId?: string, fromGameId?: string, fromLobbyFilter?: string) {
  const query = new URLSearchParams();
  if (gameId) query.set("game", gameId);
  if (fromGameId) query.set("from_game", fromGameId);
  if (fromLobbyFilter?.trim()) query.set("from_lobby", fromLobbyFilter.trim());
  const suffix = query.toString();
  return `/results${suffix ? `?${suffix}` : ""}`;
}

export function useAppRouter() {
  const [pathname, setPathname] = useState(currentPath);

  useEffect(() => {
    if (window.location.pathname === "/") {
      window.history.replaceState({}, "", "/login");
      setPathname("/login");
    }
    const syncPath = () => setPathname(currentPath());
    window.addEventListener("popstate", syncPath);
    return () => window.removeEventListener("popstate", syncPath);
  }, []);

  const navigate = useCallback((path: string) => {
    if (path === currentPath()) return;
    window.history.pushState({}, "", path);
    setPathname(path);
  }, []);

  const replace = useCallback((path: string) => {
    if (path === currentPath()) return;
    window.history.replaceState(window.history.state, "", path);
    setPathname(path);
  }, []);

  return { route: parseRoute(pathname), pathname, navigate, replace };
}

function boundedQueryValue(query: URLSearchParams, key: string) {
  const value = query.get(key)?.trim();
  return value && value.length <= 40 ? value : undefined;
}

function pathWithReturnContext(path: string, fromGameId?: string, fromLobbyFilter?: string) {
  const query = new URLSearchParams();
  if (fromGameId?.trim()) query.set("from_game", fromGameId.trim());
  if (fromLobbyFilter?.trim()) query.set("from_lobby", fromLobbyFilter.trim());
  const suffix = query.toString();
  return suffix ? `${path}?${suffix}` : path;
}
