"use client";

import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import {
  Keyboard,
  LogOut,
  Menu,
  MoreVertical,
  Pause,
  Play,
  RotateCw,
  Search,
  Star,
} from "lucide-react";
import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuthStore, usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";
import { cn } from "@/lib/utils";

export function Topbar() {
  const selectedApp = usePrefsStore((s) => s.selectedApp);
  const favorites = usePrefsStore((s) => s.favorites);
  const toggleFavorite = usePrefsStore((s) => s.toggleFavorite);
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  const setAutoRefresh = usePrefsStore((s) => s.setAutoRefresh);
  const signOut = useAuthStore((s) => s.signOut);
  const setPaletteOpen = useUiStore((s) => s.setPaletteOpen);
  const setShortcutsOpen = useUiStore((s) => s.setShortcutsOpen);
  const setMobileNavOpen = useUiStore((s) => s.setMobileNavOpen);
  const queryClient = useQueryClient();
  const fetching = useIsFetching() > 0;

  const favorite = !!selectedApp && favorites.includes(selectedApp);

  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b px-3 md:px-4">
      <Button
        variant="ghost"
        size="icon"
        className="lg:hidden"
        aria-label="Open navigation"
        onClick={() => setMobileNavOpen(true)}
      >
        <Menu aria-hidden className="size-5" />
      </Button>

      <div className="flex min-w-0 items-center gap-1.5">
        <h1 className="truncate text-sm font-semibold md:text-base">
          {selectedApp ?? "Dashboard"}
        </h1>
        {selectedApp && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-7"
                aria-label={favorite ? "Remove favorite" : "Add to favorites"}
                onClick={() => toggleFavorite(selectedApp)}
              >
                <Star
                  aria-hidden
                  className={cn(
                    "size-4",
                    favorite
                      ? "fill-status-warning text-status-warning"
                      : "text-muted-foreground",
                  )}
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {favorite ? "Remove favorite" : "Add to favorites"}
            </TooltipContent>
          </Tooltip>
        )}
      </div>

      <div className="ml-auto flex items-center gap-1.5">
        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          className="hidden h-8 w-56 items-center gap-2 rounded-md border bg-muted/40 px-2.5 text-sm text-muted-foreground hover:bg-muted md:flex"
        >
          <Search aria-hidden className="size-4" />
          <span className="flex-1 text-left">Search…</span>
          <kbd className="rounded border bg-background px-1.5 font-mono text-[10px]">
            ⌘K
          </kbd>
        </button>
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          aria-label="Search"
          onClick={() => setPaletteOpen(true)}
        >
          <Search aria-hidden className="size-4" />
        </Button>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={
                autoRefresh ? "Pause live updates" : "Resume live updates"
              }
              onClick={() => setAutoRefresh(!autoRefresh)}
            >
              {autoRefresh ? (
                <Pause aria-hidden className="size-4" />
              ) : (
                <Play aria-hidden className="size-4 text-status-warning" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {autoRefresh ? "Live updates on — pause" : "Live updates paused — resume"}
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Refresh now"
              onClick={() => queryClient.invalidateQueries()}
            >
              <RotateCw
                aria-hidden
                className={cn("size-4", fetching && "animate-spin")}
              />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Refresh now (r)</TooltipContent>
        </Tooltip>

        <ThemeToggle />

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="More options">
              <MoreVertical aria-hidden className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => setShortcutsOpen(true)}>
              <Keyboard aria-hidden className="size-4" /> Keyboard shortcuts
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={signOut}>
              <LogOut aria-hidden className="size-4" /> Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
