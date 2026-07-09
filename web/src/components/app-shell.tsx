"use client";

import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CircleDot } from "lucide-react";
import { CommandPalette } from "@/components/command-palette";
import { Dashboard } from "@/components/dashboard/dashboard";
import { GroupView } from "@/components/group-view";
import { NoAppSelected } from "@/components/dashboard/no-app-selected";
import { ShortcutsDialog } from "@/components/shortcuts-dialog";
import { Sidebar } from "@/components/sidebar";
import { Topbar } from "@/components/topbar";
import { TokenGate } from "@/components/token-gate";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useGroups } from "@/lib/queries";
import { useAuthStore, usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";

function isEditable(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return (
    el instanceof HTMLInputElement ||
    el instanceof HTMLTextAreaElement ||
    el instanceof HTMLSelectElement ||
    el.isContentEditable
  );
}

// Groups created by pre-server-side builds live in localStorage; push them to
// the server exactly once per page load, then clear the legacy field.
let migratedLegacyGroups = false;

export function AppShell() {
  const token = useAuthStore((s) => s.token);
  const selectedApp = usePrefsStore((s) => s.selectedApp);
  const selectApp = usePrefsStore((s) => s.selectApp);
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const selectedGroup = usePrefsStore((s) => s.selectedGroup);
  const groupsQuery = useGroups();
  const groups = groupsQuery.data ?? [];
  const activeGroup = selectedGroup
    ? groups.find((g) => g.id === selectedGroup)
    : undefined;
  const { paletteOpen, setPaletteOpen, setShortcutsOpen } = useUiStore();
  const mobileNavOpen = useUiStore((s) => s.mobileNavOpen);
  const setMobileNavOpen = useUiStore((s) => s.setMobileNavOpen);
  const queryClient = useQueryClient();

  // The stores are persisted in localStorage, so the first client render can
  // differ from the prerendered HTML. Render a splash until hydrated.
  const mounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );

  // Deep link: /?app=<name> or /?group=<id> selects that view on load; keep
  // the URL in sync so the current view is always shareable. Group links are
  // now meaningful across browsers since groups live on the server.
  useEffect(() => {
    if (!mounted) return;
    const params = new URLSearchParams(window.location.search);
    const groupParam = params.get("group");
    const appParam = params.get("app");
    if (groupParam) {
      selectGroup(groupParam);
    } else if (appParam) {
      // Unconditionally: selectApp also clears a persisted selectedGroup, so
      // an ?app= link always wins over whatever view was open last time.
      selectApp(appParam);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mounted]);

  // A selected group that no longer exists on the server (deleted elsewhere,
  // or a dead deep link) falls back to the app view instead of a blank page.
  useEffect(() => {
    if (
      selectedGroup &&
      groupsQuery.isSuccess &&
      !groupsQuery.data.some((g) => g.id === selectedGroup)
    ) {
      selectGroup(null);
    }
  }, [selectedGroup, groupsQuery.isSuccess, groupsQuery.data, selectGroup]);

  // One-time migration of legacy browser-local groups to the server.
  useEffect(() => {
    if (!mounted || !token || migratedLegacyGroups) return;
    const legacy = usePrefsStore.getState().groups;
    if (!legacy?.length) return;
    migratedLegacyGroups = true;
    (async () => {
      try {
        const { apps: known } = await api.apps();
        for (const g of legacy) {
          await api.createGroup(
            g.name,
            g.apps.filter((a) => known.includes(a)),
          );
        }
        usePrefsStore.getState().clearGroups();
        queryClient.invalidateQueries({ queryKey: ["groups"] });
        toast.success(
          `Moved ${legacy.length} group${legacy.length === 1 ? "" : "s"} to the server`,
          { description: "Groups are now shared with everyone on this server." },
        );
      } catch {
        migratedLegacyGroups = false; // retry on the next page load
      }
    })();
  }, [mounted, token, queryClient]);

  useEffect(() => {
    if (!mounted) return;
    const url = activeGroup
      ? `?group=${encodeURIComponent(activeGroup.id)}`
      : selectedApp
        ? `?app=${encodeURIComponent(selectedApp)}`
        : window.location.pathname;
    window.history.replaceState(null, "", url);
  }, [mounted, selectedApp, activeGroup]);

  // Global keyboard shortcuts.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen(!useUiStore.getState().paletteOpen);
        return;
      }
      if (isEditable(e.target) || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "/") {
        e.preventDefault();
        document.getElementById("app-search")?.focus();
      } else if (e.key === "?") {
        e.preventDefault();
        setShortcutsOpen(true);
      } else if (e.key.toLowerCase() === "r") {
        queryClient.invalidateQueries();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [queryClient, setPaletteOpen, setShortcutsOpen]);

  if (!mounted) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <CircleDot
          aria-hidden
          className="size-8 animate-pulse text-muted-foreground"
        />
      </div>
    );
  }

  if (!token) return <TokenGate />;

  return (
    <div className="flex h-dvh overflow-hidden">
      <aside className="hidden w-72 shrink-0 border-r bg-sidebar lg:flex lg:flex-col">
        <Sidebar searchInputId="app-search" />
      </aside>

      <Sheet open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
        <SheetContent side="left" className="w-72 bg-sidebar p-0">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <Sidebar onNavigate={() => setMobileNavOpen(false)} />
        </SheetContent>
      </Sheet>

      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <main className="flex-1 overflow-y-auto">
          {activeGroup ? (
            <GroupView key={activeGroup.id} group={activeGroup} />
          ) : selectedApp ? (
            <Dashboard app={selectedApp} />
          ) : (
            <NoAppSelected />
          )}
        </main>
      </div>

      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
      <ShortcutsDialog />
    </div>
  );
}
