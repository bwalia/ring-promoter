"use client";

import { ActivityFeed } from "@/components/dashboard/activity-feed";
import type { NodeStatus } from "@/components/group-ring";
import { SolarSystem } from "@/components/solar-system";
import { summarizeRings } from "@/lib/app-health";
import {
  useAddTopologyEdge,
  useApps,
  useDeployingApps,
  useGroupRings,
  useGroups,
  useRemoveTopologyEdge,
  useTopology,
  type GroupAppRings,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";

function baseStatus(r: GroupAppRings): NodeStatus {
  if (r.isPending || !r.rings) return "loading";
  const { active, healthy } = summarizeRings(r.rings);
  if (active.length === 0) return "empty";
  if (healthy === active.length) return "healthy";
  return healthy === 0 ? "failed" : "degraded";
}

const AGGREGATE_PRIORITY: NodeStatus[] = [
  "failed",
  "degraded",
  "deploying",
  "loading",
  "healthy",
];

/** Fleet home: all apps in one solar-system stage. */
export function FleetView() {
  const { data } = useApps();
  const members = data?.apps ?? [];
  const groups = useGroups().data ?? [];
  const results = useGroupRings(members);
  const deploying = useDeployingApps(members);
  const { data: edges = [] } = useTopology();
  const addEdge = useAddTopologyEdge();
  const removeEdge = useRemoveTopologyEdge();
  const selectApp = usePrefsStore((s) => s.selectApp);
  const setPendingAction = useUiStore((s) => s.setPendingAction);

  const statuses: NodeStatus[] = results.map((r) =>
    deploying.has(r.app) ? "deploying" : baseStatus(r),
  );
  const aggregate: NodeStatus =
    AGGREGATE_PRIORITY.find((s) => statuses.includes(s)) ?? "empty";

  const openApp = (app: string) => selectApp(app);
  const seedApp = (app: string) => {
    setPendingAction({ type: "seed", app });
    selectApp(app);
  };

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-6">
      <div>
        <h2 className="text-lg font-semibold">Fleet</h2>
        <p className="text-sm text-muted-foreground">
          All applications as a solar system — tightly coupled apps sit close;
          high health latency pushes them apart.
        </p>
      </div>

      {members.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center">
          <p className="text-sm font-medium">No applications configured</p>
          <p className="text-sm text-muted-foreground">
            Add apps under <code>apps:</code> in the server config.
          </p>
        </div>
      ) : (
        <>
          <SolarSystem
            sunLabel="Fleet"
            members={members}
            results={results}
            statuses={statuses}
            aggregate={aggregate}
            edges={edges}
            groups={groups}
            editable
            onAddEdge={(from, to) => addEdge.mutate({ from, to })}
            onRemoveEdge={(from, to) => removeEdge.mutate({ from, to })}
            onOpen={openApp}
            onSeed={seedApp}
          />
          <ActivityFeed apps={members} />
        </>
      )}
    </div>
  );
}
