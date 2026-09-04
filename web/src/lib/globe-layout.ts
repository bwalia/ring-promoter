import type { AppLocation } from "@/lib/types";
import { SOLAR_R_MAX, SOLAR_R_MIN, latencyToRadius } from "@/lib/solar-layout";

/**
 * Globe geometry, originally authored number-for-number with iOS
 * `SolarLayout`; the web stage has since diverged (corner sun, fanned
 * inclinations, geometric ring spacing). The design was authored on a
 * 400×400 canvas; live stages pass their CSS pixel size into `globeMetrics`
 * so Earth, the Sun, and the ring band are recomputed from the real width
 * and height instead of stretching a 400-unit world.
 *
 * Camera looks from +Z (towards the viewer). North is −Y (up on the canvas).
 * Earth spin is a rotation about the Y axis (longitude).
 *
 * Radius rule: every app (or group) gets a unique ring size. TTFB only
 * decides order (faster = closer in). Two apps never share nearly the same
 * ellipse — even 17 services on a training instance stay nested and separable.
 */

/** Canonical design size the fractions below were authored against. */
export const DESIGN_SIZE = 400;

/** Earth disc radius on the 400×400 design. Sized for a cinematic hub. */
export const EARTH_R = 46;

/**
 * Sun (= Ring Promoter) parks in the top-left corner of the stage, well
 * clear of the ring band — `globeMetrics` caps the outermost orbit so the
 * sun's glow can never touch it.
 */
export const SUN_R = 15;
/** Design-unit margin from the stage's top-left corner (inside insets). */
export const SUN_MARGIN = 36;
/** The glow halo reaches about this many sun radii from the disc centre. */
export const SUN_GLOW = 2.8;

/** Seconds for one full Earth revolution. Slow on purpose. */
export const EARTH_SPIN_PERIOD = 96;

/** Reduced-motion spin: one revolution per 20 minutes, barely perceptible. */
export const EARTH_SPIN_PERIOD_REDUCED = 20 * 60;

/** Altitude band kept for iOS parity / legacy altitude helpers. */
export const ALT_MIN = 14;
export const ALT_MAX = 78;

/**
 * Isolated rings: innermost at `RING_INNER_RATIO`× Earth's radius, outermost
 * near the stage edge (capped so the sun's glow stays clear). Radii step
 * geometrically, so gaps widen outward like a real planetary system.
 */
export const RING_INNER_RATIO = 1.75;
export const RING_OUTER = 210;

/** Above this count, the roster/list is the primary identifier; rings dim harder. */
export const DENSITY_CAP = 10;

/**
 * Orbital-tilt band. This camera looks from the equator, so 0° is edge-on;
 * rings fan across ±INCLINATION_MAX so satellites spread over the whole sky
 * instead of stacking in one diagonal band.
 */
export const INCLINATION_MAX = 42;
export const INCLINATION_MIN = 16;

/**
 * Distinct tilt per ring: adjacent radii alternate sign and walk the
 * magnitude band from ±42° down to ±16°, with a small per-id jitter so a
 * re-sorted fleet never looks mechanical.
 */
export function ringInclination(index: number, count: number, id: string): number {
  const pairs = Math.max(1, Math.ceil(count / 2));
  const t = pairs === 1 ? 0.5 : Math.floor(index / 2) / (pairs - 1);
  const magnitude = INCLINATION_MAX - t * (INCLINATION_MAX - INCLINATION_MIN);
  const sign = index % 2 === 0 ? 1 : -1;
  return sign * magnitude + (hash01(id + ":i") - 0.5) * 4;
}

/** Samples around one orbital ring. Shared with iOS. */
export const ORBIT_SAMPLES = 80;

/**
 * Live-canvas geometry. Earth stays circular (sized from the short side) so
 * a rectangular viewport is not squashed; the stage itself still fills both
 * axes and the extra long-side space is where the Sun and roster live.
 *
 * Vertical placement uses the *usable* band between chrome insets. Centering
 * at raw `height/2` after the fullscreen + overlay-roster change pinned Earth
 * under the bottom roster (empty sky above). Insets keep Sun/Earth/rings in
 * the clear stage — upper-middle of that band by default.
 */
