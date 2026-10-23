"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Ban,
  CheckCircle2,
  Circle,
  CircleSlash,
  FlaskConical,
  Flag,
  GitCommitHorizontal,
  Hand,
  Hourglass,
  Loader2,
  Maximize2,
  Minus,
  Plus,
  Recycle,
  Scale,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
  ShieldX,
  XCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  layoutFlow,
  MAX_CHILDREN,
  type EdgeState,
  type FlowChild,
  type FlowNode,
  type FlowPlane,
  type FlowStatus,
  type GateKind,
  type PositionedNode,
} from "@/lib/flow-layout";
import { duration } from "@/lib/time";
import type { GateState } from "@/lib/types";
import { usePrefersReducedMotion } from "@/lib/use-prefers-reduced-motion";
import { cn } from "@/lib/utils";

export const STATUS_LABEL: Record<FlowStatus, string> = {
  queued: "Queued",
  running: "Running",
  waiting: "Waiting",
  success: "Succeeded",
  failed: "Failed",
  skipped: "Skipped",
  cancelled: "Cancelled",
  reused: "Reused",
};

export function StatusIcon({
  status,
  className,
}: {
  status: FlowStatus;
  className?: string;
}) {
  const c = cn("size-4 shrink-0", className);
  switch (status) {
    case "running":
      return <Loader2 aria-hidden className={cn(c, "animate-spin text-primary")} />;
    case "waiting":
      return <Hourglass aria-hidden className={cn(c, "text-status-warning")} />;
    case "success":
      return <CheckCircle2 aria-hidden className={cn(c, "text-status-good")} />;
    case "reused":
      return <Recycle aria-hidden className={cn(c, "text-status-good/70")} />;
    case "failed":
      return <XCircle aria-hidden className={cn(c, "text-status-critical")} />;
    case "skipped":
      return <CircleSlash aria-hidden className={cn(c, "text-muted-foreground")} />;
    case "cancelled":
      return <Ban aria-hidden className={cn(c, "text-muted-foreground")} />;
    default:
      return <Circle aria-hidden className={cn(c, "text-muted-foreground/60")} />;
  }
}

export const VERDICT_LABEL: Record<GateState, string> = {
  go: "GO",
  no_go: "NO-GO",
  check: "CHECK",
  unknown: "PENDING",
};

const VERDICT_TONE: Record<GateState, string> = {
  go: "border-status-good/60 bg-status-good/15 text-status-good",
  no_go: "border-status-critical/60 bg-status-critical/15 text-status-critical",
  check: "border-status-warning/60 bg-status-warning/15 text-status-warning",
  unknown: "border-border bg-muted text-muted-foreground",
};

export function VerdictPill({
  verdict,
  className,
}: {
  verdict: GateState;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center whitespace-nowrap rounded border px-1.5 py-px font-mono text-[10px] font-semibold tracking-wide",
        VERDICT_TONE[verdict],
        className,
      )}
    >
      {VERDICT_LABEL[verdict]}
    </span>
  );
}

/** A gate's verdict implied by its status when no explicit verdict is known. */
export function gateVerdict(n: { verdict?: GateState; status: FlowStatus }): GateState {
  if (n.verdict) return n.verdict;
  if (n.status === "success" || n.status === "reused") return "go";
  if (n.status === "failed") return "no_go";
  if (n.status === "running" || n.status === "waiting") return "check";
  return "unknown";
}

export const GATE_KIND_LABEL: Record<GateKind, string> = {
  policy: "Policy",
  qa: "QA",
  security: "AppSec",
};

/** The glyph inside a gate diamond: what it checks, tinted by its verdict. */
export function GateGlyph({
  gate,
  verdict,
  className,
}: {
  gate?: GateKind;
  verdict: GateState;
  className?: string;
}) {
  const Icon =
    gate === "policy"
      ? Scale
      : gate === "qa"
        ? FlaskConical
        : verdict === "go"
          ? ShieldCheck
          : verdict === "no_go"
            ? ShieldX
            : gate === "security" && verdict === "check"
              ? ShieldAlert
              : ShieldQuestion;
  return (
    <Icon
      aria-hidden
      className={cn(
        "size-5",
        verdict === "go" && "text-status-good",
        verdict === "no_go" && "text-status-critical",
        verdict === "check" && "text-status-warning",
        verdict === "unknown" && "text-muted-foreground",
        className,
      )}
    />
  );
}

const EDGE_COLOR: Record<EdgeState, string> = {
  idle: "var(--muted-foreground)",
  flowing: "var(--primary)",
  done: "var(--status-good)",
  blocked: "var(--status-critical)",
};

const EDGE_OPACITY: Record<EdgeState, number> = {
  idle: 0.35,
  flowing: 0.95,
  done: 0.6,
  blocked: 0.5,
};

