"use client";

import { HelpCircle, Search, X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

const EXAMPLES: [string, string][] = [
  ["nebula", "name contains “nebula”"],
  ["nebula*", "starts with “nebula”"],
  ["*api", "ends with “api”"],
  ["work?rra", "? is any one character"],
  ["jobshout* pintopets", "either term (space or comma)"],
  ["workstation* -*website*", "leading - excludes"],
];

/**
 * Keyword / wildcard filter for the Rings of Apps stage. Purely presentational:
 * the query lives in the UI store and the matching in `lib/app-filter.ts`.
 */
export function RingsAppFilter({
  value,
  onChange,
  shown,
  total,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  shown: number;
  total: number;
  className?: string;
}) {
  const active = value.trim().length > 0;
  return (
    <div className={cn("pointer-events-auto flex items-center gap-2", className)}>
      <div className="relative w-full sm:w-72">
        <Search
          aria-hidden
          className="pointer-events-none absolute left-2.5 top-1/2 z-10 size-3.5 -translate-y-1/2 text-neutral-500"
        />
        <input
          id="rings-app-filter"
          type="search"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape" && value) {
              e.preventDefault();
              e.stopPropagation();
              onChange("");
            }
          }}
          placeholder="Filter apps — nebula*, *api, -test"
          aria-label="Filter the apps on the rings by keyword or wildcard"
          autoComplete="off"
          spellCheck={false}
          className={cn(
            "h-8 w-full rounded-lg border bg-black/45 pl-8 pr-8 font-mono text-[12px] text-neutral-100 shadow-sm backdrop-blur-md outline-none transition-colors placeholder:font-sans placeholder:text-neutral-500 [&::-webkit-search-cancel-button]:hidden",
            active
              ? "border-emerald-500/50 focus:border-emerald-400/70"
              : "border-white/15 focus:border-white/35",
          )}
        />
        {active && (
          <button
            type="button"
            onClick={() => onChange("")}
            aria-label="Clear the app filter"
            className="absolute right-1.5 top-1/2 flex size-5 -translate-y-1/2 items-center justify-center rounded text-neutral-400 hover:bg-white/10 hover:text-neutral-100"
          >
            <X aria-hidden className="size-3.5" />
          </button>
        )}
      </div>
      <Popover>
        <PopoverTrigger
          aria-label="How the app filter works"
          className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-white/15 bg-black/45 text-neutral-400 backdrop-blur-md transition-colors hover:bg-white/10 hover:text-neutral-100"
        >
          <HelpCircle aria-hidden className="size-3.5" />
        </PopoverTrigger>
        <PopoverContent align="start" className="w-80 text-xs">
          <p className="font-medium">Filter by keyword or wildcard</p>
          <p className="mt-1 text-muted-foreground">
            Matches the app id, its display name and its rings (groups).
            Case-insensitive.
          </p>
          <dl className="mt-3 space-y-1.5">
            {EXAMPLES.map(([q, d]) => (
              <div key={q} className="flex items-baseline justify-between gap-3">
                <dt>
                  <button
                    type="button"
                    onClick={() => onChange(q)}
                    className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] hover:bg-muted/70"
                  >
                    {q}
                  </button>
                </dt>
                <dd className="text-right text-muted-foreground">{d}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-3 text-muted-foreground">
            <kbd className="rounded border bg-muted px-1 font-mono text-[10px]">Esc</kbd>{" "}
            clears the filter.
          </p>
        </PopoverContent>
      </Popover>
      {active && (
        <span
          className="shrink-0 rounded-md border border-white/10 bg-black/45 px-2 py-1 font-mono text-[11px] tabular-nums text-neutral-300 backdrop-blur-md"
          aria-live="polite"
        >
          {shown}/{total}
        </span>
      )}
    </div>
  );
}