export type GlobeMetrics = {
  width: number;
  height: number;
  /** Design-unit → CSS-pixel scale for this stage. */
  k: number;
  cx: number;
  cy: number;
  earthR: number;
  sunR: number;
  /** Sun disc centre, absolute stage coordinates (top-left corner). */
  sunX: number;
  sunY: number;
  /** Innermost ring radius (RING_INNER_RATIO × earthR). */
  ringInner: number;
  /** Outermost ring radius, capped so orbits never reach the sun's glow. */
  ringOuter: number;
};

/** Chrome reserved around the globe so overlays do not hide the stage. */
export type GlobeInsets = {
  top?: number;
  bottom?: number;
  /** Width of a right-hand rail (roster) overlaying the stage. */
  right?: number;
  /**
   * Fraction of the usable band for Earth's centre. `0.5` is true middle;
   * slightly less sits upper-middle. Defaults to `0.5` with no insets, and
   * `0.46` when chrome insets are present.
   */
  verticalBias?: number;
};

export function globeMetrics(
  width: number,
  height: number,
  insets: GlobeInsets = {},
): GlobeMetrics {
  const w = Math.max(1, width);
  const h = Math.max(1, height);
  const top = Math.max(0, insets.top ?? 0);
  const bottom = Math.max(0, insets.bottom ?? 0);
  const right = Math.max(0, insets.right ?? 0);
  const usable = Math.max(1, h - top - bottom);
  const usableW = Math.max(1, w - right);
  const bias =
    insets.verticalBias ?? (top > 0 || bottom > 0 ? 0.46 : 0.5);
  const k = Math.min(usableW, h) / DESIGN_SIZE;
  const cx = usableW / 2;
  const cy = top + usable * bias;
  const sunR = SUN_R * k;
  const sunX = SUN_MARGIN * k;
  const sunY = top + SUN_MARGIN * k;
  const ringInner = EARTH_R * k * RING_INNER_RATIO;
  // Outer ring: as wide as the stage allows, but never into the sun's glow
  // or past the design cap.
  const sunClear =
    Math.hypot(cx - sunX, cy - sunY) - sunR * SUN_GLOW - 10 * k;
  const edgeClear =
    Math.min(cx, cy - top, h - bottom - cy, usableW - cx) - 18 * k;
  const ringOuter = Math.max(
    ringInner * 1.2,
    Math.min(RING_OUTER * k, sunClear, edgeClear),
  );
  return {
    width: w,
    height: h,
    k,
    cx,
    cy,
    earthR: EARTH_R * k,
    sunR,
    sunX,
    sunY,
    ringInner,
    ringOuter,
  };
}

export const DEFAULT_METRICS: GlobeMetrics = globeMetrics(DESIGN_SIZE, DESIGN_SIZE);

export type { AppLocation };

export type GlobeBody = {
  id: string;
  /** Isolated draw radius from Earth's centre. Unique per app. */
  r: number;
  /** Latency band (TTFB snap) — ordering hint only; `r` is the unique size. */
  track: number;
  lat: number;
  /** Geographic (or synthetic) longitude at t=0, degrees. */
  lng0: number;
  /** Motion along the ring in degrees/second (argument of latitude). */
  driftDegPerSec: number;
  /** True when this body came from a config `location` pin. */
  placed: boolean;
  /** Orbital-plane tilt from the equator, degrees. */
  inclination: number;
  /** Longitude of the ascending node at t=0, degrees. */
  raan0: number;
  /** Argument of latitude at t=0, degrees — where the satellite sits on the ring. */
  arg0: number;
};

export type GlobePoint = {
  x: number;
  y: number;
  z: number;
  /** Facing the camera (front hemisphere, including the limb). */
  front: boolean;
};

export function altitudeFromLatency(ms: number | null): number {
  const track = latencyToRadius(ms);
  const t = (track - SOLAR_R_MIN) / (SOLAR_R_MAX - SOLAR_R_MIN);
  return ALT_MIN + t * (ALT_MAX - ALT_MIN);
}

export function earthSpin(elapsedSec: number, reduceMotion: boolean): number {
  const period = reduceMotion ? EARTH_SPIN_PERIOD_REDUCED : EARTH_SPIN_PERIOD;
  return ((elapsedSec / period) * 2 * Math.PI) % (2 * Math.PI);
}

/**
 * Orthographic projection of a lat/lng onto the live stage.
 * `spin` is Earth's rotation in radians (added to longitude).
 */
