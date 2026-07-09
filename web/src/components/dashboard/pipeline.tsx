"use client";

import { Fragment } from "react";
import {
  ArrowUpRight,
  ChevronDown,
  ChevronRight,
  Clock,
  Download,
  History,
  Undo2,
} from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { HealthBadge, ringHealth } from "@/components/status";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { VersionLabel } from "@/components/version-label";
import { useActiveJob, useAutoPromoteMutation } from "@/lib/queries";
import { useUiStore } from "@/lib/ui-store";
import type { RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

export function Pipeline({
  app,
  rings,
  isPending,
}: {
  app: string;
  rings: RingView[] | undefined;
  isPending: boolean;
}) {
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const { running } = useActiveJob(app);

  const nothingDeployed =
    rings && rings.every((r) => !r.configured || !r.current_version);

  return (
    <section className="space-y-3">
      <h2 className="text-sm font-semibold">Promotion pipeline</h2>

      {isPending || !rings ? (
        <div className="flex flex-col gap-2 xl:flex-row">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-52 flex-1 rounded-xl" />
          ))}
        </div>
      ) : (
        <>
          {nothingDeployed && (
            <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-6 text-center sm:flex-row sm:justify-between sm:text-left">
              <div>
                <p className="text-sm font-medium">Nothing deployed yet</p>
                <p className="text-sm text-muted-foreground">
                  Seed a version into a ring to start the pipeline.
                </p>
              </div>
              <Button
                size="sm"
                onClick={() => setPendingAction({ type: "seed" })}
              >
                <Download aria-hidden className="size-4" /> Seed a version
              </Button>
            </div>
          )}

          <div className="flex flex-col items-stretch gap-1.5 xl:flex-row">
            {rings.map((view, i) => (
              <Fragment key={view.ring.name}>
                <RingCard
                  app={app}
                  view={view}
                  prev={i > 0 ? rings[i - 1] : undefined}
                  next={i < rings.length - 1 ? rings[i + 1] : undefined}
                  busy={running}
                />
                {i < rings.length - 1 && (
                  <div className="flex items-center justify-center self-center text-muted-foreground/60">
                    <ChevronDown aria-hidden className="size-4 xl:hidden" />
                    <ChevronRight
                      aria-hidden
                      className="hidden size-4 xl:block"
                    />
                  </div>
                )}
              </Fragment>
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function RingCard({
  app,
  view,
  prev,
  next,
  busy,
}: {
  app: string;
  view: RingView;
  prev?: RingView;
  next?: RingView;
  busy: boolean;
}) {
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const autoPromote = useAutoPromoteMutation(app);
  const health = ringHealth(view);
  const { ring } = view;

  // "When a version lands here healthy, continue to the next ring." Offered on
  // middle rings only: the first ring is fed by seeds, the last has no target.
  const showAutoPromote = view.configured && !!prev && !!next;

  // The version waiting one ring below, ready to be promoted into this ring.
  const candidate =
    view.configured &&
    prev?.can_promote_from &&
    prev.current_version &&
    prev.current_version !== view.current_version
      ? prev.current_version
      : null;

  const drift =
    view.live_version &&
    view.current_version &&
    view.live_version !== view.current_version;

  if (!view.configured) {
    return (
      <div className="flex min-w-0 flex-1 basis-0 flex-col justify-center gap-1 rounded-xl border border-dashed p-4 text-center">
        <p className="text-sm text-muted-foreground">{ring.label}</p>
        <p className="text-xs text-muted-foreground/70">Not configured</p>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex min-w-0 flex-1 basis-0 flex-col gap-3 rounded-xl border bg-card p-4",
        health === "unhealthy" && "border-status-critical/40",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <p className="min-w-0 truncate text-sm font-semibold">{ring.label}</p>
        <HealthBadge
          health={health}
          pulse
          detail={
            health === "unhealthy"
              ? view.live_health_error || "live health check failing"
              : undefined
          }
        />
      </div>

      <div className="min-w-0">
        {view.current_version ? (
          <VersionLabel
            version={view.current_version}
            className="max-w-full text-base font-semibold"
          />
        ) : (
          <p className="text-sm text-muted-foreground">no version deployed</p>
        )}
        <div className="mt-2 space-y-1 text-xs text-muted-foreground">
          {view.previous_version && (
            <div className="flex items-center gap-1.5">
              <History aria-hidden className="size-3 shrink-0" />
              <span className="shrink-0">previous:</span>
              <VersionLabel version={view.previous_version} />
            </div>
          )}
          {view.current_version && (
            <div className="flex items-center gap-1.5">
              <Clock aria-hidden className="size-3 shrink-0" />
              <span className="shrink-0">updated</span>
              <RelativeTime iso={view.updated_at} />
            </div>
          )}
          {drift && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex max-w-full items-center gap-1 rounded-md bg-status-warning/15 px-1.5 py-0.5 font-medium text-status-warning">
                  <span className="shrink-0">live:</span>
                  <span className="truncate font-mono">
                    {view.live_version}
                  </span>
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-96">
                The cluster reports a different running version than the one
                recorded here.
              </TooltipContent>
            </Tooltip>
          )}
          {candidate && (
            <div className="flex items-center gap-1.5 text-status-good">
              <ArrowUpRight aria-hidden className="size-3 shrink-0" />
              <span className="shrink-0">next up:</span>
              <span className="truncate font-mono">{candidate}</span>
            </div>
          )}
        </div>
      </div>

      {showAutoPromote && (
        <Tooltip>
          <TooltipTrigger asChild>
            <label className="flex w-fit cursor-pointer items-center gap-1.5 text-xs text-muted-foreground">
              <Switch
                checked={view.auto_promote}
                onCheckedChange={(on) =>
                  autoPromote.mutate({ ring: ring.name, enabled: on })
                }
                aria-label={`Auto-promote ${ring.name}`}
                className="scale-75"
              />
              auto → {next!.ring.name}
            </label>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">
            When a version lands here and is healthy, promote it to{" "}
            {next!.ring.label} automatically. Turn off to stop a promotion
            chain at this ring.
          </TooltipContent>
        </Tooltip>
      )}

      <div className="mt-auto flex flex-wrap gap-1.5">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() =>
                setPendingAction({ type: "seed", ring: ring.name })
              }
            >
              <Download aria-hidden className="size-3.5" /> Seed
            </Button>
          </TooltipTrigger>
          <TooltipContent>Deploy a specific version here</TooltipContent>
        </Tooltip>

        {next && (
          <Tooltip>
            <TooltipTrigger asChild>
              {/* span so the tooltip works while the button is disabled */}
              <span>
                <Button
                  variant="default"
                  size="sm"
                  disabled={busy || !view.can_promote_from}
                  onClick={() =>
                    setPendingAction({ type: "promote", fromRing: ring.name })
                  }
                >
                  <ArrowUpRight aria-hidden className="size-3.5" /> Promote
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {view.can_promote_from
                ? `Promote ${view.current_version} to ${next.ring.label}`
                : view.current_version
                  ? "Target ring not available"
                  : "Nothing to promote — seed a version first"}
            </TooltipContent>
          </Tooltip>
        )}

        {view.previous_version && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                className="text-status-critical hover:text-status-critical"
                onClick={() =>
                  setPendingAction({ type: "rollback", ring: ring.name })
                }
              >
                <Undo2 aria-hidden className="size-3.5" /> Roll back
              </Button>
            </TooltipTrigger>
            <TooltipContent>Return to {view.previous_version}</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  );
}
