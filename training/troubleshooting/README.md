# Troubleshooting

Common failures in the academy and how to read them. Ring Promoter's job view
shows per-step logs; a failed job can also be explained with **AI diagnosis**
(if Ollama is enabled).

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Promote returns **`health check failed after retries`**, ring rolled back | The endpoint isn't serving the deployed version (stale replica, wrong image tag, slow rollout) | Check the app pods; confirm `/healthz` reports the deployed version; increase `retry.count`/`delay` |
| Promote returns **409 `maintenance window closed`** | Target ring's maintenance-window gate has no open window | Open an ad-hoc window (UI/API) or wait for the recurring one — Lab 08 |
| Promote returns **409 `sign-off required`** | No GO sign-off for that exact version | Record a release-engineer GO sign-off — Lab 08 |
| Promote returns **400 `change-request code invalid`** | CR code missing/rejected (JIRA) | Supply a valid CR code; `test` always works in demos |
| Promote returns **403 `production password required`** | Deploying to prod without the password | Provide the prod password (UI prompts) |
| **`Host not configured`** (HTTP 200) at the public URL | wslproxy vhost not registered for the host | Register the vhost on pop0 — see the [instance runbook](../../deploy/instances/fictionally/wslproxy-vhost.md) |
| Public URL doesn't resolve | Cloudflare CNAME missing | Confirm external-dns published `→ pop0.wslproxy.com`; check the Ingress annotations |
| Control-plane pod `CreateContainerConfigError` | `runAsNonRoot` with a non-numeric user | Pin the distroless nonroot UID/GID (65532) — already set in the manifests |
| kubectl deployer: **`deployments.apps is forbidden`** | Control-plane RBAC missing for the target namespace | Apply [`rbac.yaml`](../../deploy/instances/fictionally/rbac.yaml); bind per-namespace in prod |
| k8sjob deployer: Job never appears | `ring-exec` namespace or executor RBAC missing | Apply the namespace + executor RBAC |

## Reading a failed job
1. Open the app, click the failed job → expand steps (`deploy`, `health`, `rollback`).
2. The failing step's logs show the exact error (e.g. `endpoint reports "v1", want "v2"`).
3. Click **Diagnose with AI** for a plain-language explanation (if enabled).
