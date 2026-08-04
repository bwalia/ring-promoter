"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
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
  latencyToRadius,
  ORBIT_TRACKS,
  planetPosition,
  SOLAR_C,
} from "@/lib/solar-layout";
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
  const hex = STATUS_HEX[aggregate];
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [linkFrom, setLinkFrom] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stageRect = useRef<DOMRect | null>(null);
  const startRef = useRef<number | null>(null);
  const active = focused ?? hovered;
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

  // Slow orbital motion — outer planets drift more slowly.
  useEffect(() => {
    let raf = 0;
    const loop = (t: number) => {
      if (startRef.current == null) startRef.current = t;
      const elapsed = (t - startRef.current) / 1000;
      setTick(elapsed);
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, []);

  const positions = useMemo(() => {
    const m = new Map<string, { x: number; y: number; angle: number }>();
    for (const p of orbits) {
      m.set(p.id, planetPosition(p, tick));
    }
    return m;
  }, [orbits, tick]);

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
      <div aria-hidden className="absolute inset-0">
        {STARS.map((s, i) => (
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
            </defs>

            {/* Concentric orbital tracks */}
            {ORBIT_TRACKS.map((r) => {
              const occupied = occupiedTracks.includes(r);
              return (
                <circle
                  key={r}
                  cx={SOLAR_C}
                  cy={SOLAR_C}
                  r={r}
                  fill="none"
                  stroke={occupied ? `url(#${orbitGrad})` : "#ffffff"}
                  strokeWidth={occupied ? 1.25 : 0.75}
                  strokeOpacity={occupied ? 0.55 : 0.08}
                  strokeDasharray={occupied ? undefined : "2 10"}
                />
              );
            })}

            {/* Dependency chords (faint constellation under planets) */}
            {visibleEdges.map((e) => {
              const a = positions.get(e.from);
              const b = positions.get(e.to);
              if (!a || !b) return null;
              return (
                <g key={`${e.from}->${e.to}`} data-edge>
                  <line
                    x1={a.x}
                    y1={a.y}
                    x2={b.x}
                    y2={b.y}
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

            {/* Business */}
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
              r={22}
              fill="#ffd56a"
              className="opacity-90"
            />
          </svg>

          {/* Business label */}
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <div className="flex max-w-[7.5rem] flex-col items-center text-center">
              <p className="text-[10px] font-semibold leading-tight tracking-tight text-neutral-950/90 drop-shadow-sm">
                {sunLabel}
              </p>
              <p className="mt-0.5 text-[9px] font-medium text-neutral-900/70">
                {members.length}{" "}
                {members.length === 1 ? bodyWord : bodyWordPlural}
              </p>
            </div>
          </div>

          {/* Services */}
          {orbits.map((p, i) => {
            const pos = positions.get(p.id);
            if (!pos) return null;
            const status = statusById.get(p.id) ?? "empty";
            const shex = STATUS_HEX[status];
            const expanded = active === p.id && !editMode;
            const linking = editMode && linkFrom === p.id;
            const lat = latencyByMember.get(p.id);
            const left = (pos.x / 400) * 100;
            const top = (pos.y / 400) * 100;
            // Planet body grows slightly for unhealthy / deploying.
            const body =
              status === "failed" || status === "deploying" ? 14 : 11;

            const dy = pos.y - SOLAR_C;
            const labelSide = dy >= 0 ? 1 : -1;

            return (
              <div key={p.id}>
                <div
                  className="absolute z-10"
                  style={{
                    left: `${left}%`,
                    top: `${top}%`,
                    transform: "translate(-50%, -50%)",
                  }}
                >
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
                        className="absolute inset-0 rounded-full [animation:node-pulse_3.5s_ease-out_infinite]"
                        style={{ background: shex, opacity: 0.35 }}
                      />
                    )}
                    {status === "deploying" && (
                      <span
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

                {/* Name plate parked just outside the planet */}
                <div
                  className={cn("absolute z-20", expanded && "z-30")}
                  style={{
                    left: `${left}%`,
                    top: `${top}%`,
                    transform: `translate(-50%, ${labelSide > 0 ? "12px" : "calc(-100% - 12px)"})`,
                  }}
                  onMouseEnter={() => hoverIn(p.id)}
                  onMouseLeave={hoverOut}
                >
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
                        className="absolute left-1/2 z-30 -translate-x-1/2"
                        style={
                          labelSide > 0
                            ? { top: "100%", marginTop: 6 }
                            : { bottom: "100%", marginBottom: 6 }
                        }
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
                    className={cn(
                      "max-w-[7.5rem] truncate rounded-full border px-2 py-0.5 text-[10px] font-medium backdrop-blur-md transition-colors",
                      expanded || linking
                        ? "border-white/30 bg-white/15 text-neutral-50"
                        : "border-white/10 bg-black/45 text-neutral-200 hover:border-white/25",
                    )}
                  >
                    {title(p.id)}
                    {lat != null && (
                      <span className="ml-1 font-mono text-[9px] text-neutral-500">
                        {Math.round(lat)}ms
                      </span>
                    )}
                  </button>
                </div>
              </div>
            );
          })}

          {/* Legend */}
          <div className="pointer-events-none absolute bottom-2 left-2 right-2 flex flex-wrap items-center justify-between gap-2 text-[10px] text-neutral-500">
            <span>{summaryLine[aggregate]}</span>
            <span>Inner orbits = lower latency · Outer = higher latency</span>
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
            <p className="truncate text-sm font-semibold text-neutral-50">
              {label}
            </p>
            {subtitle && (
              <p className="truncate text-[11px] text-neutral-500">{subtitle}</p>
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
            <span className="text-neutral-500">—</span>
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
                  <span className="text-neutral-500"> · {latest.ring.name}</span>
                </span>
              ) : (
                <span className="text-neutral-500">nothing deployed</span>
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
                <span className="text-neutral-500">never</span>
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
      <dt className="text-neutral-500">{label}</dt>
      <dd className="min-w-0 truncate text-right">{children}</dd>
    </div>
  );
}
