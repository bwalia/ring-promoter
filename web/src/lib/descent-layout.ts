import type { RingView } from "@/lib/types";

/**
 * Descent — the Rings of Apps stage.
 *
 * The promotion rings ARE the orbits: the first ring in promotion order (int)
 * is the outermost circle and the last (prod) sits just above Earth, which
 * stands for live users. Every app owns one fixed spoke; the lit trail on it
 * shows how far the app's newest version has travelled toward Earth.
 *
 * Pure geometry on a 1000×1000 design canvas centred on (0, 0), so it runs
 * anywhere and the iOS app can mirror it number-for-number
 * (ios/…/RingsUniverse/DescentLayout.swift). Keep the two in lockstep.
 */

/** Design canvas edge; the stage scales it to fit. Origin is the centre. */
export const DESCENT_DESIGN = 1000;
/** Earth's disc radius. */
export const DESCENT_EARTH_R = 86;
/** Radius of the last (prod-most) ring. */
export const DESCENT_RING_INNER = 132;
/** Radius of the first (int-most) ring. */
export const DESCENT_RING_OUTER = 318;
/** Group arcs run just outside the outer ring. */
export const DESCENT_GROUP_ARC_R = DESCENT_RING_OUTER + 16;
/** Where an app's name plate starts, measured from the centre. */
export const DESCENT_LABEL_R = DESCENT_RING_OUTER + 32;
/** Radians left free at 12 o'clock for the ring name tags. */
export const DESCENT_TOP_GAP = 0.34;
/** |cos(angle)| below this centres the label under/over its spoke. */
export const DESCENT_LABEL_CENTER_BAND = 0.12;
/** Node radii: a deployed ring, and the dot for "nothing deployed". */
export const DESCENT_NODE_R = 8;
export const DESCENT_EMPTY_R = 3;
/** Past this many spokes the labels crowd — fall back to lanes. */
export const DESCENT_MAX_SPOKES = 36;
/** Below this stage width (CSS px / points) the orbit is unreadable — lanes. */
export const DESCENT_MIN_WIDTH = 560;
/** Half-width of the amber gate bar drawn across a spoke. */
export const DESCENT_GATE_HALF = 11;
/** One comet pass between two rings, seconds. */
export const DESCENT_COMET_SECONDS = 1.6;

export type Point = { x: number; y: number };

/** Radius of the ring at `index` in promotion order (0 = first = outermost). */
export function ringRadius(index: number, count: number): number {
  if (count <= 1) return (DESCENT_RING_INNER + DESCENT_RING_OUTER) / 2;
  const t = index / (count - 1);
  return DESCENT_RING_OUTER + (DESCENT_RING_INNER - DESCENT_RING_OUTER) * t;
}

/**
 * Angle of spoke `index` of `count`, radians, clockwise from 3 o'clock (SVG /
 * UIKit convention: +y points down). Spokes share the circle evenly except
 * for the gap at 12 o'clock that holds the ring tags.
 */
export function spokeAngle(index: number, count: number): number {
  const span = Math.PI * 2 - DESCENT_TOP_GAP;
  return -Math.PI / 2 + DESCENT_TOP_GAP / 2 + (span * (index + 0.5)) / Math.max(1, count);
}

/** Half the angular width of one spoke's slice. */
export function spokeHalfWidth(count: number): number {
  return (Math.PI * 2 - DESCENT_TOP_GAP) / Math.max(1, count) / 2;
}

export function polar(angle: number, r: number): Point {
  return { x: Math.cos(angle) * r, y: Math.sin(angle) * r };
}

export type LabelAnchor = "start" | "middle" | "end";

/** Labels read outward: right half left-aligned, left half right-aligned. */
export function labelAnchor(angle: number): LabelAnchor {
  const c = Math.cos(angle);
  if (c > DESCENT_LABEL_CENTER_BAND) return "start";
  if (c < -DESCENT_LABEL_CENTER_BAND) return "end";
  return "middle";
}

/** True when the orbit layout is readable; otherwise render lanes. */
export function descentFits(stageWidth: number, spokes: number): boolean {
  return stageWidth >= DESCENT_MIN_WIDTH && spokes <= DESCENT_MAX_SPOKES;
}

// ---------------------------------------------------------------------------
// Data → spoke state
// ---------------------------------------------------------------------------

/**
 * One ring on one app's spoke.
 * - `off`: this app doesn't use the ring (not configured) — nothing drawn.
 * - `empty`: configured, nothing deployed — a small dot.
 * - `healthy` / `failed`: deployed; live health check passes or not.
 */
export type NodeState = "off" | "empty" | "healthy" | "failed";

export type SpokeNode = {
  ring: string;
  label: string;
  version: string | null;
  state: NodeState;
  /** Holds the app's newest version (solid) rather than an older one (outline). */
  fresh: boolean;
  ttfbMs: number | null;
  /** Why entry into this ring is blocked right now, if it is. */
  gateClosed: string | null;
};

export type Spoke = {
  nodes: SpokeNode[];
  /** Newest version anywhere on the spoke — the one travelling inward. */
  newest: string | null;
  /** Deepest ring index holding `newest`; -1 when nothing is deployed. */
  frontier: number;
  /** Ring index right after the frontier whose gate is closed; -1 if none. */
  gateAt: number;
};

