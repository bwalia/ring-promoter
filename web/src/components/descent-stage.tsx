"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Link2, X } from "lucide-react";
import { EarthGlobe, type EarthGlobeHandle } from "@/components/earth-globe";
import { RelativeTime } from "@/components/relative-time";
import { ReleaseLanes } from "@/components/release-lanes";
import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import { Button } from "@/components/ui/button";
import {
  buildSpoke,
  DESCENT_DESIGN,
  DESCENT_EARTH_R,
  DESCENT_EMPTY_R,
  DESCENT_GATE_HALF,
  DESCENT_LABEL_R,
  DESCENT_NODE_R,
  DESCENT_COMET_SECONDS,
  DESCENT_RING_OUTER,
  descentFits,
  groupArcPath,
  labelAnchor,
  nextRingIndex,
  orderByGroup,
  polar,
  ringRadius,
  spokeAngle,
  spokeSentence,
  type Spoke,
} from "@/lib/descent-layout";
import {
  earthSpin,
  formatLocation,
  type GlobeMetrics,
} from "@/lib/globe-layout";
import { appLatencyMs, appTtfbMs, summarizeRings } from "@/lib/app-health";
import { useApps, useAppTitle, type GroupAppRings } from "@/lib/queries";
import { usePrefersReducedMotion } from "@/lib/use-prefers-reduced-motion";
import type { AppGroup, AppLocation, TopologyEdge } from "@/lib/types";
import { cn } from "@/lib/utils";

const STATUS_WORD: Record<NodeStatus, string> = {
  healthy: "Healthy",
  deploying: "Deploying",
  degraded: "Degraded",
  failed: "Failing",
  empty: "No version",
  loading: "Checking…",
};

/** Muted per-group arc tints — neutral enough to sit under status colour. */
const GROUP_TINTS = ["#a3a3a3", "#7dd3fc", "#c4b5fd", "#86efac", "#fda4af", "#fcd34d"];

/** Sparse starfield for the void — same motif as before, quieter. */
const STAGE_STARS = [
  { x: 8, y: 12, size: 1.2, duration: 5.2, delay: 0.4 },
  { x: 18, y: 28, size: 1, duration: 6.1, delay: 1.1 },
  { x: 72, y: 9, size: 1.4, duration: 4.8, delay: 0.2 },
  { x: 88, y: 22, size: 1, duration: 7.0, delay: 2.0 },
  { x: 64, y: 18, size: 0.9, duration: 5.5, delay: 1.6 },
  { x: 42, y: 8, size: 1.1, duration: 6.4, delay: 0.8 },
  { x: 12, y: 48, size: 1, duration: 5.8, delay: 2.4 },
  { x: 91, y: 44, size: 1.3, duration: 4.4, delay: 0.6 },
  { x: 78, y: 62, size: 0.9, duration: 6.8, delay: 1.9 },
  { x: 6, y: 72, size: 1.1, duration: 5.1, delay: 3.0 },
  { x: 34, y: 78, size: 1, duration: 7.2, delay: 0.3 },
  { x: 55, y: 88, size: 1.2, duration: 4.9, delay: 2.2 },
];

type Layout = "orbit" | "lanes";

export type DescentStageProps = {
  apps: string[];
  results: GroupAppRings[];
  statuses: NodeStatus[];
  aggregate: NodeStatus;
  deploying: Set<string>;
  edges: TopologyEdge[];
  groups: AppGroup[];
  locations?: Record<string, AppLocation | null | undefined>;
  editable?: boolean;
  onAddEdge?: (from: string, to: string) => void;
  onRemoveEdge?: (from: string, to: string) => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
  onOpenGroup?: (id: string) => void;
  /** Extra controls rendered in the stage header (e.g. "New ring"). */
  actions?: React.ReactNode;
  className?: string;
};

/**
 * Descent — Rings of Apps. The promotion rings are the orbits (int outermost,
 * prod just above Earth); each app owns a spoke whose lit trail shows how far
 * its newest version has travelled. Falls back to lanes when the orbit
 * wouldn't be readable. Geometry lives in `lib/descent-layout.ts`.
 */
