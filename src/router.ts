import { useCallback, useEffect, useState } from "react";
import type { Tab } from "./types";

export type ChatView =
  | "list"
  | "notices"
  | "notices-system"
  | "notices-activity"
  | "group"
  | "service";

export type AppRoute =
  | { kind: "login" }
  | { kind: "room" }
  | { kind: "tab"; tab: Exclude<Tab, "chat"> }
  | { kind: "chat"; tab: "chat"; view: ChatView }
  | { kind: "game"; gameId: string };

const tabPaths: Record<Tab, string> = {
  lobby: "/lobby",
  shop: "/wallet",
  chat: "/messages",
  profile: "/profile",
};

function currentPath() {
  return window.location.pathname.replace(/\/+$/, "") || "/";
}

export function parseRoute(pathname: string): AppRoute {
  const path = pathname.replace(/\/+$/, "") || "/";
  if (path === "/" || path === "/login") return { kind: "login" };
  if (path === "/room") return { kind: "room" };
  if (path === "/lobby") return { kind: "tab", tab: "lobby" };
  if (path === "/wallet" || path === "/shop")
    return { kind: "tab", tab: "shop" };
  if (path === "/profile") return { kind: "tab", tab: "profile" };
  if (path === "/messages/notices")
    return { kind: "chat", tab: "chat", view: "notices" };
  if (path === "/messages/notices-system")
    return { kind: "chat", tab: "chat", view: "notices-system" };
  if (path === "/messages/notices-activity")
    return { kind: "chat", tab: "chat", view: "notices-activity" };
  if (path === "/messages/group")
    return { kind: "chat", tab: "chat", view: "group" };
  if (path === "/messages/service")
    return { kind: "chat", tab: "chat", view: "service" };
  if (path === "/messages") return { kind: "chat", tab: "chat", view: "list" };
  if (path.startsWith("/games/"))
    return {
      kind: "game",
      gameId: decodeURIComponent(path.slice("/games/".length)),
    };
  return { kind: "login" };
}

export const pathForLogin = () => "/login";
export const pathForRoom = () => "/room";

export function pathForTab(tab: Tab) {
  return tabPaths[tab];
}

export function pathForChat(view: ChatView) {
  return view === "list" ? tabPaths.chat : `/messages/${view}`;
}

export function pathForGame(gameId: string) {
  return `/games/${encodeURIComponent(gameId)}`;
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

  return { route: parseRoute(pathname), navigate };
}
