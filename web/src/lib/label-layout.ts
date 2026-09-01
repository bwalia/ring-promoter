import { SUN_GLOW, type GlobeMetrics } from "@/lib/globe-layout";

/**
 * Collision-free name-plate placement for the orbital stage.
 *
 * Pure geometry so the solver can run inside the animation frame loop:
 * the stage hands it satellite positions, measured label sizes, and the
 * keep-out boxes (sun, Earth, dials, already-placed labels); it hands back a
 * clamped, non-overlapping centre for each label plus the spot that
 * produced it, which is retried first next frame so labels stay put.
 */

/** Axis-aligned rectangle. */
export type Box = { l: number; r: number; t: number; b: number };

export function overlaps(a: Box, b: Box): boolean {
  return !(a.r < b.l || a.l > b.r || a.b < b.t || a.t > b.b);
}

function clampN(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

/** A candidate label anchor: which side, leader length, vertical shift. */
export type LabelSpot = { side: 1 | -1; len: number; vf: number };

const LEADER_LENGTHS = [22, 34, 48, 64, 84] as const;
const VERTICAL_STEPS = [0, -1.1, 1.1, -2.2, 2.2, -3.4, 3.4] as const;
/** Padding around the measured label box for collision tests. */
const BOX_PAD = 6;
/** Extra sky kept clear of Earth's disc so names never sit on continents. */
const EARTH_PAD = 14;

/** Keep-out zone around the sun: disc, glow halo, and the caption below. */
export function sunBox(m: GlobeMetrics): Box {
  const glow = m.sunR * SUN_GLOW;
  const capW = 96;
  return {
    l: Math.min(m.sunX - glow, m.sunX - capW / 2),
    r: Math.max(m.sunX + glow, m.sunX + capW / 2),
    t: m.sunY - glow,
    b: Math.max(m.sunY + glow, m.sunY + m.sunR + 26),
  };
}

/** True when `box` intersects Earth's disc (plus padding). */
export function hitsEarth(box: Box, m: GlobeMetrics, pad = EARTH_PAD): boolean {
  const x = clampN(m.cx, box.l, box.r);
  const y = clampN(m.cy, box.t, box.b);
  return Math.hypot(x - m.cx, y - m.cy) < m.earthR + pad;
}

/**
 * Find a collision-free box for one label near its satellite at (x, y).
 *
 * Candidates fan out on the side away from Earth first (then the other
 * side), then slide vertically; every candidate is clamped fully inside
 * `bounds` and rejected if it overlaps Earth, the sun, a dial, or another
 * label. The spot that worked last frame is retried first so labels stay
 * put while their satellite drifts.
 */
export function placeLabel(
  x: number,
  y: number,
  rad: number,
  w: number,
  h: number,
  bounds: Box,
  taken: Box[],
  prev: LabelSpot | null,
  earth?: GlobeMetrics,
): { cx: number; cy: number; box: Box; spot: LabelSpot } | null {
  const prefer: 1 | -1 = earth
    ? x >= earth.cx
      ? 1
      : -1
    : bounds.r - x > x - bounds.l
      ? 1
      : -1;
  const other: 1 | -1 = prefer === 1 ? -1 : 1;
  const spots: LabelSpot[] = [];
  if (prev) spots.push(prev);
  for (const len of LEADER_LENGTHS)
    for (const side of [prefer, other])
      for (const vf of VERTICAL_STEPS) spots.push({ side, len, vf });
  // Last resort: walk both sides top-to-bottom until a slot frees up.
  if (h > 0) {
    const gap = h + 8;
    for (const side of [prefer, other] as const) {
      for (let sy = bounds.t + h / 2; sy <= bounds.b - h / 2; sy += gap)
        spots.push({ side, len: 28, vf: (sy - y) / h });
    }
  }

  for (const spot of spots) {
    const cx = clampN(
      x + spot.side * (rad + spot.len + w / 2),
      bounds.l + w / 2 + BOX_PAD,
      bounds.r - w / 2 - BOX_PAD,
    );
    const cy = clampN(
      y + spot.vf * h,
      bounds.t + h / 2 + 1,
      bounds.b - h / 2 - 1,
    );
    const box = {
      l: cx - w / 2 - BOX_PAD,
      r: cx + w / 2 + BOX_PAD,
      t: cy - h / 2 - 1,
      b: cy + h / 2 + 1,
    };
    if (taken.some((k) => overlaps(box, k))) continue;
    if (earth && hitsEarth(box, earth)) continue;
    return { cx, cy, box, spot };
  }
  return null;
}
