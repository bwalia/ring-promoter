"use client";

import { useState } from "react";
import { Gauge, Sparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api";
import { useAppControls, useControlsBrief } from "@/lib/queries";
import type { ControlScore, ControlStatus, HopScore } from "@/lib/types";
import { cn } from "@/lib/utils";

/**
 * Daily Deployment Controls: a weighted scorecard per hop. CI/CD ships what is
 * in Git; this is whether the next ring has actually earned the version.
 */
export function ControlsPanel({ app }: { app: string }) {
  const { data, error, isPending } = useAppControls(app);
  if (error instanceof ApiError && error.status === 404) return null;
  if (isPending || !data) return null;
  if (data.hops.length === 0) return null;

  return (
    <section className="rounded-xl border bg-card" aria-labelledby="controls-heading">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-3">
        <h2 id="controls-heading" className="rp-heading-panel flex items-center gap-2">
          <Gauge aria-hidden className="size-4 text-muted-foreground" />
          Deployment Controls
        </h2>
        <p className="text-xs text-muted-foreground">
          QA, AppSec, windows, tickets, traffic and replica rules — scored for
          the hop in front of you.
        </p>
      </div>
      <div className="divide-y">
        {data.hops.map((hop) => (
          <HopCard
            key={`${hop.from}->${hop.to}`}
            app={app}
            hop={hop}
            ai={data.ai_enabled}
          />
        ))}
      </div>
    </section>
  );
}

function HopCard({
  app,
  hop,
  ai,
}: {
  app: string;
  hop: HopScore;
  ai: boolean;
}) {
  const brief = useControlsBrief(app);
  const [text, setText] = useState<string | null>(null);
  const move = hop.from ? `${hop.from} → ${hop.to}` : `seed ${hop.to}`;

  return (
    <article className="space-y-3 px-4 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-medium">
            {move}
            {hop.version ? (
              <span className="ml-2 font-mono text-muted-foreground">{hop.version}</span>
            ) : null}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">{hop.summary}</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={hop.ready ? "default" : "secondary"}>
            {hop.ready ? "ready" : hop.blockers.length ? "blocked" : "weak"}
          </Badge>
          <span className="tabular-nums text-sm font-medium">{hop.score}/100</span>
        </div>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div
          className={cn(
            "h-full rounded-full",
            hop.ready ? "bg-status-good" : hop.blockers.length ? "bg-status-critical" : "bg-status-warning",
          )}
          style={{ width: `${Math.max(4, hop.score)}%` }}
        />
      </div>
      <ul className="space-y-2">
        {hop.controls
          .filter((c) => c.status !== "skip")
          .map((c) => (
            <ControlRow key={c.id} control={c} />
          ))}
      </ul>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={brief.isPending}
          onClick={() =>
            brief.mutate(
              { from: hop.from, to: hop.to },
              { onSuccess: (res) => setText(res.briefing) },
            )
          }
        >
          <Sparkles aria-hidden className="size-3.5" />
          {ai ? "AI briefing" : "Brief me"}
        </Button>
        {brief.data?.source === "ai" || (text && ai) ? (
          <span className="text-[11px] uppercase tracking-wider text-muted-foreground">
            {brief.data?.source === "ai" ? "model" : "rules"}
          </span>
        ) : null}
      </div>
      {text && (
        <pre className="whitespace-pre-wrap rounded-md bg-muted/50 p-3 text-sm leading-relaxed">
          {text}
        </pre>
      )}
    </article>
  );
}

function ControlRow({ control }: { control: ControlScore }) {
  return (
    <li className="flex gap-3 text-sm">
      <StatusDot status={control.status} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <span className="font-medium">{control.label}</span>
          <span className="tabular-nums text-xs text-muted-foreground">
            {control.score}/100 · w{control.weight}
          </span>
        </div>
        <p className="text-muted-foreground">{control.detail}</p>
        {control.next_action && control.status !== "pass" && (
          <p className="mt-0.5 text-xs">{control.next_action}</p>
        )}
      </div>
    </li>
  );
}

function StatusDot({ status }: { status: ControlStatus }) {
  return (
    <span
      aria-label={status}
      className={cn(
        "mt-1.5 size-2 shrink-0 rounded-full",
        status === "pass" && "bg-status-good",
        status === "warn" && "bg-status-warning",
        status === "block" && "bg-status-critical",
        status === "skip" && "bg-muted-foreground/40",
      )}
    />
  );
}
