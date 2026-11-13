#!/usr/bin/env bash
#
# load-android-vault-secrets.sh — pull Ring Promoter Android release material
# from Vault.
#
# Reads the flat KV v2 secret `secret/ring-promoter/android` (override with
# RINGPROMOTER_ANDROID_VAULT_PATH), materialises the upload keystore and the
# Play service-account JSON as files, and exports the rest for Gradle and
# fastlane.
#
#   CI:    ./ci/load-android-vault-secrets.sh           # appends to $GITHUB_ENV
#   local: eval "$(./ci/load-android-vault-secrets.sh)" # exports to shell
#
# Mirrors ios/ci/load-ios-vault-secrets.sh on purpose: one place to look when
# either platform's release stops signing.
#
set -euo pipefail

VAULT_TOKEN_FILE="${VAULT_TOKEN_FILE:-$HOME/.secrets/vault/token.json}"
VAULT_SECRET_PATH="${RINGPROMOTER_ANDROID_VAULT_PATH:-secret/ring-promoter/android}"
WORKDIR="${ANDROID_SECRETS_DIR:-${RUNNER_TEMP:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.android-secrets}}"
mkdir -p "$WORKDIR"
chmod 700 "$WORKDIR"
export VAULT_ADDR="${VAULT_ADDR:-}" VAULT_TOKEN="${VAULT_TOKEN:-}" VAULT_TOKEN_FILE WORKDIR VAULT_SECRET_PATH

python3 - <<'PY'
import base64, json, os, sys, urllib.request, urllib.error

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
workdir = os.environ["WORKDIR"]
secret_path = os.environ.get("VAULT_SECRET_PATH", "secret/ring-promoter/android")
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

# The upload key signs the bundle; the service account uploads it. Both are
# required, because a build that cannot be uploaded is not a release.
required = [
    "UPLOAD_KEYSTORE_B64", "UPLOAD_KEYSTORE_PASSWORD",
    "UPLOAD_KEY_ALIAS", "UPLOAD_KEY_PASSWORD",
    "PLAY_SERVICE_ACCOUNT_JSON_B64",
]
missing = [k for k in required if not data.get(k)]
if missing:
    sys.exit("ERROR: %s is missing required key(s): %s" % (secret_path, ", ".join(missing)))

def write_secret_file(name, b64_value):
    path = os.path.join(workdir, name)
    with open(path, "wb") as fh:
        fh.write(base64.b64decode(b64_value, validate=True))
    os.chmod(path, 0o600)
    return path

keystore_path = write_secret_file("upload.jks", data["UPLOAD_KEYSTORE_B64"])
play_json_path = write_secret_file("play-service-account.json",
                                   data["PLAY_SERVICE_ACCOUNT_JSON_B64"])

env_lines = [
    ("RP_ANDROID_KEYSTORE", keystore_path),
    ("RP_ANDROID_KEYSTORE_PASSWORD", data["UPLOAD_KEYSTORE_PASSWORD"]),
    ("RP_ANDROID_KEY_ALIAS", data["UPLOAD_KEY_ALIAS"]),
    ("RP_ANDROID_KEY_PASSWORD", data["UPLOAD_KEY_PASSWORD"]),
    ("SUPPLY_JSON_KEY", play_json_path),
]

github_env = os.environ.get("GITHUB_ENV")
if github_env:
    with open(github_env, "a") as fh:
        for key, value in env_lines:
            # Passwords must never land in a log, even on a failing step.
            if "PASSWORD" in key:
                sys.stdout.write("::add-mask::%s\n" % value)
            fh.write("%s=%s\n" % (key, value))
else:
    for key, value in env_lines:
        sys.stdout.write("export %s=%s\n" % (key, value))

sys.stderr.write("OK: loaded Android release material from %s into %s\n"
                 % (secret_path, workdir))
PY
