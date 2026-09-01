import type { RingView } from "@/lib/types";

/** SVG stage is 400×400 with the sun at the center. */
export const SOLAR_C = 200;

/**
 * Discrete orbit tracks (solar-system style). Inner = low latency.
 *
 * The outermost track stops well short of the 200-unit edge: name plates are
 * parked radially outward of their planet (see LABEL_ORBIT_GAP), so the track
 * radius plus that gap plus half a plate has to stay on stage.
 */
export const ORBIT_TRACKS = [68, 100, 132, 162] as const;

/** Radial gap between a planet and its name plate, in SVG units. */
export const LABEL_ORBIT_GAP = 15;

export const SOLAR_R_MIN = ORBIT_TRACKS[0];
export const SOLAR_R_MAX = ORBIT_TRACKS[ORBIT_TRACKS.length - 1];
export const SOLAR_R_DEFAULT = ORBIT_TRACKS[1];

/** Prefer prod latency; else the outermost configured ring that reported RTT. */
export function appLatencyMs(rings: RingView[] | undefined): number | null {
  return pickRingTiming(rings, (r) => r.latency_ms);
}

/** Prefer prod TTFB; else the outermost configured ring that reported it. */
export function appTtfbMs(rings: RingView[] | undefined): number | null {
  return pickRingTiming(rings, (r) => r.ttfb_ms);
}

function pickRingTiming(
  rings: RingView[] | undefined,
  read: (r: RingView) => number | undefined,
): number | null {
  if (!rings?.length) return null;
  const configured = rings.filter((r) => r.configured);
  const prod = configured.find((r) => r.ring.name === "prod");
  const prodVal = prod ? read(prod) : undefined;
  if (prodVal != null) return prodVal;
  for (let i = configured.length - 1; i >= 0; i--) {
    const ms = read(configured[i]);
    if (ms != null) return ms;
  }
  return null;
}

/** Map RTT → continuous radius, then snap to a track. */
export function latencyToRadius(ms: number | null): number {
  if (ms == null) return SOLAR_R_DEFAULT;
  const t = Math.min(1, Math.log1p(ms) / Math.log1p(800));
  const continuous = SOLAR_R_MIN + t * (SOLAR_R_MAX - SOLAR_R_MIN);
  return snapToTrack(continuous);
}

function snapToTrack(r: number): number {
  let best: number = ORBIT_TRACKS[0];
  let bestD = Math.abs(r - best);
  for (const track of ORBIT_TRACKS) {
    const d = Math.abs(r - track);
    if (d < bestD) {
      best = track;
      bestD = d;
    }
  }
  return best;
}

/**
 * Human-readable bound for each orbit track, derived from `latencyToRadius`
 * itself rather than hand-written, so the axis labels can never drift out of
 * step with the placement maths.
 */
export const ORBIT_BANDS: { r: number; label: string }[] = (() => {
  const upper = new Map<number, number>();
  for (let ms = 0; ms <= 2000; ms += 1) {
    upper.set(latencyToRadius(ms), ms);
  }
  return ORBIT_TRACKS.map((r, i) => {
    if (i === ORBIT_TRACKS.length - 1) {
      const prev = upper.get(ORBIT_TRACKS[i - 1]);
      return { r, label: prev == null ? "slowest" : `>${prev}ms` };
    }
    const max = upper.get(r);
    return { r, label: max == null ? "" : `≤${max}ms` };
  });
})();

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
  preferAngle: number | null;
};

export type SimEdge = { from: string; to: string; rest: number };

/** A planet locked to a circular orbit (classic solar-system layout). */
export type OrbitPlanet = {
  id: string;
  /** Actual draw radius, i.e. the track plus any anti-crowding stagger. */
  r: number;
  /** The latency band this body belongs to (always one of ORBIT_TRACKS). */
  track: number;
  /** Starting angle (radians). */
  angle0: number;
  /** Seconds for one full revolution (outer orbits slower). */
  period: number;
};

/**
 * Bodies are nudged off the straight-up direction so the radial latency axis
 * drawn there stays readable.
 */
const AXIS_CLEARANCE = 0.22;

/** Radial stagger applied to consecutive bodies on a crowded track. */
const STAGGER = [0, 15, -15, 30] as const;

function hash01(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return ((h >>> 0) % 10_000) / 10_000;
}

/**
 * Place every id as a planet on concentric orbits.
 * High-latency apps go to outer tracks; planets on the same track are
 * evenly spaced so the stage reads as a solar system.
 */
export function buildOrbitPlanets(
  ids: string[],
  radiusOf: (id: string) => number,
): OrbitPlanet[] {
  const byTrack = new Map<number, string[]>();
  for (const track of ORBIT_TRACKS) byTrack.set(track, []);

  for (const id of ids) {
    const r = snapToTrack(radiusOf(id));
    byTrack.get(r)!.push(id);
  }

  const out: OrbitPlanet[] = [];
  for (const track of ORBIT_TRACKS) {
    const bucket = byTrack.get(track) ?? [];
    // Stable order within a track.
    bucket.sort((a, b) => a.localeCompare(b));
    const n = bucket.length;
    if (n === 0) continue;
    // Outer orbits revolve more slowly (Kepler-ish feel).
    const period = 48 + (track / SOLAR_R_MAX) * 70;
    // Bodies now carry their own name plate, so a crowded track is resolved
    // by staggering them across the width of the latency band rather than by
    // hiding names. Only kicks in once a track is actually busy.
    const crowded = n > 6;
    bucket.forEach((id, i) => {
      const spin = (hash01(id) - 0.5) * 0.12;
      const angle0 =
        (i / n) * 2 * Math.PI - Math.PI / 2 + AXIS_CLEARANCE + spin;
      const stagger = crowded ? STAGGER[i % STAGGER.length] : 0;
      out.push({ id, r: track + stagger, track, angle0, period });
    });
  }
  return out;
}

export function planetPosition(
  p: OrbitPlanet,
  elapsedSec: number,
): { x: number; y: number; angle: number } {
  const angle = p.angle0 + (elapsedSec / p.period) * 2 * Math.PI;
  return {
    angle,
    x: SOLAR_C + p.r * Math.cos(angle),
    y: SOLAR_C + p.r * Math.sin(angle),
  };
}

/** Seed nodes on their target orbits with deterministic jitter (force layout). */
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
 * One Euler step of a lightweight force layout (used for dependency clustering
 * when not in pure orbital mode).
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

    const radial = (n.targetR - dist) * 0.085;
    fx.set(n.id, fx.get(n.id)! + ux * radial);
    fy.set(n.id, fy.get(n.id)! + uy * radial);

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
