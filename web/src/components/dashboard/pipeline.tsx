"use client";

import { Fragment, useState } from "react";
import {
  ArrowUpRight,
  ChevronDown,
  ChevronRight,
  Download,
  Undo2,
} from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { HealthBadge, ringHealth } from "@/components/status";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { VersionLabel } from "@/components/version-label";
import {
  useActiveJob,
  useAppTitle,
  useAutoPromoteMutation,
  useProdProtection,
} from "@/lib/queries";
import { useUiStore } from "@/lib/ui-store";
import type { RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

// The dashboard cards are optimized for scanning: ring, health, version,
// Promote/Seed — nothing else. Clicking a card opens a details sheet that
// carries the full state (previous/live version, drift, health error) and
// the advanced operations (rollback, auto-promote).

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
  // Ring whose details sheet shows. Data is looked up fresh from `rings` on
  // every render so an open sheet keeps up with live refreshes. Closing only
  // flips `detailsOpen` — the ring itself is kept so the sheet still has
  // content to show while its slide-out animation plays.
  const [detailsRing, setDetailsRing] = useState<string | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);

  const nothingDeployed =
    rings && rings.every((r) => !r.configured || !r.current_version);

  const detailsIndex = rings
    ? rings.findIndex((r) => r.ring.name === detailsRing)
    : -1;

  return (
    <section className="space-y-3">
      <h2 className="text-sm font-semibold">Promotion pipeline</h2>

      {isPending || !rings ? (
        <div className="flex flex-col gap-3 xl:flex-row">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-44 flex-1 rounded-xl" />
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
                onClick={() => setPendingAction({ type: "seed", app })}
              >
                <Download aria-hidden className="size-4" /> Seed a version
              </Button>
            </div>
          )}

          <div className="flex flex-col items-stretch gap-2 xl:flex-row">
            {rings.map((view, i) => (
              <Fragment key={view.ring.name}>
                <RingCard
                  app={app}
                  view={view}
                  next={i < rings.length - 1 ? rings[i + 1] : undefined}
                  busy={running}
                  onOpenDetails={() => {
                    setDetailsRing(view.ring.name);
                    setDetailsOpen(true);
                  }}
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

          {detailsIndex >= 0 && (
            <RingDetailsSheet
              app={app}
              open={detailsOpen}
              view={rings[detailsIndex]}
              prev={detailsIndex > 0 ? rings[detailsIndex - 1] : undefined}
              next={
                detailsIndex < rings.length - 1
                  ? rings[detailsIndex + 1]
                  : undefined
              }
              busy={running}
              onOpenChange={setDetailsOpen}
            />
          )}
        </>
      )}
    </section>
  );
}

