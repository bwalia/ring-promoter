# Onboarding an OpenResty/Lua VM app into Ring Promoter

This is a **handoff prompt** for a fresh Claude Code session. Use it to onboard an
application that runs on **VMs** (int → test → acceptance → prod) and is built on
**OpenResty + Lua** — i.e. *not* on Kubernetes.

## Why this needs its own path

Ring Promoter's real deployer is `KubectlDeployer`, which updates Kubernetes
Deployment image tags. A VM/OpenResty app isn't on k8s, so that deployer does not
apply. Ring Promoter is built around a swappable **`Deployer` interface**, so
onboarding this app means writing a **new Deployer** (e.g. SSH/Ansible-based) that
ships the chosen version to each environment's VMs and reloads OpenResty. The
promotion rules, health checks, automatic rollback, history and UI are all reused
unchanged.

## How to use

1. Open a new Claude Code session (on a machine with access to the app's source
   and the VMs).
2. Paste the prompt below, replacing `<PASTE THE APP'S GITHUB REPO URL HERE>`
   with the app's repository.

---

## Prompt

```
I'm continuing work on the Ring Promoter platform — a Go control plane that
promotes application versions through ordered rings (int → test → acceptance →
prod) with health checks and automatic rollback. Repo: github.com/bwalia/ring-promoter.
It's already deployed on our k3s cluster (namespace ring-system, Postgres-backed).

I now want to onboard one of OUR applications. IMPORTANT: this app is NOT on
Kubernetes — it runs on VMs across four environments (int, test, acceptance,
prod) and is built on OpenResty + Lua.

App source (GitHub): <PASTE THE APP'S GITHUB REPO URL HERE>

Please start by:
1. Clone/read that repository and understand how the app is built, versioned,
   and structured (OpenResty/nginx config, Lua modules, build/release process).
2. Then ask me the details you need, specifically:
   - How the app is currently deployed to the VMs (Ansible / shell scripts / CI /
     manual?) and how OpenResty is started and reloaded (systemd service?
     `openresty -s reload`? a container?).
   - The VM inventory/hostnames for each environment and how to access them
     (SSH user, key, bastion/jump host?).
   - What "a version" means for this app (a git tag, a release tarball, a Lua
     bundle/opm package, or an image?).
   - The health-check URL/path for each environment (something that returns 200
     when the app is healthy).

Key context about Ring Promoter's design, so you plan correctly:
- It has swappable interfaces: Deployer, HealthChecker, Store. Apps are declared
  in a config file (adding an app needs NO code change), but the *deploy
  mechanism* comes from a Deployer implementation.
- It currently ships KubectlDeployer (updates k8s Deployment image tags) and a
  no-op LogDeployer. NEITHER fits VMs.
- Because this app lives on VMs, we need a NEW Deployer — e.g. an SSH/Ansible
  based one that ships the chosen version to each environment's VMs and reloads
  OpenResty — implemented behind the existing Deployer interface. The
  HealthChecker (HTTP GET on a health path) and the promotion/rollback logic are
  reused unchanged.
- The rings map to real environments via the per-app config. Ring names live in
  one place (internal/ring/ring.go) and can be renamed to
  int/test/acceptance/prod in a single edit if we want.

NEW DEPLOYER — implementation spec (this is the core of the work):
- Read internal/deployer/deployer.go for the interface to satisfy:
    type Deployer interface { Deploy(ctx, Target, version string) error }
    // optional: type LiveVersioner interface { LiveVersion(ctx, Target) (string, error) }
- The existing Target struct is Kubernetes-oriented (Namespace/Deployment/
  Container/Image). For VMs you'll need per-ring VM info instead — extend the
  config's RingConfig (internal/config/config.go) with fields such as: hosts
  (list), ssh_user, an ssh key / secret reference, deploy_path or systemd service
  name, and reuse health_url. Map those into whatever the SSH deployer needs.
- Implement internal/deployer/ssh.go — an SSHDeployer whose Deploy(t, version):
    1. connects to each VM host for that ring,
    2. places the requested version (git checkout <tag> / pull a release artifact
       / opm install),
    3. reloads OpenResty (systemctl reload openresty  OR  openresty -s reload),
    4. returns an error if ANY host fails, so Ring Promoter's auto-rollback runs.
  Optionally implement LiveVersion (read the deployed tag/version on a host).
- Deployer selection: cmd/ringpromoter/main.go currently picks ONE global
  deployer (cfg.Deployer = kubectl|log). To run k8s apps AND VM apps from one
  control plane, either (a) add a per-app `deployer` field in config and pick the
  Deployer per app, or (b) run a separate Ring Promoter instance for the VM apps.
  Recommend (a); discuss trade-offs with me before implementing.
- Reuse unchanged: HTTPChecker (health), promoter rules (retry + auto-rollback),
  Store (Postgres/memory), the REST API and web UI.
- Add unit tests for the SSHDeployer (mock the SSH/exec layer) and keep the
  existing LogDeployer + KubectlDeployer paths working.

Goal for this session: read the source, gather the details above, then propose
and implement how to represent a "version" and how to deploy/promote this
OpenResty/Lua app across its VM environments through Ring Promoter (starting with
the new SSH-based Deployer and the app's config entry).
```

---

## Good to decide up front

- **How versions get onto the VMs today.** If Ansible/CI already does it, the new
  Deployer can *trigger that* rather than reinventing deployment.
- **Network path / SSH access** from wherever Ring Promoter runs to the VMs — that's
  what the new Deployer needs.
- **What a "version" is** — a git tag, a built artifact/tarball, or a package. This
  becomes the value you seed/promote.
