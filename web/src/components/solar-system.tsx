"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Link2, X } from "lucide-react";
import { EarthGlobe, type EarthGlobeHandle } from "@/components/earth-globe";
import { RelativeTime } from "@/components/relative-time";
import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import {
  DIAL_SIZE,
  dialSizeForStatus,
  OrbitDial,
  ringSegments,
} from "@/components/orbit-body";
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
import {
  placeLabel,
  sunBox,
  type Box,
  type LabelSpot,
} from "@/lib/label-layout";
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

/** Sparse starfield for the orbital void — same motif as group-ring, quieter. */
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
  const leaderEls = useRef(new Map<string, SVGLineElement | null>());
  const chordEls = useRef(new Map<string, SVGPathElement | null>());
  const ringFrontEls = useRef(new Map<string, SVGPathElement | null>());
  const ringBackEls = useRef(new Map<string, SVGPathElement | null>());
  /** Last accepted label spot per body — retried first, so labels stay put. */
  const labelPrev = useRef(new Map<string, LabelSpot>());
  /** Measured label box per body, keyed by its current content. */
  const labelSizes = useRef(new Map<string, { w: number; h: number; key: string }>());
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
  // Roster is a bottom sheet below `lg`, a right rail at `lg+` — match the
  // Tailwind breakpoint so Earth centres in the clear sky, not under the sheet.
  const [sideRoster, setSideRoster] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia("(min-width: 1024px)");
    const sync = () => setSideRoster(mq.matches);
    sync();
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);
  const { metrics, bounds } = useMemo(() => {
    const top = 56; // mode picker / "Rings of Apps" chrome
    const bottom = sideRoster
      ? 36 // legend only — roster is a side rail
      : Math.min(200, Math.round(stageSize.h * 0.32));
    const right = sideRoster ? 312 : 0; // 19.5rem roster rail overlays the stage
    const metrics = globeMetrics(stageSize.w, stageSize.h, {
      top,
      bottom,
      right,
      verticalBias: 0.46,
    });
    // Where labels are allowed to live: the clear sky between the chrome.
    const bounds: Box = {
      l: 6,
      r: Math.max(60, stageSize.w - right - 6),
      t: top + 4,
      b: Math.max(top + 40, stageSize.h - bottom - 6),
    };
    return { metrics, bounds };
  }, [stageSize, sideRoster]);
  const strokeK = metrics.k;

  const sizeById = useMemo(() => {
    const m = new Map<string, number>();
    for (const id of members)
      m.set(id, dialSizeForStatus(statusById.get(id) ?? "empty"));
    return m;
  }, [members, statusById]);

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
  /** Animation epoch survives effect re-runs so hover/focus never rewinds time. */
  const epochRef = useRef<number | null>(null);

  const applyFrame = (elapsed: number, spin: number) => {
    frame.current = { elapsed, spin };
    const nowPos = new Map<string, ReturnType<typeof bodyPoint>>();
    for (const b of bodies) {
      const pos = bodyPoint(b, elapsed, spin, metrics);
      nowPos.set(b.id, pos);
      // Focus dims everything else to ~20%; hover only brightens its target.
      const dim = !!focused && focused !== b.id;
      const el = markerEls.current.get(b.id);
      if (el) {
        const vis = dim ? (pos.front ? 0.2 : 0.08) : pos.front ? 1 : 0.35;
        el.style.transform = `translate3d(${pos.x.toFixed(1)}px, ${pos.y.toFixed(1)}px, 0)`;
        el.style.opacity = String(vis);
        el.style.zIndex =
          active === b.id ? "60" : String(pos.front ? 20 + Math.round(pos.z) : 2);
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
          String(dim ? 0.03 : pos.front ? 0.16 : 0.05),
        );
      }
      const { front, back } = orbitPathPair(b, spin, metrics);
      const ringFront = ringFrontEls.current.get(b.id);
      const ringBack = ringBackEls.current.get(b.id);
      ringFront?.setAttribute("d", front);
      ringBack?.setAttribute("d", back);
      // Orbit paths whisper (~10–15%) until their satellite is hovered or
      // selected — then that one ring, and only that one, goes to full.
      const lit = active === b.id;
      ringFront?.setAttribute(
        "stroke-opacity",
        String(dim ? 0.05 : lit ? 1 : crowded ? 0.07 : 0.09),
      );
      ringFront?.setAttribute(
        "stroke-width",
        String((lit ? 2.2 : 1.1) * strokeK),
      );
      ringBack?.setAttribute(
        "stroke-opacity",
        String(dim ? 0.02 : lit ? 0.4 : 0.04),
      );
      ringBack?.setAttribute(
        "stroke-width",
        String((lit ? 1.3 : 0.85) * strokeK),
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
        String(
          focused
            ? 0.06
            : a.front || b.front
              ? editMode
                ? 0.5
                : 0.18
              : 0.06,
        ),
      );
    }

    // ---- Label pass: collision-free placement with leader lines. --------
    // Obstacles the solver must clear before any label is placed: the sun
    // (disc + glow + caption) and every satellite dial.
    const taken: Box[] = [sunBox(metrics)];
    for (const b of bodies) {
      const pos = nowPos.get(b.id)!;
      const half = (sizeById.get(b.id) ?? DIAL_SIZE) / 2 + 2;
      taken.push({
        l: pos.x - half,
        r: pos.x + half,
        t: pos.y - half,
        b: pos.y + half,
      });
    }
    const rank = (b: GlobeBody) => {
      if (b.id === active) return 0;
      const st = statusById.get(b.id) ?? "empty";
      const bad = st === "failed" || st === "degraded" || st === "deploying";
      const front = nowPos.get(b.id)?.front ?? false;
      if (bad) return front ? 1 : 2;
      return front ? 3 : 4;
    };
    const orderedLabels = [...bodies].sort((a, b) => rank(a) - rank(b));
    for (const p of orderedLabels) {
      const el = nameEls.current.get(p.id);
      const leader = leaderEls.current.get(p.id);
      const pos = nowPos.get(p.id);
      if (!el || !pos) continue;
      const dimmed = !!focused && focused !== p.id;
      // Back-side names stay hidden — the faded ghosts on the globe were
      // the main source of clutter. Hover/focus still reveals them.
      const show = !dimmed && (pos.front || active === p.id);
      if (!show) {
        el.style.opacity = "0";
        el.style.pointerEvents = "none";
        leader?.setAttribute("stroke-opacity", "0");
        labelPrev.current.delete(p.id);
        continue;
      }
      const key = active === p.id ? "expanded" : "name";
      let size = labelSizes.current.get(p.id);
      if (!size || size.key !== key) {
        size = { w: el.offsetWidth, h: el.offsetHeight, key };
        labelSizes.current.set(p.id, size);
      }
      const rad = (sizeById.get(p.id) ?? DIAL_SIZE) / 2;
      const placed = placeLabel(
        pos.x,
        pos.y,
        rad,
        size.w,
        size.h,
        bounds,
        taken,
        labelPrev.current.get(p.id) ?? null,
        metrics,
      );
      if (!placed) {
        el.style.opacity = "0";
        el.style.pointerEvents = "none";
        leader?.setAttribute("stroke-opacity", "0");
        labelPrev.current.delete(p.id);
        continue;
      }
      taken.push(placed.box);
      labelPrev.current.set(p.id, placed.spot);
      el.style.opacity = "1";
      el.style.pointerEvents = "auto";
      el.style.transform = `translate(-50%, -50%) translate(${(placed.cx - pos.x).toFixed(1)}px, ${(placed.cy - pos.y).toFixed(1)}px)`;
      if (leader) {
        const sideNow = placed.cx >= pos.x ? 1 : -1;
        const nearX = placed.cx - sideNow * (size.w / 2 + 2);
        const ang = Math.atan2(placed.cy - pos.y, nearX - pos.x);
        leader.setAttribute("x1", (pos.x + Math.cos(ang) * (rad + 1)).toFixed(1));
        leader.setAttribute("y1", (pos.y + Math.sin(ang) * (rad + 1)).toFixed(1));
        leader.setAttribute("x2", nearX.toFixed(1));
        leader.setAttribute("y2", placed.cy.toFixed(1));
        leader.setAttribute("stroke-opacity", "0.35");
      }
    }
  };

  useEffect(() => {
    earthRef.current?.setSpin(frame.current.spin);
    applyFrame(frame.current.elapsed, frame.current.spin);
    // Once webfonts land, measured label sizes are stale — measure again.
    document.fonts?.ready
      .then(() => {
        labelSizes.current.clear();
        applyFrame(frame.current.elapsed, frame.current.spin);
      })
      .catch(() => {});
    if (reduceMotion) return;
    if (epochRef.current == null) epochRef.current = performance.now();
    const start = epochRef.current;
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
  }, [
    bodies,
    visibleEdges,
    reduceMotion,
    editMode,
    active,
    focused,
    crowded,
    metrics,
    bounds,
    sizeById,
    statusById,
  ]);

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
      data-solar-ambient
    >
      <StageAtmosphere accent={hex} />

      <div className="pointer-events-none absolute left-3 top-3 z-40 sm:left-4">
        <p className="font-display text-[12px] font-semibold tracking-tight text-neutral-100 sm:text-[13px]">
          Rings of Apps
        </p>
        <p className="mt-0.5 font-mono text-[10px] tabular-nums text-neutral-500 sm:text-[11px]">
          {members.length} {members.length === 1 ? bodyWord : bodyWordPlural}
          {" · "}
          one ring each · Earth hub
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
                strokeOpacity={lit ? 0.45 : crowded ? 0.06 : 0.1}
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

        <SunHub label={sunLabel} metrics={metrics} reduceMotion={reduceMotion} />

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
                  strokeWidth={(lit ? 2.4 : 1.25) * strokeK}
                  strokeOpacity={lit ? 1 : crowded ? 0.14 : 0.28}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeDasharray={lit ? undefined : `${14 * strokeK} ${10 * strokeK}`}
                  className={cn(
                    "pointer-events-none",
                    !reduceMotion && !lit && "[animation:orbit-shimmer_28s_linear_infinite]",
                  )}
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
                strokeOpacity={0.16}
              />
            );
          })}

          {/* Leader lines: satellite → its placed name plate. The frame
              loop drives the endpoints alongside the label solver. */}
          {bodies.map((b) => (
            <line
              key={`leader-${b.id}`}
              ref={(el) => {
                leaderEls.current.set(b.id, el);
              }}
              x1={0}
              y1={0}
              x2={0}
              y2={0}
              stroke="#a1a1aa"
              strokeWidth={0.8 * strokeK}
              strokeOpacity={0}
              strokeLinecap="round"
              className="pointer-events-none"
            />
          ))}

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
                  strokeOpacity={editMode ? 0.5 : 0.18}
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
          const expanded = focused === p.id && !editMode;
          const linking = editMode && linkFrom === p.id;
          const lat = latencyByMember.get(p.id);
          const ttfb = ttfbByMember.get(p.id);
          const loc = locationByMember.get(p.id);
          const rings = resultById.get(p.id);
          const segs = ringSegments(rings?.rings, ringOrder);
          const { latest } = summarizeRings(rings?.rings);
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
              className="absolute left-0 top-0 z-10"
              style={{
                transform: `translate3d(${pos.x.toFixed(1)}px, ${pos.y.toFixed(1)}px, 0)`,
              }}
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
                      size={sizeById.get(p.id)}
                      reduceMotion={reduceMotion}
                    />
                  </button>

                  {/* Name plate — the frame loop places it collision-free
                      and ties it back with a leader line. Name only at
                      rest; metrics appear on hover/focus. */}
                  <button
                    ref={(el) => {
                      nameEls.current.set(p.id, el);
                    }}
                    type="button"
                    onClick={() => onBodyClick(p.id)}
                    title={title(p.id)}
                    className="absolute left-1/2 top-1/2 z-10 flex flex-col items-center whitespace-nowrap rounded-md bg-[#07070a]/92 px-1.5 py-0.5 text-center opacity-0 transition-opacity duration-200"
                  >
                    <span
                      className={cn(
                        "max-w-[8.5rem] truncate font-display text-[11px] font-medium leading-tight tracking-tight",
                        active === p.id || linking
                          ? "text-neutral-50"
                          : "text-neutral-200",
                      )}
                    >
                      {title(p.id)}
                    </span>
                    {active === p.id && (
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
                    )}
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