const matches = (q: string, ...parts: (string | undefined)[]) =>
  parts.some((p) => p?.toLowerCase().includes(q));

function childMatches(q: string, c: FlowChild) {
  return !q || matches(q, c.label, c.sublabel, c.id);
}

function nodeMatches(q: string, n: FlowNode) {
  if (!q) return true;
  return (
    matches(q, n.label, n.sublabel, n.keywords) ||
    (n.children ?? []).some((c) => childMatches(q, c))
  );
}

// Failing and live members first, so a folded box still shows what matters.
const CHILD_RANK: Record<FlowStatus, number> = {
  failed: 0,
  running: 1,
  waiting: 2,
  queued: 3,
  success: 4,
  reused: 5,
  cancelled: 6,
  skipped: 7,
};

function visibleChildrenOf(n: FlowNode, q: string): FlowChild[] {
  const kids = n.children ?? [];
  // Only the node's own visible name keeps every member listed; keywords
  // (which include member names) must not, or a search never narrows a box.
  const own = !!q && matches(q, n.label, n.sublabel);
  const filtered = q && !own ? kids.filter((c) => childMatches(q, c)) : kids;
  return [...filtered].sort((a, b) => CHILD_RANK[a.status] - CHILD_RANK[b.status]);
}

/** Re-render every second while something is live so durations count up. */
function useTick(active: boolean) {
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!active) return;
    const t = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [active]);
}

export interface FlowGraphProps {
  nodes: FlowNode[];
  /** Selected node id, or `node/child` for a member of a group box. */
  selected?: string | null;
  onSelect?: (nodeId: string, childId?: string) => void;
  /** Search text: matching nodes stay lit, the rest dim. */
  query?: string;
  /** Extra controls rendered inside a node (e.g. Approve / Reject). */
  renderActions?: (node: FlowNode) => React.ReactNode;
  className?: string;
  ariaLabel: string;
}

/**
 * A left-to-right, GitHub-Actions-style run graph. Directed edges show which
 * way work flows: solid data-plane edges carry a version/artifact forward,
 * dashed control-plane edges carry a Go/No-Go decision. Edges into live nodes
 * animate; gates render as diamonds with their verdict.
 */
