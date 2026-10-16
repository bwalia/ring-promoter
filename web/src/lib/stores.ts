"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AppGroup } from "@/lib/types";

// ---- clusters and auth ----
//
// Each cluster is a Ring Promoter server. "This server" (the origin the
// console was loaded from) is always present; others are reached cross-origin
// with their own bearer token (the server allows that; see RP_CORS_ORIGINS).
// Tokens stay in this browser's localStorage, as the single token always has.

export const LOCAL_CLUSTER_ID = "local";

export const CLUSTER_COLORS = [
  "chart-1",
  "chart-2",
  "chart-3",
  "chart-4",
  "chart-5",
] as const;
export type ClusterColor = (typeof CLUSTER_COLORS)[number];

export interface Cluster {
  id: string;
  name: string;
  /** Origin such as https://rp.example.com; "" means this server. */
  url: string;
  color: ClusterColor;
}

export const localCluster = (): Cluster => ({
  id: LOCAL_CLUSTER_ID,
  name: "This server",
  url: "",
  color: "chart-1",
});

/**
 * Normalise what an operator typed into an origin: add https:// when no
 * scheme is given, and drop any path, query or trailing slash. Returns null
 * for anything that is not an http(s) URL.
 */
export function normaliseClusterUrl(raw: string): string | null {
  const text = raw.trim();
  if (!text) return null;
  try {
    const u = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `https://${text}`);
    if (u.protocol !== "https:" && u.protocol !== "http:") return null;
    if (!u.hostname) return null;
    return u.origin;
  } catch {
    return null;
  }
}

/** A readable default name from a cluster URL: rp.int.example.com → "int". */
export function defaultClusterName(url: string): string {
  try {
    const host = new URL(url).hostname;
    if (/^[\d.]+$/.test(host) || !host.includes(".")) return host;
    const generic = new Set(["rp", "www", "api", "ring-promoter", "ringpromoter", "app", "console"]);
    const labels = host.split(".");
    const pick = labels.slice(0, -2).find((l) => !generic.has(l)) ?? labels[labels.length - 2];
    return pick.charAt(0).toUpperCase() + pick.slice(1);
  } catch {
    return url;
  }
}

interface AuthState {
  /** Token for the active cluster (null shows the token gate). */
  token: string | null;
  clusters: Cluster[];
  activeId: string;
  tokens: Record<string, string>;

  setToken: (token: string) => void;
  /** Forget the active cluster's token. */
  signOut: () => void;
  switchCluster: (id: string) => void;
  /** Save a cluster (new or edited). A null token keeps the saved one. */
  saveCluster: (cluster: Cluster, token: string | null) => void;
  removeCluster: (id: string) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      clusters: [localCluster()],
      activeId: LOCAL_CLUSTER_ID,
      tokens: {},

      setToken: (token) =>
        set((s) => ({ token, tokens: { ...s.tokens, [s.activeId]: token } })),

      signOut: () =>
        set((s) => {
          const tokens = { ...s.tokens };
          delete tokens[s.activeId];
          return { token: null, tokens };
        }),

      switchCluster: (id) =>
        set((s) =>
          s.clusters.some((c) => c.id === id)
            ? { activeId: id, token: s.tokens[id] ?? null }
            : {},
        ),

      saveCluster: (cluster, token) =>
        set((s) => {
          const exists = s.clusters.some((c) => c.id === cluster.id);
          const clusters = exists
            ? s.clusters.map((c) => (c.id === cluster.id ? cluster : c))
            : [...s.clusters, cluster];
          const tokens = token ? { ...s.tokens, [cluster.id]: token } : s.tokens;
          return {
            clusters,
            tokens,
            token: cluster.id === s.activeId ? (tokens[cluster.id] ?? null) : s.token,
          };
        }),

      removeCluster: (id) =>
        set((s) => {
          if (id === LOCAL_CLUSTER_ID) return {};
          const tokens = { ...s.tokens };
          delete tokens[id];
          const clusters = s.clusters.filter((c) => c.id !== id);
          if (s.activeId !== id) return { clusters, tokens };
          return {
            clusters,
            tokens,
            activeId: LOCAL_CLUSTER_ID,
            token: tokens[LOCAL_CLUSTER_ID] ?? null,
          };
        }),
    }),
    {
      name: "rp-auth",
      // v0 stored a single { token } for this server.
      version: 1,
      migrate: (persisted, version) => {
        const p = (persisted ?? {}) as Partial<AuthState>;
        if (version < 1) {
          const token = p.token ?? null;
          return {
            token,
            clusters: [localCluster()],
            activeId: LOCAL_CLUSTER_ID,
            tokens: token ? { [LOCAL_CLUSTER_ID]: token } : {},
          } as AuthState;
        }
        return p as AuthState;
      },
      partialize: (s) => ({
        token: s.token,
        clusters: s.clusters,
        activeId: s.activeId,
        tokens: s.tokens,
      }),
    },
  ),
);

