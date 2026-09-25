"use client";

import { useState } from "react";
import type { NodeStatus } from "@/components/group-ring";
import { GroupDialog } from "@/components/group-dialog";
import { QaStatusBar } from "@/components/qa-status-bar";
import { DescentStage } from "@/components/descent-stage";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import {
  useAddTopologyEdge,
  useAppLocations,
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
  "empty",
];

function aggregateStatuses(statuses: NodeStatus[]): NodeStatus {
  return AGGREGATE_PRIORITY.find((s) => statuses.includes(s)) ?? "empty";
}

/**
 * "Rings of Applications" in the nav; the stage itself is titled "Rings of
 * Apps". One Descent stage: every app is a spoke through the promotion
 * rings, and groups ("rings" in the nav) are arcs around the outside.
 */
export function FleetView() {
  const { data } = useApps();
  const known = data?.apps ?? [];
  const appLocations = useAppLocations();
  const groups = useGroups().data ?? [];
  const { data: appEdges = [] } = useTopology();
  const addEdge = useAddTopologyEdge();
  const removeEdge = useRemoveTopologyEdge();
  const selectApp = usePrefsStore((s) => s.selectApp);
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const [createOpen, setCreateOpen] = useState(false);

  const appResults = useGroupRings(known);
  const deploying = useDeployingApps(known);
  const appStatuses: NodeStatus[] = appResults.map((r) =>
    deploying.has(r.app) ? "deploying" : baseStatus(r),
  );

  const seedApp = (app: string) => {
    setPendingAction({ type: "seed", app });
    selectApp(app);
  };

  return (
    <div className="relative flex h-full min-h-0 flex-col overflow-hidden">
      {known.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
          <p className="text-sm font-medium">No apps configured</p>
          <p className="text-sm text-muted-foreground">
            Add apps under <code>apps:</code> in the server config.
          </p>
        </div>
      ) : (
        <DescentStage
          className="min-h-0 flex-1"
          apps={known}
          results={appResults}
          statuses={appStatuses}
          aggregate={aggregateStatuses(appStatuses)}
          deploying={deploying}
          edges={appEdges}
          groups={groups}
          locations={appLocations}
          editable
          onAddEdge={(from, to) => addEdge.mutate({ from, to })}
          onRemoveEdge={(from, to) => removeEdge.mutate({ from, to })}
          onOpen={selectApp}
          onSeed={seedApp}
          onOpenGroup={selectGroup}
          actions={
            <>
              <QaStatusBar compact />
              <Button
                variant="outline"
                size="sm"
                className="h-8 border-white/15 bg-[#07070a]/70 text-xs text-neutral-100 backdrop-blur-md hover:bg-white/10"
                onClick={() => setCreateOpen(true)}
              >
                New ring
              </Button>
            </>
          }
        />
      )}

      {createOpen && (
        <GroupDialog open apps={known} onOpenChange={setCreateOpen} />
      )}
    </div>
  );
}
