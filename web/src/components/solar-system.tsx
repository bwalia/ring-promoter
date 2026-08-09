"use client";

import { useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Link2, X } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import { DIAL_SIZE, OrbitDial, ringSegments } from "@/components/orbit-body";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import { useApps, useAppTitle, type GroupAppRings } from "@/lib/queries";
import {
  appLatencyMs,
  buildOrbitPlanets,
  latencyToRadius,
  ORBIT_BANDS,
  planetPosition,
  SOLAR_C,
  SOLAR_R_MAX,
} from "@/lib/solar-layout";
import { usePrefersReducedMotion } from "@/lib/use-prefers-reduced-motion";
import type { AppGroup, TopologyEdge } from "@/lib/types";
import { cn } from "@/lib/utils";

const STATUS_WORD: Record<NodeStatus, string> = {
  healthy: "Healthy",
  deploying: "Deploying",
  degraded: "Degraded",
  failed: "Failing",
  empty: "No version",
  loading: "Checking…",
};

export type SolarSystemProps = {
  sunLabel: string;
  members: string[];
  results: GroupAppRings[];
  statuses: NodeStatus[];
  aggregate: NodeStatus;
  edges: TopologyEdge[];
  groups?: AppGroup[];
  resolveTitle?: (id: string) => string;
  mode?: "apps" | "groups";
  subtitles?: Record<string, string>;
  latencyById?: Record<string, number | null>;
  editable?: boolean;
  onAddEdge?: (from: string, to: string) => void;
  onRemoveEdge?: (from: string, to: string) => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
};

