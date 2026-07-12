// API types mirroring the Go server's JSON responses (internal/promoter,
// internal/store, internal/api). Field names match the Go json tags exactly.

export interface Ring {
  name: string;
  label: string;
}

export interface AppsResponse {
  apps: string[];
  rings: Ring[];
  /** True when the server requires a password for production deployments. */
  prod_protected?: boolean;
  /** True when the server can explain failed jobs with AI (diagnose endpoint). */
  ai_enabled?: boolean;
}

export interface RingView {
  ring: Ring;
  configured: boolean;
  current_version: string;
  previous_version: string;
  live_version: string;
  healthy: boolean;
  live_healthy: boolean;
  live_health_error?: string;
  auto_promote: boolean;
  updated_at: string;
  can_promote_from: boolean;
}

export interface RingState {
  app: string;
  ring: string;
  current_version: string;
  previous_version: string;
  healthy: boolean;
  updated_at: string;
}

export type Action = "seed" | "promote" | "rollback";
export type HistoryResult = "success" | "failure";

export interface HistoryEntry {
  id: number;
  app: string;
  ring: string;
  action: Action;
  from_version: string;
  to_version: string;
  result: HistoryResult;
  message: string;
  /** Stored AI explanation of a failed entry (shared, survives restarts). */
  diagnosis?: string;
  created_at: string;
}

/** State of a history entry's AI diagnosis (GET .../history/{id}/diagnose). */
export interface HistoryDiagnosis {
  diagnosis_status: "none" | "running" | "done" | "failed";
  diagnosis?: string;
  diagnosis_error?: string;
}

export interface OperationResult {
  app: string;
  action: Action;
  ring: string;
  from_ring?: string;
  version: string;
  success: boolean;
  rolled_back?: boolean;
  message: string;
  state: RingState;
}

export type StepStatus = "running" | "success" | "failed" | "skipped";
export type JobStatus = "pending" | "running" | "success" | "failed";

export interface JobStep {
  id: string;
  title: string;
  status: StepStatus;
  logs: string[];
  started_at: string;
  finished_at?: string;
}

export interface Job {
  id: string;
  app: string;
  action: Action;
  status: JobStatus;
  steps: JobStep[];
  result?: OperationResult;
  error?: string;
  /**
   * AI diagnosis of a failed job. The generation runs server-side, detached
   * from the request: diagnosis_status moves running → done|failed and
   * diagnosis carries the answer once done.
   */
  diagnosis?: string;
  diagnosis_status?: "running" | "done" | "failed";
  diagnosis_error?: string;
  started_at: string;
  finished_at?: string;
}

export interface AppGroup {
  id: string;
  name: string;
  apps: string[];
}

export interface AppVersion {
  name: string;
  type: "branch" | "tag";
}

export interface VersionsResponse {
  /** false = the app's deployer can't enumerate versions (free-form input). */
  supported: boolean;
  versions: AppVersion[];
}