export function FlowGraph({
  nodes,
  selected,
  onSelect,
  query = "",
  renderActions,
  className,
  ariaLabel,
}: FlowGraphProps) {
  const q = query.trim().toLowerCase();
  const reduced = usePrefersReducedMotion();
  // null = fit to the panel width (never above 100%); a number = the user's
  // explicit zoom.
  const [userZoom, setUserZoom] = useState<number | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const [panelW, setPanelW] = useState(0);
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setPanelW(e.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const live = nodes.some((n) => n.status === "running" || n.status === "waiting");
  useTick(live);

  const layout = useMemo(
    () => layoutFlow(nodes, (n) => visibleChildrenOf(n, q).length),
    [nodes, q],
  );
  const fit =
    panelW && layout.width
      ? // Below ~80% the labels get too small to read; scroll instead.
        Math.max(0.8, Math.min(1, panelW / layout.width))
      : 1;
  const zoom = userZoom ?? fit;
  const setZoom = (f: (z: number) => number) => setUserZoom(f(zoom));

  return (
    <div className={cn("relative rounded-lg border bg-background/40", className)}>
      <div
        ref={scroller}
        className="overflow-auto"
        role="group"
        aria-label={ariaLabel}
      >
        <div
          className="relative origin-top-left"
          style={{
            width: layout.width * zoom,
            height: layout.height * zoom,
            minHeight: 120,
          }}
        >
          <div
            className="absolute left-0 top-0 origin-top-left"
            style={{
              width: layout.width,
              height: layout.height,
              transform: `scale(${zoom})`,
            }}
          >
            <svg
              aria-hidden
              className="pointer-events-none absolute inset-0 overflow-visible"
              width={layout.width}
              height={layout.height}
            >
              <defs>
                {(Object.keys(EDGE_COLOR) as EdgeState[]).map((s) => (
                  <marker
                    key={s}
                    id={`rp-arrow-${s}`}
                    viewBox="0 0 10 10"
                    refX="2"
                    refY="5"
                    markerWidth="7"
                    markerHeight="7"
                    orient="auto-start-reverse"
                  >
                    <path
                      d="M 0 0 L 10 5 L 0 10 z"
                      fill={EDGE_COLOR[s]}
                      fillOpacity={EDGE_OPACITY[s]}
                    />
                  </marker>
                ))}
              </defs>
              {layout.edges.map((e) => {
                const dim =
                  q &&
                  !(
                    nodeMatches(q, layout.nodes.find((n) => n.id === e.from)!) &&
                    nodeMatches(q, layout.nodes.find((n) => n.id === e.to)!)
                  );
                return (
                  <Edge
                    key={e.id}
                    d={e.d}
                    state={e.state}
                    plane={e.plane}
                    animate={!reduced}
                    dim={!!dim}
                  />
                );
              })}
            </svg>

            {layout.nodes.map((n) => (
              <NodeView
                key={n.id}
                node={n}
                q={q}
                selected={selected}
                onSelect={onSelect}
                actions={renderActions?.(n)}
              />
            ))}
          </div>
        </div>
      </div>

      <div className="pointer-events-none absolute bottom-2 left-3 flex flex-wrap items-center gap-3 text-[10px] text-muted-foreground">
        <LegendSwatch plane="data" label="Data plane · version/artifact" />
        <LegendSwatch plane="control" label="Control plane · Go/No-Go decision" />
        {(["policy", "qa", "security"] as const)
          .filter((k) => nodes.some((n) => n.gate === k))
          .map((k) => (
            <span key={k} className="flex items-center gap-1">
              <GateGlyph gate={k} verdict="unknown" className="size-3" />
              {k === "policy" ? "Policy gate" : k === "qa" ? "QA gate" : "AppSec gate"}
            </span>
          ))}
      </div>
      <div className="absolute bottom-2 right-2 flex gap-1">
        <Button
          variant="outline"
          size="icon"
          className="size-7"
          aria-label="Zoom out"
          onClick={() => setZoom((z) => Math.max(0.5, +(z - 0.1).toFixed(2)))}
        >
          <Minus aria-hidden className="size-3.5" />
        </Button>
        <Button
          variant="outline"
          size="icon"
          className="size-7"
          aria-label="Fit to width"
          onClick={() => setUserZoom(null)}
        >
          <Maximize2 aria-hidden className="size-3.5" />
        </Button>
        <Button
          variant="outline"
          size="icon"
          className="size-7"
          aria-label="Zoom in"
          onClick={() => setZoom((z) => Math.min(1.6, +(z + 0.1).toFixed(2)))}
        >
          <Plus aria-hidden className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

function LegendSwatch({ plane, label }: { plane: FlowPlane; label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <svg aria-hidden width="22" height="6">
        <line
          x1="0"
          y1="3"
          x2="22"
          y2="3"
          stroke="currentColor"
          strokeWidth="2"
          strokeDasharray={plane === "control" ? "4 3" : undefined}
        />
      </svg>
      {label}
    </span>
  );
}

function Edge({
  d,
  state,
  plane,
  animate,
  dim,
}: {
  d: string;
  state: EdgeState;
  plane: FlowPlane;
  animate: boolean;
  dim: boolean;
}) {
  const color = EDGE_COLOR[state];
  const dash =
    plane === "control" ? "6 5" : state === "blocked" ? "3 4" : undefined;
  return (
    <g opacity={dim ? 0.15 : 1} className="transition-opacity">
      <path
        d={d}
        fill="none"
        stroke={color}
        strokeOpacity={EDGE_OPACITY[state]}
        strokeWidth={state === "flowing" ? 2.25 : 1.75}
        strokeDasharray={dash}
        markerEnd={`url(#rp-arrow-${state})`}
        className={cn(
          state === "flowing" && plane === "control" && animate && "rp-edge-march",
        )}
      />
      {state === "flowing" && animate && plane === "data" && (
        // Packets travelling along the edge: the version on its way.
        <>
          {[0, 0.55, 1.1].map((delay) => (
            <circle key={delay} r="3" fill={color}>
              <animateMotion
                dur="1.65s"
                begin={`${delay}s`}
                repeatCount="indefinite"
                path={d}
              />
            </circle>
          ))}
        </>
      )}
    </g>
  );
}

function NodeView({
  node: n,
  q,
  selected,
  onSelect,
  actions,
}: {
  node: PositionedNode;
  q: string;
  selected?: string | null;
  onSelect?: (nodeId: string, childId?: string) => void;
  actions?: React.ReactNode;
}) {
  const dim = !nodeMatches(q, n);
  const isSel = selected === n.id;
  const style = { left: n.x, top: n.y, width: n.w, minHeight: n.h };
  const live = n.status === "running" || n.status === "waiting";

  if (n.kind === "gate") {
    const v = gateVerdict(n);
    const kids = visibleChildrenOf(n, q);
    return (
      <div
        className={cn("absolute flex flex-col items-center transition-opacity", dim && "opacity-35")}
        style={style}
      >
        <button
          type="button"
          onClick={() => onSelect?.(n.id)}
          className={cn(
            "relative flex size-11 rotate-45 items-center justify-center rounded-md border-2 bg-card transition-colors",
            v === "go" && "border-status-good",
            v === "no_go" && "border-status-critical",
            v === "check" && "border-status-warning",
            v === "unknown" && "border-border",
            // AppSec gates carry a second ring so they read apart from QA.
            n.gate === "security" && "outline-2 outline-offset-2",
            n.gate === "security" && v === "go" && "outline-status-good/40",
            n.gate === "security" && v === "no_go" && "outline-status-critical/40",
            n.gate === "security" && v === "check" && "outline-status-warning/40",
            n.gate === "security" && v === "unknown" && "outline-border",
            live && "rp-node-waiting",
            isSel && "ring-2 ring-ring ring-offset-2 ring-offset-background",
          )}
          aria-label={`${n.label}: ${VERDICT_LABEL[v]}`}
        >
          <GateGlyph gate={n.gate} verdict={v} className="-rotate-45" />
        </button>
        <div className="mt-2 flex max-w-full items-center gap-1.5">
          <span className="truncate text-xs font-medium">{n.label}</span>
          <VerdictPill verdict={v} />
        </div>
        {(n.gate || n.sublabel) && (
          <span className="flex max-w-full items-center gap-1 text-[10px] text-muted-foreground" title={n.sublabel}>
            {n.gate && (
              <span className="shrink-0 rounded-sm bg-muted px-1 font-mono text-[9px] uppercase tracking-wider">
                {GATE_KIND_LABEL[n.gate]}
              </span>
            )}
            <span className="truncate">{n.sublabel}</span>
          </span>
        )}
        {kids.slice(0, MAX_CHILDREN).map((c) => (
          <button
            key={c.id}
            type="button"
            onClick={() => onSelect?.(n.id, c.id)}
            className="mt-0.5 flex w-full items-center justify-between gap-1 rounded px-1 text-[10px] hover:bg-muted"
            title={c.sublabel}
          >
            <span className="truncate">{c.label}</span>
            {c.meta && (
              <span className="ml-auto shrink-0 font-mono text-[9px] text-muted-foreground">{c.meta}</span>
            )}
            <VerdictPill verdict={gateVerdict(c)} className="text-[9px]" />
          </button>
        ))}
      </div>
    );
  }

  const kids = visibleChildrenOf(n, q);
  const shown = kids.slice(0, MAX_CHILDREN);
  const hidden = kids.length - shown.length;
  const KindIcon =
    n.kind === "trigger" ? GitCommitHorizontal : n.kind === "terminal" ? Flag : n.kind === "approval" ? Hand : null;

  return (
    <div
      className={cn(
        "absolute overflow-hidden rounded-lg border bg-card text-card-foreground shadow-sm transition-[opacity,border-color,box-shadow]",
        n.status === "running" && "rp-node-running border-primary/70",
        n.status === "waiting" && "rp-node-waiting border-status-warning/70",
        n.status === "failed" && "border-status-critical/70",
        (n.status === "success" || n.status === "reused") && "border-status-good/40",
        (n.status === "skipped" || n.status === "cancelled") && "border-dashed opacity-70",
        isSel && "ring-2 ring-ring ring-offset-2 ring-offset-background",
        dim && "opacity-35",
      )}
      style={style}
    >
      <button
        type="button"
        onClick={() => onSelect?.(n.id)}
        className="flex w-full items-center gap-2 px-3 py-2.5 text-left hover:bg-muted/40"
      >
        {KindIcon && n.status === "queued" ? (
          <KindIcon aria-hidden className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <StatusIcon status={n.status} />
        )}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium leading-tight">
            {n.label}
          </span>
          {n.sublabel && (
            <span className="block truncate text-[11px] leading-tight text-muted-foreground">
              {n.sublabel}
            </span>
          )}
        </span>
        {n.startedAt && (
          <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
            {duration(n.startedAt, n.finishedAt)}
          </span>
        )}
      </button>
      {actions && <div className="border-t px-3 py-2">{actions}</div>}
      {shown.length > 0 && (
        <ul className="border-t py-1">
          {shown.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                onClick={() => onSelect?.(n.id, c.id)}
                className={cn(
                  "flex h-[30px] w-full items-center gap-2 px-3 text-left text-xs hover:bg-muted/50",
                  selected === `${n.id}/${c.id}` && "bg-muted",
                )}
              >
                <StatusIcon status={c.status} className="size-3.5" />
                <span className="min-w-0 flex-1 truncate">{c.label}</span>
                {c.verdict && <VerdictPill verdict={c.verdict} className="text-[9px]" />}
                {c.startedAt && (
                  <span className="shrink-0 font-mono text-[10px] tabular-nums text-muted-foreground">
                    {duration(c.startedAt, c.finishedAt)}
                  </span>
                )}
              </button>
            </li>
          ))}
          {hidden > 0 && (
            <li>
              <button
                type="button"
                onClick={() => onSelect?.(n.id)}
                className="flex h-[30px] w-full items-center px-3 text-left text-xs text-muted-foreground hover:bg-muted/50"
              >
                +{hidden} more — search or open to see all
              </button>
            </li>
          )}
        </ul>
      )}
    </div>
  );
}
