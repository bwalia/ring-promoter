# Prompt: Make Ring Auto-Promote Declarative in Config

You are enhancing the existing Ring Promoter project.

Repository:
[https://github.com/bwalia/ring-promoter](https://github.com/bwalia/ring-promoter)

## Goal

Let an app's YAML config declare whether each ring auto-promotes, so the setting
is reviewable in git and reproducible from scratch — instead of existing only as
runtime state that whoever last called the API happened to leave behind.

---

# First — the current behaviour (verified, do not re-derive)

Auto-promote today is **runtime-only state**. There is no config field for it
anywhere; `grep -rn "auto_promote\|AutoPromote" internal/config/ config.yaml`
returns nothing.

* Storage: the `auto_promote` column on `ring_state`, declared
  `BOOLEAN NOT NULL DEFAULT FALSE` (`internal/store/schema.sql:12`), with an
  idempotent `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ... DEFAULT FALSE` for
  databases predating the column (`schema.sql:18`).
* The column is deliberately **not** touched by `UpsertRingState` — deploys
  never modify it (`internal/store/postgres.go:72`, `internal/store/memory.go:77`).
  It changes through exactly one path: `SetAutoPromote`
  (`internal/store/store.go:141`, `internal/promoter/promoter.go:416`).
* The only caller is the API: `PUT /api/apps/{app}/rings/{ring}/auto-promote`
  with body `{"enabled": bool}` (`internal/api/api.go:121`, `:351`).
* It is read when deciding whether to continue onward after a healthy deploy
  (`internal/promoter/promoter.go:444`).

Consequence, and the reason for this work: a rebuilt database or a recreated
ring silently comes back with auto-promote **off**, and an operator toggling it
on leaves no reviewable trace. Both directions of drift are invisible.

Read these call sites before designing. Do not change the meaning of the
existing column or the API contract without saying so explicitly.

---

# The design decision you must make first

Two coherent models. **Pick one, state it in the PR description, and implement it
consistently.** Do not build a hybrid that leaves it ambiguous which side wins.

### Option A — config is authoritative (recommended)

`auto_promote` in config declares desired state and is reconciled onto the store
at startup and on every config reload. For a ring whose config sets the field,
the API endpoint stops being a way to drift: it returns `409 Conflict` naming
the config as the owner.

Rings that omit the field keep today's behaviour exactly — runtime-only, default
false, API-toggleable. That keeps the change backward compatible: an existing
deployment that adds nothing to its config behaves identically.

Recommended because it makes the git repo the answer to "why is this ring
auto-promoting?", which is the whole point of the request.

### Option B — config seeds the initial value only

Config sets the value when the `ring_state` row is first created; the API
remains free to change it afterwards, and config never re-asserts.

Weaker: after the first deploy, config and reality can disagree indefinitely and
nothing surfaces it. If you choose this, you must at minimum log a warning at
startup whenever stored state diverges from config.

---

# Requirements

## Config schema

Add an optional per-ring field, in `internal/config`:

```yaml
apps:
  - name: diytaxreturn
    rings:
      int:  { target_env: int,  health_url: "..." }
      test: { target_env: test, health_url: "...", auto_promote: false }
      acc:  { target_env: acc,  health_url: "..." }
```

* It must be a **pointer or equivalent** (`*bool`), so "absent" is distinct from
  "explicitly false". The whole compatibility story depends on that distinction —
  a plain `bool` would silently claim ownership of every ring in every existing
  config and disable auto-promote wherever it was on.
* Validate it like the other ring fields, with an error message that names the
  app and ring.

## Reconciliation

* Apply config-declared values on startup, before the scheduler or any promotion
  loop can act on stale state.
* Reconcile per (app, ring); never touch a ring whose config omits the field.
* Make it idempotent — a restart with unchanged config must produce no writes and
  no log noise.
* Log at INFO when a value actually changes, with app, ring, old and new — this
  is the audit trail the current setup lacks.

## The production guard — treat this as a security requirement

`handleAutoPromote` deliberately requires `RP_PROD_PASSWORD` to **enable**
auto-promote into the last ring, precisely so auto-promote cannot be used as a
way around the production password (`internal/api/api.go:360-368`; see also the
note in `README.md` around line 256 and `training/labs/lab-04-production-approval.md`).

A config-declared value reaches the store without passing through that handler.
You must not let this feature become a bypass. Decide and document how config
interacts with the guard. Acceptable approaches:

* Refuse at config-validation time: config may not enable auto-promote into the
  prod ring, and doing so is a startup error directing the operator to the API
  path. Simplest and safest.
* Or require an explicit, separately-named opt-in on the app (e.g.
  `allow_prod_auto_promote: true`) so that enabling the hands-free path into
  production is a deliberate, greppable, review-visible act — plus a WARN log on
  every startup where it is active.

Whichever you choose, add a test asserting that a config which enables
auto-promote into prod without the opt-in does **not** result in a stored
`auto_promote = true`.

## API behaviour

* Option A: `PUT .../auto-promote` on a config-owned ring returns `409` with a
  message naming the config file as the owner. Rings not declared in config keep
  working exactly as now.
* Either option: `GET` paths that expose ring state should make it discoverable
  whether the value is config-owned or runtime-set, so the UI can disable the
  toggle rather than offer a control that will fail.

## UI

If the ring's auto-promote is config-owned, render the toggle disabled with a
short explanation ("managed by config"). Do not silently show a control whose
use returns 409.

---

# Tests

* Config parsing: absent vs `true` vs `false` produce three distinguishable
  states.
* Validation rejects the prod-ring case per the guard decision above.
* Reconciliation sets, clears, and leaves-alone the right rings; second run is a
  no-op.
* API returns 409 for a config-owned ring (Option A) and still works for a ring
  config does not declare.
* A promotion cycle honours a config-declared value end to end — this is the
  behaviour that actually matters, so assert on the promotion outcome, not just
  the stored column.

Use the existing store fakes and the `gatedHarness` pattern in
`internal/promoter/gates_test.go`. Note that harness builds its store with
`store.NewMemoryWithClock` so the store and promoter share one clock — follow
that; a store left on `time.Now` makes time-dependent tests fail on a future
date rather than on a real defect.

---

# Documentation

* `README.md`: document the field in the config reference and the ring-config
  table, including the absent/true/false semantics and the prod guard rule.
* Update the auto-promote section (around `README.md:252`) so the interaction
  between config and the production password is stated where operators already
  look for it.
* Add a short migration note: how to move an existing runtime-toggled ring into
  config without a window where auto-promote flips unexpectedly.

---

# Out of scope for this repo, but note it in the PR

The `diytaxreturn` instance's app registry lives in a different repository
(`bwalia/diy-tax-return-uk`, `devops/ring-promoter/configmap.yaml`). Adopting
the new field there is a follow-up change; this PR only needs to make the field
available and documented.

---

## Final Goal

A ring's auto-promote setting should be declarable in the same YAML that already
declares its rings, health URLs, and deployer — reconciled at startup, visible in
review, reproducible on a fresh database, and unable to become a back door into
production. Configs that do not use the field must behave exactly as they do
today.
