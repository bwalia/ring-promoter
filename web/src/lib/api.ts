import { useAuthStore } from "@/lib/stores";
import type {
  AppGroup,
  AppsResponse,
  BuildInfo,
  HistoryDiagnosis,
  HistoryEntry,
  Job,
  MaintenanceView,
  MaintenanceWindow,
  RingView,
  Signoff,
  VersionsResponse,
} from "@/lib/types";

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = useAuthStore.getState().token;
  const res = await fetch(path, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init?.headers,
    },
  });

  let body: unknown;
  try {
    body = await res.json();
  } catch {
    body = undefined;
  }

  if (!res.ok) {
    if (res.status === 401) {
      // Token rejected: drop back to the token gate.
      useAuthStore.getState().signOut();
    }
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `request failed with status ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return body as T;
}

const app = (name: string) => `/api/apps/${encodeURIComponent(name)}`;

export const api = {
  /** Validate a token without touching the store (used by the token gate). */
  async validateToken(token: string): Promise<boolean> {
    const res = await fetch("/api/apps", {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (res.status === 401) return false;
    if (!res.ok) throw new ApiError(res.status, `server error (${res.status})`);
    return true;
  },

  /** Build metadata of the running server. Served outside the auth mux. */
  buildInfo: () => request<BuildInfo>("/version"),

  apps: () => request<AppsResponse>("/api/apps"),

  rings: (name: string) =>
    request<{ rings: RingView[] }>(`${app(name)}/rings`),

  history: (name: string) =>
    request<{ history: HistoryEntry[] }>(`${app(name)}/history`),

  versions: (name: string) =>
    request<VersionsResponse>(`${app(name)}/versions`),

  // Application groups live on the server (shared by every user), not in the
  // browser.
  groups: () => request<{ groups: AppGroup[] }>("/api/groups"),

  createGroup: (name: string, apps: string[]) =>
    request<AppGroup>("/api/groups", {
      method: "POST",
      body: JSON.stringify({ name, apps }),
    }),

  updateGroup: (id: string, name: string, apps: string[]) =>
    request<AppGroup>(`/api/groups/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify({ name, apps }),
    }),

  deleteGroup: (id: string) =>
    request<{ status: string }>(`/api/groups/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  job: (name: string, id: string) =>
    request<Job>(`${app(name)}/jobs/${encodeURIComponent(id)}`),

  // Newest job per app, shared by every user of the control plane — this is
  // how a promotion started on one screen shows up on all the others.
  jobs: () => request<{ jobs: Job[] }>("/api/jobs"),

  // AI diagnosis of a failed HISTORY entry. Recent failures carry the step
  // logs saved when they happened; older ones fall back to the recorded
  // summary. The answer is persisted server-side, shared by everyone.
  diagnoseHistory: (name: string, id: number) =>
    request<HistoryDiagnosis>(`${app(name)}/history/${id}/diagnose`, {
      method: "POST",
    }),

  historyDiagnosis: (name: string, id: number) =>
    request<HistoryDiagnosis>(`${app(name)}/history/${id}/diagnose`),

  // Ask the server's LLM why a failed job failed. The generation runs
  // server-side detached from this request: 202 + running means poll the job
  // until diagnosis_status resolves; 200 + diagnosis is the cached answer.
  diagnoseJob: (name: string, id: string) =>
    request<{ diagnosis?: string; diagnosis_status?: string }>(
      `${app(name)}/jobs/${encodeURIComponent(id)}/diagnose`,
      { method: "POST" },
    ),

  // Mutations always use the async job flow so the UI can render live
  // step-by-step progress; each returns the job id to poll. `password` is the
  // production password; `crCode` is the change-request code required by an
  // app's promotion policy for a change-request-gated ring.
  seed: (
    name: string,
    ring: string,
    version: string,
    password?: string,
    crCode?: string,
  ) =>
    request<{ job_id: string }>(`${app(name)}/seed?async=1`, {
      method: "POST",
      body: JSON.stringify({
        ring,
        version,
        ...(password ? { password } : {}),
        ...(crCode ? { cr_code: crCode } : {}),
      }),
    }),

  promote: (
    name: string,
    fromRing: string,
    password?: string,
    crCode?: string,
  ) =>
    request<{ job_id: string }>(`${app(name)}/promote?async=1`, {
      method: "POST",
      body: JSON.stringify({
        from_ring: fromRing,
        ...(password ? { password } : {}),
        ...(crCode ? { cr_code: crCode } : {}),
      }),
    }),

  // ---- promotion-policy gates ----

  maintenanceWindows: (name: string) =>
    request<MaintenanceView>(`${app(name)}/maintenance-windows`),

  createMaintenanceWindow: (
    name: string,
    win: {
      ring: string;
      starts_at: string;
      ends_at: string;
      reason: string;
      created_by: string;
    },
  ) =>
    request<MaintenanceWindow>(`${app(name)}/maintenance-windows`, {
      method: "POST",
      body: JSON.stringify(win),
    }),

  deleteMaintenanceWindow: (name: string, id: string) =>
    request<{ status: string }>(
      `${app(name)}/maintenance-windows/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    ),

  signoffs: (name: string) =>
    request<{ signoffs: Signoff[] }>(`${app(name)}/signoffs`),

  createSignoff: (
    name: string,
    so: {
      ring: string;
      version: string;
      decision: "go" | "no_go";
      engineer: string;
      qa_status: string;
      note?: string;
    },
  ) =>
    request<Signoff>(`${app(name)}/signoffs`, {
      method: "POST",
      body: JSON.stringify(so),
    }),

  rollback: (name: string, ring: string) =>
    request<{ job_id: string }>(`${app(name)}/rollback?async=1`, {
      method: "POST",
      body: JSON.stringify({ ring }),
    }),

  setAutoPromote: (
    name: string,
    ring: string,
    enabled: boolean,
    password?: string,
  ) =>
    request<{ app: string; ring: string; auto_promote: boolean }>(
      `${app(name)}/rings/${encodeURIComponent(ring)}/auto-promote`,
      {
        method: "PUT",
        body: JSON.stringify({ enabled, ...(password ? { password } : {}) }),
      },
    ),
};