/** Live reason a ring can't be entered, or null. Only states we can observe. */
export function closedGate(view: RingView | undefined): string | null {
  const g = view?.gates;
  if (!g) return null;
  if (g.maintenance_window && !g.maintenance_window_open) return "Maintenance window closed";
  if (g.grafana && g.grafana_status?.verdict === "no_go") return "Grafana no-go";
  return null;
}

/**
 * Build one app's spoke. `order` is the canonical promotion order
 * (int → … → prod) from `/api/apps`; rings the server didn't report are `off`.
 */
export function buildSpoke(rings: RingView[] | undefined, order: string[]): Spoke {
  const byName = new Map((rings ?? []).map((r) => [r.ring.name, r]));
  const names = order.length ? order : (rings ?? []).map((r) => r.ring.name);
  const raw = names.map((name) => {
    const v = byName.get(name);
    const configured = !!v?.configured;
    const version = configured && v?.current_version ? v.current_version : null;
    const state: NodeState = !configured
      ? "off"
      : !version
        ? "empty"
        : v?.live_healthy
          ? "healthy"
          : "failed";
    return {
      ring: name,
      label: v?.ring.label ?? name,
      version,
      state,
      ttfbMs: v?.ttfb_ms ?? v?.latency_ms ?? null,
      gateClosed: configured ? closedGate(v) : null,
    };
  });
  // Versions enter at the first ring, so the outermost deployed version is
  // the newest one in flight.
  const newest = raw.find((n) => n.version)?.version ?? null;
  let frontier = -1;
  raw.forEach((n, i) => {
    if (newest && n.version === newest) frontier = i;
  });
  let gateAt = -1;
  for (let i = frontier + 1; i < raw.length && frontier >= 0; i++) {
    if (raw[i].state === "off") continue;
    if (raw[i].gateClosed) gateAt = i;
    break;
  }
  return {
    nodes: raw.map((n) => ({ ...n, fresh: !!newest && n.version === newest })),
    newest,
    frontier,
    gateAt,
  };
}

/** Next ring the newest version would enter, skipping unused rings. */
export function nextRingIndex(spoke: Spoke): number {
  if (spoke.frontier < 0) return -1;
  for (let i = spoke.frontier + 1; i < spoke.nodes.length; i++) {
    if (spoke.nodes[i].state !== "off") return i;
  }
  return -1;
}

/** One-line reading of a spoke, for aria-labels and the detail card. */
export function spokeSentence(spoke: Spoke): string {
  if (spoke.frontier < 0) return "Nothing deployed yet";
  const at = spoke.nodes[spoke.frontier];
  const next = nextRingIndex(spoke);
  if (next < 0) return `${spoke.newest} is live in ${at.ring}`;
  let hops = 0;
  for (let i = spoke.frontier + 1; i < spoke.nodes.length; i++) {
    if (spoke.nodes[i].state !== "off") hops++;
  }
  let s = `${spoke.newest} has reached ${at.ring}, ${hops} ring${hops === 1 ? "" : "s"} to go`;
  if (spoke.gateAt >= 0) {
    s += ` · ${spoke.nodes[spoke.gateAt].ring} gate closed: ${spoke.nodes[spoke.gateAt].gateClosed}`;
  }
  return s;
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

export type GroupSpan = { id: string; name: string; from: number; to: number };

/**
 * Order apps so each group's spokes sit together, groups in config order,
 * ungrouped apps last. An app in several groups goes with the first one;
 * an app listed twice in one group appears once.
 */
export function orderByGroup(
  apps: string[],
  groups: { id: string; name: string; apps: string[] }[],
): { ordered: string[]; spans: GroupSpan[] } {
  const known = new Set(apps);
  const placed = new Set<string>();
  const ordered: string[] = [];
  const spans: GroupSpan[] = [];
  for (const g of groups) {
    const members = [...new Set(g.apps)].filter((a) => known.has(a) && !placed.has(a));
    if (!members.length) continue;
    spans.push({ id: g.id, name: g.name, from: ordered.length, to: ordered.length + members.length - 1 });
    for (const a of members) {
      placed.add(a);
      ordered.push(a);
    }
  }
  for (const a of apps) if (!placed.has(a)) ordered.push(a);
  return { ordered, spans };
}

/** SVG arc path for a group span, with a small gap at each end. */
export function groupArcPath(span: GroupSpan, count: number): string {
  const half = spokeHalfWidth(count) - 0.035;
  const a0 = spokeAngle(span.from, count) - half;
  const a1 = spokeAngle(span.to, count) + half;
  const r = DESCENT_GROUP_ARC_R;
  const p0 = polar(a0, r);
  const p1 = polar(a1, r);
  const large = a1 - a0 > Math.PI ? 1 : 0;
  return `M ${p0.x.toFixed(2)} ${p0.y.toFixed(2)} A ${r} ${r} 0 ${large} 1 ${p1.x.toFixed(2)} ${p1.y.toFixed(2)}`;
}
