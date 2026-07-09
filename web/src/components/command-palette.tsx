"use client";

import { useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import {
  ArrowUpRight,
  Boxes,
  Download,
  Keyboard,
  Layers,
  LogOut,
  Monitor,
  Moon,
  Pause,
  Play,
  Star,
  Sun,
  Tag,
  Undo2,
} from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { useApps, useRings } from "@/lib/queries";
import { useAuthStore, usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";
import type { RingView } from "@/lib/types";

export function CommandPalette({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [query, setQuery] = useState("");
  const { data } = useApps();
  const apps = data?.apps ?? [];
  const selectedApp = usePrefsStore((s) => s.selectedApp);
  const selectApp = usePrefsStore((s) => s.selectApp);
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const selectedGroup = usePrefsStore((s) => s.selectedGroup);
  const groups = usePrefsStore((s) => s.groups);
  const favorites = usePrefsStore((s) => s.favorites);
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  const setAutoRefresh = usePrefsStore((s) => s.setAutoRefresh);
  const signOut = useAuthStore((s) => s.signOut);
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const setShortcutsOpen = useUiStore((s) => s.setShortcutsOpen);
  const { setTheme } = useTheme();
  const queryClient = useQueryClient();
  const { data: rings } = useRings(selectedApp);

  const close = () => {
    onOpenChange(false);
    setQuery("");
  };

  const run = (fn: () => void) => () => {
    fn();
    close();
  };

  // Search deployed versions across every app whose rings are in the cache.
  const versionHits = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q.length < 2) return [];
    const hits: { app: string; ring: string; version: string }[] = [];
    for (const [key, data] of queryClient.getQueriesData<{
      rings: RingView[];
    }>({ queryKey: ["rings"] })) {
      const app = key[1] as string;
      for (const v of data?.rings ?? []) {
        if (v.current_version && v.current_version.toLowerCase().includes(q)) {
          hits.push({ app, ring: v.ring.name, version: v.current_version });
        }
      }
    }
    return hits.slice(0, 8);
  }, [query, queryClient, open]); // eslint-disable-line react-hooks/exhaustive-deps

  const promotable = (rings ?? []).filter((r) => r.can_promote_from);
  const rollbackable = (rings ?? []).filter(
    (r) => r.configured && r.previous_version,
  );

  return (
    <CommandDialog open={open} onOpenChange={(o) => (o ? onOpenChange(o) : close())}>
      <CommandInput
        placeholder="Search apps, versions, actions…"
        value={query}
        onValueChange={setQuery}
      />
      <CommandList>
        <CommandEmpty>No results.</CommandEmpty>

        <CommandGroup heading="Applications">
          {apps.map((app) => (
            <CommandItem
              key={app}
              value={`app ${app}`}
              onSelect={run(() => selectApp(app))}
            >
              <Boxes aria-hidden className="size-4" />
              <span className="truncate">{app}</span>
              {favorites.includes(app) && (
                <Star
                  aria-hidden
                  className="ml-auto size-3 fill-status-warning text-status-warning"
                />
              )}
            </CommandItem>
          ))}
        </CommandGroup>

        {groups.length > 0 && (
          <>
            <CommandSeparator />
            <CommandGroup heading="Groups">
              {groups.map((g) => (
                <CommandItem
                  key={g.id}
                  // id keeps the value unique when two groups share a name
                  // (cmdk keys selection by value); the name keeps it searchable.
                  value={`group ${g.name} ${g.id}`}
                  onSelect={run(() => selectGroup(g.id))}
                >
                  <Layers aria-hidden className="size-4" />
                  <span className="truncate">{g.name}</span>
                  <span className="ml-auto text-xs text-muted-foreground">
                    {g.apps.length} app{g.apps.length === 1 ? "" : "s"}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}

        {versionHits.length > 0 && (
          <>
            <CommandSeparator />
            <CommandGroup heading="Deployed versions">
              {versionHits.map((h) => (
                <CommandItem
                  key={`${h.app}-${h.ring}-${h.version}`}
                  value={`version ${h.app} ${h.ring} ${h.version}`}
                  onSelect={run(() => selectApp(h.app))}
                >
                  <Tag aria-hidden className="size-4" />
                  <span className="truncate font-mono text-xs">{h.version}</span>
                  <span className="ml-auto text-xs text-muted-foreground">
                    {h.app} · {h.ring}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}

        {/* Per-app quick actions are hidden while a group page is showing:
            their dialogs live on the app dashboard, so they could not open
            here (and a queued action would fire later on the wrong view). */}
        {selectedApp && !selectedGroup && (
          <>
            <CommandSeparator />
            <CommandGroup heading={`Actions · ${selectedApp}`}>
              <CommandItem
                value="seed a version"
                onSelect={run(() => setPendingAction({ type: "seed", app: selectedApp }))}
              >
                <Download aria-hidden className="size-4" /> Seed a version…
              </CommandItem>
              {promotable.map((r) => (
                <CommandItem
                  key={`p-${r.ring.name}`}
                  value={`promote ${r.ring.name} ${r.current_version}`}
                  onSelect={run(() =>
                    setPendingAction({ type: "promote", app: selectedApp, fromRing: r.ring.name }),
                  )}
                >
                  <ArrowUpRight aria-hidden className="size-4" />
                  Promote {r.ring.name} ({r.current_version})…
                </CommandItem>
              ))}
              {rollbackable.map((r) => (
                <CommandItem
                  key={`r-${r.ring.name}`}
                  value={`rollback ${r.ring.name} ${r.previous_version}`}
                  onSelect={run(() =>
                    setPendingAction({ type: "rollback", app: selectedApp, ring: r.ring.name }),
                  )}
                >
                  <Undo2 aria-hidden className="size-4" />
                  Roll back {r.ring.name} to {r.previous_version}…
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}

        <CommandSeparator />
        <CommandGroup heading="Preferences">
          <CommandItem value="theme light" onSelect={run(() => setTheme("light"))}>
            <Sun aria-hidden className="size-4" /> Light theme
          </CommandItem>
          <CommandItem value="theme dark" onSelect={run(() => setTheme("dark"))}>
            <Moon aria-hidden className="size-4" /> Dark theme
          </CommandItem>
          <CommandItem value="theme system" onSelect={run(() => setTheme("system"))}>
            <Monitor aria-hidden className="size-4" /> System theme
          </CommandItem>
          <CommandItem
            value="auto refresh live updates"
            onSelect={run(() => setAutoRefresh(!autoRefresh))}
          >
            {autoRefresh ? (
              <Pause aria-hidden className="size-4" />
            ) : (
              <Play aria-hidden className="size-4" />
            )}
            {autoRefresh ? "Pause live updates" : "Resume live updates"}
          </CommandItem>
          <CommandItem
            value="keyboard shortcuts help"
            onSelect={run(() => setShortcutsOpen(true))}
          >
            <Keyboard aria-hidden className="size-4" /> Keyboard shortcuts
          </CommandItem>
          <CommandItem value="sign out" onSelect={run(signOut)}>
            <LogOut aria-hidden className="size-4" /> Sign out
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
