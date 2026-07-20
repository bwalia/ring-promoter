# Workshops

Instructor-led formats built from the [labs](../labs) and [sample-apps](../sample-apps).
Each has timings, expected outputs, and speaker notes (add slides under a
`slides/` folder per workshop).

## Half-day (3h) — "Promotion basics"
| Time | Segment | Content |
|------|---------|---------|
| 0:00 | Intro | Rings, seed/promote/rollback, version-aware health (slides) |
| 0:30 | Lab 01–03 | Build → seed → promote int→test→acc (hello-world, kuard) |
| 1:30 | Break | |
| 1:45 | Lab 04–05 | Production approval + rollback |
| 2:30 | Lab 06 | Fix a failed deployment (version mismatch) |
| 2:50 | Wrap | Q&A, where to go next |

## Full-day (6h) — "Progressive delivery"
Half-day, plus: multi-service group promotion (Lab 07, shopping-cart), canary &
blue/green (Lab 09), the three deployers (kubectl/github/k8sjob), and
observability (`/metrics`, dashboards).

## 2-day — "Release engineering with Ring Promoter"
Full-day, plus: promotion policies in depth (Lab 08, bank-api — windows +
sign-off + CR/JIRA), production incident simulation (Lab 10), AI failure
diagnosis, and deploying your own instance ([runbook](../../deploy/instances/fictionally/README.md)).

## Enterprise (bespoke)
RBAC, DR/HA, scaling, multi-team governance, and integrating your own apps.
See [enterprise](../enterprise).

## Facilitator checklist
- A cluster per attendee (or shared with per-attendee namespaces).
- A Ring Promoter instance reachable by all (share `RP_URL` + tokens).
- Pre-pull the sample images; pre-create the `<app>-int` namespaces.