export function DescentStage({
  apps,
  results,
  statuses,
  aggregate,
  deploying,
  edges,
  groups,
  locations,
  editable = false,
  onAddEdge,
  onRemoveEdge,
  onOpen,
  onSeed,
  onOpenGroup,
  actions,
  className,
}: DescentStageProps) {
  const title = useAppTitle();
  const reduceMotion = usePrefersReducedMotion();
  const ringOrder = useApps().data?.rings?.map((r) => r.name) ?? [];
  const earthRef = useRef<EarthGlobeHandle>(null);

  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [linkFrom, setLinkFrom] = useState<string | null>(null);
  const [layoutPref, setLayoutPref] = useState<Layout>("orbit");
  const active = focused ?? hovered;

  // ---- data -------------------------------------------------------------
  const resultById = useMemo(() => {
    const m = new Map<string, GroupAppRings>();
    apps.forEach((id, i) => results[i] && m.set(id, results[i]));
    return m;
  }, [apps, results]);
  const statusById = useMemo(() => {
    const m = new Map<string, NodeStatus>();
    apps.forEach((id, i) => m.set(id, statuses[i] ?? "empty"));
    return m;
  }, [apps, statuses]);
  const { ordered, spans } = useMemo(() => orderByGroup(apps, groups), [apps, groups]);
  const groupOf = useMemo(() => {
    const m = new Map<string, AppGroup>();
    for (const s of spans) {
      const g = groups.find((x) => x.id === s.id);
      if (g) for (let i = s.from; i <= s.to; i++) m.set(ordered[i], g);
    }
    return m;
  }, [spans, groups, ordered]);
  const ringKey = ringOrder.join(",");
  const spokes = useMemo(() => {
    const m = new Map<string, Spoke>();
    for (const id of apps) m.set(id, buildSpoke(resultById.get(id)?.rings, ringOrder));
    return m;
    // ringOrder identity changes each render; its content is `ringKey`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apps, resultById, ringKey]);
  const ttfbById = useMemo(() => {
    const m = new Map<string, number | null>();
    for (const id of apps) {
      const rings = resultById.get(id)?.rings;
      m.set(id, appTtfbMs(rings) ?? appLatencyMs(rings));
    }
    return m;
  }, [apps, resultById]);
  const ringCount = Math.max(1, ringOrder.length || spokes.values().next().value?.nodes.length || 4);

  // ---- stage geometry ---------------------------------------------------
  const stageEl = useRef<HTMLDivElement>(null);
  const [stage, setStage] = useState({ w: 0, h: 0 });
  const [wide, setWide] = useState(false);
  useLayoutEffect(() => {
    const el = stageEl.current;
    if (!el) return;
    const measure = () =>
      setStage((p) =>
        p.w === el.clientWidth && p.h === el.clientHeight
          ? p
          : { w: el.clientWidth, h: el.clientHeight },
      );
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  useEffect(() => {
    const mq = window.matchMedia("(min-width: 1024px)");
    const sync = () => setWide(mq.matches);
    sync();
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);

  const headerEl = useRef<HTMLDivElement>(null);
  const [headerH, setHeaderH] = useState(52);
  useLayoutEffect(() => {
    const el = headerEl.current;
    if (!el) return;
    const measure = () => setHeaderH(el.offsetHeight);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const TOP = headerH + 8;
  const BOTTOM = wide ? 40 : Math.min(200, Math.round(stage.h * 0.32)) + 40;
  const RAIL = wide ? 312 : 0;
  const areaW = Math.max(0, stage.w - RAIL);
  const areaH = Math.max(0, stage.h - TOP - BOTTOM);
  // The design square reserves 500 units of radius, but labels only need
  // ~410 vertically (the 12 o'clock gap has none) and ~550 sideways, so the
  // square may overflow the band a little vertically and less sideways.
  const size = Math.max(0, Math.min(areaH * 1.18, areaW / 1.12));
  const orbitFits = stage.w > 0 && descentFits(size, apps.length);
  const layout: Layout = orbitFits ? layoutPref : "lanes";
  const k = size / DESCENT_DESIGN;
  const fontU = k > 0 ? Math.max(13, Math.min(24, 12 / k)) : 16;
  const boxLeft = (areaW - size) / 2;
  const boxTop = TOP + (areaH - size) / 2 + size * 0.02;

  const earthMetrics: GlobeMetrics = useMemo(
    () => ({
      width: size,
      height: size,
      cx: size / 2,
      cy: size / 2,
      earthR: DESCENT_EARTH_R * k,
    }),
    [size, k],
  );

  // Earth spin is the only per-frame work; spokes are static SVG.
  useEffect(() => {
    if (layout !== "orbit") return;
    earthRef.current?.setSpin(0.6);
    if (reduceMotion) return;
    const start = performance.now();
    let id = 0;
    const loop = (now: number) => {
      earthRef.current?.setSpin(0.6 + earthSpin((now - start) / 1000, false));
      id = requestAnimationFrame(loop);
    };
    id = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(id);
  }, [layout, reduceMotion]);

  // ---- interaction ------------------------------------------------------
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hoverIn = (id: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setHovered(id);
  };
  const hoverOut = () => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    closeTimer.current = setTimeout(() => setHovered(null), 140);
  };
  const onSpokeClick = (id: string) => {
    if (editMode && editable) {
      if (!linkFrom) {
        setLinkFrom(id);
        return;
      }
      if (linkFrom !== id) onAddEdge?.(linkFrom, id);
      setLinkFrom(null);
      return;
    }
    setFocused((f) => (f === id ? null : id));
  };
  const onSpokeKey = (e: React.KeyboardEvent<SVGGElement>, id: string) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onSpokeClick(id);
    } else if (e.key === "ArrowRight" || e.key === "ArrowLeft" || e.key === "Escape") {
      e.preventDefault();
      if (e.key === "Escape") {
        setFocused(null);
        return;
      }
      const all = [
        ...(e.currentTarget.ownerSVGElement?.querySelectorAll<SVGGElement>("[data-spoke]") ?? []),
      ];
      const i = all.indexOf(e.currentTarget);
      all[(i + (e.key === "ArrowRight" ? 1 : -1) + all.length) % all.length]?.focus();
    }
  };

  const visibleEdges = useMemo(() => {
    const known = new Set(apps);
    return edges.filter((e) => known.has(e.from) && known.has(e.to));
  }, [edges, apps]);
  const indexOf = useMemo(() => new Map(ordered.map((id, i) => [id, i])), [ordered]);

  const failing = statuses.filter((s) => s === "failed").length;
  const summaryLine: Record<NodeStatus, string> = {
    healthy: "All systems operational",
    deploying: "Deployment in progress",
    degraded: "Partially degraded",
    failed: `${failing} ${failing === 1 ? "app" : "apps"} failing`,
    empty: "Nothing deployed yet",
    loading: "Checking health…",
  };
  const inProd = apps.filter((id) => {
    const s = spokes.get(id);
    return s && s.frontier >= 0 && nextRingIndex(s) < 0;
  }).length;

  const n = ordered.length;
  const focusApp = focused && layout === "orbit" ? focused : null;

  return (
    <div
      className={cn("relative h-full min-h-0 overflow-hidden bg-[#07070a]", className)}
      data-solar-ambient
    >
      <StageAtmosphere accent={STATUS_HEX[aggregate]} />

      <div
        ref={headerEl}
        className="pointer-events-none absolute inset-x-0 top-0 z-40 flex flex-wrap items-start justify-between gap-2 p-3 sm:px-4"
        style={wide && layout === "orbit" ? { right: RAIL } : undefined}
      >
        <div className="min-w-0">
          <p className="font-display text-[12px] font-semibold tracking-tight text-neutral-100 sm:text-[13px]">
            Rings of Apps
          </p>
          <p className="mt-0.5 font-mono text-[10px] tabular-nums text-neutral-500 sm:text-[11px]">
            {inProd}/{apps.length} newest in {ringOrder.at(-1) ?? "prod"}
            {" · "}
            {ringOrder.length ? ringOrder.join(" → ") : "promotion rings"}
          </p>
        </div>
        <div className="pointer-events-auto ml-auto flex flex-wrap items-center justify-end gap-2">
          {editMode && linkFrom && (
            <span className="hidden rounded-md border border-white/15 bg-black/50 px-2 py-1 text-[11px] text-neutral-300 backdrop-blur-md sm:inline">
              Link from <span className="text-neutral-100">{title(linkFrom)}</span>…
            </span>
          )}
          {actions}
          {editable && layout === "orbit" && (
            <Button
              type="button"
              size="sm"
              variant={editMode ? "default" : "outline"}
              className={cn(
                "h-8 border-white/15 bg-black/40 text-xs backdrop-blur-md",
                !editMode && "text-neutral-200 hover:bg-white/10",
              )}
              onClick={() => {
                setEditMode((v) => !v);
                setLinkFrom(null);
              }}
            >
              <Link2 aria-hidden className="size-3.5" />
              {editMode ? "Done" : "Edit links"}
            </Button>
          )}
          <div
            role="group"
            aria-label="Layout"
            className="inline-flex rounded-lg border border-white/15 bg-[#07070a]/75 p-0.5 backdrop-blur-md"
          >
            {(["orbit", "lanes"] as const).map((l) => (
              <button
                key={l}
                type="button"
                aria-pressed={layout === l}
                disabled={l === "orbit" && !orbitFits}
                title={l === "orbit" && !orbitFits ? "Not enough room for the orbit" : undefined}
                onClick={() => setLayoutPref(l)}
                data-testid={`descent-layout-${l}`}
                className={cn(
                  "rounded-md px-3 py-1.5 text-xs font-medium capitalize transition-colors disabled:cursor-not-allowed disabled:opacity-40",
                  layout === l
                    ? "bg-white/12 text-neutral-50 shadow-sm"
                    : "text-neutral-400 hover:text-neutral-100",
                )}
              >
                {l}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div
        ref={stageEl}
        className="absolute inset-0"
        data-testid="solar-system"
        data-status={aggregate}
        data-layout={layout}
        onClick={(e) => {
          if (!(e.target as Element).closest("[data-spoke], button, [data-node-card], [data-roster], [data-edge], [data-group-arc]")) {
            setFocused(null);
            setLinkFrom(null);
          }
        }}
      >
        {layout === "lanes" ? (
          <div
            className="absolute inset-x-0 bottom-0 overflow-auto px-1 pb-4 sm:px-3"
            style={{ top: TOP }}
          >
            <div className="mx-auto max-w-3xl">
              <ReleaseLanes
                apps={ordered}
                spokes={spokes}
                ringOrder={ringOrder}
                spans={spans}
                title={title}
                statusById={statusById}
                ttfbById={ttfbById}
                selected={focused}
                onSelect={setFocused}
                onOpen={onOpen}
                onSeed={onSeed}
                onOpenGroup={onOpenGroup}
              />
            </div>
          </div>
        ) : (
          size > 0 && (
            <div
              className="absolute"
              style={{ left: boxLeft, top: boxTop, width: size, height: size }}
            >
              <EarthGlobe
                ref={earthRef}
                metrics={earthMetrics}
                className="pointer-events-none absolute inset-0 size-full"
              />
              <svg
                viewBox={`${-DESCENT_DESIGN / 2} ${-DESCENT_DESIGN / 2} ${DESCENT_DESIGN} ${DESCENT_DESIGN}`}
                className={cn("absolute inset-0 size-full overflow-visible", (active || linkFrom) && "descent-focusing")}
                role="group"
                aria-label={`Promotion rings ${ringOrder.join(", ")} around Earth, one spoke per app`}
              >
                {/* Rings, int outermost → prod innermost. */}
                {ringOrder.map((name, i) => (
                  <circle
                    key={name}
                    r={ringRadius(i, ringCount)}
                    fill="none"
                    stroke="#e5e5e5"
                    strokeOpacity={i === ringCount - 1 ? 0.16 : 0.09}
                    strokeWidth={1 / Math.max(k, 0.3)}
                  />
                ))}

                {/* Group arcs outside the outer ring. */}
                {spans.map((s, gi) => (
                  <path
                    key={s.id}
                    d={groupArcPath(s, n)}
                    fill="none"
                    stroke={GROUP_TINTS[gi % GROUP_TINTS.length]}
                    strokeOpacity={0.5}
                    strokeWidth={3}
                    strokeLinecap="round"
                    data-group-arc
                    className={cn(onOpenGroup && "cursor-pointer")}
                    onClick={() => onOpenGroup?.(s.id)}
                    aria-label={`Group ${s.name}`}
                  >
                    <title>{s.name}</title>
                  </path>
                ))}

                {/* Dependencies: shown for the active app, or all in edit mode. */}
                {visibleEdges.map((e) => {
                  const show = editMode || (active && (e.from === active || e.to === active));
                  if (!show) return null;
                  const ia = indexOf.get(e.from)!;
                  const ib = indexOf.get(e.to)!;
                  const a = polar(spokeAngle(ia, n), DESCENT_RING_OUTER);
                  const b = polar(spokeAngle(ib, n), DESCENT_RING_OUTER);
                  // Bow toward the ring band, never through Earth.
                  const mx = (a.x + b.x) / 2;
                  const my = (a.y + b.y) / 2;
                  const ml = Math.hypot(mx, my);
                  const dir = ml > 1 ? { x: mx / ml, y: my / ml } : { x: -(b.y - a.y), y: b.x - a.x };
                  const dl = Math.hypot(dir.x, dir.y) || 1;
                  const c = {
                    x: (dir.x / dl) * DESCENT_RING_OUTER * 0.95,
                    y: (dir.y / dl) * DESCENT_RING_OUTER * 0.95,
                  };
                  return (
                    <path
                      key={`${e.from}->${e.to}`}
                      data-edge
                      d={`M ${a.x} ${a.y} Q ${c.x} ${c.y} ${b.x} ${b.y}`}
                      fill="none"
                      stroke={e.source === "config" ? "#d4d4d4" : "#a3a3a3"}
                      strokeOpacity={editMode ? 0.55 : 0.4}
                      strokeWidth={editMode ? 2.5 : 1.6}
                      strokeDasharray={e.source === "user" ? "6 5" : undefined}
                      className={cn(editMode && "cursor-pointer")}
                      onClick={(ev) => {
                        if (!editMode || !editable) return;
                        ev.stopPropagation();
                        onRemoveEdge?.(e.from, e.to);
                      }}
                    >
                      <title>
                        {title(e.from)} → {title(e.to)}
                        {editMode ? " (click to remove)" : ""}
                      </title>
                    </path>
                  );
                })}

                {ordered.map((id, i) => (
                  <SpokeView
                    key={id}
                    id={id}
                    label={title(id)}
                    spoke={spokes.get(id)!}
                    status={statusById.get(id) ?? "empty"}
                    angle={spokeAngle(i, n)}
                    ringCount={ringCount}
                    fontU={fontU}
                    on={active === id || linkFrom === id}
                    deploying={deploying.has(id)}
                    reduceMotion={reduceMotion}
                    onEnter={() => hoverIn(id)}
                    onLeave={hoverOut}
                    onClick={() => onSpokeClick(id)}
                    onKeyDown={(e) => onSpokeKey(e, id)}
                  />
                ))}

                {/* Ring tags in the gap at 12 o'clock, drawn last so they sit on top. */}
                {ringOrder.map((name, i) => {
                  const r = ringRadius(i, ringCount);
                  const fs = Math.max(11, 10 / Math.max(k, 0.3));
                  return (
                    <g key={`tag-${name}`} aria-hidden className="pointer-events-none">
                      <rect
                        x={-fs * 1.9}
                        y={-r - fs * 0.75}
                        width={fs * 3.8}
                        height={fs * 1.5}
                        rx={3}
                        fill="#07070a"
                      />
                      <text
                        y={-r}
                        textAnchor="middle"
                        dominantBaseline="central"
                        fontSize={fs}
                        className="fill-neutral-500 font-mono uppercase"
                        letterSpacing="0.12em"
                      >
                        {name}
                      </text>
                    </g>
                  );
                })}
              </svg>

            </div>
          )
        )}

            <AnimatePresence>
              {focusApp && (
                <motion.div
                  key={focusApp}
                  initial={reduceMotion ? false : { opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: 4 }}
                  transition={{ duration: 0.16, ease: "easeOut" }}
                  className="absolute z-30"
                  style={{ top: TOP, ...(Math.cos(spokeAngle(indexOf.get(focusApp) ?? 0, n)) > 0 ? { left: 12 } : { right: RAIL + 12 }) }}
                >
                  <NodeCard
                    id={focusApp}
                    label={title(focusApp)}
                    status={statusById.get(focusApp) ?? "empty"}
                    spoke={spokes.get(focusApp)!}
                    rings={resultById.get(focusApp)}
                    ttfbMs={ttfbById.get(focusApp) ?? null}
                    location={locations?.[focusApp]}
                    group={groupOf.get(focusApp)?.name}
                    onClose={() => {
                      setFocused(null);
                      setHovered(null);
                    }}
                    onOpen={onOpen}
                    onSeed={onSeed}
                  />
                </motion.div>
              )}
            </AnimatePresence>
        {layout === "orbit" && (
          <div
            className="pointer-events-none absolute left-3 z-30 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[11px] leading-none text-neutral-400"
            style={{ bottom: wide ? 12 : BOTTOM - 30, right: RAIL + 12 }}
          >
            <span className="inline-flex items-center gap-1.5 font-medium">
              <span aria-hidden className="size-1.5 rounded-full" style={{ background: STATUS_HEX[aggregate] }} />
              {summaryLine[aggregate]}
            </span>
            <span className="inline-flex items-center gap-1.5 text-neutral-500">
              <svg width="22" height="8" aria-hidden>
                <line x1="1" y1="4" x2="21" y2="4" stroke={STATUS_HEX.healthy} strokeWidth="3" strokeLinecap="round" />
              </svg>
              how far the newest version got
            </span>
            <span className="inline-flex items-center gap-1.5 text-neutral-500">
              <svg width="26" height="10" aria-hidden>
                <circle cx="5" cy="5" r="4" fill={STATUS_HEX.healthy} />
                <circle cx="19" cy="5" r="3.2" fill="#07070a" stroke={STATUS_HEX.healthy} strokeWidth="1.6" />
              </svg>
              newest · older version
            </span>
            <span className="inline-flex items-center gap-1.5 text-neutral-500">
              <svg width="6" height="12" aria-hidden>
                <line x1="3" y1="1" x2="3" y2="11" stroke={STATUS_HEX.degraded} strokeWidth="2.5" strokeLinecap="round" />
              </svg>
              gate closed
            </span>
            <span className="text-neutral-500">Earth = live users</span>
          </div>
        )}
      </div>

      {layout === "orbit" && (
        <AppRoster
          members={ordered}
          title={title}
          statusById={statusById}
          ttfbById={ttfbById}
          spokes={spokes}
          active={active}
          onFocus={(id) => setFocused((f) => (f === id ? null : id))}
          onOpen={onOpen}
        />
      )}
    </div>
  );
}

function SpokeView({
  id,
  label,
  spoke,
  status,
  angle,
  ringCount,
  fontU,
  on,
  deploying,
  reduceMotion,
  onEnter,
  onLeave,
  onClick,
  onKeyDown,
}: {
  id: string;
  label: string;
  spoke: Spoke;
  status: NodeStatus;
  angle: number;
  ringCount: number;
  fontU: number;
  on: boolean;
  deploying: boolean;
  reduceMotion: boolean;
  onEnter: () => void;
  onLeave: () => void;
  onClick: () => void;
  onKeyDown: (e: React.KeyboardEvent<SVGGElement>) => void;
}) {
  const at = (r: number) => polar(angle, r);
  const outer = at(ringRadius(0, ringCount));
  const inner = at(DESCENT_EARTH_R + 8);
  const trailHex = STATUS_HEX[status === "loading" || status === "empty" ? "healthy" : status];
  const frontierR = spoke.frontier >= 0 ? ringRadius(spoke.frontier, ringCount) : null;
  const next = nextRingIndex(spoke);
  const c = Math.cos(angle);
  const s = Math.sin(angle);

  // Label slot at the end of the spoke.
  const anchor = labelAnchor(angle);
  const lp = at(DESCENT_LABEL_R);
  const ly = anchor === "middle" ? lp.y + (s > 0 ? fontU * 0.9 : -fontU * 1.7) : lp.y;
  const sub =
    spoke.frontier < 0
      ? "no version"
      : `${spoke.newest} · ${spoke.nodes[spoke.frontier].ring}`;
  const shortLabel = label.length > 26 ? `${label.slice(0, 25)}…` : label;

  return (
    <g
      data-spoke={id}
      tabIndex={0}
      role="button"
      aria-label={`${label}: ${STATUS_WORD[status]}. ${spokeSentence(spoke)}`}
      aria-pressed={on}
      className={cn("descent-spoke cursor-pointer outline-none", on && "is-on")}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      onFocus={onEnter}
      onBlur={onLeave}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      onKeyDown={onKeyDown}
    >
      {/* Generous invisible hit area along the spoke. */}
      <line x1={outer.x} y1={outer.y} x2={inner.x} y2={inner.y} stroke="transparent" strokeWidth={34} />
      <line
        x1={outer.x}
        y1={outer.y}
        x2={inner.x}
        y2={inner.y}
        stroke="#e5e5e5"
        strokeOpacity={on ? 0.3 : 0.1}
        strokeWidth={1.2}
        className="descent-rail"
      />
      {frontierR != null && (
        <line
          x1={outer.x}
          y1={outer.y}
          x2={at(frontierR).x}
          y2={at(frontierR).y}
          stroke={trailHex}
          strokeOpacity={0.85}
          strokeWidth={on ? 4 : 3}
          strokeLinecap="round"
        />
      )}

      {spoke.gateAt >= 0 && frontierR != null && (() => {
        const mid = at((frontierR + ringRadius(spoke.gateAt, ringCount)) / 2);
        const px = -s * DESCENT_GATE_HALF;
        const py = c * DESCENT_GATE_HALF;
        return (
          <line
            x1={mid.x - px}
            y1={mid.y - py}
            x2={mid.x + px}
            y2={mid.y + py}
            stroke={STATUS_HEX.degraded}
            strokeWidth={3}
            strokeLinecap="round"
          >
            <title>{spoke.nodes[spoke.gateAt].gateClosed}</title>
          </line>
        );
      })()}

      {deploying && frontierR != null && next >= 0 && !reduceMotion && (
        <Comet
          from={at(frontierR)}
          to={at(ringRadius(next, ringCount))}
          hex={STATUS_HEX.deploying}
        />
      )}

      {spoke.nodes.map((node, ri) => {
        if (node.state === "off") return null;
        const p = at(ringRadius(ri, ringCount));
        if (node.state === "empty") {
          return <circle key={node.ring} cx={p.x} cy={p.y} r={DESCENT_EMPTY_R} fill="#52525b" />;
        }
        const hex = node.state === "failed" ? STATUS_HEX.failed : STATUS_HEX.healthy;
        return (
          <circle
            key={node.ring}
            cx={p.x}
            cy={p.y}
            r={DESCENT_NODE_R}
            fill={node.fresh ? hex : "#07070a"}
            stroke={hex}
            strokeWidth={node.fresh ? 0 : 2.5}
          />
        );
      })}

      <text
        x={lp.x}
        y={ly}
        textAnchor={anchor}
        dominantBaseline="middle"
        fontSize={fontU}
        className={cn("font-display font-medium", on ? "fill-neutral-50" : "fill-neutral-200")}
      >
        {shortLabel}
      </text>
      <text
        x={lp.x}
        y={ly + fontU * 1.15}
        textAnchor={anchor}
        dominantBaseline="middle"
        fontSize={fontU * 0.78}
        className="fill-neutral-500 font-mono tabular-nums"
      >
        {sub}
      </text>
    </g>
  );
}

/** A promotion in flight: a dot with a short tail sliding to the next ring. */
function Comet({
  from,
  to,
  hex,
}: {
  from: { x: number; y: number };
  to: { x: number; y: number };
  hex: string;
}) {
  const path = `M ${from.x} ${from.y} L ${to.x} ${to.y}`;
  const dur = `${DESCENT_COMET_SECONDS}s`;
  return (
    <g className="pointer-events-none" aria-hidden>
      {[0, 0.08, 0.16].map((lag, i) => (
        <circle key={i} r={6 - i * 1.6} fill={hex} fillOpacity={1 - i * 0.3}>
          <animateMotion
            path={path}
            dur={dur}
            begin={`-${lag}s`}
            repeatCount="indefinite"
            keyPoints="0;1"
            keyTimes="0;1"
            calcMode="spline"
            keySplines="0.45 0 0.25 1"
          />
        </circle>
      ))}
    </g>
  );
}

function StageAtmosphere({ accent }: { accent: string }) {
  return (
    <>
      <div aria-hidden className="absolute inset-0">
        {STAGE_STARS.map((s, i) => (
          <span
            key={i}
            className="absolute rounded-full bg-white [animation:twinkle_var(--d)_ease-in-out_infinite] motion-reduce:animate-none"
            style={
              {
                left: `${s.x}%`,
                top: `${s.y}%`,
                width: s.size,
                height: s.size,
                "--d": `${s.duration}s`,
                animationDelay: `${s.delay}s`,
              } as React.CSSProperties
            }
          />
        ))}
      </div>
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_42%_48%,rgba(56,189,248,0.06)_0%,transparent_50%)]"
      />
      <div
        aria-hidden
        className="absolute -left-20 top-0 size-72 rounded-full opacity-[0.10] blur-3xl"
        style={{ background: accent }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_52%,rgba(0,0,0,0.55)_100%)]"
      />
    </>
  );
}

function AppRoster({
  members,
  title,
  statusById,
  ttfbById,
  spokes,
  active,
  onFocus,
  onOpen,
}: {
  members: string[];
  title: (id: string) => string;
  statusById: Map<string, NodeStatus>;
  ttfbById: Map<string, number | null>;
  spokes: Map<string, Spoke>;
  active: string | null;
  onFocus: (id: string) => void;
  onOpen: (id: string) => void;
}) {
  const ordered = [...members].sort((a, b) => {
    const ma = ttfbById.get(a);
    const mb = ttfbById.get(b);
    if (ma == null && mb == null) return title(a).localeCompare(title(b));
    if (ma == null) return 1;
    if (mb == null) return -1;
    return ma - mb || title(a).localeCompare(title(b));
  });

  return (
    <div
      data-roster
      data-testid="app-roster"
      className="absolute inset-x-0 bottom-0 z-30 max-h-[min(200px,32vh)] overflow-auto border-t border-white/10 bg-[#07070a]/88 backdrop-blur-xl lg:inset-y-0 lg:left-auto lg:right-0 lg:max-h-none lg:w-[19.5rem] lg:border-l lg:border-t-0"
    >
      <div className="sticky top-0 z-10 border-b border-white/10 bg-[#07070a]/95 px-3 py-2.5 backdrop-blur-md">
        <p className="font-display text-[12px] font-semibold tracking-tight text-neutral-100">
          Applications
        </p>
        <p className="mt-0.5 font-mono text-[10px] tabular-nums text-neutral-500">
          {ordered.length} · sorted by TTFB · click to focus
        </p>
      </div>
      <ul className="divide-y divide-white/[0.04] p-1.5">
        {ordered.map((id) => {
          const status = statusById.get(id) ?? "empty";
          const hex = STATUS_HEX[status];
          const ttfb = ttfbById.get(id);
          const spoke = spokes.get(id);
          const reach =
            spoke && spoke.frontier >= 0
              ? `${spoke.newest} · ${spoke.nodes[spoke.frontier].ring}`
              : null;
          const selected = active === id;
          return (
            <li key={id}>
              <button
                type="button"
                onClick={() => onFocus(id)}
                onDoubleClick={() => onOpen(id)}
                className={cn(
                  "flex w-full items-start gap-2.5 rounded-md px-2 py-2 text-left transition-colors",
                  selected ? "bg-white/[0.12] ring-1 ring-white/15" : "hover:bg-white/[0.06]",
                )}
              >
                <span
                  aria-hidden
                  className="mt-1 size-2 shrink-0 rounded-full shadow-[0_0_8px_currentColor]"
                  style={{ background: hex, color: hex }}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-display text-[12.5px] font-medium leading-tight text-neutral-50">
                    {title(id)}
                  </span>
                  <span className="mt-0.5 flex flex-wrap items-center gap-x-2 font-mono text-[10px] tabular-nums text-neutral-500">
                    <span style={{ color: hex }}>{STATUS_WORD[status]}</span>
                    <span>{ttfb != null ? `${Math.round(ttfb)}ms TTFB` : "—"}</span>
                    {reach && <span className="text-neutral-600">{reach}</span>}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function NodeCard({
  id,
  label,
  status,
  spoke,
  rings,
  ttfbMs,
  location,
  group,
  onClose,
  onOpen,
  onSeed,
}: {
  id: string;
  label: string;
  status: NodeStatus;
  spoke: Spoke;
  rings: GroupAppRings | undefined;
  ttfbMs: number | null;
  location: AppLocation | null | undefined;
  group?: string;
  onClose: () => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
}) {
  const hex = STATUS_HEX[status];
  const { lastDeploy } = summarizeRings(rings?.rings);
  const where = formatLocation(location);

  return (
    <div
      data-node-card
      className="w-64 rounded-2xl border border-white/15 bg-neutral-950/90 p-3.5 text-left shadow-2xl ring-1 ring-black/40 backdrop-blur-2xl"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          {group && (
            <p className="font-mono text-[10px] uppercase tracking-[0.12em] text-neutral-500">{group}</p>
          )}
          <p className="truncate font-display text-sm font-semibold tracking-tight text-neutral-50">
            {label}
          </p>
          <p className="mt-0.5 inline-flex items-center gap-1.5 text-[11px] font-medium" style={{ color: hex }}>
            <span aria-hidden className="size-1.5 rounded-full" style={{ background: hex }} />
            {STATUS_WORD[status]}
          </p>
        </div>
        <button
          type="button"
          aria-label="Close"
          onClick={onClose}
          className="rounded-md p-1 text-neutral-400 hover:bg-white/10 hover:text-neutral-100"
        >
          <X aria-hidden className="size-3.5" />
        </button>
      </div>

      <p className="mt-2 text-xs leading-snug text-neutral-200">{spokeSentence(spoke)}</p>

      <dl className="mt-2.5 divide-y divide-white/[0.06] overflow-hidden rounded-lg border border-white/10 font-mono text-[11px] tabular-nums">
        {spoke.nodes
          .filter((nd) => nd.state !== "off")
          .map((nd) => (
            <div key={nd.ring} className="grid grid-cols-[3rem_1fr_auto] items-center gap-2 px-2.5 py-1.5">
              <dt className="uppercase tracking-[0.08em] text-neutral-500">{nd.ring}</dt>
              <dd className="min-w-0 text-neutral-100">
                {nd.version ?? <span className="text-neutral-600">—</span>}
                {nd.version && !nd.fresh && <span className="block text-[10px] text-neutral-500">older version</span>}
                {nd.gateClosed && <span className="block text-[10px] text-amber-400">{nd.gateClosed}</span>}
              </dd>
              <dd
                className="text-[10px]"
                style={{ color: nd.state === "failed" ? STATUS_HEX.failed : nd.state === "healthy" ? STATUS_HEX.healthy : undefined }}
              >
                {nd.state === "failed"
                  ? "failing"
                  : nd.state === "healthy" && nd.ttfbMs != null
                    ? `${Math.round(nd.ttfbMs)}ms`
                    : ""}
              </dd>
            </div>
          ))}
      </dl>

      <div className="mt-2 flex flex-wrap justify-between gap-x-3 gap-y-0.5 text-[11px] text-neutral-400">
        <span>
          TTFB{" "}
          <span className="font-mono tabular-nums text-neutral-100">
            {ttfbMs != null ? `${Math.round(ttfbMs)}ms` : "—"}
          </span>
        </span>
        {where && <span className="truncate">{where}</span>}
        <span>
          Deployed{" "}
          {lastDeploy ? <RelativeTime iso={lastDeploy} className="text-neutral-100" /> : "never"}
        </span>
      </div>

      <div className="mt-3 flex gap-2">
        <button
          type="button"
          onClick={() => onOpen(id)}
          className="h-8 flex-1 rounded-md bg-white text-xs font-medium text-neutral-900 transition-colors hover:bg-white/85"
        >
          Open app
        </button>
        {onSeed && (
          <button
            type="button"
            onClick={() => onSeed(id)}
            className="h-8 flex-1 rounded-md border border-white/15 text-xs font-medium text-neutral-100 transition-colors hover:bg-white/10"
          >
            Seed
          </button>
        )}
      </div>
    </div>
  );
}
