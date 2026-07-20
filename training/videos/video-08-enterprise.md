# Video 08 — Enterprise: groups, RBAC, HA/DR

**Length:** ~8 min · **Maps to:** [Lab 07](../labs/lab-07-groups.md),
[deploy/instances/fictionally](../../deploy/instances/fictionally/README.md)

## 0:00 — Hook
> "You've shipped, gated, and rolled back. This one is about running Ring Promoter
> *for a whole org* — organising apps into groups, scoping what the control plane
> can touch, keeping it available, and standing up your own instance."

Show: the dashboard with all seven training apps; a slide titled "Ring Promoter in
production."

## 0:30 — Groups: a team's apps on one screen
Narration: "A group is a named collection of apps, stored server-side and shared by
everyone on the control plane. Groups don't change promotion mechanics — they
organise the dashboard."

On screen (UI): sidebar → **New group** → name `Storefront`, add `shopping-cart`.

On screen (curl):
```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/groups" \
  -d '{"name":"Storefront","apps":["shopping-cart","hello-world"]}'
# {"id":"g-...","name":"Storefront","apps":["shopping-cart","hello-world"], ...}
```
Narration: "Members must be configured apps; duplicates are de-duplicated. Open the
group page and every member's rings sit on one screen — handy for a multi-service
release."

## 1:45 — List, update, delete a group
On screen:
```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/groups" | jq '.groups'

GID=$(curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/groups" \
  | jq -r '.groups[] | select(.name=="Storefront") | .id')
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X PUT "$RP_URL/api/groups/$GID" -d '{"name":"Storefront","apps":["shopping-cart"]}'
# curl ... -X DELETE "$RP_URL/api/groups/$GID"   # to remove
```
Narration: "Groups are a view concern — shared, server-side, and per-app promotion
rules are unchanged."

## 2:45 — RBAC: what the control plane may touch
Narration: "The training instance really deploys, so its ServiceAccount gets scoped
Kubernetes rights — and they mirror the deployers from video 7. A cluster-wide
role to patch Deployments for the kubectl apps; a namespace-scoped role in
`ring-exec` for the k8sjob Jobs."

On screen (the deployer role):
```yaml
kind: ClusterRole
metadata: { name: ring-promoter-training-deployer }
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get","list","watch","patch","update"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get","list","watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get","list","watch"]
```
Narration: "For a dedicated training cluster the deployer role is cluster-wide for
convenience. In real production, scope it per namespace with RoleBindings — least
privilege. The k8sjob executor role is already namespace-scoped to `ring-exec`, and
the deploy Jobs run as their own `ring-deploy-job` service account, not the control
plane's."

## 4:00 — Secrets: the sensitive config
Narration: "None of the sensitive values live in the config file. They're an
instance Secret, created by hand or via a sealed-secret / external secret manager
— the API token, the DB DSN, the GitHub token for the github deployer, the JIRA
token for the change-request gate, and the prod password."

On screen:
```bash
kubectl -n fictionally-ring-promoter create secret generic ring-promoter \
  --from-literal=RP_API_TOKEN="$(openssl rand -hex 32)" \
  --from-literal=RP_DB_DSN="postgres://ringpromoter_training:REAL@ring-promoter-db.ring-system.svc.cluster.local:5432/ringpromoter_training?sslmode=require" \
  --from-literal=RP_GITHUB_TOKEN="<pat actions:write on rp-training-operator>" \
  --from-literal=RP_JIRA_TOKEN="<jira api token>" \
  --from-literal=RP_PROD_PASSWORD="<prod password>"
```

## 5:15 — HA / DR: the state is in Postgres
Narration: "Ring Promoter's control plane is stateless — every bit of state (ring
versions, history, groups, sign-offs, windows) lives in Postgres. That's what makes
it available and recoverable: run more than one control-plane replica behind the
Service for HA, and your DR story is just your Postgres backup-and-restore. Lose the
pod and nothing is lost; a fresh pod reads the same database. The schema is applied
automatically on start-up."

Show: a slide — stateless control plane (N replicas) + one durable Postgres = HA and
DR.

## 6:00 — Stand up your own instance
Narration: "The whole thing is a handful of manifests in
`deploy/instances/fictionally`. Namespaces and RBAC, the Secret, a ConfigMap built
straight from the training config, then the control plane."

On screen:
```bash
kubectl apply -f deploy/instances/fictionally/namespace.yaml
kubectl apply -f deploy/instances/fictionally/rbac.yaml
# create the Secret (above) — never apply secret.yaml as-is
kubectl -n fictionally-ring-promoter create configmap ring-promoter-config \
  --from-file=config.yaml=training/config/apps.training.yaml \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f deploy/instances/fictionally/service.yaml
kubectl apply -f deploy/instances/fictionally/deployment.yaml
kubectl apply -f deploy/instances/fictionally/ingress.yaml
kubectl -n fictionally-ring-promoter rollout status deploy/ring-promoter
```
Narration: "DNS is external-dns publishing a Cloudflare CNAME to the wslproxy POP;
the edge vhost is registered on pop0. Both are live prod steps you run with cluster
credentials."

## 7:00 — Verify the instance
On screen:
```bash
curl -sS https://ring-promoter.fictionally.org/healthz          # {"status":"ok"}
TOKEN=$(kubectl -n fictionally-ring-promoter get secret ring-promoter \
  -o jsonpath='{.data.RP_API_TOKEN}' | base64 -d)
curl -s -H "Authorization: Bearer $TOKEN" \
  https://ring-promoter.fictionally.org/api/apps | jq '.apps'
# all 7 training apps
```

## 7:30 — Why it matters
Show: a slide — groups (organise), RBAC + Secrets (scope + protect), stateless +
Postgres (HA/DR), a few manifests (own instance). "A training instance and a
production instance are the same manifests with tighter RBAC and real secrets.
Nothing new to learn to run it for real."

## 8:00 — Outro
> "That's the series — first deploy to full enterprise. Go run the labs, stand up
> your own instance, and ship with confidence." Subscribe.

---
**YouTube description:**
Run Ring Promoter for a whole org. Organise apps into shared server-side groups,
scope the control plane with deployer-matched RBAC, keep sensitive values in an
instance Secret, and get HA/DR for free from a stateless control plane over
Postgres — then stand up your own instance from a handful of manifests. Part 8, the
finale of the Ring Promoter training series. Repo: github.com/bwalia/ring-promoter
(training/).
Chapters: 0:00 Intro · 0:30 Groups · 1:45 Manage groups · 2:45 RBAC · 4:00 Secrets ·
5:15 HA/DR · 6:00 Own instance · 7:00 Verify · 7:30 Why · 8:00 Outro.
