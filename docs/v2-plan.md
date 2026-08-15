# Ring Promoter V2 — Implementation Plan (in progress)

## Context

The user supplied a comprehensive V2 product prompt: upgrade Ring Promoter from a
ring-promotion tool into an **AI-powered autonomous release control plane** ("Ring
Agent") that observes release events, scores risk, enforces policy gates, watches
Kubernetes health, and can hold/pause/rollback — while **orchestrating, not
replacing**, native k8s/Argo primitives. Core loop:
OBSERVE → ANALYSE → DECIDE → POLICY CHECK → ACT → VERIFY → CONTINUE.

Key mandates from the prompt:
- §1: audit the existing app first; preserve working V1; refactor, don't rebuild.
- Priority order §62: P0 = release/environment model, redesigned promotion UI,
  Ring Agent architecture, decision/evidence system, AI risk gate, k8s health
  integration, policy engine, audit system, safe hold/pause/rollback, production
  health verification, V2 UI/UX. P1 = CI adapters, progressive delivery, Argo,
  approvals centre, autonomy config, change intelligence. P2 = advanced intel.
- Safety §19–22: allowlisted structured actions only, deterministic policy engine
  has final authority, schema-validated agent output, prompt-injection resistance,
  INSUFFICIENT EVIDENCE is a valid conclusion, immutable audit for everything.
- Autonomy levels 0–5 (§18), global kill switch (§25), manual override (§24).
- UI §26–45: dense operational design, Geist Sans/Mono, left nav, pipeline
  visualisation, live deployment view, agent activity stream, approvals page,
  policy UI, audit timeline, integrations page, agent page. Light+dark themes.

## Known constraints (from prior work this session)

- Live instances that must keep working: **ring-promoter.fictionally.org**
  (training, 17 apps, config = training/config/apps.training.yaml via Helm
  --set-file), **workstation**, **ring-system/diytaxreturn** (config lives in the
  diy-tax-return-uk repo's configmap — schema must stay backward compatible).
- deploy-training-k3s1.yml boots the server against the training config and greps
  /api/apps for "operator" — config schema changes must stay compatible or that
  workflow must be updated in the same PR.
- iOS app (ios/RingPromoter) consumes the API (Models/, Networking/) — API
  changes must be additive.
- v1.0.1 released today on main @ 2960f83.
- Repo: Go backend (internal/, cmd/ringpromoter), Next.js UI embedded in the Go
  binary via internal/web/static, Helm chart deploy, Postgres or memory store.

## Progress log

- [x] Entered plan mode; plan file created.
- [x] Phase 1 audit complete: all 3 Explore reports folded in below.
- [x] Scope questions answered by user (see Scope decisions).
- [x] Phase 2 complete: Plan agent delivered the 6-phase design.
- [x] Phase 3 complete: verified gates.go:100-175 structure and
  origin/bw/sse-job-stream contents against the design's claims.
- [x] Phase 4: final plan written below.
- [x] Phase 5: presenting via ExitPlanMode.

## Scope decisions (answered by user via AskUserQuestion)

1. **First slice: backend foundations** — P0 core (agent/policy/audit/risk data
   model, event bus, Ring Agent loop, policy engine, structured decisions),
   surfaced minimally in existing UI; V2 UI redesign is slice 2.
2. **AI provider: provider interface, Ollama adapter first** (existing gateway
   qwen3-coder:30b + JWT already integrated); Anthropic adapter can follow.
3. **Rollout: in place, feature-flagged** — same binary/instances; V2 activates
   via config (agent block absent = V1 behaviour unchanged); training instance
   is the showcase.

## Audit findings

### Web UI audit (complete)

- **Stack**: Next.js 16.2.10 App Router + React 19.2.4, TS strict, Tailwind v4
  CSS-first (no tailwind.config), shadcn/ui "new-york" on unified radix-ui,
  react-query 5 + zustand 5, next-themes, cmdk, sonner, motion, lucide.
  **Fonts already Geist Sans** (+ JetBrains Mono as --font-geist-mono,
  Space Grotesk display), self-hosted. Zero frontend tests.
- **THEME LOCK** (web/AGENTS.md + globals.css banner): near-black neutral
  console colors + dark/emerald landing are frozen (PR #31 revert). Layout,
  components, UX, fonts are fair game. V2 tokens work = restructuring
  (semantic layers, typography/spacing/motion tokens), not re-hueing.
  Also: read node_modules/next/dist/docs/ before writing Next code.
- **Routing**: only 2 routes (`/` = AppShell console, `/landing`). Console
  "pages" are a zustand ternary + `?app=`/`?group=` URL sync in app-shell.tsx
  (239 lines). No next/link/useRouter. Static export via NEXT_OUTPUT=export;
  embed.sh or Dockerfile copies web/out → internal/web/static (committed;
  Docker always rebuilds). internal/web/web.go serves with `<path>.html`
  fallback — **no SPA catch-all**: new V2 routes must be static-exported
  routes; dynamic segments need generateStaticParams or web.go fallback.
- **Build pipeline**: `npm run build:embed` locally; Dockerfile node stage in CI.
- **Reusable as-is**: lib/ data layer (api.ts 254L, queries.ts 645L pure
  polling — jobs 2s, rings 10s, gates 15s, history 30s; no SSE/WS anywhere),
  types.ts (hand-mirrored Go json), status.tsx (ringHealth/HealthBadge/
  ActionBadge — single source of status semantics), version-label, relative-time,
  error-state, command-palette (244L, extensible), gate system (grafana-gate.tsx
  263L + gate-controls.tsx 366L incl. SignoffPopover/OpenWindowPopover),
  job-progress.tsx (242L live job panel), topbar, app-footer, providers.
- **Rewrite for V2**: app-shell (left nav + real routing), sidebar (485L),
  overview-cards (per-app KPI strip → real dashboard). **No table primitive
  exists** (no <table> anywhere; history-panel.tsx 331L is the closest — flex
  rows + filters; generalise it). Missing shadcn: table, sidebar, breadcrumb,
  progress, form etc. Free wins installed but unused: Card, Tabs, Separator;
  Button xs/icon-xs sizes; OrbitDial (orbit-body.tsx) as table micro-viz.
- **Orrery subsystem**: ~3,700 lines (solar-system 1,228, group-ring 706,
  globe-layout 534, earth-globe, fleet-view, solar-layout) — page-specific
  showpiece; decide keep-as-Fleet-page vs demote. FleetView/orrery hard-code
  hex colors bypassing tokens (don't respond to light mode).
- **V1 surface vs V2 needs**: approvals data layer complete but buried in
  dialogs (signoffs, maintenance windows — delete + recurring fetched but never
  rendered); Grafana gate badges fully built; AI diagnose built twice (job +
  history level); groups CRUD complete; topology complete incl. edit mode
  (restore edge has API but no hook/UI). **Missing entirely**: audit page (no
  backend endpoint; activity feed is client-side N-app fan-out), policy UI
  (config-only), agent panel (no concept), user identity (single shared
  bearer token; sign-off engineer is free text — attribution is sand).
- **Prior art**: docs/ui-modernization.md §4-8 (routes, state, SSE design,
  DataTable/LogViewer wireframes, phasing) = unimplemented V1 of this plan.

### Infra/compat audit (complete) — THE COMPATIBILITY PERIMETER

Three live instances: training (Helm + apps.training.yaml via --set-file),
workstation (raw k8s manifests, configmap one app: jobshout), **ring-system
(config lives in bwalia/diy-tax-return-uk repo; auto-rolls onto every new
image via repository_dispatch WITHOUT config update — binding constraint:
V2 binary must boot a V1 config it cannot see)**.

Must not break (priority order):
1. Config YAML schema — additive + optional only.
2. `/healthz` body containing `"status":"ok"` — 4 CI gates grep it.
3. Training validation gate: `-config` flag, RP_* env names, memory driver,
   bearer auth, `"operator"` substring in GET /api/apps, ready <10s
   (deploy-training-k3s1.yml:77-92).
4. schema.sql idempotent auto-migration (no versioning; whole file exec'd on
   boot; CREATE/ALTER IF NOT EXISTS convention). Workstation RollingUpdate
   overlaps old+new pods on same DB (postgres advisory lock 463-483) →
   **additive-only DDL mandatory**; every table needs memory.go twin.
5. REST API shape: iOS RingPromoterAPI.swift mirror, 10 training labs with
   verbatim curl (assert .steps[].title, step id "health"), rp.sh, 2 seed
   workflows. List-wrapper envelopes, ?async=1 → 202 {job_id}, 422 contract,
   prod_protected/ai_enabled flags. Rollback never needs password.
6. Pod is readOnlyRootFilesystem, non-root 65532, no writable volume, single
   ConfigMap at /etc/ringpromoter. No local disk for V2 features.

Other constraints: Helm chart Recreate vs raw-manifest RollingUpdate (changes
must hit both); chart fails on empty config (--set-file required; ArgoCD path
documented-broken); config-checksum rollout annotation; --force-conflicts
load-bearing; training preflight asserts 4 Secret keys (RP_API_TOKEN, RP_DB_DSN,
RP_GITHUB_TOKEN, RP_JIRA_TOKEN) — new required env = edit preflight + hand-create
on 3 instances; deploy-k3s1.yml seds the image line format; both deploys share
byte-identical image per SHA.

RBAC ceiling: training SA has cluster-wide get/list/watch on deployments/
replicasets/pods (a watcher needs NO new grant there); workstation SA has none
(namespaced ring-exec job rights only; ring-promoter-deployer ClusterRole owned
by diy-tax-return-uk repo — naming collision hazard); rbac.enabled:false mode
mounts no SA token — k8s features must degrade gracefully. No events/services/
nodes/CRD watch rights anywhere. Image bakes kubectl v1.30.4; deployer shells
out (no client-go yet).

Tests: Go ~186 tests across 25 files (promoter largest; -race in ci.yml on PRs
only). Web zero. iOS 140 swift-testing + 14 XCUITest, never run in CI.

Gold inputs: **ios/docs/API-GAPS.md** (8 costed backend asks: device registry/
APNs, SSE job stream GET .../jobs/{id}/events [branch bw/sse-job-stream exists],
cross-app GET /api/rings, signoff filters, persistent job history + logs,
jobs?limit=&all=true, ring/version on running Job, no-cancel rationale);
docs/kubernetes-executor-design.md; prompts/*.md (metrics + declarative
auto-promote, both implemented).

### Backend audit (complete)

- **Shape**: Go 1.25, ~16k lines (10k non-test). Deps: lib/pq, prometheus,
  yaml.v3 only. No client-go (kubectl shelled out), stdlib ServeMux, hand-rolled
  HS256 JWT. Packages: ring (ordered pipeline, ring.go:19), config, promoter
  (engine), api, store, deployer(+executor/k8sjob+github), health, grafana,
  changerequest, diagnose, progress, metrics, web.
- **API**: bearer auth (constant-time, api.go:173); /healthz,/version,/metrics
  unauth. `decode` uses **DisallowUnknownFields + 1MiB cap (api.go:684)** —
  new request fields are 400s for old servers; response fields are safe to add.
  422 = ran-and-failed with full Result. ?async=1 → 202 {job_id}; JobManager
  in-memory ring buffer 200, context.WithoutCancel, jobs lost on restart.
  **No SSE/WS anywhere** (metrics Unwrap is aspirational).
- **Engine** (promoter.go): Seed:389 / Promote:599 / promoteHop:621 /
  Rollback:766. Per-app lock (`store.Lock("app:"+name)`; PG session advisory
  lock). WAL: PendingOp journaled before deploy; ResumePendingOps on boot;
  awaitVersion polls. State persisted healthy=false right after deploy (:709).
  Health retries checkWithRetries:859 (count+1 × delay). Auto-rollback to
  dstPrev via rollbackTo:821. autoChain:565 walks auto_promote rings in the
  same job/lock; CR-gated ring fails closed mid-chain (no code). Timeout
  inventory captured (op 10m default; rings live-probe 8s parallel; etc.).
- **Gates**: ONE insertion point — `evaluateGates` (gates.go:100), four
  hard-coded ordered blocks: maintenance window (union config recurring +
  ad-hoc store) → qa_signoff (GetSignoff must be "go") → change_request (demo
  code "test" always accepted; JIRA validator) → grafana (only no_go blocks;
  the only overridable gate, override_reason mandatory; 60s verdict cache;
  demo mode when url empty; max_age staleness → unknown). GateInputs ride
  context (WithGateInputs:69). Gates evaluated TWICE per async op (pre-validate
  + operation). Surfaced via RingGates in every RingView (ringGates:116).
- **Config**: Load:482 = YAML → applyEnv:505 → applyDefaults → Validate:585.
  Load-once; **no hot reload, no config API**. RingConfig:383 (namespace/
  deployment/container/image, health_url/_expect_status/_version_field XOR
  _header, target_env, ref pin, auto_promote *bool three-state). Config may
  never auto-promote INTO prod (config.go:688).
- **Deployers**: Deployer iface + optional LiveVersioner/VersionSource
  capabilities (type-asserted). log/kubectl shared; github/k8sjob per-app.
  **`executor.Executor` + `deployer.FromExecutor` (executor.go:34) is the reuse
  seam** — new backends implement Executor, get logs/phases/cleanup free.
- **Health**: Checker 1-method + TimedChecker + VersionReporter capabilities.
  HTTPChecker: exact-status or 2xx, dotted-JSON-path or header version match
  ("wrong version live" guard), latency+TTFB via httptrace. Read-time Rings
  fan-out 8s status-only probes. **No k8s-level health intel at ring level**;
  k8sjob/status.go:83 mapStatus (Job+pod conditions → ImagePullBackOff etc.)
  is the nucleus for V2 k8s intelligence.
- **Store**: one fat 30-method Store iface; Memory + Postgres twins; embedded
  idempotent schema.sql executed on boot. Tables: ring_state, history,
  app_group, topology_edge, topology_suppression, maintenance_window, signoff
  (PK app,ring,version — upserts lose prior decisions), pending_op. Failure
  logs kept for newest 3 per app; success logs discarded. NOT persisted: jobs,
  grafana verdicts, gate evaluations, actor.
- **AI today**: diagnose = single-shot Ollama /api/chat, temp 0.2, per-request
  HS256 JWT in x-api-key, enabled iff url+RP_OLLAMA_JWT_SECRET; job-level
  (in-memory) + history-level (persisted, shared) diagnosis, single-flight,
  202+poll. `api.Diagnoser` (nil-able 1-method iface) is the seam. No
  streaming, no tools, no agent machinery.
- **Audit gaps** (vs V2 §23): no actor anywhere (shared token; signoff
  engineer/window created_by are free text); no gate-decision records (only
  ephemeral step logs); grafana override reason NOT durably recorded on
  success (logs only attach on failure, promoter.go:961); CR code never
  persisted; auto-promote toggles/groups/topology/windows/signoffs unaudited;
  no correlation id (job ids in-memory, reset on restart, not on history
  rows); history mutable (SetHistoryDiagnosis updates in place).
- **Groups/topology**: groups are UI collections only — **no group-promote
  primitive in Go** (frontend fans out per app). depends_on feeds topology
  edges only — no ordering/gating/risk effect. Suppression model for config
  edges.
- **Tests**: ~190 funcs; promoter deepest (fakeDeployer + scriptedChecker +
  fake clock doubles at promoter_test.go:22-80); api full-stack httptest over
  memory store (newTestServerFull:36). Not covered: postgres, cmd wiring,
  JobManager eviction, ring, web.
- **Highest-leverage seams** (agent's judgement, matches mine): (1) extract
  Gate interface at evaluateGates; (2) RingGates/grafana.Result is the
  precedent for surfacing structured verdicts; (3) new k8s-health capability
  interface on deployer, not new deployer; (4) widen Diagnoser + add SSE;
  (5) audit store as separate embedded interface; (6) metrics package
  pre-wired for growth; (7) **identity is the weakest foundation — audit
  attribution needs a principal concept first**.

## Final plan — Slice 1: V2 Backend Foundations (6 PRs)

Verified during review: gates.go:100–175 is exactly the 4 sequential gate
blocks (extraction is mechanical; keep Err* vars + messages verbatim);
origin/bw/sse-job-stream exists with internal/api/events.go (118L) +
events_test.go (143L) + web/src/lib/job-stream.ts, ready to rebase.

### Governing decisions

1. **Events**: store-backed outbox (`agent_event` table, written in the same
   code paths as history — pending_op WAL precedent) + in-process `event.Bus`
   as best-effort wake-up/SSE latency optimisation. Store is source of truth.
2. **Agent singleton**: loop runs only holding `store.Lock("agent:leader")`
   (PG session advisory lock / memory mutex), acquired in a goroutine so boot
   stays <10s for the training CI gate.
3. **Actor**: self-declared `X-Actor` header (cap 128, default "anonymous"),
   `actor_type` ∈ {human, agent, system}. Headers bypass DisallowUnknownFields;
   zero client changes. Named tokens/RBAC deferred to slice 3.
4. **Gate interface stays in internal/promoter** (new gate.go) — gates read
   promoter internals + typed Err* vars the API maps to status codes.
5. **LLM output**: typed Go decode (json.Decoder + DisallowUnknownFields +
   enum checks), no schema library (repo has 3 deps; keep it that way).
6. **Risk score is deterministic-only**; LLM may add narrative, never a number.
7. **Kill switch**: config `agent.mode` is a *ceiling*; runtime mode persisted
   in `agent_state` KV; effective = most restrictive; default recommend_only.
8. **Agent can never deploy into prod** — hard-denied in Authorize regardless
   of autonomy level (mirrors config.go:688; agent has no prod password).
9. **HOLD/BLOCK materialise as `agent_state` key `hold:<app>:<ring>`** read by
   a new agentDecisionGate in evaluateGates (one enforcement point); cleared
   via audited DELETE endpoint.

### Phase 1 (PR 1) — Principal, correlation IDs, audit ledger

- New: internal/promoter/actor.go (Actor + CorrelationID context carriers,
  WithGateInputs pattern), internal/promoter/audit.go (p.audit helper — never
  fails the op), internal/api/audit.go, store audit_test, api audit_api_test.
- Modified: schema.sql (+audit_event table, +correlation_id on history &
  pending_op — all IF NOT EXISTS), store.go (AuditEvent/AuditFilter/Auditor
  embedded in Store), memory.go + postgres.go twins, promoter.go (record()
  appends audit row + stamps correlation), gates.go (audit accepted grafana
  override on SUCCESS path — closes promoter.go:961 gap), groups/topology/
  maintenance/SetAutoPromote/RecordSignoff (config-category audits), api.go
  (actor+correlation middleware inside authenticate; GET /api/audit).
- AuditEvent: {id, occurred_at, correlation_id, actor_type, actor, app, ring,
  category(operation|gate|override|config|agent_decision), action, version,
  detail JSON}. GET /api/audit?app=&ring=&category=&actor=&before_id=&limit=
  → {"audit":[...], "next_before_id"} (keyset paging, default 50 max 500).
- V1 proof: no existing endpoint request/response changes; 190 tests untouched;
  schema-idempotency boot test (exec twice cleanly).

### Phase 2 (PR 2) — Policy engine: Gate interface extraction

- New: internal/promoter/gate.go — `Gate{Name; Evaluate(ctx,p,GateRequest)
  GateResult}`; GateResult{Gate, Verdict(pass|fail|overridden|skipped), Err
  (the exact typed error), Detail}.
- gates.go: 4 blocks become maintenanceGate/signoffGate/changeRequestGate/
  grafanaGate; evaluateGates = ordered loop + audit emitter; Err* vars,
  messages, ordering, rep.Log lines preserved byte-for-byte. First fail's Err
  returned unchanged. Skipped gates not audited; audit volume stays
  operation-scoped (ringGates read path untouched).
- V1 proof: gates_test.go, grafana_gate_test.go, gates_api_test.go + promoter
  tests pass WITHOUT modification (they are the oracle).

### Phase 3 (PR 3) — LLM provider interface + Ollama adapter

- New: internal/llm/llm.go — Provider{Name; Complete(ctx, Request) (Response,
  error)}; Request{System, Messages, Format(raw JSON schema, advisory),
  Temperature, MaxTokens}; ErrUnavailable; DecodeStrict(text, dst).
- New: internal/llm/ollama/ — chat client + signJWT moved from
  internal/diagnose (same gateway contract: fresh HS256 JWT per request in
  x-api-key); tests adapted from diagnose_test.go.
- internal/diagnose becomes the prompt layer over llm/ollama; api.Diagnoser
  and all diagnose behaviour (temp 0.2, 3m timeout, 202+poll single-flight)
  unchanged. main.go builds one llm.Provider from cfg.Ollama when enabled.
- V1 proof: diagnose tests unchanged; config schema unchanged.

### Phase 4 (PR 4) — Event outbox, bus, SSE

- New: internal/event/event.go — Type consts (deployment_started/_finished,
  health_changed, gate_completed, approval_granted, rollback_performed,
  auto_promote_changed, hold_set, hold_cleared, agent_decision); Bus with
  per-subscriber buffered channels, slow subscribers drop (store is truth).
- New: internal/promoter/events.go — p.emit = store append then bus publish;
  called from record()/journalStart/saveState health transitions/evaluateGates
  loop/RecordSignoff/SetAutoPromote. Promoter gains SetEventBus (nil-safe;
  store append happens even without bus so V1 instances build the log).
- schema.sql: +agent_event{id, occurred_at, type, app, ring, version,
  correlation_id, payload}; AppendEvent/ListEventsAfter on Store + twins.
- internal/api/events.go: rebase from bw/sse-job-stream (per-job
  GET .../jobs/{id}/events — API-GAPS #2) + new GET /api/events?after_id=&app=
  (replay from agent_event then live via bus; 15s heartbeat; polling remains
  documented fallback).
- V1 proof: new endpoints only; /api/jobs polling contract untouched; no disk.

### Phase 5 (PR 5) — Risk engine + AI Risk gate + RingView surfacing

- New: internal/risk/risk.go — Assessment{Score 0-100 deterministic, Category
  low|medium|high|critical (0-24/25-49/50-74/75+), Signals[{Name, Value,
  Weight, Evidence}], Narrative (optional LLM, never affects Score),
  ComputedAt}. Signals from data Ring has: history success/failure rate per
  (app, target ring), rollback frequency 30d, ring criticality (index,
  prod max), current health flags, failing/advisory gate count, naive semver
  delta, time since last success. Weighted sum clamped; table-driven tests.
- config/promotion.go: +AIRisk *AIRiskPolicy (`ai_risk: {rings, max_score,
  narrative}`) + validation. promoter/risk_gate.go: aiRiskGate with 60s cache
  (grafanaGateState mirror, grafana_gate.go:26); RingView gains
  `risk omitempty`; GateInputs gains OverrideAIRisk (reuses OverrideReason);
  gate order after grafana. Seed/promote bodies accept optional
  override_ai_risk (server-side addition — safe direction).
- V1 proof: apps without ai_risk get no gate and no risk field; loader test
  proves apps.training.yaml loads identically.

### Phase 6 (PR 6) — Ring Agent: loop, autonomy, kill switch, agent API

- New: internal/config/agent.go — top-level `agent:` block (absent = nil =
  V1 bit-identical); RP_AGENT_MODE env override; validation.
- New: internal/agent/{agent,decision,policy,evidence}.go + tests.
  - decision.go: 15-action allowlist enum (PROMOTE, CONTINUE_ROLLOUT, PAUSE,
    HOLD, WAIT_AND_RECHECK, RETRY, REQUEST_APPROVAL, REQUEST_QA, BLOCK,
    ABORT_DEPLOYMENT, ROLLBACK, RESTORE_PREVIOUS_STABLE, ESCALATE,
    MARK_RELEASE_HEALTHY, MARK_RELEASE_DEGRADED); Decision{App, Ring, Version,
    Proposed, Final, PolicyVerdict, Confidence, InsufficientEvidence (valid —
    Final becomes WAIT_AND_RECHECK), Reason, Evidence, LLMUsed,
    TriggerEventID}; strict decode of LLM output.
  - policy.go: `Authorize(action, level, mode, targetIsProd) (final, verdict)`
    — pure, table-driven, final authority. Autonomy ladder: L0 observe / L1
    recommend / L2 +informational / L3 +protective (PAUSE/HOLD/BLOCK/REQUEST_*/
    ESCALATE) / L4 +recovery non-prod (ROLLBACK/RESTORE/ABORT/RETRY) / L5
    +forward — never into prod. recommend_only caps to recorded-not-executed;
    paused idles loop.
  - evidence.go: bundle from store data only; free-text (history messages,
    logs) wrapped in <untrusted-data> delimiters.
  - agent.go loop: wake on bus or 30s tick; read events after persisted cursor
    (agent_state["event_cursor"]); OBSERVE fresh store state (never trust
    event alone); ANALYSE risk+rules; DECIDE (LLM strict-decoded; on
    ErrUnavailable/invalid → deterministic-only, protective actions only,
    fail-safe HOLD, never forward); POLICY CHECK Authorize; ACT via exported
    Promoter methods with actor_type=agent (gates/WAL/audit/history fire as
    for humans); VERIFY on resulting events; advance cursor. Agent-caused
    events never trigger new forward decisions (actor_type loop prevention).
- schema.sql: +agent_decision (full decision record + executed +
  execution_result), +agent_state KV; store methods + twins. Every decision
  also appends audit_event(category=agent_decision).
- gates.go: agentDecisionGate — 409 ErrAgentHold on active hold:<app>:<ring>.
- New: internal/api/agent.go — GET /api/agent/status, GET /api/agent/decisions,
  PUT /api/agent/mode, GET /api/agent/config, DELETE /api/agent/holds/{app}/
  {ring}; all inert-but-present when disabled ({"enabled":false}; mode PUT →
  409). /api/apps response gains agent_enabled.
- main.go: construct agent when cfg.Agent != nil; leader-lock goroutine;
  graceful stop.
- Tests: scripted llm.Provider fake (valid/junk/unknown-action/timeout); fake
  clock; full Authorize matrix (15×6×3×2); HOLD→409 loop test; provider-down
  = protective-only test; dual-consumer cursor no-double-execution test;
  flagship `TestAgentAbsentIsV1` (no agent: block ⇒ /healthz exact, additive-
  only /api/apps diff, nil agent, byte-identical operation Results, inert
  agent endpoints) + apps.training.yaml loads unchanged.

### Hard-constraint recheck (all six pass)

1. Config additive-only (new keys: agent:, promotion_policy.ai_risk).
2. /healthz handler untouched in every phase.
3. Training CI gate: memory boot, no new required env vars (agent reuses
   RP_OLLAMA_JWT_SECRET; absent = deterministic-only), <10s, "operator" grep OK.
4. DDL additive/idempotent, memory twins, old-binary overlap tolerated.
5. REST: response-field additions only; iOS/labs/rp.sh/seed contracts intact.
6. No disk (all V2 state store-backed; SSE/bus memory-only); no client-go;
   rbac degrade unaffected (no new k8s calls this slice).

### Delivery mechanics

- Branch per phase off main: bw/v2-p1-audit-ledger … bw/v2-p6-ring-agent;
  PR each to main (CI = go vet + race tests on PR). Also commit this plan as
  docs/v2-plan.md in PR 1 (user asked for pushable artifacts).
- Each PR merges only when: all existing tests green, new tests green,
  `go build ./...`, and the training-config validation boot check passes
  locally (same command as deploy-training-k3s1.yml:77-92).

### Verification (end-to-end, after Phase 6)

1. `go test -race ./...` — full suite.
2. Local boot with config.yaml (V1, no agent block): confirm /api/apps,
   seed/promote flows byte-identical; agent endpoints inert.
3. Local boot with an agent-enabled dev config (memory store, deployer log,
   scripted Ollama unavailable): watch a seed → events appear in
   GET /api/events; force unhealthy → agent records HOLD decision
   (recommend_only: recorded not executed); flip mode active (autonomy L3):
   hold materialises and promote 409s; DELETE hold clears it; GET /api/audit
   shows the full correlated trail.
4. Training instance after deploy: labs 01–10 curl commands unchanged;
   seed workflows re-run green.

### Roadmap after slice 1

- **Slice 2 — V2 UI**: real static-exported Next routes per page (web.go
  <path>.html fallback serves them; no SPA catch-all needed); rewrite
  app-shell/sidebar into left-nav operational layout under the THEME LOCK
  (restructure tokens, never re-hue); generalise history-panel into the
  DataTable primitive; Audit page (GET /api/audit), Agent page + activity
  stream (agent API + SSE), Approvals page over existing signoff/window data
  layer, risk badges from RingView.risk, kill-switch/autonomy controls;
  docs/ui-modernization.md §4–8 as wireframe inventory.
- **Slice 3 — adapters & progressive delivery**: Anthropic llm.Provider;
  k8s-health capability interface on the deployer seam (mapStatus nucleus,
  degrades when rbac disabled) feeding health_changed events + risk signals;
  CI ingest (GitHub webhooks → change intelligence, commit text untrusted);
  Argo/progressive delivery as new executor.Executor impls via
  deployer.FromExecutor; group-promote primitive; X-Actor → named API tokens.

### Top 5 risks

1. Gate-extraction drift → existing tests are the oracle, zero test edits in
   PR 2; Err* vars/messages verbatim.
2. Duplicate agent execution across overlapping pods → agent:leader advisory
   lock + persisted cursor + re-OBSERVE + per-app op lock (dupe → no-op/409).
3. Prompt injection / LLM abuse → allowlist enum + DecodeStrict +
   deterministic Authorize final authority + untrusted-data delimiters +
   fail-safe HOLD + never-prod + default recommend_only.
4. Audit/event volume on training (17 apps) → emission operation-scoped only
   (read polls never write); keyset paging; prune later via KeepFailureLogs
   precedent.
5. SSE fragility behind wslproxy edge/Recreate deploys → SSE strictly
   additive with heartbeat + after_id replay; polling stays primary contract.
