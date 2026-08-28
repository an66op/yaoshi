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
  | { kind: "tab"; tab: Exclude<Tab, "chat">; walletAction?: WalletActionSlug; returnGameId?: string }
  | { kind: "chat"; tab: "chat"; view: ChatView; returnGameId?: string; planGameId?: string }
  | { kind: "results"; gameId?: string; returnGameId?: string }
  | { kind: "game"; gameId: string; quickMenu?: boolean };

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
  if (path === "/lobby") return { kind: "tab", tab: "lobby" };
  const returnGameId = query.get("from_game")?.trim() || undefined;
  if (path === "/wallet" || path === "/shop")
    return { kind: "tab", tab: "shop", returnGameId };
  if (path.startsWith("/wallet/")) {
    const slug = path.slice("/wallet/".length) as WalletActionSlug;
    return { kind: "tab", tab: "shop", walletAction: slug, returnGameId };
  }
  if (path === "/profile") return { kind: "tab", tab: "profile" };
  if (path === "/results") return { kind: "results", gameId: query.get("game")?.trim() || undefined, returnGameId };
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
    return { kind: "chat", tab: "chat", view: "service", returnGameId: returnGameId || undefined };
  }
  if (path === "/messages") return { kind: "chat", tab: "chat", view: "list" };
  if (path.startsWith("/games/"))
    return {
      kind: "game",
      gameId: decodeURIComponent(path.slice("/games/".length)),
      quickMenu: query.get("quick_menu") === "1",
    };
  return { kind: "login" };
}

export const pathForLogin = () => "/login";
export const pathForRegister = () => "/register";
export const pathForRoom = () => "/room";

export function pathForTab(tab: Tab) {
  return tabPaths[tab];
}

export function pathForWallet(action?: WalletActionSlug, fromGameId?: string) {
  const path = action ? `/wallet/${action}` : tabPaths.shop;
  return fromGameId ? `${path}?from_game=${encodeURIComponent(fromGameId)}` : path;
}

export function pathForChat(view: ChatView, fromGameId?: string) {
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
  return view === "service" && fromGameId ? `${path}?from_game=${encodeURIComponent(fromGameId)}` : path;
}

export function pathForPlanGame(gameId: string) {
  return `/messages/plans/${encodeURIComponent(gameId)}`;
}

export function pathForGame(gameId: string, quickMenu = false) {
  const path = `/games/${encodeURIComponent(gameId)}`;
  return quickMenu ? `${path}?quick_menu=1` : path;
}

export function pathForResults(gameId?: string, fromGameId?: string) {
  const query = new URLSearchParams();
  if (gameId) query.set("game", gameId);
  if (fromGameId) query.set("from_game", fromGameId);
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
