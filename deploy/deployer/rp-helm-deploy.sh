#!/bin/sh
# rp-helm-deploy — Ring Promoter k8sjob runner: helm-deploy one app release.
#
# Runs as a Job in ring-exec (see internal/executor/k8sjob). Ring Promoter
# injects RP_APP, RP_RING, RP_VERSION (the image tag) and RP_TARGET_ENV (the
# namespace); the rest comes from the app's k8sjob `env` block:
#
#   GIT_REPO          ssh clone URL of the repo holding the chart (required)
#   CHART_PATH        chart directory inside the repo (required)
#   RELEASE_PREFIX    release = <prefix><env>, e.g. "shop-" -> shop-int (required)
#   DEPLOYMENT_SUFFIX Deployment = <release><suffix>, for rollout status (required)
#   IMAGE_REPOSITORY  --set image.repository (required)
#   GIT_REF           commit to deploy the chart from (default RP_VERSION: the
#                     image tag IS the commit, so the chart matches the build)
#   SMOKE_URL         health URL; "{env}" is replaced by the namespace
#   SMOKE_URL_PROD    health URL used instead when the namespace is prod
#   SMOKE_VERSION_FIELD  JSON field that must equal RP_VERSION (default "version")
#   HELM_TIMEOUT      helm --timeout (default 15m0s)
#   HELM_DRY_RUN      "1" = helm --dry-run=server, no rollout/smoke (a safe
#                     end-to-end check of image, key and RBAC)
#   GIT_SSH_KEY       read-only deploy key (PEM), from env_from_secrets; omit
#                     for a public repo
#
# Exit code is the outcome; stdout/stderr is the live deploy log in Ring Promoter.
set -eu

need() { eval "v=\${$1:-}"; [ -n "$v" ] || { echo "rp-helm-deploy: $1 is required" >&2; exit 2; }; }
for v in RP_VERSION RP_TARGET_ENV GIT_REPO CHART_PATH RELEASE_PREFIX DEPLOYMENT_SUFFIX IMAGE_REPOSITORY; do need "$v"; done

NS="$RP_TARGET_ENV"
case "$NS" in *[!a-z0-9-]*|"") echo "rp-helm-deploy: bad namespace '$NS'" >&2; exit 2 ;; esac
case "$RP_VERSION" in *[!A-Za-z0-9._-]*) echo "rp-helm-deploy: bad version '$RP_VERSION'" >&2; exit 2 ;; esac
REL="${RELEASE_PREFIX}${NS}"
DEPLOY="${REL}${DEPLOYMENT_SUFFIX}"
REF="${GIT_REF:-$RP_VERSION}"
TIMEOUT="${HELM_TIMEOUT:-15m0s}"
FIELD="${SMOKE_VERSION_FIELD:-version}"
echo "deploy ${RP_APP:-app} ${RP_VERSION} -> release ${REL} in ns ${NS} (chart ${CHART_PATH} @ ${REF})"

# ── chart at the promoted commit (sparse, blobless: only the chart is fetched) ──
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
SSH_OPTS="-o StrictHostKeyChecking=yes -o UserKnownHostsFile=/etc/ssh/ssh_known_hosts -o ConnectTimeout=20"
if [ -n "${GIT_SSH_KEY:-}" ]; then
  (umask 077; printf '%s\n' "$GIT_SSH_KEY" > "$WORK/key")
  unset GIT_SSH_KEY
  SSH_OPTS="-i $WORK/key -o IdentitiesOnly=yes $SSH_OPTS"
fi
export GIT_SSH_COMMAND="ssh $SSH_OPTS"
n=0
until git clone --quiet --filter=blob:none --no-checkout "$GIT_REPO" "$WORK/src"; do
  n=$((n + 1)); [ "$n" -lt 5 ] || { echo "git clone failed 5 times" >&2; exit 1; }
  echo "git clone failed (attempt $n), retrying..."; sleep $((n * 5))
done
git -C "$WORK/src" sparse-checkout set --no-cone "/${CHART_PATH}/"
git -C "$WORK/src" -c advice.detachedHead=false checkout --quiet "$REF" \
  || { echo "commit ${REF} not found in ${GIT_REPO}" >&2; exit 1; }
CHART="$WORK/src/$CHART_PATH"
VALUES="$CHART/values-${NS}.yaml"
[ -f "$VALUES" ] || { echo "no ${CHART_PATH}/values-${NS}.yaml at ${REF}" >&2; exit 1; }
echo "chart from commit $(git -C "$WORK/src" rev-parse --short HEAD)"

# ── helm: clear a stuck release, then upgrade and wait on workloads ──
ST="$(helm status "$REL" -n "$NS" -o json 2>/dev/null | jq -r '.info.status // empty' || true)"
case "${HELM_DRY_RUN:-}:$ST" in 1:*) ;; *:pending-*|*:failed)
  echo "release ${REL} is ${ST}: rolling back to the last good revision first"
  helm rollback "$REL" 0 -n "$NS" --wait=false || true ;;
esac
if [ "${HELM_DRY_RUN:-}" = 1 ]; then
  helm upgrade --install "$REL" "$CHART" -f "$VALUES" --namespace "$NS" \
    --set-string image.repository="$IMAGE_REPOSITORY" --set-string image.tag="$RP_VERSION" \
    --dry-run=server >/dev/null
  echo "dry run OK: ${REL} ${RP_VERSION} would deploy to ${NS}"
  exit 0
fi
# Helm 4's default watcher also waits on the ExternalSecret; wait on workloads only.
if ! helm upgrade --install "$REL" "$CHART" -f "$VALUES" \
     --namespace "$NS" \
     --set-string image.repository="$IMAGE_REPOSITORY" \
     --set-string image.tag="$RP_VERSION" \
     --wait=legacy --timeout "$TIMEOUT"; then
  echo "=== HELM UPGRADE FAILED: diagnostics ==="
  kubectl -n "$NS" get pods -l app.kubernetes.io/instance="$REL" -o wide || true
  kubectl -n "$NS" get events --sort-by=.lastTimestamp 2>/dev/null | tail -30 || true
  exit 1
fi
if ! kubectl -n "$NS" rollout status "deployment/$DEPLOY" --timeout=600s; then
  echo "=== ROLLOUT FAILED: diagnostics ==="
  kubectl -n "$NS" get pods -l app.kubernetes.io/instance="$REL" -o wide || true
  kubectl -n "$NS" logs -l app.kubernetes.io/instance="$REL" --tail=50 || true
  exit 1
fi

# ── smoke: the running build must report the tag just deployed ──
URL="${SMOKE_URL:-}"
[ "$NS" = prod ] && [ -n "${SMOKE_URL_PROD:-}" ] && URL="$SMOKE_URL_PROD"
if [ -n "$URL" ]; then
  URL="$(printf '%s' "$URL" | sed "s/{env}/$NS/g")"
  i=0
  while :; do
    i=$((i + 1))
    VER="$(curl -s --max-time 10 "$URL" | jq -r --arg f "$FIELD" 'getpath($f | split(".")) // empty' 2>/dev/null || true)"
    echo "smoke $i/12: $URL version=${VER:-none} (want ${RP_VERSION})"
    [ "$VER" = "$RP_VERSION" ] && break
    [ "$i" -lt 12 ] || { echo "smoke test FAILED: $URL never reported ${RP_VERSION}" >&2; exit 1; }
    sleep 10
  done
fi
echo "${RP_APP:-app} ${RP_VERSION} live in ${NS}"
