# Ring Promoter — UI/UX Modernization Design

Status: **PROPOSED** (design sign-off required before implementation)
Reference implementation: [bwalia/kubepilot](https://github.com/bwalia/kubepilot) `dashboard/`
Scope: transform the embedded vanilla-JS UI into a modern, production-grade,
developer-first control-plane dashboard — *without* breaking the single-binary
Go deployment model.

---

## 0. Executive summary

| Decision | Choice | Why |
|---|---|---|
| Framework | **Vite + React 18 + TypeScript (strict)** | SPA with client routing; no SSR need; simplest fit for `go:embed`. kubepilot ships Next.js *static-export* served by Go — same net result, but Vite removes the Next runtime/export quirks for dynamic routes. |
| Styling | **Tailwind CSS + local shadcn-style primitives on Radix** | Exactly kubepilot's approach (cva + `cn()` + Radix dialog/tabs/select/tooltip). Full shadcn/ui optional later. |
| Server state | **TanStack Query** | kubepilot's only state manager; fits poll+push hybrid. |
| Client state | **Zustand (1 small store)** | Token/session, selected app, active job id. kubepilot uses ad-hoc `useState`; a tiny store is cleaner for auth. |
| Real-time | **SSE (Phase 2 backend addition), polling fallback** | kubepilot's biggest self-admitted weakness is poll-only "live" logs. Ring Promoter watches deploys in real time — SSE via stdlib `net/http` is ~50 lines of Go, no new deps. |
| Charts | **Recharts** | Matches kubepilot; used sparingly (history/metrics), custom SVG for the pipeline. |
| Icons / fonts | **lucide-react; Inter + JetBrains Mono (self-hosted)** | kubepilot design language. Self-host fonts — the UI must work inside air-gapped clusters (no Google Fonts CDN). |
| Theme | **Dark cockpit first, CSS-variable tokens** | Copy kubepilot's `pilot-*` palette but as CSS variables so light mode is possible later (kubepilot hard-codes dark — flagged avoid). |
| Deployment | **Unchanged: single Go binary, SPA embedded via `go:embed`** | Node appears only as a Docker *build* stage. |

The Go backend already exposes everything Phase 1 needs: apps, ring views with
live health/versions, full history, and an async job model with per-step status,
logs and durations. No backend change is required to ship the new UI; backend
work starts in Phase 2 (SSE, jobs list, pagination) and Phase 3 (Prometheus).

---

## 1. Current state (inventory)

**Frontend** — `internal/web/static/`: 3 hand-written files (66-line
`index.html`, 690-line `style.css`, 399-line vanilla `app.js`). One screen:
token gate → app picker → 3 stat tiles → horizontal ring pipeline with inline
seed/promote/rollback → last-30 history list → build-info footer. Async job
progress panel polls `GET /api/apps/{app}/jobs/{id}` at 700 ms and survives
page reload via localStorage. No router, no build step, no framework.

**API** — stdlib `net/http` (Go 1.22 method+path routing), same-origin, bearer
token (single shared secret, constant-time compare):

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz`, `/version` | unauthenticated |
| GET | `/api/apps` | apps + ring pipeline |
| GET | `/api/apps/{app}/rings` | `RingView[]`: configured, current/previous/live version, healthy, live_healthy, live_health_error, can_promote_from |
| GET | `/api/apps/{app}/history` | newest-first, unpaginated |
| GET | `/api/apps/{app}/jobs/{id}` | `jobState`: status, steps[{id,title,status,logs[],started_at,finished_at}], result, error |
| POST | `/api/apps/{app}/seed\|promote\|rollback` | sync `Result`, or `?async=1` → `202 {job_id}` |

**Confirmed absent today** (relevant to the spec): SSE/WebSockets, Prometheus
`/metrics`, policy engine, Telegram/ChatOps, topology model, jobs-list
endpoint, history pagination, multi-user identity/RBAC.

**Embedding constraints**: `go:embed` requires built assets under
`internal/web/` at Go build time; `http.FileServer` has **no SPA fallback**
(deep links 404 → custom handler needed); Dockerfile has no Node stage.

---

## 2. Gap analysis vs kubepilot reference UX

### Adopt (kubepilot strengths)
| Pattern | kubepilot source | Ring Promoter use |
|---|---|---|
| `pilot-*` design-token palette, dark cockpit, low-opacity tints (`bg-color/15`) | `tailwind.config.js` | Same palette family, as CSS variables |
| Inter UI + JetBrains Mono for identifiers/versions/logs, `tabular-nums` | `globals.css` | Versions, ring names, log lines |
| Generic `ResourceTable<T>` with typed columns + built-in loading/error/empty states | `components/dashboard/ResourceTable.tsx` | History table, apps table, runs table |
| Right-side tabbed slide-over drawer for drill-down | `DrawerContent` + Radix Tabs (`PodDetailDrawer`) | Ring detail, run detail, history-entry detail |
| cva `Badge`/`Button` variants + `cn()` helper | `components/ui/*` | Status pills (healthy/deploying/failed/skipped) |
| Status→color→icon record maps; live count badges on tabs | `VERDICT_META` pattern | Job step icons, ring health |
| KPI stat cards with alert flip | `KPICard` | Dashboard overview |
| Decision-ledger UX (append-only, verdict badge, drill-in) | `AutopilotPanel` | Release history / promotion audit |
| Capability flag from backend gating write controls | `/config → mutations_enabled` | Read-only mode / future RBAC gate |
| Skeleton pulse loaders everywhere | `ResourceTable` | All queries |

### Fix (kubepilot weaknesses we will NOT copy)
| Weakness in reference | Our correction |
|---|---|
| No auth at all (namespace-lock is cosmetic) | Keep bearer-token auth; proper login screen; 401 → session store reset. Multi-user/RBAC = Phase 4 backend item |
| Poll-only "live" logs (5 s lag) | SSE for job streams (Phase 2); TanStack Query polling as automatic fallback |
| State-based tabs → views not deep-linkable | Real routes for everything; tab state synced to URL |
| Dark theme hard-coded in body CSS | CSS-variable tokens from day one |
| PascalCase/snake_case leak from Go into TS | RP API is consistently snake_case; keep one wire convention, typed client normalizes nothing |
| 681-line hand-maintained SVG topology | Pipeline is a fixed linear 4-ring flow — simple flex/SVG connectors; adopt React Flow only if/when a true DAG exists (Phase 4) |
| Google-Fonts `@import` | Self-hosted fonts (air-gap safe) |

---

## 3. UI architecture

```
web/                          # NEW: frontend workspace at repo root
├── package.json  vite.config.ts  tsconfig.json  tailwind.config.ts
├── index.html
└── src/
    ├── main.tsx              # QueryClientProvider, Router, theme
    ├── app/
    │   ├── routes.tsx        # route table (see §4)
    │   └── layout/           # GlobalNav, AppShell, page headers
    ├── lib/
    │   ├── api.ts            # typed fetch client (bearer injection, 401 handling)
    │   ├── types.ts          # TS mirrors of Go JSON shapes (snake_case)
    │   ├── sse.ts            # EventSource wrapper w/ poll fallback (Phase 2)
    │   └── utils.ts          # cn()
    ├── stores/
    │   └── session.ts        # Zustand: token, selectedApp, activeJobId (persist)
    ├── components/
    │   ├── ui/               # badge, button, dialog+drawer, tabs, tooltip,
    │   │                     # select, card, skeleton, separator  (Radix + cva)
    │   ├── DataTable.tsx     # generic typed table (kubepilot ResourceTable)
    │   ├── StatusBadge.tsx   # status→color→icon single source of truth
    │   ├── LogViewer.tsx     # mono, line numbers, severity regex highlight
    │   ├── VersionChip.tsx   # mono version w/ copy
    │   └── RelativeTime.tsx
    └── features/
        ├── auth/             # TokenGate (login), session guard
        ├── dashboard/        # KPI cards, fleet ring matrix, active jobs rail
        ├── pipeline/         # RingPipeline, RingStageCard, ActionBar,
        │   └──               # RingDetailDrawer (tabs: state/history/health/config)
        ├── jobs/             # JobProgressPanel, StepRow, JobDrawer, JobsList*
        ├── releases/         # HistoryTable, ReleaseTimeline, EntryDrawer
        └── metrics/          # Phase 3: charts, SLO tiles
```

Go-side changes (Phase 1, minimal):
- `internal/web/`: embed `dist/` instead of `static/`; new `spaHandler` —
  serve file if it exists, else `index.html` (SPA fallback), correct
  cache headers (`immutable` for hashed assets, `no-cache` for index.html).
- Dockerfile: add `node:22-alpine` stage building `web/` → copy `dist/` into
  the Go build stage before `go build` (embed happens at compile time).
- `GET /api/config` (tiny): `{mutations_enabled: true, rings:[...]}` —
  capability flag copied from kubepilot's best pattern.

---

## 4. Routing structure

All views deep-linkable (fixes kubepilot's state-tab weakness):

```
/login                                  token gate (redirects back)
/                                       Dashboard overview (fleet)
/apps/:app                              Pipeline view (default app surface)
/apps/:app/rings/:ring                  Ring detail drawer route (drawer over pipeline)
/apps/:app/releases                     Release explorer (full history, filters)
/apps/:app/releases/:id                 History entry drawer route
/apps/:app/runs                         Runs list (Phase 2 – needs jobs-list API)
/apps/:app/runs/:jobId                  Run detail: steps + live logs
/metrics                                Phase 3 (Prometheus-driven)
/settings                               token mgmt, theme, build info (/version)
```

Drawer-over-page routes render the parent page with the drawer open —
back button closes the drawer (kubepilot drawer UX, but URL-addressable).

## 5. State management strategy

- **Server state = TanStack Query, exclusively.**
  - `['apps']` staleTime 60 s; `['rings', app]` refetchInterval 15 s
    (matches kubepilot global default; the endpoint does live health checks
    server-side with 8 s timeout — do not poll faster);
  - `['history', app]` 30 s; `['job', app, id]` 1 s while
    `status ∈ {pending,running}` else `false` (preserves today's ~700 ms feel,
    switched off by SSE in Phase 2);
  - mutations (seed/promote/rollback) → always `?async=1` → optimistic
    "deploying" state on the target ring card → invalidate `rings`+`history`
    on job completion.
- **Client state = one Zustand store, persisted**: `{token, selectedApp,
  activeJobId}` — direct migration of today's `rp_token`/`rp_app`/`rp_job`
  localStorage keys (same keys, so sessions survive the UI swap).
- 401 from any query → clear session → redirect `/login` (today's behavior).
- One in-flight job per app enforced client-side (matches current UI) until a
  backend queue exists.

## 6. Real-time data flow

**Phase 1 (no backend change)** — polling profile above; the job panel's 1 s
poll of `jobState` (steps + logs accumulate server-side) already gives a
near-live feel; job survives page reload via persisted `activeJobId`.

**Phase 2 (SSE, stdlib-only Go)**:
```
GET /api/apps/{app}/jobs/{id}/events     text/event-stream
  event: step     data: stepView          (step started/finished)
  event: log      data: {step_id, line}
  event: done     data: jobState          (terminal snapshot)
GET /api/events                          fleet stream (ring-state changes,
                                          job started/finished) → dashboard
```
Client `lib/sse.ts`: EventSource feeding the TanStack Query cache
(`setQueryData`), automatic downgrade to polling on error/proxy issues.
Also Phase 2: `GET /api/apps/{app}/jobs` (list, newest-first) and
`GET /api/apps/{app}/history?limit&before_id` (pagination).

**Phase 3 (metrics)**: Prometheus registry + `/metrics` in Go (promotion
counts/durations/failures, health-check latency, deployer API latency);
UI SLO tiles + Recharts panels; per-metric "open in Grafana" links
(configurable base URL via `/api/config`).

## 7. Core pages & wireframes (textual)

**Dashboard `/`** — global nav (brand | Dashboard · Apps · Releases | build
version · sign-out). KPI row: Apps, Deployed rings, Healthy/Unhealthy (alert
flip), Active jobs. Fleet matrix: one row per app × columns int/test/acc/prod,
each cell = version chip + health dot, click → `/apps/:app`. Right rail:
active/recent jobs w/ live status. Below: latest fleet activity (10, from
per-app histories merged client-side Phase 1; `/api/events` Phase 2).

**Pipeline `/apps/:app`** — header: app name, deployer-type badge
(kubectl/github/log), Refresh. Horizontal 4-stage pipeline, animated
connectors; each stage card: ring label, version chip (mono), live vs stored
version delta hint, health pill w/ `live_health_error` tooltip, updated-at;
actions per card: Seed (version input), Promote → (when `can_promote_from`),
Rollback (when `previous_version`). Active job renders an inline progress
strip on the target stage + expandable step/log panel (today's revamp UX,
componentized). Click stage → `RingDetailDrawer` (tabs: State · History ·
Health · Config).

**Release explorer `/apps/:app/releases`** — filter bar (ring, action,
result, free-text). `DataTable`: result icon, action badge, ring,
`from → to` (mono chips), message (truncate+tooltip), relative time.
Row click → drawer: full message, linked GH run (parsed URL when present),
"Rollback to this version" quick action. Timeline toggle: per-ring swimlanes
of version transitions (Recharts).

**Run detail `/apps/:app/runs/:jobId`** — step list (icon, title, duration,
running spinner) + `LogViewer` with severity highlighting; header shows
outcome + rolled-back flag; live via poll (P1) / SSE (P2).

## 8. Migration plan

| Phase | Scope | Backend touch |
|---|---|---|
| **1. SPA replacement** | `web/` workspace; all §7 pages on the existing API; spaHandler + embed swap; Dockerfile node stage; delete old `static/` after parity check | Go: embed+fallback handler, `/api/config` only |
| **2. Real-time & lists** | SSE job/fleet streams, jobs-list, history pagination; UI switches job panel + dashboard to SSE | Go: 3 endpoints + reporter fan-out |
| **3. Metrics & SLOs** | `/metrics` Prometheus, `/metrics` UI page, Grafana deep links | Go: prometheus client lib (first new dep), instrumentation |
| **4. Deferred (needs new subsystems)** | Policy-engine UI (rules editor/simulation), Telegram ChatOps panel, topology view, multi-user RBAC | Major backend features — separate designs |

Parity gate for Phase 1 (must all pass before deleting the old UI): token
gate & 401 flow · app switching · seed/promote/rollback with async progress ·
job survival across reload · pod-restart 404 job recovery · history render ·
version footer. Rollback story: the old `static/` UI stays in-tree during
Phase 1 behind a build flag; an image tag with the old UI is always one
`kubectl set image` away.

**Explicitly out of scope for Phase 1** (spec items with no backend today):
Prometheus/SLOs, policy engine, Telegram ChatOps, Argo integration, topology,
WebSockets. They are phased, not silently dropped.

## 9. Risks

- **Image size/build time**: node stage adds ~1–2 min CI; final image
  unchanged (dist is static files in the Go binary).
- **SSE through ingress**: wslproxy/traefik buffering may delay events —
  poll fallback is mandatory, not optional.
- **Single shared token remains the auth model** until Phase 4; the new UI
  must not pretend otherwise (no fake user menus).
- **History endpoint unpaginated** — fine at current volume; Phase 2 fixes
  before it isn't.
