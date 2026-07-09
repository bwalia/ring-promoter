import { useAuthStore } from "@/lib/stores";
import type {
  AppsResponse,
  HistoryEntry,
  Job,
  RingView,
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

  apps: () => request<AppsResponse>("/api/apps"),

  rings: (name: string) =>
    request<{ rings: RingView[] }>(`${app(name)}/rings`),

  history: (name: string) =>
    request<{ history: HistoryEntry[] }>(`${app(name)}/history`),

  versions: (name: string) =>
    request<VersionsResponse>(`${app(name)}/versions`),

  job: (name: string, id: string) =>
    request<Job>(`${app(name)}/jobs/${encodeURIComponent(id)}`),

  // Mutations always use the async job flow so the UI can render live
  // step-by-step progress; each returns the job id to poll. `password` is the
  // production password, required by the server for prod-bound operations.
  seed: (name: string, ring: string, version: string, password?: string) =>
    request<{ job_id: string }>(`${app(name)}/seed?async=1`, {
      method: "POST",
      body: JSON.stringify({ ring, version, ...(password ? { password } : {}) }),
    }),

  promote: (name: string, fromRing: string, password?: string) =>
    request<{ job_id: string }>(`${app(name)}/promote?async=1`, {
      method: "POST",
      body: JSON.stringify({
        from_ring: fromRing,
        ...(password ? { password } : {}),
      }),
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
