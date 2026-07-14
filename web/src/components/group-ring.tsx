"use client";

import { useId, useRef, useState } from "react";
import {
  AnimatePresence,
  motion,
  useMotionValue,
  useSpring,
  useTransform,
} from "motion/react";
import { X } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { summarizeRings } from "@/lib/app-health";
import { useAppTitle } from "@/lib/queries";
import type { GroupAppRings } from "@/lib/queries";
import type { AppGroup } from "@/lib/types";
import { cn } from "@/lib/utils";

// ---- status model (shared with GroupView) ----

export type NodeStatus =
  | "healthy"
  | "deploying"
  | "degraded"
  | "failed"
  | "empty"
  | "loading";

export const STATUS_HEX: Record<NodeStatus, string> = {
  healthy: "#22c55e",
  deploying: "#3b82f6",
  degraded: "#f59e0b",
  failed: "#ef4444",
  empty: "#71717a",
  loading: "#71717a",
};

const STATUS_WORD: Record<NodeStatus, string> = {
  healthy: "Healthy",
  deploying: "Deploying",
  degraded: "Degraded",
  failed: "Failing",
  empty: "No version",
  loading: "Checking…",
};

// ---- geometry (SVG box is 400×400, orbit radius 150 → 37.5% of container) ----

const C = 200;
const R = 150;
const RING_PCT = 37.5;
const PILL_PCT = 48;

function polar(r: number, angle: number) {
  return { x: C + r * Math.cos(angle), y: C + r * Math.sin(angle) };
}

function arcPath(r: number, start: number, end: number) {
  const s = polar(r, start);
  const e = polar(r, end);
  const large = end - start > Math.PI ? 1 : 0;
  return `M ${s.x} ${s.y} A ${r} ${r} 0 ${large} 1 ${e.x} ${e.y}`;
}

/** Connector: a gently curved path from the orbit out toward the badge. */
function connectorPath(angle: number) {
  const s = polar(R + 5, angle);
  const e = polar(R + 27, angle);
  const m = polar(R + 16, angle);
  // Bend the midpoint slightly perpendicular to the radius.
  const px = -Math.sin(angle) * 7;
  const py = Math.cos(angle) * 7;
  return `M ${s.x} ${s.y} Q ${m.x + px} ${m.y + py} ${e.x} ${e.y}`;
}

function appAngle(i: number, n: number) {
  return (i / n) * 2 * Math.PI - Math.PI / 2;
}

// Deterministic pseudo-random (SSR-safe) star field.
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

// ---- the stage ----

