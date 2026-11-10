#!/usr/bin/env bash
#
# load-branding-from-vault.sh — publisher branding for the legal pages and the
# App Store listing, from Vault.
#
# Reads the flat KV v2 secret `secret/ring-promoter/branding` (override with
# RINGPROMOTER_BRANDING_VAULT_PATH) and exports the values the website build,
# fastlane and verify-app-store-readiness.sh all read from the environment.
#
#   CI:    ./ci/load-branding-from-vault.sh           # appends to $GITHUB_ENV
#   local: eval "$(./ci/load-branding-from-vault.sh)" # exports to shell
#
# Auth resolution matches ios/ci/load-ios-vault-secrets.sh: VAULT_ADDR and
# VAULT_TOKEN if set, otherwise the JSON token file at VAULT_TOKEN_FILE.
#
# None of these values is a secret — every one ends up on a published legal
# page or the public App Store listing. They live in Vault anyway so that the
# publisher identity has a single source of truth shared by the website and the
# iOS release, rather than being duplicated into each CI provider's settings.
# Nothing here is masked, for the same reason: masking published strings only
# makes the build log harder to read.
#
# Expected keys (see ios/docs/APP-STORE-RELEASE.md §1):
#   required  PUBLISHER CONTACT_EMAIL JURISDICTION POSTAL_ADDRESS
#             REVIEW_FIRST_NAME REVIEW_LAST_NAME REVIEW_PHONE
#   optional  REVIEW_EMAIL COPYRIGHT APP_NAME BUNDLE_ID GITHUB_URL SITE_URL
#             LEGAL_LAST_UPDATED
#
set -euo pipefail

VAULT_TOKEN_FILE="${VAULT_TOKEN_FILE:-$HOME/.secrets/vault/token.json}"
VAULT_SECRET_PATH="${RINGPROMOTER_BRANDING_VAULT_PATH:-secret/ring-promoter/branding}"
export VAULT_ADDR="${VAULT_ADDR:-}" VAULT_TOKEN="${VAULT_TOKEN:-}" VAULT_TOKEN_FILE VAULT_SECRET_PATH

python3 - <<'PY'
import json, os, shlex, sys, urllib.request, urllib.error

def resolve_auth():
    addr = os.environ.get("VAULT_ADDR") or ""
    token = os.environ.get("VAULT_TOKEN") or ""
    path = os.environ.get("VAULT_TOKEN_FILE") or ""
    if (not addr or not token) and path and os.path.isfile(path):
        with open(path) as fh:
            data = json.load(fh)
        if not isinstance(data, dict):
            sys.exit("ERROR: %s is not a JSON object" % path)

        def pick(keys):
            for key in keys:
                if data.get(key):
                    return data[key]
            return ""

        addr = addr or pick(("VAULT_ADDR", "VAULT_URI", "vault_addr", "addr", "url"))
        token = token or pick(("VAULT_TOKEN", "vault_token", "token", "client_token"))
        if not token and isinstance(data.get("auth"), dict):
            token = data["auth"].get("client_token") or ""
    if not addr or not token:
        sys.exit("ERROR: Vault auth unavailable — set VAULT_ADDR/VAULT_TOKEN or %s"
                 % (path or "$VAULT_TOKEN_FILE"))
    return addr.rstrip("/"), token

addr, token = resolve_auth()
secret_path = os.environ["VAULT_SECRET_PATH"]
if not secret_path.startswith("secret/data/"):
    secret_path = "secret/data/" + secret_path.removeprefix("secret/")
url = addr + "/v1/" + secret_path

req = urllib.request.Request(url, method="GET")
req.add_header("X-Vault-Token", token)
try:
    with urllib.request.urlopen(req, timeout=30) as resp:
        raw = resp.read()
except urllib.error.HTTPError as exc:
    detail = exc.read().decode("utf-8", "replace")[:500]
    sys.exit("ERROR: Vault GET %s -> HTTP %s: %s" % (url, exc.code, detail))
except urllib.error.URLError as exc:
    sys.exit("ERROR: Vault GET %s -> %s" % (url, exc))

try:
    payload = json.loads(raw)
except ValueError:
    sys.exit("ERROR: Vault response from %s is not JSON" % url)

data = (payload.get("data") or {}).get("data")
if not isinstance(data, dict):
    sys.exit("ERROR: unexpected Vault response at %s (no .data.data map)" % url)

REQUIRED = ["PUBLISHER", "CONTACT_EMAIL", "JURISDICTION", "POSTAL_ADDRESS",
            "REVIEW_FIRST_NAME", "REVIEW_LAST_NAME", "REVIEW_PHONE"]
OPTIONAL = ["REVIEW_EMAIL", "COPYRIGHT", "APP_NAME", "BUNDLE_ID", "GITHUB_URL",
            "SITE_URL", "LEGAL_LAST_UPDATED"]

missing = [k for k in REQUIRED if not str(data.get(k) or "").strip()]
if missing:
    sys.exit(
        "ERROR: %s is missing required key(s): %s\n"
        "       These are the publisher details the legal pages and the App\n"
        "       Store listing are built from. See ios/docs/APP-STORE-RELEASE.md\n"
        "       and web/.env.example." % (secret_path, ", ".join(missing))
    )

env_lines = []
for key in REQUIRED + OPTIONAL:
    value = str(data.get(key) or "").strip()
    if value:
        # A newline would let a value forge further GITHUB_ENV assignments.
        if "\n" in value or "\r" in value:
            sys.exit("ERROR: %s contains a newline; use | to separate address lines" % key)
        env_lines.append((key, value))

github_env = os.environ.get("GITHUB_ENV")
if github_env:
    with open(github_env, "a") as fh:
        for key, value in env_lines:
            fh.write("%s=%s\n" % (key, value))
else:
    # Quoted, unlike the GITHUB_ENV branch above, because this output is meant
    # to be eval'd: every one of these values can contain spaces
    # ("England and Wales", "+44 20 7946 0000") and an unquoted export would
    # assign the first word and try to run the rest as a command.
    for key, value in env_lines:
        sys.stdout.write("export %s=%s\n" % (key, shlex.quote(value)))

sys.stderr.write("OK: loaded %d branding value(s) from %s (publisher: %s)\n"
                 % (len(env_lines), secret_path, data["PUBLISHER"]))
PY
