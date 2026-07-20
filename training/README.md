# Ring Promoter Training Academy

A self-contained academy for learning modern release promotion, GitOps, and
progressive delivery with **Ring Promoter**. Clone the repo, follow a learning
path, and go from your first deployment to running gated production promotions
in an afternoon.

> **What is Ring Promoter?** A control plane that promotes an application's
> version through ordered rings — **int → test → acc → prod** — one ring at a
> time, health-checking each hop and rolling back automatically when a ring
> stays unhealthy. It deploys via `kubectl`, a GitHub Actions workflow, or a
> Kubernetes Job, and can gate sensitive rings behind maintenance windows, a
> QA/release sign-off, and a valid change-request code.

## How this academy is organised

```
training/
  getting-started/   prerequisites, cluster access, first login
  beginner/          concepts: rings, seed, promote, rollback, health
  intermediate/      deployers, health strategies, groups, auto-promote
  advanced/          promotion gates, ref-pinning, canary, blue/green
  enterprise/        RBAC, DR/HA, scaling, multi-team governance
  labs/              10 hands-on labs (deploy → incident response)
  workshops/         instructor-led: half-day, full-day, 2-day, enterprise
  sample-apps/       7 realistic apps covering every RP feature
  config/            the Ring Promoter config that registers all 7 apps
  docs/              reference docs (architecture, gitops, canary, ...)
  diagrams/          Mermaid source for the architecture diagrams
  videos/            narrated demo scripts (screencast-ready)
  troubleshooting/   common failures and fixes
  scripts/           helper scripts for the labs
```

## Learning paths

| Path | Audience | Time | Start |
|------|----------|------|-------|
| **Beginner** | New to CD / GitOps | ~1h | [getting-started](./getting-started) → [Lab 01](./labs/lab-01-first-deployment.md) |
| **Intermediate** | App/platform engineers | ~2h | [intermediate](./intermediate) → Labs 02–07 |
| **Advanced** | Release engineers | ~2h | [advanced](./advanced) → Labs 08–10 |
| **Enterprise** | Platform teams | ~1 day | [workshops](./workshops) (2-day) |

## The apps (and what each teaches)

Seven apps that between them exercise **every** Ring Promoter capability — see
the [sample-apps catalog + feature matrix](./sample-apps). Start with
[`hello-world`](./sample-apps/hello-world) (the reference template) and
[`kuard`](./sample-apps/kuard) (the canonical lab app).

## The labs

1. [First deployment](./labs/lab-01-first-deployment.md) — build → seed → healthy
2. Promote dev → test
3. Promote test → staging (acc)
4. Production approval (prod password)
5. Rollback
6. Fix a failed deployment (version mismatch)
7. Application groups
8. Promotion policies (windows / sign-off / CR code)
9. Canary & blue/green
10. Production incident (end-to-end)

*(All ten labs are written in full — see the [labs README](./labs).)*

## Prerequisites

- A Kubernetes cluster (the academy targets **k3s1**; any cluster works).
- `kubectl`, `helm` (v3+), `go` (1.26+), `docker`.
- A running Ring Promoter instance — the training instance lives at
  **https://ring-promoter.fictionally.org** (deploy your own with the
  [instance runbook](../deploy/instances/fictionally/README.md)).

Start at [getting-started](./getting-started).