export function projectOrtho(
  latDeg: number,
  lngDeg: number,
  radius: number,
  spinRad: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): GlobePoint {
  const lat = (latDeg * Math.PI) / 180;
  const lng = (lngDeg * Math.PI) / 180 + spinRad;
  const cosLat = Math.cos(lat);
  const x = radius * cosLat * Math.sin(lng);
  const y = -radius * Math.sin(lat);
  const z = radius * cosLat * Math.cos(lng);
  return { x: metrics.cx + x, y: metrics.cy + y, z, front: z >= -0.5 };
}

/**
 * Circular orbit in the Earth's equatorial frame, then into the same canvas
 * coordinates as `projectOrtho` (X east, −Y north, Z toward the camera).
 *
 * `spinRad` is added to the ascending node so rings ride with Earth.
 */
export function projectOrbit(
  radius: number,
  inclinationDeg: number,
  raanDeg: number,
  argDeg: number,
  spinRad: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): GlobePoint {
  const i = rad(inclinationDeg);
  const Ω = rad(raanDeg) + spinRad;
  const u = rad(argDeg);
  const cosI = Math.cos(i);
  const sinI = Math.sin(i);
  const cosO = Math.cos(Ω);
  const sinO = Math.sin(Ω);
  const cosU = Math.cos(u);
  const sinU = Math.sin(u);
  // Standard ECI: X through lng 0, Z north.
  const x = radius * (cosO * cosU - sinO * sinU * cosI);
  const y = radius * (sinO * cosU + cosO * sinU * cosI);
  const z = radius * (sinU * sinI);
  return { x: metrics.cx + y, y: metrics.cy - z, z: x, front: x >= -0.5 };
}

export function bodyPoint(
  body: GlobeBody,
  elapsedSec: number,
  spinRad: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): GlobePoint {
  const arg = body.arg0 + body.driftDegPerSec * elapsedSec;
  return projectOrbit(body.r, body.inclination, body.raan0, arg, spinRad, metrics);
}

export function surfacePoint(
  body: GlobeBody,
  elapsedSec: number,
  spinRad: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): GlobePoint {
  const arg = body.arg0 + body.driftDegPerSec * elapsedSec;
  return projectOrbit(
    metrics.earthR,
    body.inclination,
    body.raan0,
    arg,
    spinRad,
    metrics,
  );
}

export function sampleOrbit(
  body: GlobeBody,
  spinRad: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
  samples = ORBIT_SAMPLES,
): GlobePoint[] {
  const pts: GlobePoint[] = [];
  for (let k = 0; k <= samples; k++) {
    pts.push(
      projectOrbit(
        body.r,
        body.inclination,
        body.raan0,
        (k / samples) * 360,
        spinRad,
        metrics,
      ),
    );
  }
  return pts;
}

/** SVG path `d` for the visible (front) and far (back) halves of a ring. */
export function orbitPathPair(
  body: GlobeBody,
  spinRad: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): { front: string; back: string } {
  const pts = sampleOrbit(body, spinRad, metrics);
  return { front: pathForHemisphere(pts, true), back: pathForHemisphere(pts, false) };
}

