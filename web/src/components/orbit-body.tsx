"use client";

import { STATUS_HEX, type NodeStatus } from "@/components/group-ring";
import type { RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Promotion state of one ring, as drawn on a body's dial. */
export type SegState = "healthy" | "unhealthy" | "empty" | "absent";

/** Baseline body diameter in px. Shared so label layout can reason about it. */
export const DIAL_SIZE = 30;

/**
 * Severity → diameter. Trouble grows, calm shrinks, so fleet status reads
 * at a glance from size alone — the color language stays green/orange.
 */
export function dialSizeForStatus(status: NodeStatus): number {
  switch (status) {
    case "failed":
      return 40;
    case "degraded":
      return 35;
    case "deploying":
      return 33;
    case "loading":
      return 26;
    case "empty":
      return 22;
    default:
      return 26; // healthy: small and quiet
  }
}

const SEG_HEX: Record<SegState, string> = {
  healthy: STATUS_HEX.healthy,
  unhealthy: STATUS_HEX.failed,
  empty: "#3f3f46",
  absent: "#27272a",
};

/**
 * Collapse every RingView for a body down to one segment per ring name.
 *
 * In "rings" mode a body is a whole group, so the same ring name arrives once
 * per member app; the worst member wins, because a dial that reads healthy
 * while one of its apps is down would be a lie.
 */
export function ringSegments(
  rings: RingView[] | undefined,
  order: string[],
): { name: string; state: SegState }[] {
  const byName = new Map<string, RingView[]>();
  for (const r of rings ?? []) {
    const list = byName.get(r.ring.name) ?? [];
    list.push(r);
    byName.set(r.ring.name, list);
  }
  const names = order.length
    ? order
    : [...new Set((rings ?? []).map((r) => r.ring.name))];

  return names.map((name) => {
    const views = (byName.get(name) ?? []).filter((v) => v.configured);
    if (views.length === 0) return { name, state: "absent" as SegState };
    const deployed = views.filter((v) => v.current_version);
    if (deployed.length === 0) return { name, state: "empty" as SegState };
    const bad = deployed.some((v) => !v.healthy);
    return { name, state: (bad ? "unhealthy" : "healthy") as SegState };
  });
}

function polar(c: number, r: number, a: number): [number, number] {
  return [c + r * Math.cos(a), c + r * Math.sin(a)];
}

function arcPath(c: number, r: number, a0: number, a1: number): string {
  const [x0, y0] = polar(c, r, a0);
  const [x1, y1] = polar(c, r, a1);
  const large = a1 - a0 > Math.PI ? 1 : 0;
  return `M ${x0} ${y0} A ${r} ${r} 0 ${large} 1 ${x1} ${y1}`;
}

/**
 * One orbiting body: a dial whose outer arc segments are the deployment rings
 * in promotion order (int → test → acc → prod), reading clockwise from the
 * top, with overall health in the middle.
 *
 * This is the part that turns the stage from decoration into an instrument —
 * the promotion state of every app is legible without opening anything.
 */
export function OrbitDial({
  segments,
  status,
  size = DIAL_SIZE,
  reduceMotion,
}: {
  segments: { name: string; state: SegState }[];
  status: NodeStatus;
  size?: number;
  reduceMotion: boolean;
}) {
  const c = size / 2;
  const r = c - 3;
  const hex = STATUS_HEX[status];
  const n = Math.max(segments.length, 1);
  // Leave a visible notch between segments; without it four arcs read as one
  // continuous ring and the promotion boundaries disappear.
  const gap = n > 1 ? 0.22 : 0;
  const step = (2 * Math.PI) / n;
  // Severity-sized dials keep their stroke weight in proportion.
  const sw = Math.max(0.85, Math.min(1.3, size / DIAL_SIZE));

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      aria-hidden
      className="block overflow-visible"
    >
      {segments.map((seg, i) => {
        const a0 = -Math.PI / 2 + i * step + gap / 2;
        const a1 = -Math.PI / 2 + (i + 1) * step - gap / 2;
        return (
          <path
            key={seg.name}
            d={arcPath(c, r, a0, a1)}
            fill="none"
            stroke={SEG_HEX[seg.state]}
            strokeWidth={(seg.state === "absent" ? 1.5 : 2.5) * sw}
            strokeLinecap="round"
            opacity={seg.state === "absent" ? 0.5 : 1}
          />
        );
      })}

      {/* Core: overall health. Deploying and failing are the only states that
          earn movement — everything else stays still on purpose. */}
      {status === "deploying" && !reduceMotion && (
        <circle
          cx={c}
          cy={c}
          r={r}
          fill="none"
          stroke={hex}
          strokeWidth={2.5 * sw}
          strokeLinecap="round"
          strokeDasharray={`${2 * Math.PI * r * 0.22} ${2 * Math.PI * r}`}
          className="origin-center animate-spin"
          style={{ animationDuration: "1.6s" }}
        />
      )}
      <circle
        cx={c}
        cy={c}
        r={size * 0.17}
        fill={hex}
        className={cn(
          status === "failed" && !reduceMotion && "animate-pulse",
          status === "loading" && !reduceMotion && "animate-pulse",
        )}
      />
    </svg>
  );
}
