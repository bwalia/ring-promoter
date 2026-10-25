// API types mirroring the Go server's JSON responses (internal/promoter,
// internal/store, internal/api). Field names match the Go json tags exactly.

export interface Ring {
  name: string;
  label: string;
}

/** Geographic pin from config `location`. Apps without one are omitted. */
export interface AppLocation {
  lat: number;
  lng: number;
  city?: string;
  region?: string;
}

export interface AppsResponse {
  apps: string[];
  /** Display title per app (config display_name), falling back to the name. */
  titles?: Record<string, string>;
  /** Config `location` pins, keyed by app name. Missing means unplaced. */
  locations?: Record<string, AppLocation>;
  rings: Ring[];
  /** True when the server requires a password for production deployments. */
  prod_protected?: boolean;
  /** True when the server can explain failed jobs with AI (diagnose endpoint). */
  ai_enabled?: boolean;
  /** True when an external QA agent is registered in config. */
  qa_enabled?: boolean;
  /** True when the server can suggest what to test with AI (test-kit plan). */
  ai_test_plans?: boolean;
}

/** Coarse category of a test link; picks its icon and group. */
export type TestLinkKind =
  | "web"
  | "ios"
  | "android"
  | "api"
  | "docs"
  | "dashboard"
  | "release"
  | "artifact"
  | "ci"
  | "other";

/** One place to try or inspect a deployed version. */
export interface TestLink {
  label: string;
  url: string;
  kind: TestLinkKind | string;
  /** config | health (ring health host) | run (deploy outputs) | log (found in deploy logs) */
  source: "config" | "health" | "run" | "log" | string;
  /** AI's reason for recommending the link (AI picks only). */
  why?: string;
}

/** AI suggestion of how to test a deployed version. */
export interface TestPlan {
  summary: string;
  checklist: string[];
  /** Picked only from the kit's own links — never invented. */
  links: TestLink[];
  generated_at: string;
}

/** Where to test a ring's current version (GET .../rings/{ring}/test-kit). */
export interface TestKit {
  app: string;
  ring: string;
  version: string;
  links: TestLink[];
  plan?: TestPlan;
  /** When the deploy's links were captured; absent for older versions. */
  captured_at?: string;
  ai_enabled: boolean;
  plan_status: "none" | "running" | "failed" | "done";
  plan_error?: string;
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
  /** Operator-named extra gates (config only — tickets are not live-queried here). */
  custom_gates?: CustomGateInfo[];
}

