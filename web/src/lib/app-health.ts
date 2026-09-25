import type { RingView } from "@/lib/types";

/**
 * One shared aggregation over an app's rings, so the group ring, the member
 * list and the node card can never disagree about the same app's health.
 */
export interface RingsSummary {
  /** Rings that are configured AND have something deployed. */
  active: RingView[];
  /** How many of the active rings pass their live health check. */
  healthy: number;
  /** The highest active ring (e.g. the prod-most deployment). */
  latest?: RingView;
  /** Most recent updated_at across active rings. */
  lastDeploy?: string;
}

export function summarizeRings(rings: RingView[] | undefined): RingsSummary {
  const active = (rings ?? []).filter((v) => v.configured && v.current_version);
  const healthy = active.filter((v) => v.live_healthy).length;
  return {
    active,
    healthy,
    latest: active[active.length - 1],
    lastDeploy: active
      .map((v) => v.updated_at)
      .sort()
      .at(-1),
  };
}

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
