"use client";

import { useEffect } from "react";
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useNow } from "@/lib/use-now";
import { useAuthStore, useJobsStore, usePrefsStore } from "@/lib/stores";
import type { HistoryEntry, Job, RingView } from "@/lib/types";

// Polling cadence. The UI never asks the user to refresh: server state is
// re-fetched on these intervals (and instantly after a job finishes).
const RINGS_INTERVAL = 10_000;
const HISTORY_INTERVAL = 30_000;
const APPS_INTERVAL = 60_000;
const JOBS_INTERVAL = 2_000;

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

export interface GroupAppRings {
  app: string;
  rings: RingView[] | undefined;
  isPending: boolean;
  error: Error | null;
}

/** Ring state for every app of a group (shares the per-app rings cache). */
export function useGroupRings(apps: string[]): GroupAppRings[] {
  const token = useAuthStore((s) => s.token);
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  return useQueries({
    queries: apps.map((app) => ({
      queryKey: ["rings", app],
      queryFn: () => api.rings(app),
      enabled: !!token,
      refetchInterval: autoRefresh ? RINGS_INTERVAL : (false as const),
    })),
    combine: (results) =>
      results.map((r, i) => ({
        app: apps[i],
        rings: r.data?.rings,
        isPending: r.isPending,
        error: r.error,
      })),
  });
}

/**
 * Newest job per application — SHARED server state, so a promotion started on
 * one person's screen is visible on everyone's.
 */
export function useJobs() {
  const token = useAuthStore((s) => s.token);
  return useQuery({
    queryKey: ["jobs"],
    queryFn: api.jobs,
    enabled: !!token,
    refetchInterval: JOBS_INTERVAL,
    select: (data) => data.jobs ?? [],
  });
}

/**
 * Which of the given apps currently have a seed/promote/rollback running —
 * by anyone, not just this browser.
 */
export function useDeployingApps(apps: string[]): Set<string> {
  const { data: jobs } = useJobs();
  return new Set(
    (jobs ?? [])
      .filter((j) => apps.includes(j.app) && !isTerminal(j))
      .map((j) => j.app),
  );
}

/**
 * Versions that exist in the app's source repository (github-deployed apps).
 * `supported: false` means the deployer can't enumerate them — free-form input.
 */
export function useVersions(app: string | null) {
  const token = useAuthStore((s) => s.token);
  return useQuery({
    queryKey: ["versions", app],
    queryFn: () => api.versions(app!),
    enabled: !!token && !!app,
    staleTime: 60_000,
  });
}

function isTerminal(job: Job | undefined): boolean {
  return job?.status === "success" || job?.status === "failed";
}

/** Stable identity of a job card (ids restart with the server, dates don't). */
const jobKey = (job: Job) => `${job.app}:${job.id}:${job.started_at}`;

// Module-level so several components watching the same job don't duplicate
// the completion toast/invalidation.
const handledJobs = new Set<string>();
// Jobs this browser saw running: their completion deserves a toast. A job
// first seen already-finished (e.g. on page load) does not.
const seenRunning = new Set<string>();

/** How long a finished card stays visible to users who never saw it run. */
const FINISHED_CARD_WINDOW_MS = 15 * 60 * 1000;

/**
 * The app's newest job from the SHARED server list — every user sees the same
 * card, whoever started the operation. On completion it shows a toast and
 * refreshes rings + history. The finished card stays visible until dismissed
 * (dismissal is per-browser); old finished jobs don't reappear on page load.
 */
export function useActiveJob(app: string | null) {
  const queryClient = useQueryClient();
  const { data: jobs } = useJobs();
  const dismissed = useJobsStore((s) => s.dismissed);
  const dismissKey = useJobsStore((s) => s.dismiss);
  const now = useNow(60_000);

  const job = app ? jobs?.find((j) => j.app === app) : undefined;

  useEffect(() => {
    if (!job) return;
    if (!isTerminal(job)) {
      seenRunning.add(jobKey(job));
      return;
    }
    const key = `${jobKey(job)}:${job.status}`;
    if (handledJobs.has(key)) return;
    handledJobs.add(key);
    // Only completions we watched happen get a toast — not history replayed
    // into a freshly opened tab.
    if (!seenRunning.has(jobKey(job))) return;

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
    queryClient.invalidateQueries({ queryKey: ["rings", job.app] });
    queryClient.invalidateQueries({ queryKey: ["history", job.app] });
  }, [job, queryClient]);

  // A finished card is shown if this browser saw it run or it finished
  // recently; anything older is stale context, not news.
  const finishedRecently =
    !job?.finished_at ||
    now - new Date(job.finished_at).getTime() < FINISHED_CARD_WINDOW_MS;
  const visible =
    !!job &&
    !dismissed.includes(jobKey(job)) &&
    (!isTerminal(job) || seenRunning.has(jobKey(job)) || finishedRecently);

  return {
    job: visible ? job : undefined,
    running: !!job && !isTerminal(job),
    dismiss: () => job && dismissKey(jobKey(job)),
  };
}

/**
 * Ask the server's LLM to explain a failed job. The generation runs
 * server-side, detached from this request; the shared jobs poll delivers the
 * result to every viewer.
 */