export interface CustomGateInfo {
  name: string;
  label: string;
  kind: string;
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
  /** Time to first byte of the live health check, when the transport reported it. */
  ttfb_ms?: number;
  auto_promote: boolean;
  /** Config owns this ring's auto-promote switch, so the API toggle returns 409. */
  auto_promote_managed: boolean;
  updated_at: string;
  can_promote_from: boolean;
  gates: RingGates;
  /** Where to test the current version (config, health host, deploy outputs). */
  links?: TestLink[];
  /** An AI test plan is stored for the current version. */
  has_test_plan?: boolean;
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

/** Latest status push from an external QA agent for one (app, ring). */
export interface QAReport {
  app: string;
  /** Empty = app-level summary. */
  ring?: string;
  workflow_verdict: GateState;
  env_healthy?: boolean;
  summary?: string;
  detail?: string;
  source: string;
  checked_at: string;
  updated_at: string;
}

/** GET /api/qa — registration + latest reports (or {enabled:false}). */
export interface QAStatus {
  enabled: boolean;
  name?: string;
  url?: string;
  reports?: QAReport[];
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

export type Action = "seed" | "promote" | "rollback" | "restart";
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
  /**
   * Live sub-stages of this step reported by the execution backend — the
   * GitHub Actions jobs of the dispatched workflow run, or the phases of a
   * Kubernetes Job. Replaced wholesale on every poll.
   */
  stages?: JobStage[];
}

export type StageStatus =
  | "queued"
  | "running"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled";

export interface JobStageStep {
  number: number;
  name: string;
  status: StageStatus;
  started_at?: string;
  finished_at?: string;
}

export interface JobStage {
  id: string;
  name: string;
  status: StageStatus;
  url?: string;
  started_at?: string;
  finished_at?: string;
  steps?: JobStageStep[];
}

/** One promotion-policy gate verdict recorded while the job ran. */
export interface JobGate {
  gate: string;
  verdict: "pass" | "fail" | "overridden" | string;
  detail?: string;
  ring?: string;
  /** The version the gate judged. */
  version?: string;
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
  /** Target ring of the operation, when known. */
  ring?: string;
  /** Policy gates evaluated before the deploy (Go/No-Go). */
  gates?: JobGate[];
  started_at: string;
  finished_at?: string;
}

// ---- multi-service pipelines (config `pipelines:`) ----

export type PipelineStageType = "deploy" | "gate" | "approval";

export type PipelineStatus =
  | "queued"
  | "running"
  | "waiting"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled"
  | "reused";

export interface PipelineStageDef {
  name: string;
  display_name?: string;
  type: PipelineStageType;
  apps?: string[];
  needs?: string[];
  gate?: string;
  timeout_seconds?: number;
}

export interface PipelineRunItem {
  app: string;
  status: PipelineStatus;
  job_id?: string;
  version?: string;
  /** Gate stages only: the QA agent's verdict for this app on the run's ring. */
  verdict?: GateState;
  detail?: string;
  /** Security gate items only: one check per scan kind. */
  checks?: SecurityCheck[];
}

export interface PipelineRunStage {
  name: string;
  display_name?: string;
  type: PipelineStageType;
  /** Gate stages: which gate ("qa" or "security"). */
  gate?: string;
  needs?: string[];
  status: PipelineStatus;
  message?: string;
  started_at?: string;
  finished_at?: string;
  approved_by?: string;
  items?: PipelineRunItem[];
}

export interface PipelineRun {
  id: string;
  pipeline: string;
  display_name?: string;
  ring: string;
  status: PipelineStatus;
  triggered_by?: string;
  error?: string;
  rerun_of?: string;
  started_at: string;
  finished_at?: string;
  stages: PipelineRunStage[];
}

export interface PipelineDef {
  name: string;
  display_name?: string;
  description?: string;
  stages: PipelineStageDef[];
  last_run?: PipelineRun | null;
}

export interface StartPipelineRequest {
  ring: string;
  versions?: Record<string, string>;
  change_request_code?: string;
  /** Production password — required when the run targets prod. */
  password?: string;
  triggered_by?: string;
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

// ---- security & compliance gates ----

export type Severity = "critical" | "high" | "medium" | "low";
export const SEVERITIES: Severity[] = ["critical", "high", "medium", "low"];

export type ScanKind =
  | "sast"
  | "sca"
  | "secrets"
  | "container"
  | "iac"
  | "dast"
  | "license";

export type SeverityCounts = Record<Severity, number>;

export interface SecurityFinding {
  id: string;
  severity: Severity;
  title?: string;
  location?: string;
  package?: string;
  installed?: string;
  fixed?: string;
  waived?: boolean;
}

export interface SecurityScan extends SeverityCounts {
  id: string;
  app: string;
  version: string;
  kind: ScanKind;
  scanner: string;
  findings?: SecurityFinding[];
  summary?: string;
  url?: string;
  scanned_at: string;
  created_at: string;
}

export interface SecurityCheck extends Partial<SeverityCounts> {
  kind: ScanKind;
  status: "pass" | "fail" | "missing" | "stale";
  scanners?: string[];
  waived?: number;
  scanned_at?: string;
  detail?: string;
}

export interface SecurityEvaluation {
  ring?: string;
  version?: string;
  guarded: boolean;
  verdict: "go" | "no_go" | "pending" | "none";
  detail?: string;
  checks?: SecurityCheck[];
  counts?: SeverityCounts;
  waived?: number;
}

export interface SecurityPolicy {
  rings: string[];
  require: ScanKind[];
  max: Partial<Record<Severity, number>>;
  max_age_seconds?: number;
  source: "app" | "default" | "builtin";
}

export interface SecurityWaiver {
  id: string;
  app: string;
  finding: string;
  ring?: string;
  reason: string;
  created_by?: string;
  created_at: string;
  expires_at: string;
}

export interface SecurityView {
  policy: SecurityPolicy | null;
  posture: SecurityEvaluation[];
  scans: SecurityScan[];
  waivers: SecurityWaiver[];
}

/** GET /api/apps/{app}/controls — weighted hop scorecard. */
export type ControlStatus = "pass" | "warn" | "block" | "skip";

export interface ControlScore {
  id: string;
  label: string;
  kind: string;
  weight: number;
  score: number;
  status: ControlStatus;
  detail: string;
  next_action?: string;
}

export interface HopScore {
  from: string;
  to: string;
  to_label: string;
  version: string;
  score: number;
  ready: boolean;
  blockers: string[];
  controls: ControlScore[];
  summary: string;
}

export interface ControlsView {
  app: string;
  ai_enabled: boolean;
  hops: HopScore[];
}