/** Collapsed, scannable card: ring + health, version, updated, Promote/Seed. */
function RingCard({
  app,
  view,
  next,
  busy,
  onOpenDetails,
}: {
  app: string;
  view: RingView;
  next?: RingView;
  busy: boolean;
  onOpenDetails: () => void;
}) {
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const health = ringHealth(view);
  const { ring } = view;

  if (!view.configured) {
    return (
      <div className="flex min-w-0 flex-1 basis-0 flex-col justify-center gap-1 rounded-xl border border-dashed p-5 text-center">
        <p className="text-sm text-muted-foreground">{ring.label}</p>
        <p className="text-xs text-muted-foreground/70">Not configured</p>
      </div>
    );
  }

  // The card's Promote answers "is anything ready to move on?": it is only
  // active when this ring holds something the next ring doesn't have. The
  // unrestricted promote (e.g. re-deploying the same version) lives in the
  // details sheet.
  const inSync =
    !!next && !!view.current_version &&
    next.current_version === view.current_version;
  const promoteDisabled = busy || !view.can_promote_from || inSync;
  const promoteHint = !view.current_version
    ? "Nothing to promote — seed a version first"
    : !view.can_promote_from
      ? "Target ring not available"
      : inSync
        ? `${next!.ring.label} already has ${view.current_version}`
        : `Promote ${view.current_version} to ${next!.ring.label}`;

  return (
    <div
      data-testid={`ring-card-${ring.name}`}
      role="button"
      tabIndex={0}
      aria-label={`${ring.label} details`}
      onClick={onOpenDetails}
      onKeyDown={(e) => {
        // Only when the card itself is focused — Enter/Space on the inner
        // buttons bubbles up here and must keep its normal meaning.
        if (
          (e.key === "Enter" || e.key === " ") &&
          e.target === e.currentTarget
        ) {
          e.preventDefault();
          onOpenDetails();
        }
      }}
      className={cn(
        "group flex min-w-0 flex-1 basis-0 cursor-pointer flex-col rounded-xl border bg-card p-5 text-left outline-none transition-colors",
        "hover:border-muted-foreground/40 focus-visible:ring-[3px] focus-visible:ring-ring/50",
        health === "unhealthy" && "border-status-critical/40",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <p className="min-w-0 truncate text-sm font-semibold">{ring.label}</p>
        <span className="flex shrink-0 items-center gap-1">
          <HealthBadge health={health} pulse />
          <ChevronRight
            aria-hidden
            className="size-4 text-muted-foreground/0 transition-colors group-hover:text-muted-foreground/70"
          />
        </span>
      </div>

      <div className="mt-4 min-w-0">
        {view.current_version ? (
          // The version label copies itself on click — keep that from also
          // toggling the card's details sheet.
          <span
            className="inline-flex max-w-full"
            onClick={(e) => e.stopPropagation()}
          >
            <VersionLabel
              version={view.current_version}
              className="max-w-full text-xl font-semibold tracking-tight"
            />
          </span>
        ) : (
          <p className="text-sm text-muted-foreground">No version deployed</p>
        )}
        {view.current_version && (
          <p className="mt-1 text-xs text-muted-foreground">
            Updated <RelativeTime iso={view.updated_at} />
          </p>
        )}
      </div>

      {/* Only the buttons swallow clicks — the rest of the card, including
          the space around them, opens the details sheet. Stacked full-width
          so every card looks the same regardless of label length. */}
      <div className="mt-auto flex flex-col gap-2 pt-5">
        {next && (
          <Tooltip>
            <TooltipTrigger asChild>
              {/* span so the tooltip works while the button is disabled */}
              <span className="w-full" onClick={(e) => e.stopPropagation()}>
                <Button
                  variant="default"
                  size="sm"
                  className="w-full"
                  disabled={promoteDisabled}
                  onClick={() =>
                    setPendingAction({ type: "promote", app, fromRing: ring.name })
                  }
                >
                  Promote to {next.ring.label}
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{promoteHint}</TooltipContent>
          </Tooltip>
        )}

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="secondary"
              size="sm"
              className="w-full"
              disabled={busy}
              onClick={(e) => {
                e.stopPropagation();
                setPendingAction({ type: "seed", app, ring: ring.name });
              }}
            >
              Seed
            </Button>
          </TooltipTrigger>
          <TooltipContent>Deploy a specific version here</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

/** Full ring state and advanced operations, opened by clicking a card. */
function RingDetailsSheet({
  app,
  open,
  view,
  prev,
  next,
  busy,
  onOpenChange,
}: {
  app: string;
  open: boolean;
  view: RingView;
  prev?: RingView;
  next?: RingView;
  busy: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const autoPromote = useAutoPromoteMutation(app);
  const { prodProtected, prodRing } = useProdProtection();
  const title = useAppTitle();
  const health = ringHealth(view);
  const { ring } = view;

  const showAutoPromote = view.configured && !!prev && !!next;
  const drift =
    view.live_version &&
    view.current_version &&
    view.live_version !== view.current_version;

  const toggleAutoPromote = (on: boolean) => {
    // Turning on the hands-free path INTO production requires the production
    // password — collected by a dedicated dialog. Everything else is direct.
    if (on && prodProtected && next?.ring.name === prodRing) {
      setPendingAction({ type: "autoPromote", app, ring: ring.name });
      return;
    }
    autoPromote.mutate({ ring: ring.name, enabled: on });
  };

  // Kicking off an operation closes the sheet so its confirmation dialog and
  // the job progress get the stage.
  const act = (action: () => void) => {
    action();
    onOpenChange(false);
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        className="w-full gap-0 sm:max-w-md"
        // Don't auto-focus the first control (the version's copy button) —
        // it would pop its tooltip on every open and eat the first Escape.
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <div className="flex items-center gap-3">
            <SheetTitle className="text-base">{ring.label}</SheetTitle>
            <HealthBadge
              health={health}
              detail={
                health === "unhealthy"
                  ? view.live_health_error || "live health check failing"
                  : undefined
              }
            />
          </div>
          <SheetDescription>{title(app)}</SheetDescription>
        </SheetHeader>

        <div className="flex-1 space-y-6 overflow-y-auto p-4 pt-2">
          <section>
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Current version
            </p>
            <div className="mt-1.5">
              {view.current_version ? (
                <VersionLabel
                  version={view.current_version}
                  className="max-w-full text-lg font-semibold tracking-tight"
                />
              ) : (
                <p className="text-sm text-muted-foreground">
                  No version deployed
                </p>
              )}
              {view.current_version && (
                <p className="mt-1 text-xs text-muted-foreground">
                  Updated <RelativeTime iso={view.updated_at} />
                </p>
              )}
            </div>
          </section>

          <section className="space-y-3 text-sm">
            <DetailRow label="Previous version">
              {view.previous_version ? (
                <VersionLabel version={view.previous_version} className="text-sm" />
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </DetailRow>

            <DetailRow label="Live version">
              {view.live_version ? (
                <span className="min-w-0 truncate font-mono">
                  {view.live_version}
                </span>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </DetailRow>
            {drift && (
              <p className="rounded-md bg-status-warning/15 px-2.5 py-1.5 text-xs font-medium text-status-warning">
                The cluster reports a different running version than the one
                recorded here.
              </p>
            )}

            <DetailRow label="Health check">
              {health === "unhealthy" ? (
                <span className="min-w-0 break-words font-medium text-status-critical">
                  {view.live_health_error || "Live health check failing"}
                </span>
              ) : health === "healthy" ? (
                <span className="text-status-good">Passing</span>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </DetailRow>
          </section>

          {showAutoPromote && (
            <section className="rounded-lg border p-3">
              <label className="flex cursor-pointer items-center justify-between gap-3 text-sm font-medium">
                Auto-promote to {next!.ring.label}
                <Switch
                  checked={view.auto_promote}
                  onCheckedChange={toggleAutoPromote}
                  aria-label={`Auto-promote ${ring.name}`}
                />
              </label>
              <p className="mt-1.5 text-xs text-muted-foreground">
                When a version lands here and is healthy, promote it to{" "}
                {next!.ring.label} automatically. Turn off to stop a promotion
                chain at this ring.
              </p>
            </section>
          )}
        </div>

        <SheetFooter>
          {next && (
            <Button
              disabled={busy || !view.can_promote_from}
              onClick={() =>
                act(() =>
                  setPendingAction({ type: "promote", app, fromRing: ring.name }),
                )
              }
            >
              <ArrowUpRight aria-hidden className="size-4" />
              Promote to {next.ring.label}
            </Button>
          )}
          <Button
            variant="secondary"
            disabled={busy}
            onClick={() =>
              act(() => setPendingAction({ type: "seed", app, ring: ring.name }))
            }
          >
            <Download aria-hidden className="size-4" />
            Seed a version
          </Button>
          {view.previous_version && (
            <Button
              variant="outline"
              disabled={busy}
              className="text-status-critical hover:text-status-critical"
              onClick={() =>
                act(() =>
                  setPendingAction({ type: "rollback", app, ring: ring.name }),
                )
              }
            >
              <Undo2 aria-hidden className="size-4" />
              Roll back to {view.previous_version}
            </Button>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function DetailRow({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}
