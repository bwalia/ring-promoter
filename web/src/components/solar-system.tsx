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
  bodyPoint,
  buildGlobeBodies,
  DENSITY_CAP,
  DESIGN_SIZE,
  earthSpin,
  estimateRttMs,
  formatLocation,
  globeMetrics,
  haversineKm,
  orbitPathPair,
  surfacePoint,
  type GlobeBody,
  type GlobeMetrics,
} from "@/lib/globe-layout";
import { appLatencyMs, appTtfbMs } from "@/lib/solar-layout";
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
  /** Group → member app ids, used in Rings mode when a ring is focused. */
  groupMembers?: Record<string, string[]>;
  editable?: boolean;
  onAddEdge?: (from: string, to: string) => void;
  onRemoveEdge?: (from: string, to: string) => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
  className?: string;
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
  groupMembers,
  editable = false,
  onAddEdge,
  onRemoveEdge,
  onOpen,
  onSeed,
  className,
}: SolarSystemProps) {
  const appTitle = useAppTitle();
  const title = resolveTitle ?? appTitle;
  const hex = STATUS_HEX[aggregate];
  const earthRef = useRef<EarthGlobeHandle>(null);
  const markerEls = useRef(new Map<string, HTMLElement | null>());
  const stemEls = useRef(new Map<string, SVGLineElement | null>());
  const chordEls = useRef(new Map<string, SVGPathElement | null>());
  const ringFrontEls = useRef(new Map<string, SVGPathElement | null>());
  const ringBackEls = useRef(new Map<string, SVGPathElement | null>());
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [linkFrom, setLinkFrom] = useState<string | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const active = focused ?? hovered;
  const crowded = members.length > DENSITY_CAP;
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

  const stageEl = useRef<HTMLDivElement>(null);
  const [stageSize, setStageSize] = useState({ w: DESIGN_SIZE, h: DESIGN_SIZE });
  const metrics = useMemo(
    () => globeMetrics(stageSize.w, stageSize.h),
    [stageSize],
  );
  const strokeK = Math.min(metrics.width, metrics.height) / DESIGN_SIZE;

  const bodies = useMemo(
    () =>
      buildGlobeBodies(
        members,
        (id) => locationByMember.get(id),
        (id) => ttfbByMember.get(id) ?? latencyByMember.get(id) ?? null,
        metrics,
      ),
    [members, locationByMember, ttfbByMember, latencyByMember, metrics],
  );

  const fleetCentroid = useMemo(() => {
    const pins = [...locationByMember.values()].filter((l): l is AppLocation => !!l);
    if (pins.length < 2) return pins[0] ?? null;
    return {
      lat: pins.reduce((s, p) => s + p.lat, 0) / pins.length,
      lng: pins.reduce((s, p) => s + p.lng, 0) / pins.length,
    };
  }, [locationByMember]);

  const restPoint = (body: GlobeBody) => bodyPoint(body, 0, 0, metrics);

  const positions = useMemo(() => {
    const m = new Map<string, { x: number; y: number; z: number; front: boolean }>();
    for (const b of bodies) m.set(b.id, restPoint(b));
    return m;
  }, [bodies, metrics]);

  const visibleEdges = useMemo(
    () => edges.filter((e) => members.includes(e.from) && members.includes(e.to)),
    [edges, members],
  );

  const nameEls = useRef(new Map<string, HTMLElement | null>());
  const frame = useRef({ elapsed: 0, spin: 0 });

  const applyFrame = (elapsed: number, spin: number) => {
    frame.current = { elapsed, spin };
    const nowPos = new Map<string, ReturnType<typeof bodyPoint>>();
    for (const b of bodies) {
      const pos = bodyPoint(b, elapsed, spin, metrics);
      nowPos.set(b.id, pos);
      const dim = !!active && active !== b.id;
      const el = markerEls.current.get(b.id);
      if (el) {
        const vis = dim ? (pos.front ? 0.28 : 0.1) : pos.front ? 1 : 0.28;
        el.style.left = `${(pos.x / metrics.width) * 100}%`;
        el.style.top = `${(pos.y / metrics.height) * 100}%`;
        el.style.opacity = String(vis);
        el.style.zIndex = String(pos.front ? 20 + Math.round(pos.z) : 2);
        el.style.pointerEvents = pos.front || active === b.id ? "auto" : "none";
      }
      const stem = stemEls.current.get(b.id);
      if (stem) {
        const ground = surfacePoint(b, elapsed, spin, metrics);
        stem.setAttribute("x1", String(ground.x));
        stem.setAttribute("y1", String(ground.y));
        stem.setAttribute("x2", String(pos.x));
        stem.setAttribute("y2", String(pos.y));
        stem.setAttribute(
          "stroke-opacity",
          String(dim ? 0.04 : pos.front ? 0.28 : 0.06),
        );
      }
      const { front, back } = orbitPathPair(b, spin, metrics);
      const ringFront = ringFrontEls.current.get(b.id);
      const ringBack = ringBackEls.current.get(b.id);
      ringFront?.setAttribute("d", front);
      ringBack?.setAttribute("d", back);
      const lit = active === b.id;
      ringFront?.setAttribute(
        "stroke-opacity",
        String(dim ? (crowded ? 0.1 : 0.18) : lit ? 0.95 : crowded ? 0.4 : 0.58),
      );
      ringFront?.setAttribute("stroke-width", lit ? "2.2" : crowded ? "1.05" : "1.25");
      ringBack?.setAttribute(
        "stroke-opacity",
        String(dim ? 0.05 : lit ? 0.42 : 0.18),
      );
    }
    for (const e of visibleEdges) {
      const a = nowPos.get(e.from);
      const b = nowPos.get(e.to);
      const path = chordEls.current.get(`${e.from}->${e.to}`);
      if (!a || !b || !path) continue;
      path.setAttribute("d", chordPath(a, b, metrics));
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
    // Marker refs are populated after commit; bodies/edges/metrics are the inputs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bodies, visibleEdges, reduceMotion, editMode, active, crowded, metrics]);

  useLayoutEffect(() => {
    const stage = stageEl.current;
    if (!stage) return;
    const measure = () => {
      const w = stage.clientWidth;
      const h = stage.clientHeight;
      if (w < 2 || h < 2) return;
      setStageSize((prev) => (prev.w === w && prev.h === h ? prev : { w, h }));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(stage);
    return () => ro.disconnect();
  }, []);

  useLayoutEffect(() => {
    const stage = stageEl.current;
    if (!stage) return;

    const run = () => {
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
        const pos = bodyPoint(p, elapsed, spin, metrics);
        if (!el || !pos) continue;
        if (!pos.front && p.id !== active) {
          el.style.opacity = "0";
          el.style.pointerEvents = "none";
          continue;
        }
        // Crowded fleets: only the focused/hovered nameplate stays on the
        // globe — the roster is how you read the rest.
        if (crowded && p.id !== active) {
          el.style.opacity = "0";
          el.style.pointerEvents = "none";
          continue;
        }
        el.style.opacity = "1";

        const w = el.offsetWidth;
        const h = el.offsetHeight;
        const nameRight =
          pos.x > metrics.width * 0.75
            ? false
            : pos.x < metrics.width * 0.25
              ? true
              : pos.x >= metrics.cx;
        const offset = DIAL_SIZE / 2 + 4 + w / 2;
        const cx = pos.x + (nameRight ? offset : -offset);
        const cy = pos.y;
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
  }, [bodies, statusById, active, title, crowded, metrics]);

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

  const focusBody = focused ? bodies.find((b) => b.id === focused) : null;
  const focusPos = focusBody ? positions.get(focusBody.id) : null;
  const camScale = focused && !reduceMotion ? 1.12 : 1;
  const camX =
    focusPos && !reduceMotion
      ? ((metrics.cx - focusPos.x) / metrics.width) * 18
      : 0;
  const camY =
    focusPos && !reduceMotion
      ? ((metrics.cy - focusPos.y) / metrics.height) * 18
      : 0;

  return (
    <div
      className={cn(
        "relative h-full min-h-0 overflow-hidden bg-[#07070a]",
        className,
      )}
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,rgba(56,189,248,0.06)_0%,transparent_55%)]"
      />

      <div className="pointer-events-none absolute left-4 top-3 z-40">
        <p className="font-display text-[11px] font-semibold uppercase tracking-[0.14em] text-neutral-400">
          Rings of Apps
        </p>
        <p className="mt-0.5 font-mono text-[11px] tabular-nums text-neutral-500">
          {members.length} {members.length === 1 ? bodyWord : bodyWordPlural}
          {" · "}
          isolated rings
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
        className="absolute inset-0"
        data-testid="solar-system"
        data-status={aggregate}
        onClick={(e) => {
          if (
            !(e.target as HTMLElement).closest(
              "button, [data-node-card], [data-edge], [data-orbit-ring], [data-roster]",
            )
          ) {
            setFocused(null);
            if (editMode) setLinkFrom(null);
          }
        }}
      >
        <div
          className="absolute inset-0 origin-center will-change-transform"
          style={{
            transform: `translate(${camX}%, ${camY}%) scale(${camScale})`,
            transition: reduceMotion ? "none" : "transform 0.55s cubic-bezier(0.22, 1, 0.36, 1)",
          }}
        >
        {/* Far-side orbits sit behind Earth so the disc occults them. */}
        <svg
          viewBox={`0 0 ${metrics.width} ${metrics.height}`}
          className="pointer-events-none absolute inset-0 size-full overflow-visible"
          aria-hidden
        >
          {bodies.map((b) => {
            const status = statusById.get(b.id) ?? "empty";
            const hex = STATUS_HEX[status];
            const { back } = orbitPathPair(b, 0, metrics);
            const lit = active === b.id;
            return (
              <path
                key={`orbit-back-${b.id}`}
                ref={(el) => {
                  ringBackEls.current.set(b.id, el);
                }}
                d={back}
                fill="none"
                stroke={hex}
                strokeWidth={(lit ? 1.3 : 0.85) * strokeK}
                strokeOpacity={lit ? 0.4 : 0.22}
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            );
          })}
        </svg>

        <EarthGlobe
          ref={earthRef}
          metrics={metrics}
          className="pointer-events-none absolute inset-0 size-full"
        />

        <SunHub label={sunLabel} metrics={metrics} />

        <svg
          viewBox={`0 0 ${metrics.width} ${metrics.height}`}
          className="absolute inset-0 size-full overflow-visible"
        >
          {bodies.map((b) => {
            const status = statusById.get(b.id) ?? "empty";
            const hex = STATUS_HEX[status];
            const { front } = orbitPathPair(b, 0, metrics);
            const lit = active === b.id;
            return (
              <g key={`orbit-front-${b.id}`}>
                <path
                  d={front}
                  fill="none"
                  stroke="transparent"
                  strokeWidth={10 * strokeK}
                  strokeLinecap="round"
                  className="cursor-pointer"
                  data-orbit-ring={b.id}
                  onClick={(ev) => {
                    ev.stopPropagation();
                    onBodyClick(b.id);
                  }}
                />
                <path
                  ref={(el) => {
                    ringFrontEls.current.set(b.id, el);
                  }}
                  d={front}
                  fill="none"
                  stroke={hex}
                  strokeWidth={(lit ? 2.2 : crowded ? 1.05 : 1.25) * strokeK}
                  strokeOpacity={lit ? 0.95 : crowded ? 0.4 : 0.58}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  className="pointer-events-none"
                />
              </g>
            );
          })}

          {bodies.map((b) => {
            const pos = positions.get(b.id);
            if (!pos) return null;
            const ground = surfacePoint(b, 0, 0, metrics);
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
                strokeWidth={0.7 * strokeK}
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
                  d={chordPath(a, b, metrics)}
                  fill="none"
                  stroke={e.source === "config" ? "#a3a3a3" : "#737373"}
                  strokeWidth={(editMode ? 1.6 : 0.9) * strokeK}
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
          const left = (pos.x / metrics.width) * 100;
          const top = (pos.y / metrics.height) * 100;
          const nameRight =
            pos.x > metrics.width * 0.75
              ? false
              : pos.x < metrics.width * 0.25
                ? true
                : pos.x >= metrics.cx;
          const below = pos.y >= metrics.cy;
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
                          members={mode === "groups" ? groupMembers?.[p.id] : undefined}
                          resolveTitle={title}
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
        </div>

        {/* Legend — inset from the overlay roster so it stays readable. */}
        <div className="pointer-events-none absolute bottom-[min(13.5rem,34vh)] left-2 right-2 z-30 flex flex-wrap items-center justify-between gap-2 text-[11px] leading-none text-neutral-400 lg:bottom-2 lg:right-[19.5rem]">
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
            <span>
              Ring Promoter · Earth · one ring per {bodyWord} · closer = lower
              TTFB
            </span>
          </span>
        </div>
      </div>

      <AppRoster
        members={members}
        title={title}
        statusById={statusById}
        ttfbByMember={ttfbByMember}
        latencyByMember={latencyByMember}
        locationByMember={locationByMember}
        subtitles={subtitles}
        mode={mode}
        active={active}
        groupMembers={groupMembers}
        onFocus={(id) => setFocused((f) => (f === id ? null : id))}
        onOpen={onOpen}
      />
    </div>
  );
}

function SunHub({ label, metrics }: { label: string; metrics: GlobeMetrics }) {
  const left = ((metrics.cx + metrics.sunOffsetX) / metrics.width) * 100;
  const top = (metrics.cy / metrics.height) * 100;
  const size = Math.max(36, metrics.sunR * 2);
  // Two lines when the brand is "Ring Promoter" so it stays readable on the
  // disc without competing with Earth; otherwise keep a single centered line.
  const lines =
    label.trim().toLowerCase() === "ring promoter"
      ? (["Ring", "Promoter"] as const)
      : ([label] as const);
  const fontPx = Math.max(7, Math.min(11, size * 0.145));
  return (
    <div
      className="pointer-events-none absolute z-[8]"
      style={{ left: `${left}%`, top: `${top}%` }}
      data-sun-hub
      aria-label={label}
    >
      <div className="-translate-x-1/2 -translate-y-1/2">
        <div
          className="relative flex items-center justify-center rounded-full"
          style={{
            width: size,
            height: size,
            background:
              "radial-gradient(circle at 35% 32%, #fff7d6 0%, #f5b942 42%, #c2410c 100%)",
            boxShadow:
              "0 0 18px 6px rgba(245, 185, 66, 0.28), 0 0 42px 12px rgba(245, 185, 66, 0.12)",
          }}
        >
          <p
            className="m-0 px-1 text-center font-display font-semibold uppercase leading-[1.05] tracking-[0.06em] text-black"
            style={{ fontSize: fontPx }}
          >
            {lines.map((line) => (
              <span key={line} className="block">
                {line}
              </span>
            ))}
          </p>
        </div>
      </div>
    </div>
  );
}

function AppRoster({
  members,
  title,
  statusById,
  ttfbByMember,
  latencyByMember,
  locationByMember,
  subtitles,
  mode,
  active,
  groupMembers,
  onFocus,
  onOpen,
}: {
  members: string[];
  title: (id: string) => string;
  statusById: Map<string, NodeStatus>;
  ttfbByMember: Map<string, number | null>;
  latencyByMember: Map<string, number | null>;
  locationByMember: Map<string, AppLocation | null>;
  subtitles?: Record<string, string>;
  mode: "apps" | "groups";
  active: string | null;
  groupMembers?: Record<string, string[]>;
  onFocus: (id: string) => void;
  onOpen: (id: string) => void;
}) {
  const appTitle = useAppTitle();
  const ordered = [...members].sort((a, b) => {
    const ma = ttfbByMember.get(a) ?? latencyByMember.get(a);
    const mb = ttfbByMember.get(b) ?? latencyByMember.get(b);
    if (ma == null && mb == null) return title(a).localeCompare(title(b));
    if (ma == null) return 1;
    if (mb == null) return -1;
    if (ma !== mb) return ma - mb;
    return title(a).localeCompare(title(b));
  });

  return (
    <div
      data-roster
      data-testid="app-roster"
      className="absolute inset-x-0 bottom-0 z-30 max-h-[min(200px,32vh)] overflow-auto border-t border-white/10 bg-[#07070a]/80 backdrop-blur-md lg:inset-y-0 lg:left-auto lg:right-0 lg:max-h-none lg:w-[19.5rem] lg:border-l lg:border-t-0"
    >
      <div className="sticky top-0 z-10 border-b border-white/10 bg-[#07070a]/90 px-3 py-2 backdrop-blur-md">
        <p className="font-display text-[10px] font-semibold uppercase tracking-[0.14em] text-neutral-500">
          {mode === "groups" ? "Rings" : "Applications"}
        </p>
      </div>
      <ul className="divide-y divide-white/5 p-1.5">
        {ordered.map((id) => {
          const status = statusById.get(id) ?? "empty";
          const hex = STATUS_HEX[status];
          const ttfb = ttfbByMember.get(id);
          const lat = latencyByMember.get(id);
          const shown = ttfb ?? lat;
          const loc = formatLocation(locationByMember.get(id));
          const selected = active === id;
          const kids = mode === "groups" ? groupMembers?.[id] : undefined;
          return (
            <li key={id}>
              <button
                type="button"
                onClick={() => onFocus(id)}
                onDoubleClick={() => onOpen(id)}
                className={cn(
                  "flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors",
                  selected ? "bg-white/10" : "hover:bg-white/[0.06]",
                )}
              >
                <span
                  aria-hidden
                  className="mt-1 size-2 shrink-0 rounded-full"
                  style={{ background: hex }}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-display text-[12px] font-medium leading-tight text-neutral-100">
                    {title(id)}
                  </span>
                  <span className="mt-0.5 flex flex-wrap items-center gap-x-2 font-mono text-[10px] tabular-nums text-neutral-500">
                    <span style={{ color: hex }}>{STATUS_WORD[status]}</span>
                    <span>
                      {shown != null ? `${Math.round(shown)}ms TTFB` : "—"}
                    </span>
                    {loc ? <span className="truncate text-neutral-600">{loc}</span> : null}
                    {subtitles?.[id] ? (
                      <span className="text-neutral-600">{subtitles[id]}</span>
                    ) : null}
                  </span>
                  {selected && kids && kids.length > 0 && (
                    <span className="mt-1 block text-[10.5px] leading-snug text-neutral-400">
                      {kids.map((m) => appTitle(m)).join(" · ")}
                    </span>
                  )}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
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
  metrics: GlobeMetrics,
): string {
  const mx = (a.x + b.x) / 2;
  const my = (a.y + b.y) / 2;
  let dx = mx - metrics.cx;
  let dy = my - metrics.cy;
  let dist = Math.hypot(dx, dy);
  if (dist < 1) {
    // Chord passes dead through the hub: bow along the segment's normal.
    dx = -(b.y - a.y);
    dy = b.x - a.x;
    dist = Math.hypot(dx, dy) || 1;
  }
  // The closer the midpoint sits to the hub, the harder it needs to bow. The
  // threshold is comfortably wider than the hub radius so chords clear
  // its edge rather than grazing it.
  const bow = Math.max(0, metrics.earthR + 46 * (Math.min(metrics.width, metrics.height) / DESIGN_SIZE) - dist) * 1.35;
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
  members,
  resolveTitle,
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
  members?: string[];
  resolveTitle?: (id: string) => string;
  pinned: boolean;
  openLabel: string;
  onClose: () => void;
  onOpen: (id: string) => void;
  onSeed?: (id: string) => void;
}) {
  const hex = STATUS_HEX[status];
  const { active, healthy, latest, lastDeploy } = summarizeRings(rings?.rings);
  const appTitle = useAppTitle();
  const memberLabel = (m: string) =>
    mode === "groups" ? appTitle(m) : (resolveTitle?.(m) ?? m);

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
            {members && members.length > 0 && (
              <p className="text-[11px] leading-snug text-neutral-400">
                {members.map((m) => memberLabel(m)).join(" · ")}
              </p>
            )}
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
