"use client";

import { ActionDialogs } from "@/components/dashboard/action-dialogs";
import { HistoryPanel } from "@/components/dashboard/history-panel";
import { JobProgress } from "@/components/dashboard/job-progress";
import { OverviewCards } from "@/components/dashboard/overview-cards";
import { Pipeline } from "@/components/dashboard/pipeline";
import { ErrorState } from "@/components/error-state";
import { useHistory, useRings } from "@/lib/queries";
import { ApiError } from "@/lib/api";

export function Dashboard({ app }: { app: string }) {
  const rings = useRings(app);
  const history = useHistory(app);

  if (rings.error && !rings.data) {
    const notFound =
      rings.error instanceof ApiError && rings.error.status === 404;
    return (
      <div className="mx-auto max-w-3xl p-6">
        <ErrorState
          title={notFound ? `Unknown application “${app}”` : "Failed to load"}
          message={
            notFound
              ? "It may have been removed from the server configuration. Pick another app from the sidebar."
              : rings.error.message
          }
          onRetry={notFound ? undefined : () => rings.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl space-y-8 p-4 md:p-6 xl:p-8">
      <OverviewCards
        app={app}
        rings={rings.data}
        history={history.data}
        updatedAt={rings.dataUpdatedAt}
      />

      <JobProgress app={app} />

      <Pipeline app={app} rings={rings.data} isPending={rings.isPending} />

      <HistoryPanel
        app={app}
        history={history.data}
        isPending={history.isPending}
        error={history.error}
        onRetry={() => history.refetch()}
      />

      <ActionDialogs app={app} rings={rings.data ?? []} />
    </div>
  );
}
