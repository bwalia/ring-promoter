"use client";

import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CircleDot } from "lucide-react";
import { CommandPalette } from "@/components/command-palette";
import { Dashboard } from "@/components/dashboard/dashboard";
import { NoAppSelected } from "@/components/dashboard/no-app-selected";
import { ShortcutsDialog } from "@/components/shortcuts-dialog";
import { Sidebar } from "@/components/sidebar";
import { Topbar } from "@/components/topbar";
import { TokenGate } from "@/components/token-gate";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
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

export function AppShell() {
  const token = useAuthStore((s) => s.token);
  const selectedApp = usePrefsStore((s) => s.selectedApp);
  const selectApp = usePrefsStore((s) => s.selectApp);
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

  // Deep link: /?app=<name> selects that app on load; keep the URL in sync so
  // the current view is always shareable.
  useEffect(() => {
    if (!mounted) return;
    const fromUrl = new URLSearchParams(window.location.search).get("app");
    if (fromUrl && fromUrl !== usePrefsStore.getState().selectedApp) {
      selectApp(fromUrl);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mounted]);

  useEffect(() => {
    if (!mounted) return;
    const url = selectedApp
      ? `?app=${encodeURIComponent(selectedApp)}`
      : window.location.pathname;
    window.history.replaceState(null, "", url);
  }, [mounted, selectedApp]);

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
          {selectedApp ? <Dashboard app={selectedApp} /> : <NoAppSelected />}
        </main>
      </div>

      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
      <ShortcutsDialog />
    </div>
  );
}
