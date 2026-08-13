import type { AppLocation } from "@/lib/types";
import { SOLAR_C, SOLAR_R_MAX, SOLAR_R_MIN, latencyToRadius } from "@/lib/solar-layout";

/**
 * Globe geometry, shared number-for-number with iOS `SolarLayout` globe
 * helpers. The stage is still the 400×400 canvas with the Earth at the centre.
 *
 * Camera looks from +Z (towards the viewer). North is −Y (up on the canvas).
 * Earth spin is a rotation about the Y axis (longitude).
 */

/** Earth disc radius in canvas units. */
export const EARTH_R = 58;

/** Seconds for one full Earth revolution. Slow on purpose. */
export const EARTH_SPIN_PERIOD = 96;

/** Reduced-motion spin: one revolution per 20 minutes, barely perceptible. */
export const EARTH_SPIN_PERIOD_REDUCED = 20 * 60;

/** Altitude band above the surface, derived from the existing latency tracks. */
export const ALT_MIN = 14;
export const ALT_MAX = 78;

/**
 * Floor on orbital inclination. This camera looks from the equator, so a
 * 0° orbit is edge-on (a line through the disc). A little tilt keeps every
 * ring readable as an ellipse.
 */
export const MIN_INCLINATION = 22;

/** Samples around one orbital ring. Shared with iOS. */
export const ORBIT_SAMPLES = 80;

export type { AppLocation };

export type GlobeBody = {
  id: string;
  /** Draw radius from Earth's centre (surface + altitude). */
  r: number;
  /** Latency track this altitude snapped to (for shells / legends). */
  track: number;
  lat: number;
  /** Geographic (or synthetic) longitude at t=0, degrees. */
  lng0: number;
  /**
   * Motion along the ring in degrees/second (argument of latitude).
   * Geo pins stay fixed on their ring (0); unplaced apps drift.
   */
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
 * Orthographic projection of a lat/lng onto the 400×400 stage.
 * `spin` is Earth's rotation in radians (added to longitude).
 */
export function projectOrtho(
  latDeg: number,
  lngDeg: number,
  radius: number,
  spinRad: number,
): GlobePoint {
  const lat = (latDeg * Math.PI) / 180;
  const lng = (lngDeg * Math.PI) / 180 + spinRad;
  const cosLat = Math.cos(lat);
  const x = radius * cosLat * Math.sin(lng);
  const y = -radius * Math.sin(lat);
  const z = radius * cosLat * Math.cos(lng);
  return { x: SOLAR_C + x, y: SOLAR_C + y, z, front: z >= -0.5 };
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
  return { x: SOLAR_C + y, y: SOLAR_C - z, z: x, front: x >= -0.5 };
}

export function bodyPoint(
  body: GlobeBody,
  elapsedSec: number,
  spinRad: number,
): GlobePoint {
  const arg = body.arg0 + body.driftDegPerSec * elapsedSec;
  return projectOrbit(body.r, body.inclination, body.raan0, arg, spinRad);
}

export function surfacePoint(
  body: GlobeBody,
  elapsedSec: number,
  spinRad: number,
): GlobePoint {
  const arg = body.arg0 + body.driftDegPerSec * elapsedSec;
  return projectOrbit(EARTH_R, body.inclination, body.raan0, arg, spinRad);
}

export function sampleOrbit(
  body: GlobeBody,
  spinRad: number,
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
      ),
    );
  }
  return pts;
}

