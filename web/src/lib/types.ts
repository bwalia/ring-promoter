// API types mirroring the Go server's JSON responses (internal/promoter,
// internal/store, internal/api). Field names match the Go json tags exactly.

export interface Ring {
  name: string;
  label: string;
}

export interface AppsResponse {
  apps: string[];
  /** Display title per app (config display_name), falling back to the name. */
  titles?: Record<string, string>;
  rings: Ring[];
  /** True when the server requires a password for production deployments. */
  prod_protected?: boolean;
  /** True when the server can explain failed jobs with AI (diagnose endpoint). */
  ai_enabled?: boolean;
}

/** Which promotion-policy gates guard entering a ring, plus live window state. */
export interface RingGates {
  /** A maintenance window is required to promote/seed into this ring. */
  maintenance_window: boolean;
  /** A QA/release Go-No-Go sign-off is required for the exact version. */
  qa_signoff: boolean;
  /** A valid change-request code is required. */
  change_request: boolean;
  /** CR validation backend (e.g. "jira"), set when change_request is true. */
  change_request_provider?: string;
  /** Whether a maintenance window is open right now for this ring. */
  maintenance_window_open: boolean;
  /** A Grafana dashboard's go/no-go verdict is required to enter this ring. */
  grafana: boolean;
  /** The live verdict, present only when `grafana` is true. */
  grafana_status?: GrafanaVerdict;
}

/**
 * One evaluation of a ring's Grafana go/no-go gate. `no_go` blocks promotion
 * into the ring; `check` is advisory; `unknown` means Grafana could not answer
 * (an observability outage must not stop a release, so it never blocks).
 */
export interface GrafanaVerdict {
  verdict: GateState;
  /** The individual dashboard queries behind the verdict, in config order. */
  checks?: GrafanaCheck[];
  /** Human name of the dashboard behind the verdict. */
  dashboard?: string;
  /** Deep link to that dashboard for this ring. */
  dashboard_url?: string;
  /** One-line explanation of a gate-level failure. */
  error?: string;
  checked_at: string;
  /** No Grafana was contacted — the verdict comes from config. */
  demo?: boolean;
}

export type GateState = "go" | "check" | "no_go" | "unknown";

/** One check behind a gate — typically one nightly E2E suite. */
export interface GrafanaCheck {
  /** e.g. "Register & Login". */
  name: string;
  verdict: GateState;
  /** The number the query returned (absent when unknown). */
  value?: number;
  /** Suffix for the value, e.g. "%". */
  unit?: string;
  /** Thresholds the value was judged against. */
  go_min: number;
  no_go_max: number;
  /** When the underlying test run happened — not when we queried it. */
  ran_at?: string;
  /** The run is older than the gate's max_age, so its verdict is not trusted. */
  stale?: boolean;
  /** One-line explanation of an `unknown` verdict. */
  error?: string;
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
  /** Live health-check RTT in milliseconds, when measured. */
  latency_ms?: number;
  auto_promote: boolean;
  /** Config owns this ring's auto-promote switch, so the API toggle returns 409. */
  auto_promote_managed: boolean;
  updated_at: string;
  can_promote_from: boolean;
  gates: RingGates;
}

/** One effective dependency edge in the fleet topology. */
export interface TopologyEdge {
  from: string;
  to: string;
  source: "config" | "user";
}

export interface TopologyResponse {
  edges: TopologyEdge[];
}

/** A QA/release Go-No-Go sign-off for one exact (ring, version). */
export interface Signoff {
  app: string;
  ring: string;
  version: string;
  decision: "go" | "no_go";
  engineer: string;
  qa_status: string;
  note?: string;
  updated_at: string;
}

/** An operator-created ad-hoc maintenance window. */
export interface MaintenanceWindow {
  id: string;
  app: string;
  /** Target ring, or "" for all guarded rings. */
  ring: string;
  starts_at: string;
  ends_at: string;
  reason: string;
  created_by: string;
  created_at: string;
}

/** A config-defined permanent recurring maintenance window. */
export interface RecurringWindow {
  days?: string[];
  start: string;
  end: string;
  timezone?: string;
}

/** Aggregate maintenance view for an app (GET .../maintenance-windows). */
export interface MaintenanceView {
  gated: boolean;
  gated_rings: string[] | null;
  recurring: RecurringWindow[] | null;
  windows: MaintenanceWindow[] | null;
  open_rings: Record<string, boolean>;
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

/** Build metadata of the running Ring Promoter server itself (GET /version). */
export interface BuildInfo {
  /** Image tag the binary was built as, e.g. "sha-1a2b3c4" ("dev" locally). */
  version: string;
  commit: string;
  built_at: string;
  /**
   * When this process started. The image is immutable, so with a rolling
   * deploy this is also when this build was rolled out.
   */
  started_at: string;
}
