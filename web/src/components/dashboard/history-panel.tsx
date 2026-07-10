"use client";

import { useMemo, useState } from "react";
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
      <div className="flex flex-wrap items-center gap-2 border-b p-3">
        <h2 className="mr-auto text-sm font-semibold">History</h2>
        <div className="relative">
          <Search
            aria-hidden
            className="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Search…"
            className="h-8 w-36 pl-7 text-sm"
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
        <div className="flex flex-wrap items-center gap-2 border-b bg-muted/30 p-3">
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
        <div className="space-y-2 p-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : error && !history ? (
        <div className="p-3">
          <ErrorState
            title="Failed to load history"
            message={error.message}
            onRetry={onRetry}
          />
        </div>
      ) : shown.length === 0 ? (
        <p className="p-6 text-center text-sm text-muted-foreground">
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
                className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5 text-sm"
              >
                <ResultIcon result={h.result} message={h.message} />
                <ActionBadge action={h.action} />
                <Badge variant="outline" className="font-mono text-[10px]">
                  {h.ring}
                </Badge>
                <span className="flex min-w-0 items-center gap-1 font-mono text-xs">
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
                <span className="ml-auto flex items-center gap-3">
                  <span className="hidden max-w-64 truncate text-xs text-muted-foreground md:inline">
                    {h.message}
                  </span>
                  <RelativeTime
                    iso={h.created_at}
                    className="text-xs text-muted-foreground"
                  />
                  {h.result === "failure" && apps?.ai_enabled && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-6 shrink-0"
                      aria-label="Diagnose with AI"
                      title="Diagnose with AI"
                      onClick={() => {
                        if (diagnosing === h.id) {
                          setDiagnosing(null);
                          return;
                        }
                        setDiagnosing(h.id);
                        // Already-stored answers just expand; anything else
                        // (re)starts the server-side generation.
                        if (!h.diagnosis) diagnose.mutate(h.id);
                      }}
                    >
                      <Sparkles
                        aria-hidden
                        className={cn(
                          "size-3.5",
                          diagnosing === h.id && "text-primary",
                        )}
                      />
                    </Button>
                  )}
                </span>
                {diagnosing === h.id && (
                  <HistoryDiagnosis
                    app={app}
                    entry={h}
                    onRetry={() => diagnose.mutate(h.id)}
                  />
                )}
              </li>
            ))}
          </ol>
          {filtered.length > limit && (
            <div className="border-t p-2 text-center">
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
 * Expanded AI explanation of one failed history entry. History rows carry no
 * step logs, so the model works from the recorded summary; the answer is
 * stored server-side and shared by every user.
 */
function HistoryDiagnosis({
  app,
  entry,
  onRetry,
}: {
  app: string;
  entry: HistoryEntry;
  onRetry: () => void;
}) {
  // Polls while the model runs; entry.diagnosis covers already-stored answers.
  const { data } = useHistoryDiagnosis(app, entry.diagnosis ? null : entry.id);
  const status = entry.diagnosis
    ? "done"
    : (data?.diagnosis_status ?? "running");
  const text = entry.diagnosis || data?.diagnosis;

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
          <Button variant="outline" size="sm" onClick={onRetry}>
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
