"use client";

import { useState } from "react";
import {
  ExternalLink,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
  ShieldX,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { RelativeTime } from "@/components/relative-time";
import type { GateState, GrafanaCheck, GrafanaVerdict, RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

// The Grafana go/no-go gate, drawn in the gap BETWEEN two ring cards: a tinted
// rail spanning the card height with the verdict chip sitting on it. The gate
// belongs to the ring being entered, so the chip between "Acceptance" and
// "Production" shows Production's verdict.

/** The verdict guarding entry into `target`, or undefined when it is ungated. */
export function gateVerdict(target: RingView | undefined): GrafanaVerdict | undefined {
  if (!target?.gates?.grafana) return undefined;
  return target.gates.grafana_status;
}

/** Only an explicit no-go blocks a promotion; check/unknown are advisory. */
export function gateBlocks(target: RingView | undefined): boolean {
  return gateVerdict(target)?.verdict === "no_go";
}

type Tone = {
  label: string;
  full: string;
  Icon: typeof ShieldCheck;
  text: string;
  border: string;
  rail: string;
};

// Status colour is always paired with the word, never carried by colour alone.
const TONE: Record<GateState, Tone> = {
  go: {
    label: "GO",
    full: "Go",
    Icon: ShieldCheck,
    text: "text-status-good",
    border: "border-status-good/40",
    rail: "bg-status-good/40",
  },
  check: {
    label: "CHECK",
    full: "Worth a look",
    Icon: ShieldAlert,
    text: "text-status-warning",
    border: "border-status-warning/40",
    rail: "bg-status-warning/40",
  },
  no_go: {
    label: "NO-GO",
    full: "No-Go",
    Icon: ShieldX,
    text: "text-status-critical",
    border: "border-status-critical/40",
    rail: "bg-status-critical/40",
  },
  unknown: {
    label: "NO DATA",
    full: "No data",
    Icon: ShieldQuestion,
    text: "text-muted-foreground",
    border: "border-muted-foreground/30",
    rail: "bg-muted-foreground/25",
  },
};

/**
 * One suite in the gate's popover. Shows the verdict word rather than the raw
 * number, because "2" means nothing to a reader — and when the run is stale or
 * unreadable, says so instead of implying a pass.
 */
function CheckRow({ check }: { check: GrafanaCheck }) {
  const tone = TONE[check.verdict] ?? TONE.unknown;
  return (
    <li className="flex items-baseline justify-between gap-3 text-xs">
      <span className="min-w-0 truncate">{check.name}</span>
      <span className={cn("shrink-0 text-right", tone.text)}>
        {check.stale ? "stale" : tone.label}
        {check.ran_at && !check.stale && (
          <span className="ml-1 text-muted-foreground">
            <RelativeTime iso={check.ran_at} />
          </span>
        )}
        {check.stale && check.ran_at && (
          <span className="ml-1 text-muted-foreground">
            last ran <RelativeTime iso={check.ran_at} />
          </span>
        )}
      </span>
    </li>
  );
}

export function GrafanaGate({
  targetLabel,
  verdict,
}: {
  targetLabel: string;
  verdict: GrafanaVerdict;
}) {
  const [open, setOpen] = useState(false);
  const tone = TONE[verdict.verdict] ?? TONE.unknown;

  return (
    <div className="relative flex shrink-0 items-center justify-center py-1 xl:w-16">
      {/* The rail: horizontal while the cards stack, vertical once they sit in
          a row, so the gate always spans the gap it guards. */}
      <span
        aria-hidden
        className={cn(
          "absolute inset-x-0 top-1/2 h-px -translate-y-1/2",
          "xl:inset-x-auto xl:inset-y-0 xl:left-1/2 xl:h-auto xl:w-px xl:-translate-x-1/2 xl:translate-y-0",
          tone.rail,
        )}
      />
      <Popover open={open} onOpenChange={setOpen}>
        <Tooltip>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <button
                type="button"
                aria-label={`Grafana gate into ${targetLabel}: ${tone.full}`}
                className={cn(
                  "relative flex items-center gap-1.5 rounded-md border bg-background px-2 py-1.5 outline-none transition-colors xl:flex-col xl:gap-1",
                  "hover:bg-muted focus-visible:ring-[3px] focus-visible:ring-ring/50",
                  tone.border,
                  tone.text,
                )}
              >
                <tone.Icon aria-hidden className="size-4" />
                <span className="whitespace-nowrap text-[11px] font-semibold leading-none tracking-wide">
                  {tone.label}
                </span>
              </button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent>
            {tone.full} into {targetLabel} — click for details
          </TooltipContent>
        </Tooltip>
        <PopoverContent align="center" className="w-72 space-y-3">
          <GateDetail targetLabel={targetLabel} verdict={verdict} />
        </PopoverContent>
      </Popover>
    </div>
  );
}

/**
 * Shown in the Seed/Promote dialogs when the target ring is NO-GO: the
 * dashboard's verdict, and the reason field that has to be filled in to
 * overrule it. The reason is written into the job log and history, so the
 * decision stays attributable long after the dashboard has moved on.
 */
export function GrafanaOverrideField({
  target,
  reason,
  onReasonChange,
}: {
  target: RingView;
  reason: string;
  onReasonChange: (v: string) => void;
}) {
  const verdict = gateVerdict(target);
  if (!verdict || verdict.verdict !== "no_go") return null;
  return (
    <div className="space-y-3 rounded-lg border border-status-critical/40 p-3">
      <GateDetail targetLabel={target.ring.label} verdict={verdict} />
      <div className="space-y-1.5">
        <Label htmlFor="gate-override-reason">
          Reason for overriding this no-go
        </Label>
        <Input
          id="gate-override-reason"
          value={reason}
          onChange={(e) => onReasonChange(e.target.value)}
          placeholder="e.g. failure is a known flaky test, agreed with QA"
          autoComplete="off"
        />
        <p className="text-xs text-muted-foreground">
          Recorded against this promotion — required to continue.
        </p>
      </div>
    </div>
  );
}

/** The gate's reasoning: what was measured, against what, and where to look. */
export function GateDetail({
  targetLabel,
  verdict,
}: {
  targetLabel: string;
  verdict: GrafanaVerdict;
}) {
  const tone = TONE[verdict.verdict] ?? TONE.unknown;
  return (
    <>
      <div>
        <p className={cn("text-sm font-semibold", tone.text)}>
          {tone.full} into {targetLabel}
        </p>
        <p className="text-xs text-muted-foreground">
          {verdict.dashboard || "Grafana"} · checked{" "}
          <RelativeTime iso={verdict.checked_at} />
          {verdict.demo && " · demo mode"}
        </p>
      </div>

      {verdict.checks?.length ? (
        // One row per suite: the point of the gate is knowing WHICH is red.
        <ul className="space-y-1.5">
          {verdict.checks.map((c) => (
            <CheckRow key={c.name} check={c} />
          ))}
        </ul>
      ) : (
        <p className="text-xs text-muted-foreground">
          {verdict.error
            ? `Grafana could not be read: ${verdict.error}`
            : "No checks configured for this gate."}
        </p>
      )}

      {verdict.verdict === "unknown" && (
        <p className="text-xs text-muted-foreground">
          A gate that cannot be read never blocks a promotion.
        </p>
      )}

      {verdict.dashboard_url && (
        <Button variant="outline" size="sm" className="w-full" asChild>
          <a href={verdict.dashboard_url} target="_blank" rel="noreferrer">
            <ExternalLink aria-hidden className="size-3.5" />
            Open dashboard
          </a>
        </Button>
      )}
    </>
  );
}
