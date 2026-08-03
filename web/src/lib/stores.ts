"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AppGroup } from "@/lib/types";

// ---- auth ----

interface AuthState {
  token: string | null;
  setToken: (token: string) => void;
  signOut: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      setToken: (token) => set({ token }),
      signOut: () => set({ token: null }),
    }),
    { name: "rp-auth" },
  ),
);

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