/** The active cluster; falls back to this server. */
export function activeCluster(s: Pick<AuthState, "clusters" | "activeId">): Cluster {
  return s.clusters.find((c) => c.id === s.activeId) ?? localCluster();
}

/** Prefix for API paths on the active cluster ("" for this server). */
export function activeBaseUrl(): string {
  return activeCluster(useAuthStore.getState()).url;
}

// ---- UI preferences (all persisted) ----

interface PrefsState {
  selectedApp: string | null;
  /** When set, the group page is shown instead of the app dashboard. */
  selectedGroup: string | null;
  favorites: string[];
  recents: string[];
  /**
   * LEGACY: groups now live on the server (see lib/queries.ts useGroups).
   * This field only holds groups created by older builds until the one-time
   * migration in AppShell pushes them to the server and clears it.
   */
  groups: AppGroup[];
  collapsed: Record<string, boolean>;
  autoRefresh: boolean;

  selectApp: (app: string) => void;
  selectGroup: (id: string | null) => void;
  toggleFavorite: (app: string) => void;
  clearGroups: () => void;
  toggleCollapsed: (key: string) => void;
  setAutoRefresh: (on: boolean) => void;
}

export const usePrefsStore = create<PrefsState>()(
  persist(
    (set) => ({
      selectedApp: null,
      selectedGroup: null,
      favorites: [],
      recents: [],
      groups: [],
      collapsed: {},
      autoRefresh: true,

      selectApp: (app) =>
        set((s) => ({
          selectedApp: app,
          selectedGroup: null,
          recents: [app, ...s.recents.filter((a) => a !== app)].slice(0, 5),
        })),

      selectGroup: (id) =>
        set(
          id
            ? { selectedGroup: id, selectedApp: null }
            : { selectedGroup: null, selectedApp: null },
        ),

      toggleFavorite: (app) =>
        set((s) => ({
          favorites: s.favorites.includes(app)
            ? s.favorites.filter((a) => a !== app)
            : [...s.favorites, app],
        })),

      clearGroups: () => set({ groups: [] }),

      toggleCollapsed: (key) =>
        set((s) => ({
          collapsed: { ...s.collapsed, [key]: !s.collapsed[key] },
        })),

      setAutoRefresh: (on) => set({ autoRefresh: on }),
    }),
    {
      name: "rp-prefs",
      // Bump when navigation storage shape changes so old localStorage
      // (which persisted selectedGroup and bounced bare `/` to ?group=) is reset.
      version: 1,
      // selectedApp / selectedGroup are navigation (URL is the source of truth).
      // Persisting them made bare `/` snap back to the last ?group= after
      // rehydrate and look like an auto-redirect.
      partialize: (s) => ({
        favorites: s.favorites,
        recents: s.recents,
        groups: s.groups,
        collapsed: s.collapsed,
        autoRefresh: s.autoRefresh,
      }),
      migrate: (persisted) => {
        const p = (persisted ?? {}) as Partial<PrefsState>;
        return {
          favorites: p.favorites ?? [],
          recents: p.recents ?? [],
          groups: p.groups ?? [],
          collapsed: p.collapsed ?? {},
          autoRefresh: p.autoRefresh ?? true,
          selectedApp: null,
          selectedGroup: null,
        };
      },
      // Drop any legacy navigation keys still sitting in older localStorage.
      merge: (persisted, current) => {
        const p = (persisted ?? {}) as Partial<PrefsState>;
        return {
          ...current,
          ...p,
          selectedApp: null,
          selectedGroup: null,
        };
      },
    },
  ),
);

// ---- job cards ----
//
// Jobs themselves are SHARED server state (GET /api/jobs): everyone sees the
// same running/finished cards. Only the dismissal of a finished card is a
// per-browser choice, tracked here.

interface JobsState {
  /** Keys (`app:jobId:startedAt`) of job cards this browser dismissed. */
  dismissed: string[];
  dismiss: (key: string) => void;
}

export const useJobsStore = create<JobsState>()(
  persist(
    (set) => ({
      dismissed: [],
      dismiss: (key) =>
        set((s) => ({
          dismissed: [key, ...s.dismissed.filter((k) => k !== key)].slice(
            0,
            100,
          ),
        })),
    }),
    { name: "rp-jobs" },
  ),
);