export function SolarSystem({
  sunLabel,
  members,
  results,
  statuses,
  aggregate,
  edges,
  resolveTitle,
  mode = "apps",
  subtitles,
  latencyById,
  editable = false,
  onAddEdge,
  onRemoveEdge,
  onOpen,
  onSeed,
}: SolarSystemProps) {
  const appTitle = useAppTitle();
  const title = resolveTitle ?? appTitle;
  const hubGrad = useId();
  const hex = STATUS_HEX[aggregate];
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [linkFrom, setLinkFrom] = useState<string | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const active = focused ?? hovered;
  const reduceMotion = usePrefersReducedMotion();
  const failing = statuses.filter((s) => s === "failed").length;
  const openLabel = mode === "groups" ? "Open ring" : "Open app";
  const bodyWord = mode === "groups" ? "ring" : "app";
  const bodyWordPlural = mode === "groups" ? "rings" : "apps";

  // Canonical promotion order (int → test → acc → prod) straight from the
  // server, so the dial segments read in the order a version travels.
  const ringOrder = useApps().data?.rings?.map((r) => r.name) ?? [];

  const latencyByMember = useMemo(() => {
    const m = new Map<string, number | null>();
    members.forEach((id, i) => {
      if (latencyById && id in latencyById) {
        m.set(id, latencyById[id] ?? null);
      } else {
        m.set(id, appLatencyMs(results[i]?.rings));
      }
    });
    return m;
  }, [members, results, latencyById]);

  const statusById = useMemo(() => {
    const m = new Map<string, NodeStatus>();
    members.forEach((id, i) => m.set(id, statuses[i] ?? "empty"));
    return m;
  }, [members, statuses]);

  const resultById = useMemo(() => {
    const m = new Map<string, GroupAppRings>();
    members.forEach((id, i) => {
      if (results[i]) m.set(id, results[i]);
    });
    return m;
  }, [members, results]);

  const orbits = useMemo(
    () =>
      buildOrbitPlanets(members, (id) =>
        latencyToRadius(latencyByMember.get(id) ?? null),
      ),
    [members, latencyByMember],
  );

  // Bodies hold station. Motion is reserved for things that mean something —
  // a deploy running, a service failing — so movement on this stage is always
  // worth looking at. Positions therefore resolve once, at t=0.
  const positions = useMemo(() => {
    const m = new Map<string, { x: number; y: number; angle: number }>();
    for (const p of orbits) m.set(p.id, planetPosition(p, 0));
    return m;
  }, [orbits]);

  const visibleEdges = useMemo(
    () => edges.filter((e) => members.includes(e.from) && members.includes(e.to)),
    [edges, members],
  );

  const occupiedTracks = useMemo(
    () => new Set(orbits.map((o) => o.track)),
    [orbits],
  );

  // ── Name decluttering ──────────────────────────────────────────────────
  // Bodies bunch up near the top and bottom of a track, where the names are
  // far wider than the gap between them. Overlapping names are hidden rather
  // than left to overprint into mush — the dial always stays, because losing
  // a body would lose information, whereas a name comes back on hover.
  // Layout is static, so this runs on data/size changes only, never per frame.
  const stageEl = useRef<HTMLDivElement>(null);
  const nameEls = useRef(new Map<string, HTMLElement | null>());

  useLayoutEffect(() => {
    const stage = stageEl.current;
    if (!stage) return;

    const run = () => {
      const scale = stage.clientWidth / 400 || 1;
      const ordered = [...orbits].sort((x, y) => {
        const rank = (id: string) => {
          if (id === active) return 0;
          const st = statusById.get(id) ?? "empty";
          return st === "failed" || st === "degraded" || st === "deploying"
            ? 1
            : 2;
        };
        return rank(x.id) - rank(y.id);
      });

      // Only names are deconflicted against each other. Seeding this with the
      // dials as well was tried and is far too strict: past ~20 bodies every
      // name box grazes somebody's dial and the entire stage goes anonymous.
      // Names carry their own backdrop instead, so crossing an orbit line or
      // clipping a neighbouring dial stays readable.
      const kept: { l: number; r: number; t: number; b: number }[] = [];
      for (const p of ordered) {
        const el = nameEls.current.get(p.id);
        const pos = positions.get(p.id);
        if (!el || !pos) continue;
        el.style.opacity = "1";

        // Boxes are derived from layout size plus the body's known position
        // rather than read back with getBoundingClientRect: the entrance
        // spring is still scaling these nodes when this first runs, and
        // measured rects would be snapshots of a mid-animation frame.
        const w = el.offsetWidth;
        const h = el.offsetHeight;
        const nameRight =
          pos.x > 300 ? false : pos.x < 100 ? true : Math.cos(pos.angle) >= 0;
        const offset = DIAL_SIZE / 2 + 4 + w / 2;
        const cx = pos.x * scale + (nameRight ? offset : -offset);
        const cy = pos.y * scale;
        const box = {
          l: cx - w / 2 - 2,
          r: cx + w / 2 + 2,
          t: cy - h / 2 - 1,
          b: cy + h / 2 + 1,
        };
        const clash = kept.some(
          (k) => !(box.r < k.l || box.l > k.r || box.b < k.t || box.t > k.b),
        );
        if (clash) {
          el.style.opacity = "0";
          el.style.pointerEvents = "none";
        } else {
          el.style.pointerEvents = "";
          kept.push(box);
        }
      }
    };

    run();
    // Widths depend on the webfont, so redo it once that has actually landed.
    document.fonts?.ready.then(run).catch(() => {});
    const ro = new ResizeObserver(run);
    ro.observe(stage);
    return () => ro.disconnect();
  }, [orbits, positions, statusById, active, title]);

  const hoverIn = (id: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setHovered(id);
  };
  const hoverOut = () => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    closeTimer.current = setTimeout(() => setHovered(null), 170);
  };

  const onBodyClick = (id: string) => {
    if (editMode && editable) {
      if (!linkFrom) {
        setLinkFrom(id);
        setFocused(id);
        return;
      }
      if (linkFrom === id) {
        setLinkFrom(null);
        return;
      }
      onAddEdge?.(linkFrom, id);
      setLinkFrom(null);
      setFocused(null);
      return;
    }
    setFocused((f) => (f === id ? null : id));
  };

  const summaryLine: Record<NodeStatus, string> = {
    healthy: "All systems operational",
    deploying: "Deployment in progress",
    degraded: "Partially degraded",
    failed: `${failing} ${failing === 1 ? bodyWord : bodyWordPlural} failing`,
    empty: "Nothing deployed yet",
    loading: "Checking health…",
  };

  return (
    <div className="relative overflow-hidden rounded-2xl border border-black/20 bg-[#07070a] dark:border-border">
      {/* A single, restrained pool of light at the centre. The old starfield
          and heavy glow competed with the data for attention. */}
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,rgba(245,185,66,0.05)_0%,transparent_55%)]"
      />

      <div className="pointer-events-none absolute left-4 top-3 z-40">
        <p className="font-display text-[11px] font-semibold uppercase tracking-[0.14em] text-neutral-400">
          {sunLabel}
        </p>
        <p className="mt-0.5 font-mono text-[11px] tabular-nums text-neutral-500">
          {members.length} {members.length === 1 ? bodyWord : bodyWordPlural}
        </p>
      </div>

      {editable && (
        <div className="absolute right-3 top-3 z-40 flex items-center gap-2">
          {editMode && linkFrom && (
            <span className="rounded-md border border-white/15 bg-black/50 px-2 py-1 text-[11px] text-neutral-300 backdrop-blur-md">
              Link from <span className="text-neutral-100">{title(linkFrom)}</span>…
            </span>
          )}
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
        </div>
      )}

      <div
        ref={stageEl}
        className="relative mx-auto aspect-square w-full max-w-[760px] p-3 sm:p-6"
        data-testid="solar-system"
        data-status={aggregate}
        onClick={(e) => {
          if (
            !(e.target as HTMLElement).closest(
              "button, [data-node-card], [data-edge]",
            )
          ) {
            setFocused(null);
            if (editMode) setLinkFrom(null);
          }
        }}
      >
        <svg
          viewBox="0 0 400 400"
          className="absolute inset-0 size-full overflow-visible"
        >
          <defs>
            <radialGradient id={hubGrad} cx="42%" cy="36%" r="70%">
              <stop offset="0%" stopColor="#2a2417" />
              <stop offset="100%" stopColor="#131316" />
            </radialGradient>
          </defs>

          {/* Latency bands. Each occupied track is a real axis now, not a
              decorative circle — the "inner = faster" rule is finally
              something you can read off the chart instead of a footnote. */}
          {ORBIT_BANDS.map(({ r }) => {
            const occupied = occupiedTracks.has(r);
            return (
              <circle
                key={r}
                cx={SOLAR_C}
                cy={SOLAR_C}
                r={r}
                fill="none"
                stroke="#ffffff"
                strokeWidth={occupied ? 0.9 : 0.6}
                strokeOpacity={occupied ? 0.14 : 0.05}
                strokeDasharray={occupied ? undefined : "2 8"}
              />
            );
          })}

          {/* The radial axis itself, drawn straight up; bodies are angled off
              it (AXIS_CLEARANCE) so the scale stays readable. */}
          <line
            x1={SOLAR_C}
            y1={SOLAR_C}
            x2={SOLAR_C}
            y2={SOLAR_C - SOLAR_R_MAX - 6}
            stroke="#ffffff"
            strokeOpacity={0.1}
            strokeWidth={0.8}
          />
          {ORBIT_BANDS.map(({ r }) => (
            <line
              key={`tick-${r}`}
              x1={SOLAR_C - 2.5}
              y1={SOLAR_C - r}
              x2={SOLAR_C + 2.5}
              y2={SOLAR_C - r}
              stroke="#ffffff"
              strokeOpacity={0.22}
              strokeWidth={0.9}
            />
          ))}

          {/* Dependency chords */}
          {visibleEdges.map((e) => {
            const a = positions.get(e.from);
            const b = positions.get(e.to);
            if (!a || !b) return null;
            return (
              <g key={`${e.from}->${e.to}`} data-edge>
                <path
                  d={chordPath(a, b)}
                  fill="none"
                  stroke={e.source === "config" ? "#a3a3a3" : "#737373"}
                  strokeWidth={editMode ? 1.6 : 0.9}
                  strokeOpacity={editMode ? 0.5 : 0.22}
                  className={cn(editMode && "cursor-pointer")}
                  onClick={(ev) => {
                    if (!editMode || !editable) return;
                    ev.stopPropagation();
                    onRemoveEdge?.(e.from, e.to);
                  }}
                >
                  <title>
                    {title(e.from)} → {title(e.to)}
                  </title>
                </path>
              </g>
            );
          })}

          {/* Hub: a machined centre, not a sun. Deliberately quiet so the
              bodies carry the eye. */}
          <circle
            cx={SOLAR_C}
            cy={SOLAR_C}
            r={42}
            fill={`url(#${hubGrad})`}
            stroke="#f5b942"
            strokeOpacity={0.28}
            strokeWidth={1}
          />
          <circle
            cx={SOLAR_C}
            cy={SOLAR_C}
            r={35}
            fill="none"
            stroke="#f5b942"
            strokeOpacity={0.12}
            strokeWidth={0.6}
          />
        </svg>

        {/* Axis scale. Rendered as HTML, not SVG <text>: inside a 400-unit
            viewBox that stretches to the container, any font-size is scaled by
            the viewport and these came out roughly twice the size of every
            other label on the stage. */}
        {ORBIT_BANDS.map(({ r, label }) => (
          <span
            key={`band-${r}`}
            aria-hidden
            className="pointer-events-none absolute -translate-x-full -translate-y-1/2 pr-2 font-mono text-[10px] leading-none tabular-nums text-neutral-600"
            style={{
              left: "50%",
              top: `${((SOLAR_C - r) / 400) * 100}%`,
            }}
          >
            {label}
          </span>
        ))}

        {/* Hub label */}
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          <div className="flex w-[7.5rem] flex-col items-center text-center">
            <p className="font-display text-[12px] font-semibold uppercase tracking-[0.12em] text-[#f5b942]">
              Rings
            </p>
            <p className="mt-1 font-mono text-[15px] font-medium tabular-nums leading-none text-neutral-100">
              {members.length}
            </p>
            <p className="mt-0.5 font-mono text-[9px] uppercase tracking-wider text-neutral-500">
              {members.length === 1 ? bodyWord : bodyWordPlural}
            </p>
          </div>
        </div>

        {/* Bodies */}
        {orbits.map((p, i) => {
          const pos = positions.get(p.id);
          if (!pos) return null;
          const status = statusById.get(p.id) ?? "empty";
          const expanded = active === p.id && !editMode;
          const linking = editMode && linkFrom === p.id;
          const lat = latencyByMember.get(p.id);
          const rings = resultById.get(p.id);
          const segs = ringSegments(rings?.rings, ringOrder);
          const { latest } = summarizeRings(rings?.rings);
          const left = (pos.x / 400) * 100;
          const top = (pos.y / 400) * 100;
          // Names normally sit on the outward side so they never point back
          // through the hub — but near the left and right extremes "outward"
          // runs straight off the stage, so those flip inward instead.
          const outwardRight = Math.cos(pos.angle) >= 0;
          const nameRight =
            pos.x > 300 ? false : pos.x < 100 ? true : outwardRight;
          const below = Math.sin(pos.angle) >= 0;

          return (
            <div
              key={p.id}
              className="absolute z-10"
              style={{ left: `${left}%`, top: `${top}%` }}
              onMouseEnter={() => hoverIn(p.id)}
              onMouseLeave={hoverOut}
            >
              <motion.div
                className="-translate-x-1/2 -translate-y-1/2"
                initial={reduceMotion ? false : { opacity: 0, scale: 0.6 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{
                  delay: Math.min(0.04 * i, 0.5),
                  type: "spring",
                  stiffness: 240,
                  damping: 20,
                }}
              >
                <div className="relative">
                  <button
                    type="button"
                    aria-label={`${title(p.id)}: ${STATUS_WORD[status]}`}
                    onClick={() => onBodyClick(p.id)}
                    className={cn(
                      "block rounded-full transition-transform",
                      "hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70",
                      linking && "ring-2 ring-white/80",
                    )}
                  >
                    <OrbitDial
                      segments={segs}
                      status={status}
                      reduceMotion={reduceMotion}
                    />
                  </button>

                  {/* Name plate, attached to the body rather than floating */}
                  <button
                    ref={(el) => {
                      nameEls.current.set(p.id, el);
                    }}
                    type="button"
                    onClick={() => onBodyClick(p.id)}
                    title={title(p.id)}
                    className={cn(
                      "absolute top-1/2 flex -translate-y-1/2 flex-col whitespace-nowrap rounded-md bg-[#07070a]/80 px-1.5 py-0.5 text-left transition-opacity",
                      nameRight ? "left-full ml-1" : "right-full mr-1 items-end",
                    )}
                  >
                    <span
                      className={cn(
                        "max-w-[7.5rem] truncate font-display text-[11px] font-medium leading-tight tracking-tight",
                        expanded || linking
                          ? "text-neutral-50"
                          : "text-neutral-200",
                      )}
                    >
                      {title(p.id)}
                    </span>
                    <span className="font-mono text-[9.5px] leading-tight tabular-nums text-neutral-500">
                      {mode === "groups"
                        ? (subtitles?.[p.id] ?? "")
                        : (latest?.current_version ?? "—")}
                      {lat != null && (
                        <span className="text-neutral-600">
                          {" · "}
                          {Math.round(lat)}ms
                        </span>
                      )}
                    </span>
                  </button>

                  <AnimatePresence>
                    {expanded && (
                      <motion.div
                        key="card"
                        initial={{ opacity: 0, scale: 0.96 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: 0.97 }}
                        transition={{ duration: 0.14, ease: "easeOut" }}
                        className={cn(
                          "absolute left-1/2 z-30 -translate-x-1/2",
                          below ? "bottom-full mb-2" : "top-full mt-2",
                        )}
                      >
                        <NodeCard
                          id={p.id}
                          label={title(p.id)}
                          status={status}
                          rings={rings}
                          latencyMs={lat ?? null}
                          mode={mode}
                          subtitle={subtitles?.[p.id]}
                          pinned={focused === p.id}
                          openLabel={openLabel}
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
                </div>
              </motion.div>
            </div>
          );
        })}

        {/* Legend */}
        <div className="pointer-events-none absolute bottom-2 left-2 right-2 flex flex-wrap items-center justify-between gap-2 text-[11px] leading-none text-neutral-400">
          <span className="inline-flex items-center gap-1.5 font-medium">
            <span
              aria-hidden
              className="size-1.5 rounded-full"
              style={{ background: hex }}
            />
            {summaryLine[aggregate]}
          </span>
          <span className="inline-flex items-center gap-2 text-neutral-500">
            <span className="inline-flex items-center gap-1">
              <RingKey />
              {ringOrder.length ? ringOrder.join(" · ") : "promotion rings"}
            </span>
            <span className="text-neutral-600">·</span>
            <span>ring = latency</span>
          </span>
        </div>
      </div>
    </div>
  );
}

/**
 * Dependency chord, bowed away from the centre.
 *
 * A straight line between two bodies on opposite sides of the stage runs
 * right through the hub and reads as a strike-through. Curving it outward
 * keeps the hub clean and makes overlapping dependencies distinguishable.
 */
function chordPath(
  a: { x: number; y: number },
  b: { x: number; y: number },
): string {
  const mx = (a.x + b.x) / 2;
  const my = (a.y + b.y) / 2;
  let dx = mx - SOLAR_C;
  let dy = my - SOLAR_C;
  let dist = Math.hypot(dx, dy);
  if (dist < 1) {
    // Chord passes dead through the hub: bow along the segment's normal.
    dx = -(b.y - a.y);
    dy = b.x - a.x;
    dist = Math.hypot(dx, dy) || 1;
  }
  // The closer the midpoint sits to the hub, the harder it needs to bow. The
  // threshold is comfortably wider than the hub radius (42) so chords clear
  // its edge rather than grazing it.
  const bow = Math.max(0, 88 - dist) * 1.35;
  const cx = mx + (dx / dist) * bow;
  const cy = my + (dy / dist) * bow;
  return `M ${a.x} ${a.y} Q ${cx} ${cy} ${b.x} ${b.y}`;
}

/** Tiny four-segment mark used in the legend to explain the body dials. */
function RingKey() {
  return (
    <svg width="11" height="11" viewBox="0 0 11 11" aria-hidden>
      {[0, 1, 2, 3].map((i) => {
        const step = (2 * Math.PI) / 4;
        const a0 = -Math.PI / 2 + i * step + 0.22;
        const a1 = -Math.PI / 2 + (i + 1) * step - 0.22;
        const r = 4;
        const x0 = 5.5 + r * Math.cos(a0);
        const y0 = 5.5 + r * Math.sin(a0);
        const x1 = 5.5 + r * Math.cos(a1);
        const y1 = 5.5 + r * Math.sin(a1);
        return (
          <path
            key={i}
            d={`M ${x0} ${y0} A ${r} ${r} 0 0 1 ${x1} ${y1}`}
            fill="none"
            stroke="#a1a1aa"
            strokeWidth={1.6}
            strokeLinecap="round"
          />
        );
      })}
    </svg>
  );
}

function NodeCard({
  id,
  label,
  status,
  rings,
  latencyMs,
  mode,
  subtitle,
  pinned,
  openLabel,
  onClose,
  onOpen,
  onSeed,
}: {
  id: string;
  label: string;
  status: NodeStatus;
  rings: GroupAppRings | undefined;
  latencyMs: number | null;
  mode: "apps" | "groups";
  subtitle?: string;
  pinned: boolean;
  openLabel: string;
  onClose: () => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
}) {
  const hex = STATUS_HEX[status];
  const { active, healthy, latest, lastDeploy } = summarizeRings(rings?.rings);

  return (
    <div
      data-node-card
      className="w-60 rounded-2xl border border-white/15 bg-neutral-950/85 p-3.5 text-left shadow-2xl ring-1 ring-black/40 backdrop-blur-2xl"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span
            className="size-2.5 shrink-0 rounded-full"
            style={{ background: hex }}
          />
          <div className="min-w-0">
            <p className="truncate font-display text-sm font-semibold tracking-tight text-neutral-50">
              {label}
            </p>
            {subtitle && (
              <p className="truncate text-[11px] text-neutral-400">{subtitle}</p>
            )}
          </div>
        </div>
        {pinned && (
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="rounded-md p-0.5 text-neutral-400 hover:bg-white/10 hover:text-neutral-100"
          >
            <X aria-hidden className="size-3.5" />
          </button>
        )}
      </div>

      <dl className="mt-2.5 space-y-1.5 text-xs">
        <Row label="Status">
          <span
            className="inline-flex items-center gap-1.5 font-medium"
            style={{ color: hex }}
          >
            {STATUS_WORD[status]}
          </span>
        </Row>
        <Row label="Latency">
          {latencyMs != null ? (
            <span className="font-mono tabular-nums text-neutral-100">
              {Math.round(latencyMs)}ms
            </span>
          ) : (
            <span className="text-neutral-400">—</span>
          )}
        </Row>
        {mode === "groups" ? (
          <>
            <Row label="Apps">
              <span className="text-neutral-100">{subtitle ?? "—"}</span>
            </Row>
            <Row label="Health">
              <span className="text-neutral-100">
                {active.length === 0
                  ? "nothing deployed"
                  : `${healthy}/${active.length} rings healthy`}
              </span>
            </Row>
          </>
        ) : (
          <>
            <Row label="Version">
              {latest ? (
                <span className="font-mono tabular-nums text-neutral-100">
                  {latest.current_version}
                  <span className="text-neutral-400"> · {latest.ring.name}</span>
                </span>
              ) : (
                <span className="text-neutral-400">nothing deployed</span>
              )}
            </Row>
            <Row label="Rings">
              <span className="text-neutral-100">
                {active.length === 0 ? "—" : `${healthy}/${active.length} healthy`}
              </span>
            </Row>
            <Row label="Last deploy">
              {lastDeploy ? (
                <RelativeTime iso={lastDeploy} className="text-neutral-100" />
              ) : (
                <span className="text-neutral-400">never</span>
              )}
            </Row>
          </>
        )}
      </dl>

      <div className="mt-3 flex gap-2">
        <button
          type="button"
          onClick={() => onOpen(id)}
          className="h-7 flex-1 rounded-md bg-white text-xs font-medium text-neutral-900 transition-colors hover:bg-white/85"
        >
          {openLabel}
        </button>
        {mode === "apps" && onSeed && (
          <button
            type="button"
            onClick={() => onSeed(id)}
            className="h-7 flex-1 rounded-md border border-white/15 text-xs font-medium text-neutral-100 transition-colors hover:bg-white/10"
          >
            Seed
          </button>
        )}
      </div>
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-neutral-400">{label}</dt>
      <dd className="min-w-0 truncate text-right">{children}</dd>
    </div>
  );
}
