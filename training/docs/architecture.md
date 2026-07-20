# Ring Promoter — architecture (training reference)

Ring Promoter is a small Go control plane with an embedded web UI. It promotes
an application's version through ordered **rings** (`int → test → acc → prod`),
one ring at a time, health-checking each hop and rolling back on failure.

## Components

```mermaid
flowchart TB
  subgraph CP["Ring Promoter control plane"]
    API["REST API + Web UI"]
    PROM["Promoter (rules engine)"]
    STORE["Store (Postgres / memory)"]
    DEP["Deployers: kubectl · github · k8sjob"]
    HC["Health checker (version-aware)"]
    GATES["Gates: maintenance window · sign-off · change-request"]
    AI["AI diagnosis (Ollama)"]
  end
  API --> PROM
  PROM --> STORE
  PROM --> GATES
  PROM --> DEP
  PROM --> HC
  DEP -->|kubectl set image| K["k3s1 Deployments"]
  DEP -->|workflow_dispatch| GH["GitHub Actions"]
  DEP -->|batch/v1 Job| JOB["ring-exec Jobs"]
  HC -->|GET health_url| K
  API -.failed job.-> AI
```

## Promotion decision flow

```mermaid
flowchart TD
  S["Promote app from ring N"] --> G{"Gates satisfied?<br/>(window · sign-off · CR code)"}
  G -- no --> X["Reject 4xx — nothing deployed"]
  G -- yes --> SH{"Source ring healthy?"}
  SH -- no --> X2["Abort — source unhealthy"]
  SH -- yes --> D["Deploy version to ring N+1"]
  D --> H{"Healthy? (version-aware, retries)"}
  H -- yes --> OK["Record success · maybe auto-promote onward"]
  H -- no --> RB["Auto-rollback to previous · record failure"]
```

## Key ideas the academy teaches

- **One ring at a time, never skip** — order is the single source of truth.
- **Version-aware health** — a ring is healthy only when its endpoint reports
  the *deployed* version (`health_version_field` / `health_version_header`); a
  stale replica answering `200 OK` fails.
- **Gates run before any deploy** — a closed gate leaves all state untouched.
- **Pluggable deployers** — the same promotion engine drives Kubernetes,
  GitHub Actions, and Kubernetes Jobs.

See the [sample-apps feature matrix](../sample-apps/README.md#feature-coverage-matrix)
for which app demonstrates each capability.