/** SVG path `d` for the visible (front) and far (back) halves of a ring. */
export function orbitPathPair(
  body: GlobeBody,
  spinRad: number,
): { front: string; back: string } {
  const pts = sampleOrbit(body, spinRad);
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
 * Place bodies on orbital rings around Earth. One ring per app:
 * radius is TTFB, colour is applied by the renderer from health.
 *
 * Config locations sit on their ring at that longitude (the plane is tilted
 * to pass through the pin). Apps without a pin get a unique inclination /
 * node / phase and drift along the ring. Same-city apps fan out by radius
 * and inclination so the rings don't collapse onto one equator.
 */
export function buildGlobeBodies(
  ids: string[],
  locationOf: (id: string) => AppLocation | null | undefined,
  radiusMsOf: (id: string) => number | null,
): GlobeBody[] {
  const placed: { id: string; loc: AppLocation; alt: number; track: number }[] = [];
  const unplaced: { id: string; alt: number; track: number }[] = [];

  for (const id of ids) {
    const loc = locationOf(id);
    const ms = radiusMsOf(id);
    const track = latencyToRadius(ms);
    const alt = altitudeFromLatency(ms);
    if (loc && Number.isFinite(loc.lat) && Number.isFinite(loc.lng)) {
      placed.push({ id, loc, alt, track });
    } else {
      unplaced.push({ id, alt, track });
    }
  }

  const out: GlobeBody[] = [];

  for (const p of placed) {
    // Tiny deterministic nudge so two apps in the same city don't stack.
    const jitterLat = (hash01(p.id + ":lat") - 0.5) * 1.6;
    const jitterLng = (hash01(p.id + ":lng") - 0.5) * 2.2;
    const lat = clamp(p.loc.lat + jitterLat, -80, 80);
    const lng0 = wrapLng(p.loc.lng + jitterLng);
    const orbit = orbitThrough(lat, lng0, p.id);
    out.push({
      id: p.id,
      r: ringRadius(p.id, p.alt),
      track: p.track,
      lat,
      lng0,
      driftDegPerSec: 0,
      placed: true,
      inclination: orbit.inclination,
      raan0: orbit.raan,
      arg0: orbit.arg,
    });
  }

  const n = unplaced.length;
  unplaced
    .slice()
    .sort((a, b) => a.id.localeCompare(b.id))
    .forEach((p, i) => {
      const inclination = MIN_INCLINATION + hash01(p.id) * 50;
      const raan0 = n === 0 ? 0 : (i / n) * 360 - 180;
      const arg0 = hash01(p.id + ":arg") * 360;
      const period = 72 + (p.track / SOLAR_R_MAX) * 50;
      const geo = latLngOnOrbit(inclination, raan0, arg0);
      out.push({
        id: p.id,
        r: ringRadius(p.id, p.alt),
        track: p.track,
        lat: geo.lat,
        lng0: geo.lng,
        driftDegPerSec: 360 / period,
        placed: false,
        inclination,
        raan0,
        arg0,
      });
    });

  return out;
}

/** TTFB altitude plus a per-app stagger so equal-latency rings don't coincide. */
function ringRadius(id: string, alt: number): number {
  const nudge = (hash01(id + ":r") - 0.5) * 12;
  return Math.max(EARTH_R + 10, EARTH_R + alt + nudge);
}

/**
 * Inclination / node / anomaly so the satellite sits at (lat, lng). Extra
 * tilt (from the id hash) opens the ellipse without leaving the city.
 */
function orbitThrough(
  lat: number,
  lng: number,
  id: string,
): { inclination: number; raan: number; arg: number } {
  const absLat = Math.abs(lat);
  const extra = 6 + hash01(id + ":i") * 20;
  let inclination = clamp(
    Math.max(absLat, MIN_INCLINATION) + extra * 0.45,
    MIN_INCLINATION,
    78,
  );
  if (inclination <= absLat + 0.25) {
    inclination = clamp(absLat + 0.5, MIN_INCLINATION, 80);
  }
  const i = rad(inclination);
  const sinU = clamp(Math.sin(rad(lat)) / Math.max(Math.sin(i), 1e-6), -1, 1);
  const u = Math.asin(sinU);
  const raan = rad(lng) - Math.atan2(Math.cos(i) * Math.sin(u), Math.cos(u));
  return {
    inclination,
    raan: wrapLng((raan * 180) / Math.PI),
    arg: (u * 180) / Math.PI,
  };
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
