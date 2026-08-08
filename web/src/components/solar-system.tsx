"use client";

import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import {
  AnimatePresence,
  motion,
  useMotionValue,
  useSpring,
  useTransform,
} from "motion/react";
import { Link2, X } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import { useAppTitle, type GroupAppRings } from "@/lib/queries";
import {
  appLatencyMs,
  buildOrbitPlanets,
  LABEL_ORBIT_GAP,
  latencyToRadius,
  ORBIT_TRACKS,
  planetPosition,
  SOLAR_C,
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

const STARS = Array.from({ length: 52 }, (_, i) => {
  const h = (n: number) => (((n * 9301 + 49297) % 233280) / 233280 + 1) % 1;
  return {
    x: h(i * 3 + 1) * 100,
    y: h(i * 7 + 2) * 100,
    size: 1 + h(i * 11 + 3) * 1.6,
    duration: 2.5 + h(i * 13 + 5) * 4,
    delay: h(i * 17 + 7) * 5,
    // Only a third of the field twinkles. A sky where every star pulses
    // reads as noise and pulls attention off the planets.
    twinkles: i % 3 === 0,
  };
});

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
  const sunGrad = useId();
  const orbitGrad = useId();
  const coronaGrad = useId();
  const bodyGrad = useId();
  const hex = STATUS_HEX[aggregate];
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [linkFrom, setLinkFrom] = useState<string | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stageRect = useRef<DOMRect | null>(null);
  const active = focused ?? hovered;
  const reduceMotion = usePrefersReducedMotion();
  const failing = statuses.filter((s) => s === "failed").length;
  const openLabel = mode === "groups" ? "Open ring" : "Open service";
  const bodyWord = mode === "groups" ? "ring" : "service";
  const bodyWordPlural = mode === "groups" ? "rings" : "services";

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

  // ── Orbital motion ───────────────────────────────────────────────────
  // One rAF loop writes transforms straight onto the DOM. Nothing here goes
  // through React state: driving `tick` through setState re-rendered this
  // whole subtree (every planet, plate and open card) 60 times a second,
  // which is what made the stage feel cheap. Positions are still derived
  // from the same pure `planetPosition` math.
  const stageEl = useRef<HTMLDivElement>(null);
  const planetEls = useRef(new Map<string, HTMLDivElement | null>());
  const plateEls = useRef(new Map<string, HTMLDivElement | null>());
  const edgeEls = useRef(new Map<string, SVGLineElement | null>());
  // Pixels per SVG unit; the stage is square and scales with the viewport.
  const scale = useRef(1);
  const elapsed = useRef(0);

  // Hovering or pinning a planet holds the system still so the card you are
  // reading does not drift out from under the pointer. Kept in a ref so the
  // rAF loop can read it without being torn down and restarted on every
  // hover.
  const paused = useRef(false);
  useEffect(() => {
    paused.current = active !== null || editMode;
  }, [active, editMode]);

  // Inputs the rAF loop needs but must not be re-created for: keeping these
  // in refs stops every hover from tearing down and restarting the loop.
  const activeRef = useRef<string | null>(null);
  useEffect(() => {
    activeRef.current = active;
  }, [active]);
  const plateSizes = useRef(new Map<string, { w: number; h: number }>());
  // Label priority: whatever you are pointing at wins, then anything that
  // needs attention, then a stable fallback so the same plate keeps winning
  // frame to frame (a tie-break that flickers is worse than a hidden label).
  const rankRef = useRef(new Map<string, number>());
  useEffect(() => {
    const m = new Map<string, number>();
    orbits.forEach((p, i) => {
      const st = statusById.get(p.id) ?? "empty";
      const attention =
        st === "failed" || st === "degraded" || st === "deploying";
      m.set(p.id, (attention ? 1000 : 2000) + i);
    });
    rankRef.current = m;
  }, [orbits, statusById]);

  const measurePlates = useCallback(() => {
    for (const [id, el] of plateEls.current) {
      if (!el) continue;
      plateSizes.current.set(id, { w: el.offsetWidth, h: el.offsetHeight });
    }
  }, []);

  const writeFrame = useCallback(
    (seconds: number) => {
      const s = scale.current;
      const coords = new Map<string, { x: number; y: number }>();
      const platePos: { id: string; x: number; y: number }[] = [];

      for (const p of orbits) {
        const { x, y, angle } = planetPosition(p, seconds);
        coords.set(p.id, { x, y });

        const planet = planetEls.current.get(p.id);
        if (planet) {
          planet.style.transform = `translate3d(${x * s}px, ${y * s}px, 0)`;
        }

        // Name plates ride a slightly wider orbit than their planet, so they
        // always sit on the far side from the core and never flip sides
        // mid-revolution the way a simple above/below rule does.
        const plate = plateEls.current.get(p.id);
        if (plate) {
          const px = x + Math.cos(angle) * LABEL_ORBIT_GAP;
          const py = y + Math.sin(angle) * LABEL_ORBIT_GAP;
          plate.style.transform = `translate3d(${px * s}px, ${py * s}px, 0)`;
          platePos.push({ id: p.id, x: px * s, y: py * s });
        }
      }

      // ── Declutter ──────────────────────────────────────────────────────
      // Planets bunch up near the top and bottom of a track, where their
      // plates are far wider than the gap between them. Rather than let the
      // names overprint each other into mush, drop the lower-priority plate
      // of any overlapping pair; it comes back as soon as the orbit opens up,
      // and hovering always restores it.
      const activeId = activeRef.current;
      platePos.sort(
        (a, b) =>
          (a.id === activeId ? -1 : (rankRef.current.get(a.id) ?? 9999)) -
          (b.id === activeId ? -1 : (rankRef.current.get(b.id) ?? 9999)),
      );
      const kept: { l: number; r: number; t: number; b: number }[] = [];
      for (const pos of platePos) {
        const plate = plateEls.current.get(pos.id);
        const size = plateSizes.current.get(pos.id);
        if (!plate) continue;
        if (!size || size.w === 0) {
          plate.style.opacity = "1";
          continue;
        }
        const halfW = size.w / 2 + 3;
        const halfH = size.h / 2 + 2;
        const box = {
          l: pos.x - halfW,
          r: pos.x + halfW,
          t: pos.y - halfH,
          b: pos.y + halfH,
        };
        const clash = kept.some(
          (k) => !(box.r < k.l || box.l > k.r || box.b < k.t || box.t > k.b),
        );
        if (clash) {
          plate.style.opacity = "0";
          plate.style.pointerEvents = "none";
        } else {
          plate.style.opacity = "1";
          plate.style.pointerEvents = "";
          kept.push(box);
        }
      }

      // Dependency chords are SVG user units, so they need no scaling.
      for (const [key, line] of edgeEls.current) {
        if (!line) continue;
        const [from, to] = key.split(" ");
        const a = coords.get(from);
        const b = coords.get(to);
        if (!a || !b) continue;
        line.setAttribute("x1", String(a.x));
        line.setAttribute("y1", String(a.y));
        line.setAttribute("x2", String(b.x));
        line.setAttribute("y2", String(b.y));
      }
    },
    [orbits],
  );

  useEffect(() => {
    const el = stageEl.current;
    if (!el) return;
    const measure = () => {
      scale.current = el.clientWidth / 400 || 1;
      measurePlates();
      writeFrame(elapsed.current);
    };
    measure();
    // Plate widths are text-dependent, so they are wrong until the webfont
    // actually lands — re-measure once it has, or every label is deconflicted
    // against fallback-font metrics.
    document.fonts?.ready.then(measure).catch(() => {});
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [writeFrame, measurePlates]);

  useEffect(() => {
    // Reduced motion: draw the system once at t=0 and leave it there. The
    // layout still reads correctly — it just does not revolve.
    if (reduceMotion) {
      writeFrame(elapsed.current);
      return;
    }
    let raf = 0;
    let last: number | null = null;
    const loop = (t: number) => {
      if (last != null && !paused.current) {
        elapsed.current += (t - last) / 1000;
      }
      last = t;
      writeFrame(elapsed.current);
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [reduceMotion, writeFrame]);

  const visibleEdges = useMemo(
    () =>
      edges.filter(
        (e) => members.includes(e.from) && members.includes(e.to),
      ),
    [edges, members],
  );

  const occupiedTracks = useMemo(() => {
    const set = new Set(orbits.map((o) => o.r));
    return ORBIT_TRACKS.filter((r) => set.has(r));
  }, [orbits]);

  const mx = useMotionValue(0);
  const my = useMotionValue(0);
  const sceneX = useSpring(useTransform(mx, (v) => v * -6), {
    stiffness: 50,
    damping: 18,
  });
  const sceneY = useSpring(useTransform(my, (v) => v * -6), {
    stiffness: 50,
    damping: 18,
  });

  const hoverIn = (id: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setHovered(id);
  };
  const hoverOut = () => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    closeTimer.current = setTimeout(() => setHovered(null), 170);
  };

  const onPlanetClick = (id: string) => {
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
    <div
      className="relative overflow-hidden rounded-2xl border border-black/20 bg-[#07070a] dark:border-border"
      onMouseEnter={(e) => {
        stageRect.current = e.currentTarget.getBoundingClientRect();
      }}
      onMouseMove={(e) => {
        const b = (stageRect.current ??=
          e.currentTarget.getBoundingClientRect());
        mx.set((e.clientX - b.left) / b.width - 0.5);
        my.set((e.clientY - b.top) / b.height - 0.5);
      }}
      onMouseLeave={() => {
        stageRect.current = null;
        mx.set(0);
        my.set(0);
      }}
    >
      <div aria-hidden data-solar-ambient className="absolute inset-0">
        {STARS.map((s, i) => (
          <span
            key={i}
            className={cn(
              "absolute rounded-full bg-white",
              s.twinkles
                ? "[animation:twinkle_var(--d)_ease-in-out_infinite]"
                : "opacity-20",
            )}
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
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,rgba(255,200,80,0.07)_0%,transparent_42%,rgba(0,0,0,0.72)_100%)]"
      />

      {editable && (
        <div className="absolute right-3 top-3 z-40 flex items-center gap-2">
          {editMode && linkFrom && (
            <span className="rounded-md border border-white/15 bg-black/50 px-2 py-1 text-[11px] text-neutral-300 backdrop-blur-md">
              Link from{" "}
              <span className="text-neutral-100">{title(linkFrom)}</span>…
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
        className="relative mx-auto aspect-square w-full max-w-[720px] p-3 sm:p-5"
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
        <motion.div
          className="absolute inset-0"
          style={{ x: sceneX, y: sceneY }}
        >
          <svg
            viewBox="0 0 400 400"
            className="absolute inset-0 size-full overflow-visible"
          >
            <defs>
              <radialGradient id={sunGrad} cx="50%" cy="45%" r="50%">
                <stop offset="0%" stopColor="#fff4c2" stopOpacity="1" />
                <stop offset="35%" stopColor="#f5b942" stopOpacity="0.95" />
                <stop offset="70%" stopColor="#e07820" stopOpacity="0.55" />
                <stop offset="100%" stopColor="#e07820" stopOpacity="0" />
              </radialGradient>
              <linearGradient id={orbitGrad} x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stopColor={hex} stopOpacity="0.55" />
                <stop offset="100%" stopColor="#ffffff" stopOpacity="0.12" />
              </linearGradient>
              <radialGradient id={coronaGrad} cx="50%" cy="50%" r="50%">
                <stop offset="45%" stopColor="#f5b942" stopOpacity="0.28" />
                <stop offset="75%" stopColor="#e07820" stopOpacity="0.12" />
                <stop offset="100%" stopColor="#e07820" stopOpacity="0" />
              </radialGradient>
              <radialGradient id={bodyGrad} cx="38%" cy="32%" r="72%">
                <stop offset="0%" stopColor="#fffbe8" />
                <stop offset="55%" stopColor="#ffd56a" />
                <stop offset="100%" stopColor="#f0a63a" />
              </radialGradient>
            </defs>

            {/* Concentric orbital tracks. Occupied tracks carry a slow
                dash drift so a populated orbit reads as live even while the
                system is paused under the pointer. */}
            {ORBIT_TRACKS.map((r) => {
              const occupied = occupiedTracks.includes(r);
              if (!occupied) {
                return (
                  <circle
                    key={r}
                    cx={SOLAR_C}
                    cy={SOLAR_C}
                    r={r}
                    fill="none"
                    stroke="#ffffff"
                    strokeWidth={0.75}
                    strokeOpacity={0.08}
                    strokeDasharray="2 10"
                  />
                );
              }
              return (
                <g key={r}>
                  <circle
                    cx={SOLAR_C}
                    cy={SOLAR_C}
                    r={r}
                    fill="none"
                    stroke={`url(#${orbitGrad})`}
                    strokeWidth={1.25}
                    strokeOpacity={0.5}
                  />
                  <circle
                    data-solar-ambient
                    cx={SOLAR_C}
                    cy={SOLAR_C}
                    r={r}
                    fill="none"
                    stroke={hex}
                    strokeWidth={1.25}
                    strokeOpacity={0.35}
                    strokeDasharray="1 17"
                    strokeLinecap="round"
                    style={{
                      animation: `orbit-shimmer ${34 + r * 0.22}s linear infinite`,
                    }}
                  />
                </g>
              );
            })}

            {/* Dependency chords (faint constellation under planets).
                Endpoints are written by the rAF loop, keyed "from to". */}
            {visibleEdges.map((e) => {
              return (
                <g key={`${e.from}->${e.to}`} data-edge>
                  <line
                    ref={(el) => {
                      edgeEls.current.set(`${e.from} ${e.to}`, el);
                    }}
                    stroke={e.source === "config" ? "#a3a3a3" : "#737373"}
                    strokeWidth={editMode ? 2 : 1}
                    strokeOpacity={0.35}
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
                  </line>
                </g>
              );
            })}

            {/* Core. Four layers instead of two flat discs: an outer corona
                that breathes, the falloff halo, a bright limb, and the body
                with an off-centre highlight so it reads as a sphere. Same
                amber family as before — no new hues. */}
            <circle
              data-solar-ambient
              cx={SOLAR_C}
              cy={SOLAR_C}
              r={52}
              fill={`url(#${coronaGrad})`}
              style={{
                animation: "sun-corona 7s ease-in-out infinite",
                transformOrigin: `${SOLAR_C}px ${SOLAR_C}px`,
              }}
            />
            <circle
              cx={SOLAR_C}
              cy={SOLAR_C}
              r={34}
              fill={`url(#${sunGrad})`}
              className="opacity-90"
            />
            <circle
              cx={SOLAR_C}
              cy={SOLAR_C}
              r={23}
              fill="none"
              stroke="#fff4c2"
              strokeWidth={1.5}
              strokeOpacity={0.55}
            />
            <circle cx={SOLAR_C} cy={SOLAR_C} r={22} fill={`url(#${bodyGrad})`} />
          </svg>

          {/* Core label */}
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <div className="flex max-w-[8rem] flex-col items-center text-center">
              <p className="font-display text-[13px] font-semibold leading-none tracking-tight text-neutral-950">
                {sunLabel}
              </p>
              <p className="mt-1 font-mono text-[10px] font-medium tabular-nums leading-none text-neutral-900/75">
                {members.length}{" "}
                {members.length === 1 ? bodyWord : bodyWordPlural}
              </p>
            </div>
          </div>

          {/* Services. Both the planet and its name plate are positioned by
              the rAF loop writing translate3d onto these wrappers; the inner
              element handles its own centring so the loop only ever touches
              one transform per node. */}
          {orbits.map((p, i) => {
            const status = statusById.get(p.id) ?? "empty";
            const shex = STATUS_HEX[status];
            const expanded = active === p.id && !editMode;
            const linking = editMode && linkFrom === p.id;
            const lat = latencyByMember.get(p.id);
            // Planet body grows slightly for unhealthy / deploying.
            const body =
              status === "failed" || status === "deploying" ? 14 : 11;

            return (
              <div key={p.id}>
                <div
                  ref={(el) => {
                    planetEls.current.set(p.id, el);
                  }}
                  className="absolute left-0 top-0 z-10 will-change-transform"
                >
                  <div className="-translate-x-1/2 -translate-y-1/2">
                  <motion.button
                    type="button"
                    aria-label={`${title(p.id)}: ${STATUS_WORD[status]}`}
                    onClick={() => onPlanetClick(p.id)}
                    onMouseEnter={() => hoverIn(p.id)}
                    onMouseLeave={hoverOut}
                    initial={{ opacity: 0, scale: 0.4 }}
                    animate={{
                      opacity: 1,
                      scale: linking ? 1.25 : 1,
                    }}
                    transition={{
                      delay: Math.min(0.08 * i, 0.6),
                      type: "spring",
                      stiffness: 220,
                      damping: 18,
                    }}
                    whileHover={{ scale: 1.18 }}
                    className="relative block"
                    style={{ width: body + 8, height: body + 8 }}
                  >
                    {status === "healthy" && (
                      <span
                        data-solar-ambient
                        className="absolute inset-0 rounded-full [animation:node-pulse_3.5s_ease-out_infinite]"
                        style={{ background: shex, opacity: 0.35 }}
                      />
                    )}
                    {status === "deploying" && (
                      <span
                        data-solar-ambient
                        className="absolute -inset-1 animate-spin rounded-full border-2 border-transparent"
                        style={{ borderTopColor: shex }}
                      />
                    )}
                    <span
                      className={cn(
                        "absolute left-1/2 top-1/2 block -translate-x-1/2 -translate-y-1/2 rounded-full border border-black/40 shadow-[0_0_12px_var(--glow)]",
                        status === "loading" && "animate-pulse",
                        linking && "ring-2 ring-white/80",
                      )}
                      style={
                        {
                          width: body,
                          height: body,
                          background: `radial-gradient(circle at 35% 30%, #ffffffaa, ${shex} 55%, ${shex}cc)`,
                          "--glow": `${shex}99`,
                        } as React.CSSProperties
                      }
                    />
                  </motion.button>
                  </div>
                </div>

                {/* Name plate, parked on a slightly wider orbit than its
                    planet so it always points away from the core. */}
                <div
                  ref={(el) => {
                    plateEls.current.set(p.id, el);
                  }}
                  className={cn(
                    "absolute left-0 top-0 z-20 transition-opacity duration-200 will-change-transform",
                    expanded && "z-30",
                  )}
                  onMouseEnter={() => hoverIn(p.id)}
                  onMouseLeave={hoverOut}
                >
                  <div className="-translate-x-1/2 -translate-y-1/2">
                  <AnimatePresence>
                    {expanded && (
                      <motion.div
                        key="card"
                        initial={{ opacity: 0, scale: 0.94 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: 0.96 }}
                        transition={{
                          type: "spring",
                          stiffness: 380,
                          damping: 28,
                        }}
                        className="absolute left-1/2 top-full z-30 mt-1.5 -translate-x-1/2"
                      >
                        <NodeCard
                          id={p.id}
                          label={title(p.id)}
                          status={status}
                          rings={resultById.get(p.id)}
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
                  <button
                    type="button"
                    onClick={() => onPlanetClick(p.id)}
                    // Names in a fleet often share a long prefix
                    // ("diytaxreturn-…"), so a narrow plate ellipsises away
                    // the only part that identifies the service. Give the
                    // plate real room and keep the full name in the tooltip.
                    title={title(p.id)}
                    className={cn(
                      "flex max-w-[11rem] items-baseline gap-1.5 rounded-full border px-2.5 py-[3px] backdrop-blur-md transition-colors",
                      expanded || linking
                        ? "border-white/30 bg-black/70 text-neutral-50"
                        : "border-white/10 bg-black/60 text-neutral-100 hover:border-white/25",
                    )}
                  >
                    <span className="truncate font-display text-[11px] font-medium leading-none tracking-tight">
                      {title(p.id)}
                    </span>
                    {lat != null && (
                      <span className="shrink-0 font-mono text-[10px] leading-none tabular-nums text-neutral-400">
                        {Math.round(lat)}
                        <span className="text-neutral-500">ms</span>
                      </span>
                    )}
                  </button>
                  </div>
                </div>
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
            <span className="text-neutral-500">
              Inner orbits = lower latency · Outer = higher
            </span>
          </div>
        </motion.div>
      </div>
    </div>
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
      className="w-60 rounded-2xl border border-white/15 bg-white/[0.08] p-3.5 text-left shadow-2xl ring-1 ring-black/40 backdrop-blur-2xl"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span
            className="flex size-6 shrink-0 items-center justify-center rounded-full border"
            style={{
              borderColor: `${hex}55`,
              background: `radial-gradient(circle at 35% 30%, #ffffff88, ${hex})`,
            }}
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
            <span
              className="size-1.5 rounded-full"
              style={{ background: hex }}
            />
            {STATUS_WORD[status]}
          </span>
        </Row>
        <Row label="Latency">
          {latencyMs != null ? (
            <span className="font-mono text-neutral-100">
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
                <span className="font-mono text-neutral-100">
                  {latest.current_version}
                  <span className="text-neutral-400"> · {latest.ring.name}</span>
                </span>
              ) : (
                <span className="text-neutral-400">nothing deployed</span>
              )}
            </Row>
            <Row label="Rings">
              <span className="text-neutral-100">
                {active.length === 0
                  ? "—"
                  : `${healthy}/${active.length} healthy`}
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

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      {/* neutral-500 on this glass sits around 4:1 — under the 4.5:1 floor
          for text this size. neutral-400 clears it and still reads as the
          quieter half of the pair. */}
      <dt className="text-neutral-400">{label}</dt>
      <dd className="min-w-0 truncate text-right">{children}</dd>
    </div>
  );
}