export function GroupRing({
  group,
  members,
  results,
  statuses,
  aggregate,
  onOpen,
  onSeed,
}: {
  group: AppGroup;
  members: string[];
  results: GroupAppRings[];
  statuses: NodeStatus[];
  aggregate: NodeStatus;
  onOpen: (app: string) => void;
  onSeed: (app: string) => void;
}) {
  const title = useAppTitle();
  const gradId = useId();
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stageRect = useRef<DOMRect | null>(null);
  const hex = STATUS_HEX[aggregate];
  const active = focused ?? hovered;
  const failing = statuses.filter((s) => s === "failed").length;

  // Subtle camera parallax: the scene leans a few px away from the cursor.
  const mx = useMotionValue(0);
  const my = useMotionValue(0);
  const sceneX = useSpring(useTransform(mx, (v) => v * -10), {
    stiffness: 50,
    damping: 18,
  });
  const sceneY = useSpring(useTransform(my, (v) => v * -10), {
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

  return (
    <div
      className="relative overflow-hidden rounded-2xl border border-black/20 bg-[#090909] dark:border-border"
      onMouseEnter={(e) => {
        // Measure once per hover session — a getBoundingClientRect on every
        // mousemove forces layout while animations are running.
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
      {/* Backdrop: grid, twinkling stars, ambient lights, vignette. */}
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

      <div
        className="relative mx-auto aspect-square w-full max-w-[560px] p-4"
        data-testid="group-ring"
        data-status={aggregate}
        onClick={(e) => {
          // Unpin on background click. Full-bleed SVG/backdrop layers cover
          // the container, so match "not an interactive element" via closest
          // rather than comparing against the container itself.
          if (!(e.target as HTMLElement).closest("button, [data-node-card]")) {
            setFocused(null);
          }
        }}
      >
        <motion.div
          className="absolute inset-0"
          style={{ x: sceneX, y: sceneY }}
        >
          {/* Concentric guide rings around the main orbit. */}
          <svg viewBox="0 0 400 400" className="absolute inset-0 size-full">
            <circle
              cx={C}
              cy={C}
              r={112}
              fill="none"
              stroke="#ffffff"
              strokeWidth="1"
              className="opacity-[0.045]"
            />
            <circle
              cx={C}
              cy={C}
              r={130}
              fill="none"
              stroke="#ffffff"
              strokeWidth="1"
              strokeDasharray="1 7"
              className="opacity-[0.09]"
            />
          </svg>
          <div
            aria-hidden
            className="absolute inset-0 animate-[spin_80s_linear_infinite_reverse] p-4"
          >
            <svg viewBox="0 0 400 400" className="size-full overflow-visible">
              <circle
                cx={C}
                cy={C}
                r={186}
                fill="none"
                stroke="#ffffff"
                strokeWidth="1"
                strokeDasharray="3 14"
                className="opacity-[0.07]"
              />
            </svg>
          </div>

          {/* Main orbit: an animated gradient ring that draws itself on load,
              rotating slowly so the gradient flows. */}
          <div
            aria-hidden
            className="absolute inset-0 animate-[spin_30s_linear_infinite] p-4"
          >
            <svg viewBox="0 0 400 400" className="size-full overflow-visible">
              <defs>
                <linearGradient id={gradId} x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stopColor={hex} stopOpacity="0.75" />
                  <stop offset="50%" stopColor={hex} stopOpacity="0.08" />
                  <stop offset="100%" stopColor={hex} stopOpacity="0.55" />
                </linearGradient>
              </defs>
              <motion.circle
                cx={C}
                cy={C}
                r={R}
                fill="none"
                stroke={`url(#${gradId})`}
                strokeWidth="2.5"
                strokeLinecap="round"
                initial={{ pathLength: 0 }}
                animate={{ pathLength: 1 }}
                transition={{ duration: 1.3, ease: "easeInOut" }}
              />
            </svg>
          </div>

          {/* A small bright light travelling the orbit, trailing a soft tail. */}
          <div
            aria-hidden
            className="absolute inset-0 animate-[spin_14s_linear_infinite] p-4"
          >
            <svg viewBox="0 0 400 400" className="size-full overflow-visible">
              <circle
                cx={C}
                cy={C}
                r={R}
                fill="none"
                stroke={hex}
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeDasharray="64 878"
                className="opacity-40"
              />
              <circle
                cx={C}
                cy={C}
                r={R}
                fill="none"
                stroke={hex}
                strokeWidth="3.5"
                strokeLinecap="round"
                strokeDasharray="7 935"
                strokeDashoffset="-60"
                style={{ filter: `drop-shadow(0 0 7px ${hex})` }}
              />
            </svg>
          </div>

          {/* Connectors + hover highlight arc. */}
          <svg viewBox="0 0 400 400" className="absolute inset-0 size-full">
            {members.map((app, i) => {
              const isActive = active === app;
              const shex = STATUS_HEX[statuses[i]];
              return (
                <motion.path
                  key={app}
                  d={connectorPath(appAngle(i, members.length))}
                  fill="none"
                  stroke={shex}
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  initial={{ opacity: 0, pathLength: 0 }}
                  animate={{
                    opacity: isActive ? 0.95 : 0.35,
                    pathLength: 1,
                  }}
                  transition={{ delay: 0.7 + i * 0.08, duration: 0.4 }}
                  style={
                    isActive
                      ? { filter: `drop-shadow(0 0 4px ${shex})` }
                      : undefined
                  }
                />
              );
            })}
            <AnimatePresence>
              {active !== null &&
                (() => {
                  const i = members.indexOf(active);
                  if (i < 0) return null;
                  const a = appAngle(i, members.length);
                  const span =
                    Math.min(Math.PI / members.length, 0.7) * 0.85;
                  return (
                    <motion.path
                      key={active}
                      d={arcPath(R, a - span, a + span)}
                      fill="none"
                      stroke={STATUS_HEX[statuses[i]]}
                      strokeWidth="4"
                      strokeLinecap="round"
                      initial={{ opacity: 0, pathLength: 0.2 }}
                      animate={{ opacity: 1, pathLength: 1 }}
                      exit={{ opacity: 0 }}
                      transition={{ duration: 0.3, ease: "easeOut" }}
                      style={{
                        filter: `drop-shadow(0 0 6px ${STATUS_HEX[statuses[i]]})`,
                      }}
                    />
                  );
                })()}
            </AnimatePresence>
          </svg>

          {/* Center: breathing radial glow + group summary. */}
          <div
            aria-hidden
            className="pointer-events-none absolute left-1/2 top-1/2 size-[46%] -translate-x-1/2 -translate-y-1/2 rounded-full opacity-25 blur-2xl [animation:pulse_5s_ease-in-out_infinite]"
            style={{
              background: `radial-gradient(circle, ${hex} 0%, transparent 70%)`,
            }}
          />
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <motion.div
              className="flex max-w-[58%] flex-col items-center gap-1.5 text-center"
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ delay: 0.4, duration: 0.5 }}
            >
              {/* Full group name, sized down (and clamped) for long names so
                  it always fits inside the orbit. */}
              <p
                className={cn(
                  "line-clamp-2 break-words font-semibold leading-tight tracking-tight text-neutral-50",
                  group.name.length <= 10
                    ? "text-3xl md:text-4xl"
                    : "text-xl md:text-2xl",
                )}
              >
                {group.name}
              </p>
              <p className="text-sm font-medium" style={{ color: hex }}>
                {summaryLine[aggregate]}
              </p>
              <p className="text-xs text-neutral-500">
                {members.length} Application{members.length === 1 ? "" : "s"}
              </p>
            </motion.div>
          </div>

          {/* App nodes: dot on the orbit + glass badge that expands into a
              deployment card on hover/click. */}
          {members.map((app, i) => {
            const a = appAngle(i, members.length);
            const cos = Math.cos(a);
            const sin = Math.sin(a);
            const status = statuses[i];
            const shex = STATUS_HEX[status];
            const expanded = active === app;
            const toggleFocus = () =>
              setFocused((f) => (f === app ? null : app));
            const pillAnchor =
              Math.abs(cos) < 0.35 ? "center" : cos > 0 ? "left" : "right";
            const delay = 0.7 + i * 0.08;

            const pillTx =
              pillAnchor === "center" ? "-50%" : pillAnchor === "left" ? "0%" : "-100%";

            // Card placement relative to the (stationary) pill: it opens
            // toward the stage center, sharing an edge with the pill so the
            // combined hover region is contiguous — and it stays inside the
            // stage (no clipping at the edges).
            const cardPlacement: React.CSSProperties =
              pillAnchor === "center"
                ? sin < 0
                  ? { top: "100%", left: "50%", translate: "-50% 0" } // top node → below
                  : { bottom: "100%", left: "50%", translate: "-50% 0" } // bottom node → above
                : {
                    ...(cos > 0 ? { right: "100%" } : { left: "100%" }), // side node → inward
                    ...(sin <= 0 ? { top: 0 } : { bottom: 0 }),
                  };

            return (
              <div key={app}>
                {/* Status dot sitting on the orbit. */}
                <div
                  className="absolute z-10"
                  style={{
                    left: `${50 + RING_PCT * cos}%`,
                    top: `${50 + RING_PCT * sin}%`,
                    transform: "translate(-50%, -50%)",
                  }}
                >
                  <motion.button
                    type="button"
                    aria-label={`${title(app)}: ${STATUS_WORD[status]}`}
                    onClick={toggleFocus}
                    onMouseEnter={() => hoverIn(app)}
                    onMouseLeave={hoverOut}
                    initial={{ opacity: 0, scale: 0 }}
                    animate={{ opacity: 1, scale: 1 }}
                    transition={{
                      delay,
                      type: "spring",
                      stiffness: 220,
                      damping: 19,
                    }}
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
                        )}
                        style={{
                          background: shex,
                          boxShadow: `0 0 10px ${shex}80`,
                        }}
                      />
                    </span>
                  </motion.button>
                </div>

                {/* Badge + adjacent deployment card. The pill NEVER moves or
                    unmounts — it is the hover anchor under the cursor. The
                    card opens as an edge-touching sibling growing toward the
                    stage center, so pill ∪ card is one contiguous hover
                    region: the pointer never ends up over nothing (which
                    caused enter/leave churn — flicker). */}
                <motion.div
                  className={cn("absolute", expanded ? "z-30" : "z-10")}
                  style={{
                    left: `${50 + PILL_PCT * cos}%`,
                    top: `${50 + PILL_PCT * sin}%`,
                    transform: `translate(${pillTx}, -50%)`,
                  }}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  transition={{ delay: delay + 0.08, duration: 0.4 }}
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
                    onClick={toggleFocus}
                    className={cn(
                      "flex items-center gap-1.5 rounded-full border bg-white/[0.06] px-2 py-1 shadow-lg backdrop-blur-md transition-colors",
                      expanded
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

/** VisionOS-style glass deployment card a badge expands into. */
function NodeCard({
  app,
  status,
  rings,
  pinned,
  onClose,
  onOpen,
  onSeed,
}: {
  app: string;
  status: NodeStatus;
  rings: GroupAppRings | undefined;
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
