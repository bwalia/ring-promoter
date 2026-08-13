"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Link2, X } from "lucide-react";
import { EarthGlobe, type EarthGlobeHandle } from "@/components/earth-globe";
import { RelativeTime } from "@/components/relative-time";
import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import { DIAL_SIZE, OrbitDial, ringSegments } from "@/components/orbit-body";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import {
  ALT_MAX,
  ALT_MIN,
  bodyPoint,
  buildGlobeBodies,
  EARTH_R,
  earthSpin,
  estimateRttMs,
  formatLocation,
  haversineKm,
  surfacePoint,
  type GlobeBody,
} from "@/lib/globe-layout";
import { appLatencyMs, appTtfbMs, SOLAR_C } from "@/lib/solar-layout";
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
  ttfbById?: Record<string, number | null>;
  locations?: Record<string, AppLocation | null | undefined>;
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
  ttfbById,
  locations,
  editable = false,
  onAddEdge,
  onRemoveEdge,
  onOpen,
  onSeed,
}: SolarSystemProps) {
  const appTitle = useAppTitle();
  const title = resolveTitle ?? appTitle;
  const hex = STATUS_HEX[aggregate];
  const earthRef = useRef<EarthGlobeHandle>(null);
  const markerEls = useRef(new Map<string, HTMLElement | null>());
  const stemEls = useRef(new Map<string, SVGLineElement | null>());
  const chordEls = useRef(new Map<string, SVGPathElement | null>());
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

  const ttfbByMember = useMemo(() => {
    const m = new Map<string, number | null>();
    members.forEach((id, i) => {
      if (ttfbById && id in ttfbById) {
        m.set(id, ttfbById[id] ?? null);
      } else {
        m.set(id, appTtfbMs(results[i]?.rings));
      }
    });
    return m;
  }, [members, results, ttfbById]);

  const locationByMember = useMemo(() => {
    const m = new Map<string, AppLocation | null>();
    for (const id of members) m.set(id, locations?.[id] ?? null);
    return m;
  }, [members, locations]);

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

  const bodies = useMemo(
    () =>
      buildGlobeBodies(
        members,
        (id) => locationByMember.get(id),
        (id) => ttfbByMember.get(id) ?? latencyByMember.get(id) ?? null,
      ),
    [members, locationByMember, ttfbByMember, latencyByMember],
  );

  const fleetCentroid = useMemo(() => {
    const pins = [...locationByMember.values()].filter((l): l is AppLocation => !!l);
    if (pins.length < 2) return pins[0] ?? null;
    return {
      lat: pins.reduce((s, p) => s + p.lat, 0) / pins.length,
      lng: pins.reduce((s, p) => s + p.lng, 0) / pins.length,
    };
  }, [locationByMember]);

  const restPoint = (body: GlobeBody) => bodyPoint(body, 0, 0);

  const positions = useMemo(() => {
    const m = new Map<string, { x: number; y: number; z: number; front: boolean }>();
    for (const b of bodies) m.set(b.id, restPoint(b));
    return m;
  }, [bodies]);

  const visibleEdges = useMemo(
    () => edges.filter((e) => members.includes(e.from) && members.includes(e.to)),
    [edges, members],
  );

  const stageEl = useRef<HTMLDivElement>(null);
  const nameEls = useRef(new Map<string, HTMLElement | null>());
  const frame = useRef({ elapsed: 0, spin: 0 });

  const applyFrame = (elapsed: number, spin: number) => {
    frame.current = { elapsed, spin };
    const nowPos = new Map<string, ReturnType<typeof bodyPoint>>();
    for (const b of bodies) {
      const pos = bodyPoint(b, elapsed, spin);
      nowPos.set(b.id, pos);
      const el = markerEls.current.get(b.id);
      if (el) {
        el.style.left = `${(pos.x / 400) * 100}%`;
        el.style.top = `${(pos.y / 400) * 100}%`;
        el.style.opacity = pos.front ? "1" : "0.22";
        el.style.zIndex = String(pos.front ? 20 + Math.round(pos.z) : 2);
        el.style.pointerEvents = pos.front || active === b.id ? "auto" : "none";
      }
      const stem = stemEls.current.get(b.id);
      if (stem) {
        const ground = surfacePoint(b, elapsed, spin);
        stem.setAttribute("x1", String(ground.x));
        stem.setAttribute("y1", String(ground.y));
        stem.setAttribute("x2", String(pos.x));
        stem.setAttribute("y2", String(pos.y));
        stem.setAttribute(
          "stroke-opacity",
          String(pos.front ? 0.38 : 0.08),
        );
      }
    }
    for (const e of visibleEdges) {
      const a = nowPos.get(e.from);
      const b = nowPos.get(e.to);
      const path = chordEls.current.get(`${e.from}->${e.to}`);
      if (!a || !b || !path) continue;
      path.setAttribute("d", chordPath(a, b));
      path.setAttribute(
        "stroke-opacity",
        String(a.front || b.front ? (editMode ? 0.5 : 0.22) : 0.06),
      );
    }
  };

  useEffect(() => {
    earthRef.current?.setSpin(0);
    applyFrame(0, 0);
    if (reduceMotion) return;
    const start = performance.now();
    let id = 0;
    const loop = (now: number) => {
      const elapsed = (now - start) / 1000;
      const spin = earthSpin(elapsed, false);
      earthRef.current?.setSpin(spin);
      applyFrame(elapsed, spin);
      id = requestAnimationFrame(loop);
    };
    id = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(id);
    // Marker refs are populated after commit; bodies/edges are the inputs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bodies, visibleEdges, reduceMotion, editMode, active]);

  useLayoutEffect(() => {
    const stage = stageEl.current;
    if (!stage) return;

    const run = () => {
      const scale = stage.clientWidth / 400 || 1;
      const spin = frame.current.spin;
      const elapsed = frame.current.elapsed;
      const ordered = [...bodies].sort((x, y) => {
        const rank = (id: string) => {
          if (id === active) return 0;
          const st = statusById.get(id) ?? "empty";
          return st === "failed" || st === "degraded" || st === "deploying"
            ? 1
            : 2;
        };
        return rank(x.id) - rank(y.id);
      });

      const kept: { l: number; r: number; t: number; b: number }[] = [];
      for (const p of ordered) {
        const el = nameEls.current.get(p.id);
        const pos = bodyPoint(p, elapsed, spin);
        if (!el || !pos) continue;
        if (!pos.front && p.id !== active) {
          el.style.opacity = "0";
          el.style.pointerEvents = "none";
          continue;
        }
        el.style.opacity = "1";

        const w = el.offsetWidth;
        const h = el.offsetHeight;
        const nameRight =
          pos.x > 300 ? false : pos.x < 100 ? true : pos.x >= SOLAR_C;
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
        if (clash && p.id !== active) {
          el.style.opacity = "0";
          el.style.pointerEvents = "none";
        } else {
          el.style.pointerEvents = "";
          kept.push(box);
        }
      }
    };

    run();
    document.fonts?.ready.then(run).catch(() => {});
    const ro = new ResizeObserver(run);
    ro.observe(stage);
    const id = window.setInterval(run, 280);
    return () => {
      ro.disconnect();
      window.clearInterval(id);
    };
  }, [bodies, statusById, active, title]);

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
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,rgba(56,189,248,0.06)_0%,transparent_55%)]"
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
        <EarthGlobe
          ref={earthRef}
          className="pointer-events-none absolute inset-0 size-full"
        />

        <svg
          viewBox="0 0 400 400"
          className="absolute inset-0 size-full overflow-visible"
        >
          {/* Altitude shells: TTFB as height above Earth. */}
          {[EARTH_R + ALT_MIN, EARTH_R + (ALT_MIN + ALT_MAX) / 2, EARTH_R + ALT_MAX].map(
            (r, i) => (
              <circle
                key={r}
                cx={SOLAR_C}
                cy={SOLAR_C}
                r={r}
                fill="none"
                stroke="#7dd3fc"
                strokeWidth={0.6}
                strokeOpacity={i === 1 ? 0.1 : 0.06}
                strokeDasharray="2 7"
              />
            ),
          )}

          {bodies.map((b) => {
            const pos = positions.get(b.id);
            if (!pos) return null;
            const ground = surfacePoint(b, 0, 0);
            return (
              <line
                key={`stem-${b.id}`}
                ref={(el) => {
                  stemEls.current.set(b.id, el);
                }}
                x1={ground.x}
                y1={ground.y}
                x2={pos.x}
                y2={pos.y}
                stroke="#7dd3fc"
                strokeWidth={0.7}
                strokeOpacity={0.38}
              />
            );
          })}

          {visibleEdges.map((e) => {
            const a = positions.get(e.from);
            const b = positions.get(e.to);
            if (!a || !b) return null;
            const key = `${e.from}->${e.to}`;
            return (
              <g key={key} data-edge>
                <path
                  ref={(el) => {
                    chordEls.current.set(key, el);
                  }}
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
        </svg>

        {/* Bodies */}
        {bodies.map((p, i) => {
          const pos = positions.get(p.id);
          if (!pos) return null;
          const status = statusById.get(p.id) ?? "empty";
          const expanded = active === p.id && !editMode;
          const linking = editMode && linkFrom === p.id;
          const lat = latencyByMember.get(p.id);
          const ttfb = ttfbByMember.get(p.id);
          const loc = locationByMember.get(p.id);
          const rings = resultById.get(p.id);
          const segs = ringSegments(rings?.rings, ringOrder);
          const { latest } = summarizeRings(rings?.rings);
          const left = (pos.x / 400) * 100;
          const top = (pos.y / 400) * 100;
          const nameRight =
            pos.x > 300 ? false : pos.x < 100 ? true : pos.x >= SOLAR_C;
          const below = pos.y >= SOLAR_C;
          const estMs =
            loc && fleetCentroid
              ? estimateRttMs(haversineKm(loc, fleetCentroid))
              : null;
          const shownMs = ttfb ?? lat ?? estMs;

          return (
            <div
              key={p.id}
              ref={(el) => {
                markerEls.current.set(p.id, el);
              }}
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
                      {shownMs != null && (
                        <span className="text-neutral-600">
                          {" · "}
                          {ttfb == null && lat == null ? "~" : ""}
                          {Math.round(shownMs)}ms TTFB
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
                          ttfbMs={ttfb ?? null}
                          estimatedMs={estMs}
                          location={loc}
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
            <span>closer = lower TTFB</span>
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
  const bow = Math.max(0, EARTH_R + 46 - dist) * 1.35;
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
  ttfbMs,
  estimatedMs,
  location,
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
  ttfbMs: number | null;
  estimatedMs: number | null;
  location: AppLocation | null | undefined;
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
        {formatLocation(location) && (
          <Row label="Location">
            <span className="text-neutral-100">{formatLocation(location)}</span>
          </Row>
        )}
        <Row label="TTFB">
          {ttfbMs != null ? (
            <span className="font-mono tabular-nums text-neutral-100">
              {Math.round(ttfbMs)}ms
            </span>
          ) : estimatedMs != null ? (
            <span className="font-mono tabular-nums text-neutral-300">
              ~{estimatedMs}ms
              <span className="text-neutral-500"> est.</span>
            </span>
          ) : (
            <span className="text-neutral-400">—</span>
          )}
        </Row>
        <Row label="Check">
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
