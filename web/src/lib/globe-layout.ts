import type { AppLocation } from "@/lib/types";

/**
 * Earth for the Rings of Apps stage: the orthographic projection, spin and
 * coastlines behind `EarthGlobe`. Stage geometry (rings, spokes, labels)
 * lives in `descent-layout.ts`.
 *
 * Camera looks from +Z (towards the viewer). North is −Y (up on the canvas).
 * Earth spin is a rotation about the Y axis (longitude).
 */

/** Seconds for one full Earth revolution. Slow on purpose. */
export const EARTH_SPIN_PERIOD = 96;

/** Reduced-motion spin: one revolution per 20 minutes, barely perceptible. */
export const EARTH_SPIN_PERIOD_REDUCED = 20 * 60;

/** Where `EarthGlobe` paints: the canvas size and Earth's centre and radius. */
export type GlobeMetrics = {
  width: number;
  height: number;
  cx: number;
  cy: number;
  earthR: number;
};

export const DEFAULT_METRICS: GlobeMetrics = {
  width: 400,
  height: 400,
  cx: 200,
  cy: 200,
  earthR: 46,
};

export type { AppLocation };

export type GlobePoint = {
  x: number;
  y: number;
  z: number;
  front: boolean;
};

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