function pathForHemisphere(pts: GlobePoint[], front: boolean): string {
  const parts: string[] = [];
  let run: GlobePoint[] = [];
  const flush = () => {
    if (run.length >= 2) {
      parts.push(
        run
          .map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(2)} ${p.y.toFixed(2)}`)
          .join(" "),
      );
    }
    run = [];
  };
  for (const p of pts) {
    if (p.front === front) run.push(p);
    else flush();
  }
  flush();
  return parts.join(" ");
}

function hash01(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return ((h >>> 0) % 10_000) / 10_000;
}

/**
 * Distinct ring radii for `count` apps, stepped geometrically from
 * RING_INNER_RATIO× Earth's radius to the outer sky, so gaps widen outward.
 * A fleet of 1 parks on a comfortable inner-mid ring; a fleet of 17 still
 * gets 17 different sizes.
 */
export function isolatedRadii(
  count: number,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): number[] {
  if (count <= 0) return [];
  const inner = metrics.ringInner;
  const outer = Math.max(inner * 1.2, metrics.ringOuter);
  if (count === 1) return [inner * Math.pow(outer / inner, 0.42)];
  const step = Math.pow(outer / inner, 1 / (count - 1));
  return Array.from({ length: count }, (_, i) => inner * step ** i);
}

/** Sort by TTFB/latency (faster = inner), then id for a stable order. */
function latencySorted(
  ids: string[],
  radiusMsOf: (id: string) => number | null,
): string[] {
  return [...ids].sort((a, b) => {
    const ma = radiusMsOf(a);
    const mb = radiusMsOf(b);
    if (ma == null && mb == null) return a.localeCompare(b);
    if (ma == null) return 1;
    if (mb == null) return -1;
    if (ma !== mb) return ma - mb;
    return a.localeCompare(b);
  });
}

/**
 * Assign unique radii. Sort by TTFB/latency (faster = inner), then id so
 * equal timings still get different ellipses.
 */
export function assignIsolatedRadii(
  ids: string[],
  radiusMsOf: (id: string) => number | null,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): Map<string, number> {
  const sorted = latencySorted(ids, radiusMsOf);
  const radii = isolatedRadii(sorted.length, metrics);
  const out = new Map<string, number>();
  sorted.forEach((id, i) => out.set(id, radii[i]!));
  return out;
}

/**
 * Place bodies on isolated orbital rings around Earth. One ring per app
 * (or per group): unique radius, fanned inclinations, satellites drifting
 * in the sky. Config location only sets the starting longitude — it never
 * stacks two apps on the same ellipse.
 */
export function buildGlobeBodies(
  ids: string[],
  locationOf: (id: string) => AppLocation | null | undefined,
  radiusMsOf: (id: string) => number | null,
  metrics: GlobeMetrics = DEFAULT_METRICS,
): GlobeBody[] {
  const sorted = latencySorted(ids, radiusMsOf);
  const ringIndex = new Map(sorted.map((id, i) => [id, i]));
  const radii = assignIsolatedRadii(ids, radiusMsOf, metrics);
  const n = ids.length || 1;

  return ids.map((id, i) => {
    const loc = locationOf(id);
    const ms = radiusMsOf(id);
    const r = radii.get(id) ?? metrics.ringInner;
    const track = latencyToRadius(ms);
    const placed = !!(loc && Number.isFinite(loc.lat) && Number.isFinite(loc.lng));
    const lat = placed ? clamp(loc!.lat, -80, 80) : 0;
    const lng0 = placed ? wrapLng(loc!.lng) : wrapLng((i / n) * 360 - 180);
    const idx = ringIndex.get(id) ?? i;
    const inclination = ringInclination(idx, n, id);
    const raan0 = (hash01(id + ":raan") - 0.5) * 56;
    // Even argument-of-latitude, regardless of config pin. Co-located
    // fleets (every app in London) used to share one longitude and stack
    // as a radial blob; unique radii were not enough.
    const arg0 = wrapLng((idx / n) * 360 + hash01(id + ":arg") * 14);
    const period = 56 + (r / metrics.ringOuter) * 48;
    const geo = latLngOnOrbit(inclination, raan0, arg0);
    return {
      id,
      r,
      track,
      lat: placed ? lat : geo.lat,
      lng0: placed ? lng0 : geo.lng,
      driftDegPerSec: 360 / period,
      placed,
      inclination,
      raan0,
      arg0,
    };
  });
}

function latLngOnOrbit(
  inclination: number,
  raan: number,
  arg: number,
): { lat: number; lng: number } {
  const i = rad(inclination);
  const u = rad(arg);
  return {
    lat: (Math.asin(Math.sin(i) * Math.sin(u)) * 180) / Math.PI,
    lng: wrapLng(raan + (Math.atan2(Math.cos(i) * Math.sin(u), Math.cos(u)) * 180) / Math.PI),
  };
}

export function centroidLocation(
  locs: Array<AppLocation | null | undefined>,
): AppLocation | null {
  const pts = locs.filter((l): l is AppLocation => !!l && Number.isFinite(l.lat));
  if (pts.length === 0) return null;
  if (pts.length === 1) return pts[0];
  let x = 0;
  let y = 0;
  let z = 0;
  for (const p of pts) {
    const lat = (p.lat * Math.PI) / 180;
    const lng = (p.lng * Math.PI) / 180;
    x += Math.cos(lat) * Math.cos(lng);
    y += Math.cos(lat) * Math.sin(lng);
    z += Math.sin(lat);
  }
  x /= pts.length;
  y /= pts.length;
  z /= pts.length;
  const hyp = Math.hypot(x, y);
  return {
    lat: (Math.atan2(z, hyp) * 180) / Math.PI,
    lng: (Math.atan2(y, x) * 180) / Math.PI,
    city: pts[0].city,
    region: pts[0].region,
  };
}

/** Great-circle distance in kilometres. */
export function haversineKm(a: AppLocation, b: AppLocation): number {
  const R = 6371;
  const dLat = rad(b.lat - a.lat);
  const dLng = rad(b.lng - a.lng);
  const s =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(dLng / 2) ** 2;
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(s)));
}

/**
 * Honest-ish Internet RTT from great-circle km: fiber ~200 km/ms one-way,
 * plus a small handshake floor. Not a substitute for measured TTFB.
 */
export function estimateRttMs(km: number): number {
  return Math.round(2 * (km / 200) + 18);
}

function rad(deg: number): number {
  return (deg * Math.PI) / 180;
}

function clamp(n: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, n));
}

function wrapLng(lng: number): number {
  let x = lng;
  while (x > 180) x -= 360;
  while (x < -180) x += 360;
  return x;
}

export function formatLocation(loc: AppLocation | null | undefined): string {
  if (!loc) return "";
  if (loc.city && loc.region) return `${loc.city}, ${loc.region}`;
  return loc.city || loc.region || `${loc.lat.toFixed(1)}°, ${loc.lng.toFixed(1)}°`;
}

/**
 * Simplified continent outlines (lng, lat), coarse enough to paint a globe
 * without shipping GeoJSON. Marker placement uses real coordinates; these
 * shapes are atmosphere.
 */
export const LAND_POLYS: [number, number][][] = [
  // North America
  [
    [-168, 65], [-141, 70], [-128, 71], [-105, 68], [-89, 68], [-80, 62],
    [-70, 58], [-60, 47], [-67, 44], [-74, 40], [-81, 25], [-97, 26],
    [-106, 22], [-110, 24], [-117, 32], [-124, 40], [-124, 48], [-130, 55],
    [-153, 57], [-166, 54], [-168, 65],
  ],
  // Greenland
  [
    [-73, 76], [-60, 82], [-20, 81], [-22, 70], [-44, 60], [-58, 61], [-73, 76],
  ],
  // South America
  [
    [-81, 12], [-60, 8], [-50, 0], [-35, -8], [-38, -20], [-54, -35],
    [-68, -55], [-75, -50], [-73, -18], [-81, -5], [-81, 12],
  ],
  // Europe
  [
    [-10, 52], [-9, 43], [-1, 43], [3, 42], [10, 44], [16, 40], [29, 41],
    [30, 46], [24, 60], [12, 58], [5, 61], [-5, 59], [-10, 52],
  ],
  // Africa
  [
    [-17, 21], [-10, 12], [8, 5], [10, -4], [14, -12], [40, -16],
    [32, -28], [20, -35], [18, -32], [12, -17], [-5, -5], [-14, 4],
    [-17, 14], [-6, 36], [10, 37], [25, 32], [32, 31], [11, 33],
    [-5, 36], [-17, 28], [-17, 21],
  ],
  // Middle East + India
  [
    [27, 40], [36, 36], [44, 40], [60, 37], [67, 25], [77, 8], [80, 15],
    [88, 22], [73, 25], [62, 25], [48, 30], [36, 21], [32, 31], [27, 40],
  ],
  // Asia
  [
    [30, 60], [40, 68], [70, 72], [90, 75], [130, 71], [160, 66], [180, 65],
    [170, 60], [142, 46], [130, 43], [122, 30], [105, 20], [100, 10],
    [104, 1], [98, 8], [94, 18], [78, 28], [74, 40], [80, 50], [60, 50],
    [45, 55], [30, 60],
  ],
  // SE Asia islands (simplified)
  [
    [95, 6], [104, -6], [119, -8], [131, -8], [120, 5], [105, 7], [95, 6],
  ],
  // Australia
  [
    [113, -22], [114, -34], [137, -35], [153, -28], [153, -12],
    [142, -11], [129, -14], [113, -22],
  ],
  // New Zealand
  [
    [166, -41], [178, -37], [178, -46], [166, -47], [166, -41],
  ],
  // Antarctica (hint)
  [
    [-180, -72], [-90, -70], [0, -70], [90, -72], [180, -72], [180, -90],
    [-180, -90], [-180, -72],
  ],
];
