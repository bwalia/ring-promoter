# Screenshots

Taken by the UI tests against demo mode, in both appearances, and extracted from
the result bundle. Regenerate with `../../scripts/capture-screenshots.sh`.

Because the tests take them, they cannot drift: a screen that changes changes
its screenshot, and a screen that breaks fails its test instead of quietly
shipping a stale picture.

| File | Screen | What it is there to show |
|---|---|---|
| `00-onboarding` | First run | The two ways in: connect, or explore with no server |
| `01-overview` | Overview | Trouble pinned above healthy apps; icon **and** colour on every chip |
| `02-app-detail` | App detail | Per-ring versions, drift, gate badges, auto-promote, and only the legal actions |
| `03-promote-sheet` | Promote | What a gated ring demands before it will submit |
| `04-production-guard` | Promote → production | Password *and* typed confirmation, both required, submit disabled |
| `05-job-running` | Live job | Steps appearing, log console filling |
| `06-job-succeeded` | Live job | An explicit terminal state with per-step durations |
| `07-rollback-sheet` | Roll back | No password, no gate, no typed confirmation — incident response is never blocked |
| `08-activity` | Activity | The cross-app feed, shared with everyone on the control plane |
| `09-settings` | Settings | Instances, security, refresh, and what the server says it is |
| `10-history` | History | Grouped by day, failures expandable to a diagnosis |
| `11-maintenance` | Maintenance | Guarded rings, whether a window is open, scheduled vs ad-hoc |

`light/` holds the committed set. Run the capture script to regenerate it, or to
produce the matching `dark/` set — the script does both appearances.
