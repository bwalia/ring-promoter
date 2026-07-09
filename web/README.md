# Ring Promoter — web UI

The frontend of Ring Promoter: a Next.js (App Router) + TypeScript single-page
app. It is exported to static files and embedded into the Go binary
(`internal/web/static`), so in production the UI and the REST API share one
origin and one process.

## Stack

| Concern       | Choice                                             |
|---------------|----------------------------------------------------|
| Framework     | Next.js (App Router), static export                |
| Language      | TypeScript                                         |
| Styling       | Tailwind CSS v4 + shadcn/ui (Radix primitives)     |
| Server state  | TanStack Query (polling; 1s for live jobs)         |
| Client state  | Zustand (persisted to localStorage)                |
| Theme         | next-themes (light / dark / system)                |

## Development

```bash
# terminal 1 — the Go API (in the repo root)
go run ./cmd/ringpromoter --config config.yaml

# terminal 2 — the UI with hot reload
npm install
npm run dev            # http://localhost:3000, proxies /api to :8080
```

Token for local dev: `local-dev-token`. Point the proxy elsewhere with
`RP_BACKEND=http://host:port npm run dev`.

## Shipping UI changes

```bash
npm run build:embed
```

This builds the static export and replaces `internal/web/static/`. Commit the
result — the Go binary embeds whatever is in that directory at build time.

## How it works

- **Live updates, no refreshes.** TanStack Query polls: ring state every 10s,
  history every 30s, and a running seed/promote/rollback job every second.
  Seed/promote/rollback POST with `?async=1`; the returned `job_id` is polled
  and rendered as a step-by-step live progress panel (statuses, logs,
  durations). On completion, rings + history are refetched immediately and a
  toast reports the outcome. The pause button in the top bar stops polling.
- **Auth.** The API token is entered once (validated against `GET /api/apps`),
  kept in localStorage and attached as a `Bearer` header. Any 401 signs out
  back to the token gate.
- **Version picker.** For apps whose deployer can enumerate versions (the
  `github` deployer: repo branches + tags via `GET .../versions`), the Seed
  dialog is a searchable picker — only versions that exist in the source
  repository can be chosen; a pasted commit SHA is verified by the server on
  submit. Other apps keep free-form input.
- **Auto-promote switches.** Middle rings carry an `auto → <next>` switch
  (server-side setting): a healthy landing there continues onward in the same
  job. The switch updates optimistically and the whole chain renders in the
  live progress panel.
- **Production password.** When the server has `RP_PROD_PASSWORD` set
  (`prod_protected` in `GET /api/apps`), the promote/seed dialogs targeting
  the last ring add a password field, and enabling `auto → prod` opens a
  dedicated confirmation. A wrong password shows inline and keeps the dialog
  open. Enforcement is server-side; the UI only collects the password.
- **Persisted preferences** (localStorage): token, theme, selected app,
  favorites, recents, collapsed sidebar sections, auto-refresh on/off, and the
  running-job reference per app (so a mid-deploy refresh resumes the live
  view). **Application groups are NOT local** — they live on the server
  (`/api/groups`, Postgres in production) and are shared by every user;
  groups created by older builds are migrated automatically on first load.
- **URL state.** The selected app is mirrored to `?app=<name>` so views are
  shareable/bookmarkable.
- **Keyboard.** `⌘K`/`Ctrl-K` command palette (apps, versions, actions,
  preferences), `/` filters apps, `r` refreshes, `?` shows all shortcuts.

## Layout

```
src/
  app/                 layout (fonts, providers, metadata) + the single page
  components/
    app-shell.tsx      token gate → sidebar + topbar + dashboard; shortcuts
    sidebar.tsx        search, favorites, recents, custom groups, all apps
    command-palette.tsx  ⌘K: apps, deployed-version search, quick actions
    dashboard/
      dashboard.tsx    composition of the panels below
      overview-cards.tsx  KPI row: prod version, ring health, last activity
      pipeline.tsx     ring cards: versions, live health, drift, actions
      job-progress.tsx live CI-style step/log view of the running operation
      history-panel.tsx  filterable per-app deployment history
      action-dialogs.tsx seed dialog + promote/rollback confirmations
      activity-feed.tsx  recent operations across all apps
  lib/
    api.ts             typed fetch client for the REST API
    types.ts           API types mirroring the Go JSON
    queries.ts         TanStack Query hooks (polling, job tracking, mutations)
    stores.ts          Zustand stores (auth, prefs, active jobs)
    ui-store.ts        ephemeral UI state (palette, dialogs, pending action)
```
