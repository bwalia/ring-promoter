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
| 02 | Promote dev → test | `promote`, one-ring-at-a-time |
| 03 | Promote test → staging | health check + retries |
| 04 | Production approval | production password |
| 05 | Rollback | `rollback`, previous version |
| 06 | Fix a failed deployment | version-mismatch → auto-rollback → fix |
| 07 | Application groups | group promote across apps |
| 08 | Promotion policies | maintenance window + sign-off + CR code |
| 09 | Canary & blue/green | progressive delivery patterns |
| 10 | Production incident | end-to-end incident response |

**Lab 01 is written in full** as the worked example that establishes the format;
Labs 02–10 follow the same shape and build on it. The
[sample-apps feature matrix](../sample-apps/README.md#feature-coverage-matrix)
maps every feature to the lab and app that exercises it.
