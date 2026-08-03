import type { RingView } from "@/lib/types";

/** SVG stage is 400×400 with the sun at the center. */
export const SOLAR_C = 200;
export const SOLAR_R_MIN = 72;
export const SOLAR_R_MAX = 172;
export const SOLAR_R_DEFAULT = 118;

/** Prefer prod latency; else the outermost configured ring that reported RTT. */
export function appLatencyMs(rings: RingView[] | undefined): number | null {
  if (!rings?.length) return null;
  const configured = rings.filter((r) => r.configured);
  const prod = configured.find((r) => r.ring.name === "prod");
  if (prod?.latency_ms != null) return prod.latency_ms;
  for (let i = configured.length - 1; i >= 0; i--) {
    const ms = configured[i].latency_ms;
    if (ms != null) return ms;
  }
  return null;
}

/** Map RTT → orbit radius. High latency pushes the planet farther from the sun. */
export function latencyToRadius(ms: number | null): number {
  if (ms == null) return SOLAR_R_DEFAULT;
  // 0ms → min, ~800ms+ → max (log-ish soft clamp).
  const t = Math.min(1, Math.log1p(ms) / Math.log1p(800));
  return SOLAR_R_MIN + t * (SOLAR_R_MAX - SOLAR_R_MIN);
}

/** Spring rest-length between two linked apps; high latency stretches them apart. */
export function linkRestLength(msA: number | null, msB: number | null): number {
  const a = msA ?? 80;
  const b = msB ?? 80;
  const ms = Math.max(a, b);
  const t = Math.min(1, Math.log1p(ms) / Math.log1p(800));
  return 48 + t * 110;
}

export type SimNode = {
  id: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  targetR: number;
  /** Preferred angle (radians) for group sector clustering. */
  preferAngle: number | null;
};

export type SimEdge = { from: string; to: string; rest: number };

function hash01(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return ((h >>> 0) % 10_000) / 10_000;
}

/** Seed nodes on their target orbits with deterministic jitter. */
export function seedNodes(
  apps: string[],
  radiusOf: (app: string) => number,
  preferAngleOf: (app: string) => number | null,
): SimNode[] {
  const n = apps.length || 1;
  return apps.map((id, i) => {
    const prefer = preferAngleOf(id);
    const angle =
      prefer ?? (i / n) * 2 * Math.PI - Math.PI / 2 + (hash01(id) - 0.5) * 0.35;
    const r = radiusOf(id);
    return {
      id,
      x: SOLAR_C + r * Math.cos(angle),
      y: SOLAR_C + r * Math.sin(angle),
      vx: 0,
      vy: 0,
      targetR: r,
      preferAngle: prefer,
    };
  });
}

/**
 * One Euler step of a lightweight solar-system force layout.
 * - Radial spring toward targetR (latency)
 * - Angular bias toward group sector
 * - Link springs (dependency proximity / latency stretch)
 * - Soft node–node repulsion
 */
export function stepSimulation(
  nodes: SimNode[],
  edges: SimEdge[],
  dt = 0.35,
): void {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const fx = new Map<string, number>();
  const fy = new Map<string, number>();
  for (const n of nodes) {
    fx.set(n.id, 0);
    fy.set(n.id, 0);
  }

  for (const n of nodes) {
    const dx = n.x - SOLAR_C;
    const dy = n.y - SOLAR_C;
    const dist = Math.hypot(dx, dy) || 0.001;
    const ux = dx / dist;
    const uy = dy / dist;

    // Pull toward target orbit radius.
    const radial = (n.targetR - dist) * 0.085;
    fx.set(n.id, fx.get(n.id)! + ux * radial);
    fy.set(n.id, fy.get(n.id)! + uy * radial);

    // Soft angular bias into the group's sector.
    if (n.preferAngle != null) {
      const ang = Math.atan2(dy, dx);
      let dAng = n.preferAngle - ang;
      while (dAng > Math.PI) dAng -= 2 * Math.PI;
      while (dAng < -Math.PI) dAng += 2 * Math.PI;
      const tangX = -uy;
      const tangY = ux;
      fx.set(n.id, fx.get(n.id)! + tangX * dAng * 0.55);
      fy.set(n.id, fy.get(n.id)! + tangY * dAng * 0.55);
    }
  }

  for (const e of edges) {
    const a = byId.get(e.from);
    const b = byId.get(e.to);
    if (!a || !b) continue;
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const dist = Math.hypot(dx, dy) || 0.001;
    const force = ((dist - e.rest) / dist) * 0.045;
    fx.set(a.id, fx.get(a.id)! + dx * force);
    fy.set(a.id, fy.get(a.id)! + dy * force);
    fx.set(b.id, fx.get(b.id)! - dx * force);
    fy.set(b.id, fy.get(b.id)! - dy * force);
  }

  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      const a = nodes[i];
      const b = nodes[j];
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.hypot(dx, dy) || 0.001;
      if (dist > 90) continue;
      const push = ((90 - dist) / dist) * 0.09;
      fx.set(a.id, fx.get(a.id)! - dx * push);
      fy.set(a.id, fy.get(a.id)! - dy * push);
      fx.set(b.id, fx.get(b.id)! + dx * push);
      fy.set(b.id, fy.get(b.id)! + dy * push);
    }
  }

  for (const n of nodes) {
    n.vx = (n.vx + fx.get(n.id)!) * 0.82;
    n.vy = (n.vy + fy.get(n.id)!) * 0.82;
    n.x += n.vx * dt * 16;
    n.y += n.vy * dt * 16;
    // Keep inside the stage with a soft margin.
    n.x = Math.min(380, Math.max(20, n.x));
    n.y = Math.min(380, Math.max(20, n.y));
  }
}

/** Assign each group a mid-angle sector; apps in multiple groups use the first. */
export function groupSectorAngles(
  apps: string[],
  groups: { id: string; name: string; apps: string[] }[],
): Map<string, number> {
  const relevant = groups.filter((g) =>
    g.apps.some((a) => apps.includes(a)),
  );
  const out = new Map<string, number>();
  if (relevant.length === 0) return out;
  const n = relevant.length;
  relevant.forEach((g, i) => {
    const mid = (i / n) * 2 * Math.PI - Math.PI / 2;
    for (const a of g.apps) {
      if (apps.includes(a) && !out.has(a)) out.set(a, mid);
    }
  });
  return out;
}
