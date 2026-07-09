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