/**
 * Sun = Ring Promoter. Parks top-left, clear of the ring band — a readable
 * hub identity, not a decorative speck. `globeMetrics` keeps the glow off orbits.
 */
function SunHub({
  label,
  metrics,
  reduceMotion,
}: {
  label: string;
  metrics: GlobeMetrics;
  reduceMotion: boolean;
}) {
  const left = (metrics.sunX / metrics.width) * 100;
  const top = (metrics.sunY / metrics.height) * 100;
  const size = Math.max(22, metrics.sunR * 2);
  return (
    <div
      className="pointer-events-none absolute z-[8]"
      style={{ left: `${left}%`, top: `${top}%` }}
      data-sun-hub
      aria-label={label}
    >
      <div className="flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-2">
        <div className="relative" style={{ width: size, height: size }}>
          {!reduceMotion && (
            <div
              aria-hidden
              className="absolute inset-[-40%] rounded-full [animation:sun-corona_5.5s_ease-in-out_infinite]"
              style={{
                background:
                  "radial-gradient(circle, rgba(245,185,66,0.42) 0%, rgba(245,185,66,0.08) 45%, transparent 70%)",
              }}
            />
          )}
          <div
            className="relative size-full rounded-full"
            style={{
              background:
                "radial-gradient(circle at 35% 32%, #fff7d6 0%, #f5b942 42%, #c2410c 100%)",
              boxShadow: `0 0 ${Math.round(size * 0.65)}px ${Math.round(size * 0.22)}px rgba(245, 185, 66, 0.34), 0 0 ${Math.round(size * 1.4)}px ${Math.round(size * 0.45)}px rgba(245, 185, 66, 0.12)`,
            }}
          />
        </div>
        <div className="text-center">
          <p className="m-0 max-w-[9rem] whitespace-nowrap text-center font-display text-[11px] font-semibold leading-tight tracking-tight text-neutral-50 sm:text-[12px]">
            {label}
          </p>
          <p className="m-0 mt-0.5 font-mono text-[9px] uppercase leading-none tracking-[0.14em] text-neutral-500">
            Control plane
          </p>
        </div>
      </div>
    </div>
  );
}