export function useDiagnoseJob(app: string | null, jobId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => {
      if (!app || !jobId) throw new Error("no job to diagnose");
      return api.diagnoseJob(app, jobId);
    },
    onSuccess: (res) => {
      // Cached answer (200) renders immediately; otherwise mark the job as
      // diagnosing so the button disables while the server generates.
      queryClient.setQueryData<{ jobs: Job[] }>(
        ["jobs"],
        (data) =>
          data && {
            jobs: data.jobs.map((j) =>
              j.app === app && j.id === jobId
                ? res.diagnosis
                  ? {
                      ...j,
                      diagnosis: res.diagnosis,
                      diagnosis_status: "done" as const,
                    }
                  : { ...j, diagnosis_status: "running" as const }
                : j,
            ),
          },
      );
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
    onError: (err: Error) =>
      toast.error("AI diagnosis failed", { description: err.message }),
  });
}

/**
 * State of a failed HISTORY entry's diagnosis; polls while the model runs.
 */
export function useHistoryDiagnosis(app: string | null, id: number | null) {
  const token = useAuthStore((s) => s.token);
  return useQuery({
    queryKey: ["history-diagnosis", app, id],
    queryFn: () => api.historyDiagnosis(app!, id!),
    enabled: !!token && !!app && id !== null,
    refetchInterval: (q) =>
      q.state.data?.diagnosis_status === "running" ? JOBS_INTERVAL : false,
  });
}

/** Start the AI diagnosis of a failed history entry. */
export function useDiagnoseHistory(app: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => {
      if (!app) throw new Error("no application selected");
      return api.diagnoseHistory(app, id);
    },
    onSuccess: (_res, id) => {
      queryClient.invalidateQueries({
        queryKey: ["history-diagnosis", app, id],
      });
    },
    onError: (err: Error) =>
      toast.error("AI diagnosis failed", { description: err.message }),
  });
}

// ---- mutations (async job flow) ----

// ---- application groups (server-side, shared by every user) ----

export function useGroups() {
  const token = useAuthStore((s) => s.token);
  return useQuery({
    queryKey: ["groups"],
    queryFn: api.groups,
    enabled: !!token,
    refetchInterval: 30_000,
    select: (data) => data.groups ?? [],
  });
}

export function useCreateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, apps }: { name: string; apps: string[] }) =>
      api.createGroup(name, apps),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["groups"] }),
    onError: (err: Error) =>
      toast.error("Could not create group", { description: err.message }),
  });
}

export function useUpdateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      name,
      apps,
    }: {
      id: string;
      name: string;
      apps: string[];
    }) => api.updateGroup(id, name, apps),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["groups"] }),
    onError: (err: Error) =>
      toast.error("Could not update group", { description: err.message }),
  });
}

export function useDeleteGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteGroup(id),
    onSuccess: (_res, id) => {
      queryClient.invalidateQueries({ queryKey: ["groups"] });
      // Leaving a deleted group's page selected would strand the view.
      if (usePrefsStore.getState().selectedGroup === id) {
        usePrefsStore.getState().selectGroup(null);
      }
    },
    onError: (err: Error) =>
      toast.error("Could not delete group", { description: err.message }),
  });
}

/** Whether prod deploys need a password, and which ring is "prod" (the last). */
export function useProdProtection() {
  const { data } = useApps();
  return {
    prodProtected: !!data?.prod_protected,
    prodRing: data?.rings?.[data.rings.length - 1]?.name,
  };
}

export function useSeedMutation(app: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      ring,
      version,
      password,
    }: {
      ring: string;
      version: string;
      password?: string;
    }) => {
      if (!app) throw new Error("no application selected");
      return api.seed(app, ring, version, password);
    },
    onSuccess: (_res, { ring, version }) => {
      // The shared jobs poll picks the new job up; refetch now for snappiness.
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
      toast.info("Seed started", {
        description: `${app}: ${version} → ${ring}`,
      });
    },
    onError: (err: Error) =>
      toast.error("Seed failed", { description: err.message }),
  });
}

export function usePromoteMutation(app: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      fromRing,
      password,
    }: {
      fromRing: string;
      password?: string;
    }) => {
      if (!app) throw new Error("no application selected");
      return api.promote(app, fromRing, password);
    },
    onSuccess: (_res, { fromRing }) => {
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
      toast.info("Promotion started", {
        description: `${app}: promoting from ${fromRing}`,
      });
    },
    onError: (err: Error) =>
      toast.error("Promotion failed", { description: err.message }),
  });
}

/** Flip a ring's auto-promote switch, updating the cached rings optimistically. */
export function useAutoPromoteMutation(app: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      ring,
      enabled,
      password,
    }: {
      ring: string;
      enabled: boolean;
      password?: string;
    }) => {
      if (!app) throw new Error("no application selected");
      return api.setAutoPromote(app, ring, enabled, password);
    },
    onMutate: async ({ ring, enabled }) => {
      await queryClient.cancelQueries({ queryKey: ["rings", app] });
      const prev = queryClient.getQueryData<{ rings: RingView[] }>([
        "rings",
        app,
      ]);
      queryClient.setQueryData<{ rings: RingView[] }>(
        ["rings", app],
        (data) =>
          data && {
            rings: data.rings.map((r) =>
              r.ring.name === ring ? { ...r, auto_promote: enabled } : r,
            ),
          },
      );
      return { prev };
    },
    onError: (err: Error, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(["rings", app], ctx.prev);
      toast.error("Could not change auto-promote", {
        description: err.message,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["rings", app] });
    },
  });
}

export function useRollbackMutation(app: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ ring }: { ring: string }) => {
      if (!app) throw new Error("no application selected");
      return api.rollback(app, ring);
    },
    onSuccess: (_res, { ring }) => {
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
      toast.info("Rollback started", {
        description: `${app}: rolling back ${ring}`,
      });
    },
    onError: (err: Error) =>
      toast.error("Rollback failed", { description: err.message }),
  });
}
