"use client";

import { useEffect } from "react";
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { useAuthStore, useJobsStore, usePrefsStore } from "@/lib/stores";
import type { HistoryEntry, Job } from "@/lib/types";

// Polling cadence. The UI never asks the user to refresh: server state is
// re-fetched on these intervals (and instantly after a job finishes).
const RINGS_INTERVAL = 10_000;
const HISTORY_INTERVAL = 30_000;
const APPS_INTERVAL = 60_000;
const JOB_INTERVAL = 1_000;

export function useApps() {
  const token = useAuthStore((s) => s.token);
  return useQuery({
    queryKey: ["apps"],
    queryFn: api.apps,
    enabled: !!token,
    refetchInterval: APPS_INTERVAL,
    staleTime: 30_000,
  });
}

export function useRings(app: string | null) {
  const token = useAuthStore((s) => s.token);
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  return useQuery({
    queryKey: ["rings", app],
    queryFn: () => api.rings(app!),
    enabled: !!token && !!app,
    refetchInterval: autoRefresh ? RINGS_INTERVAL : false,
    select: (data) => data.rings,
  });
}

export function useHistory(app: string | null) {
  const token = useAuthStore((s) => s.token);
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  return useQuery({
    queryKey: ["history", app],
    queryFn: () => api.history(app!),
    enabled: !!token && !!app,
    refetchInterval: autoRefresh ? HISTORY_INTERVAL : false,
    select: (data) => data.history ?? [],
  });
}

/** Merged, newest-first history across all apps for the activity feed. */
export function useActivityFeed(apps: string[]) {
  const token = useAuthStore((s) => s.token);
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  return useQueries({
    queries: apps.map((app) => ({
      queryKey: ["history", app],
      queryFn: () => api.history(app),
      enabled: !!token,
      refetchInterval: autoRefresh ? HISTORY_INTERVAL : (false as const),
      select: (data: { history: HistoryEntry[] }) => data.history ?? [],
    })),
    combine: (results) => ({
      isPending: results.some((r) => r.isPending),
      items: results
        .flatMap((r) => r.data ?? [])
        .sort(
          (a, b) =>
            new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
        ),
    }),
  });
}

function isTerminal(job: Job | undefined): boolean {
  return job?.status === "success" || job?.status === "failed";
}

// Module-level so several components can watch the same job (the query itself
// is deduped by key) without duplicating the completion toast/invalidation.
const handledJobs = new Set<string>();

/**
 * Polls the app's active job (if any) every second and, on completion, shows a
 * toast and refreshes rings + history so the dashboard reflects the outcome
 * immediately. The finished job stays visible until dismissed.
 */
export function useActiveJob(app: string | null) {
  const queryClient = useQueryClient();
  const active = useJobsStore((s) => (app ? s.active[app] : undefined));
  const clearActive = useJobsStore((s) => s.clearActive);

  const query = useQuery({
    queryKey: ["job", app, active?.jobId],
    queryFn: () => api.job(app!, active!.jobId),
    enabled: !!app && !!active,
    refetchInterval: (q) => (isTerminal(q.state.data) ? false : JOB_INTERVAL),
    retry: (failureCount, error) =>
      // A 404 means the server restarted or evicted the job — stop tracking.
      error instanceof ApiError && error.status === 404
        ? false
        : failureCount < 3,
  });

  useEffect(() => {
    if (!app) return;
    if (query.error instanceof ApiError && query.error.status === 404) {
      clearActive(app);
      return;
    }
    const job = query.data;
    if (!job || !isTerminal(job)) return;
    const key = `${app}:${job.id}:${job.status}`;
    if (handledJobs.has(key)) return;
    handledJobs.add(key);

    const label = `${job.action} · ${job.app}`;
    if (job.status === "success") {
      toast.success(label, {
        description: job.result?.message ?? "completed successfully",
      });
    } else {
      toast.error(label, {
        description: job.error ?? job.result?.message ?? "operation failed",
      });
    }
    queryClient.invalidateQueries({ queryKey: ["rings", app] });
    queryClient.invalidateQueries({ queryKey: ["history", app] });
  }, [app, query.data, query.error, clearActive, queryClient]);

  return {
    job: query.data,
    running: !!active && !isTerminal(query.data),
    dismiss: () => app && clearActive(app),
  };
}

// ---- mutations (async job flow) ----

export function useSeedMutation(app: string | null) {
  const setActive = useJobsStore((s) => s.setActive);
  return useMutation({
    mutationFn: ({ ring, version }: { ring: string; version: string }) => {
      if (!app) throw new Error("no application selected");
      return api.seed(app, ring, version);
    },
    onSuccess: ({ job_id }, { ring, version }) => {
      setActive(app!, { jobId: job_id, action: "seed" });
      toast.info("Seed started", {
        description: `${app}: ${version} → ${ring}`,
      });
    },
    onError: (err: Error) =>
      toast.error("Seed failed", { description: err.message }),
  });
}

export function usePromoteMutation(app: string | null) {
  const setActive = useJobsStore((s) => s.setActive);
  return useMutation({
    mutationFn: ({ fromRing }: { fromRing: string }) => {
      if (!app) throw new Error("no application selected");
      return api.promote(app, fromRing);
    },
    onSuccess: ({ job_id }, { fromRing }) => {
      setActive(app!, { jobId: job_id, action: "promote" });
      toast.info("Promotion started", {
        description: `${app}: promoting from ${fromRing}`,
      });
    },
    onError: (err: Error) =>
      toast.error("Promotion failed", { description: err.message }),
  });
}

export function useRollbackMutation(app: string | null) {
  const setActive = useJobsStore((s) => s.setActive);
  return useMutation({
    mutationFn: ({ ring }: { ring: string }) => {
      if (!app) throw new Error("no application selected");
      return api.rollback(app, ring);
    },
    onSuccess: ({ job_id }, { ring }) => {
      setActive(app!, { jobId: job_id, action: "rollback" });
      toast.info("Rollback started", {
        description: `${app}: rolling back ${ring}`,
      });
    },
    onError: (err: Error) =>
      toast.error("Rollback failed", { description: err.message }),
  });
}
