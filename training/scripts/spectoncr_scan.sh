#!/usr/bin/env bash
# Scan one image for CVEs with SpectonCR and save the result JSON.
#
#   spectoncr_scan.sh <source-image-ref> <repo-name> <tag> <out.json>
#
# Copies the image (linux/amd64) into SpectonCR as
# <registry>/<tenant>/<project>/<repo-name>:<tag>; the push triggers SpectonCR's
# OSV scan of that digest, which this polls at GET /v2/scan/live/{digest}
# until it completes. Prints "digest=… scan_id=… report_url=…" on stdout.
#
# Env:
#   SPECTONCR_REGISTRY   default acc-spectoncr.diytaxreturn.co.uk (scanner on)
#   SPECTONCR_USER / SPECTONCR_PASSWORD   registry credentials (push + pull)
#   SPECTONCR_TENANT / SPECTONCR_PROJECT  default demo / default
#   SPECTONCR_TIMEOUT    seconds to wait for the scan, default 600
#
# Uses skopeo when installed, else the skopeo container image.
set -euo pipefail

SRC="$1" NAME="$2" TAG="$3" OUT="$4"
REG="${SPECTONCR_REGISTRY:-acc-spectoncr.diytaxreturn.co.uk}"
TENANT="${SPECTONCR_TENANT:-demo}"
PROJECT="${SPECTONCR_PROJECT:-default}"
TIMEOUT="${SPECTONCR_TIMEOUT:-600}"
: "${SPECTONCR_USER:?SPECTONCR_USER is required}" "${SPECTONCR_PASSWORD:?SPECTONCR_PASSWORD is required}"
REPO="${TENANT}/${PROJECT}/${NAME}"

skopeo() {
  if type -P skopeo >/dev/null; then
    command skopeo "$@"
  else
    docker run --rm quay.io/skopeo/stable:v1.16 "$@"
  fi
}

# Credentials go through a file-less --dest-creds/--creds; they never reach
# the step log because set -x is off.
skopeo copy --retry-times 3 --override-os linux --override-arch amd64 \
  --dest-creds "${SPECTONCR_USER}:${SPECTONCR_PASSWORD}" \
  "docker://${SRC}" "docker://${REG}/${REPO}:${TAG}" >&2
DIGEST=$(skopeo inspect --creds "${SPECTONCR_USER}:${SPECTONCR_PASSWORD}" "docker://${REG}/${REPO}:${TAG}" | jq -r .Digest)
[[ "$DIGEST" == sha256:* ]] || { echo "could not resolve digest of ${REPO}:${TAG}" >&2; exit 1; }

TOK=$(curl -fsS -u "${SPECTONCR_USER}:${SPECTONCR_PASSWORD}" \
  "https://${REG}/auth/token?service=spectoncr-registry&scope=repository:${REPO}:pull" | jq -r .token)
[[ -n "$TOK" && "$TOK" != null ]] || { echo "no SpectonCR token" >&2; exit 1; }

# The push enqueues the scan; an image pushed before (same digest) already has
# a result. Ask for one explicitly as well, so a digest the webhook missed is
# still scanned (202, or 4xx when it's already queued — both fine).
curl -sS -o /dev/null -X POST "https://${REG}/v2/scan" -H "Authorization: Bearer ${TOK}" \
  -H "Content-Type: application/json" \
  -d "$(jq -nc --arg t "$TENANT" --arg p "$PROJECT" --arg r "$NAME" --arg ref "$TAG" --arg d "$DIGEST" \
        '{tenant:$t, project:$p, repository:$r, reference:$ref, digest:$d}')" || true

deadline=$(( $(date +%s) + TIMEOUT ))
status=unknown
while :; do
  curl -fsS -H "Authorization: Bearer ${TOK}" "https://${REG}/v2/scan/live/${DIGEST}" > "$OUT" || echo '{}' > "$OUT"
  status=$(jq -r '.status // "unknown"' "$OUT")
  case "$status" in completed|failed) break ;; esac
  if [ "$(date +%s)" -ge "$deadline" ]; then break; fi
  sleep 8
done
echo "spectoncr ${REPO}:${TAG} ${DIGEST} -> ${status}" >&2
[ "$status" = completed ] || { echo "SpectonCR scan did not complete (status=${status})" >&2; exit 1; }

SCAN_ID=$(jq -r '.result.id // empty' "$OUT")
echo "digest=${DIGEST}"
echo "scan_id=${SCAN_ID}"
echo "report_url=https://${REG}/v2/scan/${SCAN_ID}/report"