function StageAtmosphere({ accent }: { accent: string }) {
  return (
    <>
      <div
        aria-hidden
        className="absolute inset-0 opacity-[0.35]"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.025) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.025) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
          maskImage:
            "radial-gradient(ellipse 70% 60% at 45% 42%, black 20%, transparent 75%)",
        }}
      />
      <div aria-hidden className="absolute inset-0">
        {STAGE_STARS.map((s, i) => (
          <span
            key={i}
            className="absolute rounded-full bg-white [animation:twinkle_var(--d)_ease-in-out_infinite]"
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
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_45%_42%,rgba(56,189,248,0.07)_0%,transparent_52%)]"
      />
      <div
        aria-hidden
        className="absolute -left-20 top-0 size-72 rounded-full opacity-[0.12] blur-3xl [animation:blob-drift_18s_ease-in-out_infinite]"
        style={{ background: accent }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_48%,rgba(0,0,0,0.55)_100%)]"
      />
    </>
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
      className="absolute inset-x-0 bottom-0 z-30 max-h-[min(200px,32vh)] overflow-auto border-t border-white/10 bg-[#07070a]/88 backdrop-blur-xl lg:inset-y-0 lg:left-auto lg:right-0 lg:max-h-none lg:w-[19.5rem] lg:border-l lg:border-t-0"
    >
      <div className="sticky top-0 z-10 border-b border-white/10 bg-[#07070a]/95 px-3 py-2.5 backdrop-blur-md">
        <p className="font-display text-[12px] font-semibold tracking-tight text-neutral-100">
          {mode === "groups" ? "Rings" : "Applications"}
        </p>
        <p className="mt-0.5 font-mono text-[10px] tabular-nums text-neutral-500">
          {ordered.length} · sorted by TTFB · click to focus
        </p>
      </div>
      <ul className="divide-y divide-white/[0.04] p-1.5">
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
                  "flex w-full items-start gap-2.5 rounded-md px-2 py-2 text-left transition-colors",
                  selected
                    ? "bg-white/[0.12] ring-1 ring-white/15"
                    : "hover:bg-white/[0.06]",
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
  const bow = Math.max(0, metrics.earthR + 46 * metrics.k - dist) * 1.35;
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
