# Labs

Hands-on, do-it-yourself exercises. Each lab has a goal, prerequisites, steps
(UI **and** `curl`), an expected result, and a "what you learned" recap.

Set these once (see [getting-started](../getting-started)):

```bash
export RP_URL=https://ring-promoter.fictionally.org
export RP_TOKEN=<your-token>
```

| # | Lab | Ring Promoter feature |
|---|-----|-----------------------|
| 01 | [First deployment](./lab-01-first-deployment.md) | build → seed → healthy (version-aware) |
| 02 | [Promote dev → test](./lab-02-promote-dev-to-test.md) | `promote`, one-ring-at-a-time |
| 03 | [Promote test → staging](./lab-03-promote-to-acc.md) | health check + retries |
| 04 | [Production approval](./lab-04-production-approval.md) | production password |
| 05 | [Rollback](./lab-05-rollback.md) | `rollback`, previous version |
| 06 | [Fix a failed deployment](./lab-06-fix-failed-deployment.md) | version-mismatch → auto-rollback → fix |
| 07 | [Application groups](./lab-07-groups.md) | group promote across apps |
| 08 | [Promotion policies](./lab-08-promotion-policies.md) | maintenance window + sign-off + CR code |
| 09 | [Canary & blue/green](./lab-09-canary-blue-green.md) | progressive delivery patterns |
| 10 | [Production incident](./lab-10-production-incident.md) | end-to-end incident response |

All ten labs are written in full; each builds on the last. The
[sample-apps feature matrix](../sample-apps/README.md#feature-coverage-matrix)
maps every feature to the lab and app that exercises it.
