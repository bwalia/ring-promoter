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
import {
  STATUS_HEX,
  type NodeStatus,
} from "@/components/group-ring";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import {
  useAppTitle,
  type GroupAppRings,
} from "@/lib/queries";
import {
  appLatencyMs,
  groupSectorAngles,
  latencyToRadius,
  linkRestLength,
  seedNodes,
  SOLAR_C,
  stepSimulation,
  type SimEdge,
  type SimNode,
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

const STARS = Array.from({ length: 46 }, (_, i) => {
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
  /** Center label (group name or "Fleet"). */
  sunLabel: string;
  members: string[];
  results: GroupAppRings[];
  statuses: NodeStatus[];
  aggregate: NodeStatus;
  edges: TopologyEdge[];
  /** When set, apps are pulled into group sectors (fleet home). */
  groups?: AppGroup[];
  editable?: boolean;
  onAddEdge?: (from: string, to: string) => void;
  onRemoveEdge?: (from: string, to: string) => void;
  onOpen: (app: string) => void;
  onSeed: (app: string) => void;
};

export function SolarSystem({
  sunLabel,
  members,
  results,
  statuses,
  aggregate,
  edges,
  groups,
  editable = false,
  onAddEdge,
  onRemoveEdge,
  onOpen,
  onSeed,
}: SolarSystemProps) {
  const title = useAppTitle();
  const gradId = useId();
  const hex = STATUS_HEX[aggregate];
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [linkFrom, setLinkFrom] = useState<string | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stageRect = useRef<DOMRect | null>(null);
  const active = focused ?? hovered;
  const failing = statuses.filter((s) => s === "failed").length;

  const latencyByApp = useMemo(() => {
    const m = new Map<string, number | null>();
    members.forEach((app, i) => {
      m.set(app, appLatencyMs(results[i]?.rings));
    });
    return m;
  }, [members, results]);

  const sectorByApp = useMemo(
    () => groupSectorAngles(members, groups ?? []),
    [members, groups],
  );

  const visibleEdges = useMemo(
    () =>
      edges.filter(
        (e) => members.includes(e.from) && members.includes(e.to),
      ),
    [edges, members],
  );

  const [nodes, setNodes] = useState<SimNode[]>([]);
  const nodesRef = useRef<SimNode[]>([]);
  const memberKey = members.join("\0");

  // Reseed when the member set changes.
  useEffect(() => {
    const seeded = seedNodes(
      members,
      (app) => latencyToRadius(latencyByApp.get(app) ?? null),
      (app) => sectorByApp.get(app) ?? null,
    );
    nodesRef.current = seeded;
    setNodes(seeded.map((n) => ({ ...n })));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reseed on membership only; radii update below
  }, [memberKey]);

  // Keep target radii / sector prefs in sync without resetting positions.
  useEffect(() => {
    for (const n of nodesRef.current) {
      n.targetR = latencyToRadius(latencyByApp.get(n.id) ?? null);
      n.preferAngle = sectorByApp.get(n.id) ?? null;
    }
  }, [latencyByApp, sectorByApp]);

  // Run the force simulation (~20fps React updates).
  useEffect(() => {
    if (members.length === 0) return;
    let raf = 0;
    let alive = true;
    let lastPublish = 0;
    const tick = (t: number) => {
      if (!alive) return;
      const simEdges: SimEdge[] = visibleEdges.map((e) => ({
        from: e.from,
        to: e.to,
        rest: linkRestLength(
          latencyByApp.get(e.from) ?? null,
          latencyByApp.get(e.to) ?? null,
        ),
      }));
      stepSimulation(nodesRef.current, simEdges);
      if (t - lastPublish > 50) {
        lastPublish = t;
        setNodes(nodesRef.current.map((n) => ({ ...n })));
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => {
      alive = false;
      cancelAnimationFrame(raf);
    };
  }, [members.length, visibleEdges, latencyByApp]);

  const mx = useMotionValue(0);
  const my = useMotionValue(0);
  const sceneX = useSpring(useTransform(mx, (v) => v * -8), {
    stiffness: 50,
    damping: 18,
  });
  const sceneY = useSpring(useTransform(my, (v) => v * -8), {
    stiffness: 50,
    damping: 18,
  });

  const hoverIn = (app: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setHovered(app);
  };
  const hoverOut = () => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    closeTimer.current = setTimeout(() => setHovered(null), 170);
  };

  const summaryLine: Record<NodeStatus, string> = {
    healthy: "All systems operational",
    deploying: "Deployment in progress",
    degraded: "Partially degraded",
    failed: `${failing} app${failing === 1 ? "" : "s"} failing`,
    empty: "Nothing deployed yet",
    loading: "Checking health…",
  };

  const onPlanetClick = (app: string) => {
    if (editMode && editable) {
      if (!linkFrom) {
        setLinkFrom(app);
        setFocused(app);
        return;
      }
      if (linkFrom === app) {
        setLinkFrom(null);
        return;
      }
      onAddEdge?.(linkFrom, app);
      setLinkFrom(null);
      setFocused(null);
      return;
    }
    setFocused((f) => (f === app ? null : app));
  };

  const posById = useMemo(() => {
    const m = new Map<string, SimNode>();
    for (const n of nodes) m.set(n.id, n);
    return m;
  }, [nodes]);

  return (
    <div
      className="relative overflow-hidden rounded-2xl border border-black/20 bg-[#090909] dark:border-border"
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
      <div
        aria-hidden
        className="absolute inset-0"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.03) 1px, transparent 1px)",
          backgroundSize: "36px 36px",
        }}
      />
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
        className="absolute -left-24 -top-24 size-96 rounded-full opacity-15 blur-3xl [animation:blob-drift_16s_ease-in-out_infinite]"
        style={{ background: hex }}
      />
      <div
        aria-hidden
        className="absolute -bottom-28 -right-24 size-96 rounded-full bg-[#3b82f6] opacity-[0.08] blur-3xl [animation:blob-drift_22s_ease-in-out_infinite_reverse]"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_52%,rgba(0,0,0,0.65)_100%)]"
      />

      {editable && (
        <div className="absolute right-3 top-3 z-40 flex items-center gap-2">
          {editMode && linkFrom && (
            <span className="rounded-md border border-white/15 bg-black/50 px-2 py-1 text-[11px] text-neutral-300 backdrop-blur-md">
              Link from <span className="text-neutral-100">{title(linkFrom)}</span>
              … click another app
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
        className="relative mx-auto aspect-square w-full max-w-[640px] p-4"
        data-testid="solar-system"
        data-status={aggregate}
        onClick={(e) => {
          if (!(e.target as HTMLElement).closest("button, [data-node-card], [data-edge]")) {
            setFocused(null);
            if (editMode) setLinkFrom(null);
          }
        }}
      >
        <motion.div
          className="absolute inset-0"
          style={{ x: sceneX, y: sceneY }}
        >
          <svg viewBox="0 0 400 400" className="absolute inset-0 size-full overflow-visible">
            <defs>
              <linearGradient id={gradId} x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stopColor={hex} stopOpacity="0.55" />
                <stop offset="50%" stopColor={hex} stopOpacity="0.08" />
                <stop offset="100%" stopColor={hex} stopOpacity="0.4" />
              </linearGradient>
            </defs>
            {/* Guide orbits */}
            {[SOLAR_C * 0.36, SOLAR_C * 0.52, SOLAR_C * 0.68].map((r) => (
              <circle
                key={r}
                cx={SOLAR_C}
                cy={SOLAR_C}
                r={r}
                fill="none"
                stroke="#ffffff"
                strokeWidth="1"
                strokeDasharray={r > 120 ? "1 7" : undefined}
                className="opacity-[0.06]"
              />
            ))}
            <circle
              cx={SOLAR_C}
              cy={SOLAR_C}
              r={140}
              fill="none"
              stroke={`url(#${gradId})`}
              strokeWidth="1.5"
              className="opacity-40"
            />

            {/* Group sector labels (fleet mode) */}
            {(groups ?? [])
              .filter((g) => g.apps.some((a) => members.includes(a)))
              .map((g, i, arr) => {
                const mid = (i / arr.length) * 2 * Math.PI - Math.PI / 2;
                const lx = SOLAR_C + 186 * Math.cos(mid);
                const ly = SOLAR_C + 186 * Math.sin(mid);
                return (
                  <text
                    key={g.id}
                    x={lx}
                    y={ly}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    className="fill-neutral-500 text-[9px]"
                  >
                    {g.name}
                  </text>
                );
              })}

            {/* Dependency constellation */}
            {visibleEdges.map((e) => {
              const a = posById.get(e.from);
              const b = posById.get(e.to);
              if (!a || !b) return null;
              const mx2 = (a.x + b.x) / 2;
              const my2 = (a.y + b.y) / 2;
              // Slight curve perpendicular to the chord.
              const dx = b.x - a.x;
              const dy = b.y - a.y;
              const len = Math.hypot(dx, dy) || 1;
              const cx = mx2 - (dy / len) * 14;
              const cy = my2 + (dx / len) * 14;
              return (
                <g key={`${e.from}->${e.to}`} data-edge>
                  <path
                    d={`M ${a.x} ${a.y} Q ${cx} ${cy} ${b.x} ${b.y}`}
                    fill="none"
                    stroke={e.source === "config" ? "#a3a3a3" : "#737373"}
                    strokeWidth={editMode ? 2.2 : 1.4}
                    strokeOpacity={0.45}
                    className={cn(editMode && "cursor-pointer")}
                    onClick={(ev) => {
                      if (!editMode || !editable) return;
                      ev.stopPropagation();
                      onRemoveEdge?.(e.from, e.to);
                    }}
                  >
                    <title>
                      {title(e.from)} → {title(e.to)} ({e.source}
                      {editMode ? " — click to remove" : ""})
                    </title>
                  </path>
                  {e.source === "config" && (
                    <circle
                      cx={cx}
                      cy={cy}
                      r={2}
                      fill="#a3a3a3"
                      className="opacity-50"
                    />
                  )}
                </g>
              );
            })}
          </svg>

          {/* Sun */}
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <motion.div
              className="relative flex size-28 flex-col items-center justify-center rounded-full border border-white/10 bg-white/[0.04] text-center shadow-[0_0_40px_rgba(0,0,0,0.45)] backdrop-blur-md md:size-32"
              initial={{ opacity: 0, scale: 0.9 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ delay: 0.15, duration: 0.45 }}
            >
              <span
                aria-hidden
                className="absolute inset-2 rounded-full opacity-30 blur-md"
                style={{ background: hex }}
              />
              <p
                className={cn(
                  "relative z-[1] line-clamp-2 max-w-[6.5rem] break-words px-2 font-semibold leading-tight tracking-tight text-neutral-50",
                  sunLabel.length <= 10 ? "text-lg md:text-xl" : "text-sm md:text-base",
                )}
              >
                {sunLabel}
              </p>
              <p
                className="relative z-[1] mt-0.5 text-[11px] font-medium"
                style={{ color: hex }}
              >
                {summaryLine[aggregate]}
              </p>
              <p className="relative z-[1] text-[10px] text-neutral-500">
                {members.length} app{members.length === 1 ? "" : "s"}
              </p>
            </motion.div>
          </div>

          {/* Planets */}
          {members.map((app, i) => {
            const n = posById.get(app);
            if (!n) return null;
            const status = statuses[i];
            const shex = STATUS_HEX[status];
            const expanded = active === app && !editMode;
            const left = (n.x / 400) * 100;
            const top = (n.y / 400) * 100;
            const dx = n.x - SOLAR_C;
            const dy = n.y - SOLAR_C;
            const cos = dx / (Math.hypot(dx, dy) || 1);
            const sin = dy / (Math.hypot(dx, dy) || 1);
            const pillAnchor =
              Math.abs(cos) < 0.35 ? "center" : cos > 0 ? "left" : "right";
            const pillTx =
              pillAnchor === "center"
                ? "-50%"
                : pillAnchor === "left"
                  ? "0%"
                  : "-100%";
            const cardPlacement: React.CSSProperties =
              pillAnchor === "center"
                ? sin < 0
                  ? { top: "100%", left: "50%", translate: "-50% 0" }
                  : { bottom: "100%", left: "50%", translate: "-50% 0" }
                : {
                    ...(cos > 0 ? { right: "100%" } : { left: "100%" }),
                    ...(sin <= 0 ? { top: 0 } : { bottom: 0 }),
                  };
            const lat = latencyByApp.get(app);
            const linking = editMode && linkFrom === app;

            return (
              <div key={app}>
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
                    aria-label={`${title(app)}: ${STATUS_WORD[status]}`}
                    onClick={() => onPlanetClick(app)}
                    onMouseEnter={() => hoverIn(app)}
                    onMouseLeave={hoverOut}
                    initial={{ opacity: 0, scale: 0 }}
                    animate={{ opacity: 1, scale: linking ? 1.35 : 1 }}
                    transition={{ type: "spring", stiffness: 220, damping: 19 }}
                    whileHover={{ scale: 1.25 }}
                    className="block"
                  >
                    <span className="relative flex size-3.5 items-center justify-center">
                      {status === "healthy" && (
                        <span
                          className="absolute inset-0 rounded-full [animation:node-pulse_3s_ease-out_infinite]"
                          style={{ background: shex }}
                        />
                      )}
                      {status === "deploying" && (
                        <span
                          className="absolute -inset-1.5 animate-spin rounded-full border-2 border-transparent"
                          style={{ borderTopColor: shex }}
                        />
                      )}
                      <span
                        className={cn(
                          "relative size-3.5 rounded-full border-2 border-[#090909]",
                          status === "loading" && "animate-pulse",
                          linking && "ring-2 ring-white/70",
                        )}
                        style={{
                          background: shex,
                          boxShadow: `0 0 10px ${shex}80`,
                        }}
                      />
                    </span>
                  </motion.button>
                </div>

                <motion.div
                  className={cn("absolute", expanded ? "z-30" : "z-10")}
                  style={{
                    left: `${left + cos * 6}%`,
                    top: `${top + sin * 6}%`,
                    transform: `translate(${pillTx}, -50%)`,
                  }}
                  onMouseEnter={() => hoverIn(app)}
                  onMouseLeave={hoverOut}
                >
                  <AnimatePresence>
                    {expanded && (
                      <motion.div
                        key="card"
                        initial={{ opacity: 0, scale: 0.94 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: 0.96 }}
                        transition={{ type: "spring", stiffness: 380, damping: 28 }}
                        className="absolute"
                        style={cardPlacement}
                      >
                        <NodeCard
                          app={app}
                          status={status}
                          rings={results[i]}
                          latencyMs={lat ?? null}
                          pinned={focused === app}
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
                    onClick={() => onPlanetClick(app)}
                    className={cn(
                      "flex items-center gap-1.5 rounded-full border bg-white/[0.06] px-2 py-1 shadow-lg backdrop-blur-md transition-colors",
                      expanded || linking
                        ? "border-white/25 bg-white/[0.1]"
                        : "border-white/10 hover:border-white/20",
                    )}
                  >
                    <span
                      className="flex size-4 shrink-0 items-center justify-center rounded-[5px] border bg-gradient-to-b from-white/15 to-white/5 text-[9px] font-semibold text-neutral-200"
                      style={{ borderColor: `${shex}55` }}
                    >
                      {title(app)[0]?.toUpperCase()}
                    </span>
                    <span className="max-w-28 truncate text-xs font-medium text-neutral-200">
                      {title(app)}
                    </span>
                    {lat != null && (
                      <span className="font-mono text-[9px] text-neutral-500">
                        {Math.round(lat)}ms
                      </span>
                    )}
                  </button>
                </motion.div>
              </div>
            );
          })}
        </motion.div>
      </div>
    </div>
  );
}

function NodeCard({
  app,
  status,
  rings,
  latencyMs,
  pinned,
  onClose,
  onOpen,
  onSeed,
}: {
  app: string;
  status: NodeStatus;
  rings: GroupAppRings | undefined;
  latencyMs: number | null;
  pinned: boolean;
  onClose: () => void;
  onOpen: (app: string) => void;
  onSeed: (app: string) => void;
}) {
  const title = useAppTitle();
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
            className="flex size-6 shrink-0 items-center justify-center rounded-md border bg-gradient-to-b from-white/15 to-white/5 text-[11px] font-semibold text-neutral-100"
            style={{ borderColor: `${hex}55` }}
          >
            {title(app)[0]?.toUpperCase()}
          </span>
          <p className="min-w-0 truncate text-sm font-semibold text-neutral-50">
            {title(app)}
          </p>
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
            {active.length === 0 ? "—" : `${healthy}/${active.length} healthy`}
          </span>
        </Row>
        <Row label="Last deploy">
          {lastDeploy ? (
            <RelativeTime iso={lastDeploy} className="text-neutral-100" />
          ) : (
            <span className="text-neutral-500">never</span>
          )}
        </Row>
      </dl>

      <div className="mt-3 flex gap-2">
        <button
          type="button"
          onClick={() => onOpen(app)}
          className="h-7 flex-1 rounded-md bg-white text-xs font-medium text-neutral-900 transition-colors hover:bg-white/85"
        >
          Open
        </button>
        <button
          type="button"
          onClick={() => onSeed(app)}
          className="h-7 flex-1 rounded-md border border-white/15 text-xs font-medium text-neutral-100 transition-colors hover:bg-white/10"
        >
          Seed
        </button>
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
