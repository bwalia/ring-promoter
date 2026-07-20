#!/usr/bin/env bash
# Tiny Ring Promoter API helper for the labs.
#
#   export RP_URL=https://ring-promoter.fictionally.org
#   export RP_TOKEN=<your-token>
#   ./rp.sh apps
#   ./rp.sh seed hello-world int v0.1.0
#   ./rp.sh promote hello-world int
#   ./rp.sh rings hello-world
#   ./rp.sh rollback hello-world test
set -euo pipefail

: "${RP_URL:?set RP_URL}"; : "${RP_TOKEN:?set RP_TOKEN}"
auth=(-H "Authorization: Bearer ${RP_TOKEN}" -H "Content-Type: application/json")
cmd="${1:-help}"; shift || true

case "$cmd" in
  apps)     curl -fsS "${auth[@]}" "$RP_URL/api/apps" ;;
  rings)    curl -fsS "${auth[@]}" "$RP_URL/api/apps/$1/rings" ;;
  seed)     curl -fsS "${auth[@]}" -X POST "$RP_URL/api/apps/$1/seed?async=1" \
              -d "{\"ring\":\"$2\",\"version\":\"$3\"${4:+,\"cr_code\":\"$4\"}}" ;;
  promote)  curl -fsS "${auth[@]}" -X POST "$RP_URL/api/apps/$1/promote?async=1" \
              -d "{\"from_ring\":\"$2\"${3:+,\"cr_code\":\"$3\"}}" ;;
  rollback) curl -fsS "${auth[@]}" -X POST "$RP_URL/api/apps/$1/rollback?async=1" \
              -d "{\"ring\":\"$2\"}" ;;
  *) echo "usage: rp.sh {apps|rings <app>|seed <app> <ring> <version> [cr]|promote <app> <from-ring> [cr]|rollback <app> <ring>}" ;;
esac
