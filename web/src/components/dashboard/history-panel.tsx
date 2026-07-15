"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowRight, ListFilter, Loader2, Search, Sparkles } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { ActionBadge, ResultIcon } from "@/components/status";
import { ErrorState } from "@/components/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useApps,
  useDiagnoseHistory,
  useHistoryDiagnosis,
} from "@/lib/queries";
import type { HistoryEntry } from "@/lib/types";
import { cn } from "@/lib/utils";

const PAGE = 15;

/** Filterable deployment history for the selected app, newest first. */
export function HistoryPanel({
  app,
  history,
  isPending,
  error,
  onRetry,
  className,
}: {
  app: string;
  history: HistoryEntry[] | undefined;
  isPending: boolean;
  error: Error | null;
  onRetry: () => void;
  className?: string;
}) {
  const [action, setAction] = useState("all");
  const [result, setResult] = useState("all");
  const [ring, setRing] = useState("all");
  const [text, setText] = useState("");
  const [limit, setLimit] = useState(PAGE);
  const [showFilters, setShowFilters] = useState(false);
  // History row whose AI diagnosis is expanded (one at a time).
  const [diagnosing, setDiagnosing] = useState<number | null>(null);
  const { data: apps } = useApps();
  const diagnose = useDiagnoseHistory(app);

  const rings = useMemo(
    () => [...new Set((history ?? []).map((h) => h.ring))],
    [history],
  );

  const filtered = useMemo(() => {
    const q = text.trim().toLowerCase();
    return (history ?? []).filter(
      (h) =>
        (action === "all" || h.action === action) &&
        (result === "all" || h.result === result) &&
        (ring === "all" || h.ring === ring) &&
        (!q ||
          h.to_version.toLowerCase().includes(q) ||
          h.from_version.toLowerCase().includes(q) ||
          h.message.toLowerCase().includes(q)),
    );
  }, [history, action, result, ring, text]);

  const shown = filtered.slice(0, limit);
  const hasSelectFilters =
    action !== "all" || result !== "all" || ring !== "all";
  const hasFilters = hasSelectFilters || !!text.trim();

  return (
    <section
      className={cn("rounded-xl border bg-card", className)}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-3">
        <h2 className="text-sm font-semibold">History</h2>
        {history && history.length > 0 && (
          <p className="mr-auto text-xs text-muted-foreground">
            {hasFilters ? `${filtered.length} of ${history.length}` : history.length}{" "}
            deployment{history.length === 1 ? "" : "s"}
          </p>
        )}
        <div className="relative ml-auto">
          <Search
            aria-hidden
            className="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Search…"
            className="h-8 w-40 pl-7 text-sm md:w-56"
            aria-label="Search history"
          />
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="relative size-8"
          aria-label="Toggle filters"
          onClick={() => setShowFilters((v) => !v)}
        >
          <ListFilter aria-hidden className="size-4" />
          {hasSelectFilters && (
            <span className="absolute right-1 top-1 size-1.5 rounded-full bg-primary" />
          )}
        </Button>
      </div>

      {showFilters && (
        <div className="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-4 py-3">
          <Select value={action} onValueChange={setAction}>
            <SelectTrigger size="sm" className="w-28" aria-label="Filter by action">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All actions</SelectItem>
              <SelectItem value="seed">Seed</SelectItem>
              <SelectItem value="promote">Promote</SelectItem>
              <SelectItem value="rollback">Rollback</SelectItem>
            </SelectContent>
          </Select>
          <Select value={result} onValueChange={setResult}>
            <SelectTrigger size="sm" className="w-28" aria-label="Filter by result">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All results</SelectItem>
              <SelectItem value="success">Success</SelectItem>
              <SelectItem value="failure">Failure</SelectItem>
            </SelectContent>
          </Select>
          <Select value={ring} onValueChange={setRing}>
            <SelectTrigger size="sm" className="w-24" aria-label="Filter by ring">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All rings</SelectItem>
              {rings.map((r) => (
                <SelectItem key={r} value={r}>
                  {r}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {isPending ? (
        <div className="space-y-2 p-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-11 w-full" />
          ))}
        </div>
      ) : error && !history ? (
        <div className="p-4">
          <ErrorState
            title="Failed to load history"
            message={error.message}
            onRetry={onRetry}
          />
        </div>
      ) : shown.length === 0 ? (
        <p className="p-8 text-center text-sm text-muted-foreground">
          {hasFilters
            ? "Nothing matches these filters."
            : "No deployments recorded yet."}
        </p>
      ) : (
        <>
          <ol className="divide-y">
            {shown.map((h) => (
              <li
                key={h.id}
                className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 text-sm"
              >
                <ResultIcon result={h.result} message={h.message} />
                <span className="w-24 shrink-0">
                  <ActionBadge action={h.action} />
                </span>
                <span className="w-14 shrink-0">
                  <Badge variant="outline" className="font-mono text-[10px]">
                    {h.ring}
                  </Badge>
                </span>
                <span className="flex min-w-0 basis-60 items-center gap-1 font-mono text-xs">
                  {h.from_version ? (
                    <>
                      <span className="max-w-28 truncate text-muted-foreground">
                        {h.from_version}
                      </span>
                      <ArrowRight
                        aria-hidden
                        className="size-3 shrink-0 text-muted-foreground"
                      />
                    </>
                  ) : null}
                  <span className="max-w-32 truncate font-medium">
                    {h.to_version || "—"}
                  </span>
                </span>
                <span
                  className={cn(
                    "hidden min-w-0 flex-1 truncate text-xs md:inline",
                    h.result === "failure"
                      ? "text-status-critical"
                      : "text-muted-foreground",
                  )}
                >
                  {h.message}
                </span>
                <span className="ml-auto flex shrink-0 items-center gap-3">
                  {h.result === "failure" && apps?.ai_enabled && (
                    <Button
                      variant="outline"
                      size="xs"
                      aria-expanded={diagnosing === h.id}
                      onClick={() =>
                        setDiagnosing(diagnosing === h.id ? null : h.id)
                      }
                    >
                      <Sparkles
                        aria-hidden
                        className={cn(
                          "size-3",
                          diagnosing === h.id && "text-primary",
                        )}
                      />
                      AI Diagnose
                    </Button>
                  )}
                  <RelativeTime
                    iso={h.created_at}
                    className="inline-block min-w-16 text-right text-xs tabular-nums text-muted-foreground"
                  />
                </span>
                {diagnosing === h.id && (
                  <HistoryDiagnosis
                    app={app}
                    entry={h}
                    onStart={() => diagnose.mutate(h.id)}
                  />
                )}
              </li>
            ))}
          </ol>
          {filtered.length > limit && (
            <div className="border-t p-2.5 text-center">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setLimit((l) => l + PAGE)}
              >
                Show more ({filtered.length - limit} older)
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  );
}

/**
 * Expanded AI explanation of one failed history entry. Recent failures include
 * the step logs saved when they happened; older entries fall back to the
 * recorded summary. The answer is stored server-side, shared by every user.
 *
 * Expanding starts a diagnosis ONLY when the server reports none has run yet;
 * a previously failed one shows its error and waits for "Try again" instead
 * of silently restarting the model call.
 */
function HistoryDiagnosis({
  app,
  entry,
  onStart,
}: {
  app: string;
  entry: HistoryEntry;
  onStart: () => void;
}) {
  // Polls while the model runs; entry.diagnosis covers already-stored answers.
  const { data } = useHistoryDiagnosis(app, entry.diagnosis ? null : entry.id);
  const status = entry.diagnosis ? "done" : data?.diagnosis_status;
  const text = entry.diagnosis || data?.diagnosis;

  const started = useRef(false);
  useEffect(() => {
    if (status === "none" && !started.current) {
      started.current = true;
      onStart();
    }
  }, [status, onStart]);

  return (
    <div className="basis-full rounded-md bg-muted/40 px-3 py-2">
      {status === "done" && text ? (
        <>
          <p className="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Sparkles aria-hidden className="size-3" />
            AI diagnosis
          </p>
          <p className="text-sm leading-relaxed whitespace-pre-wrap">{text}</p>
        </>
      ) : status === "failed" ? (
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs text-status-critical">
            Diagnosis failed: {data?.diagnosis_error ?? "unknown error"}
          </p>
          <Button variant="outline" size="sm" onClick={onStart}>
            Try again
          </Button>
        </div>
      ) : (
        <p className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 aria-hidden className="size-3.5 animate-spin" />
          Analyzing failure — can take a minute…
        </p>
      )}
    </div>
  );
}
