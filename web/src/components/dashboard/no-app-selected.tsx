"use client";

import { CircleDashed } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useApps, useAppTitle } from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";

export function NoAppSelected() {
  const { data } = useApps();
  const title = useAppTitle();
  const apps = data?.apps ?? [];
  const favorites = usePrefsStore((s) => s.favorites);
  const selectApp = usePrefsStore((s) => s.selectApp);

  // Offer favorites first, then the rest.
  const suggestions = [
    ...favorites.filter((a) => apps.includes(a)),
    ...apps.filter((a) => !favorites.includes(a)),
  ].slice(0, 5);

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-center">
      <CircleDashed aria-hidden className="size-10 text-muted-foreground" />
      <div>
        <h2 className="font-display text-lg font-bold tracking-tight">Select an application</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Pick one from the sidebar, or press{" "}
          <kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs">
            ⌘K
          </kbd>{" "}
          to search.
        </p>
      </div>
      {suggestions.length > 0 && (
        <div className="flex flex-wrap items-center justify-center gap-2">
          {suggestions.map((app) => (
            <Button
              key={app}
              variant="outline"
              size="sm"
              onClick={() => selectApp(app)}
            >
              {title(app)}
            </Button>
          ))}
        </div>
      )}
    </div>
  );
}
