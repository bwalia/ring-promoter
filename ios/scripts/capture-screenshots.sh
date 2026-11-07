#!/usr/bin/env bash
#
# Regenerate docs/screenshots/{light,dark} by running the UI tests against demo
# mode in both appearances and extracting the attachments they record.
#
# The tests attach a screenshot at each interesting point (see
# `attachScreenshot` in RingPromoterUITests/CriticalPathUITests.swift), so the
# documented set can never drift from what the app actually renders — if a
# screen changes, the test that photographs it changes with it.
#
#   ./scripts/capture-screenshots.sh [simulator-name-or-id]
#
# Env overrides, used by capture-app-store-screenshots.sh:
#   APPEARANCES  appearances to capture (default "light dark")
#   OUT_ROOT     directory the <appearance>/ folders go under
#                (default "docs/screenshots")
#   NAME_PREFIX  prefix for every output filename (default none)
#
set -euo pipefail

cd "$(dirname "$0")/.."

APPEARANCES="${APPEARANCES:-light dark}"
FAILED=0
OUT_ROOT="${OUT_ROOT:-docs/screenshots}"
NAME_PREFIX="${NAME_PREFIX:-}"

# `xcodebuild -destination id=…` needs a UDID, so a name has to be resolved to
# one first — passing a name straight through (as this script's own usage line
# used to suggest) fails with "Unable to find a device with identifier".
DEVICE=$(REQUESTED="${1:-}" python3 -c '
import json, os, subprocess, sys

requested = os.environ.get("REQUESTED", "").strip()
data = json.loads(subprocess.run(
    ["xcrun", "simctl", "list", "devices", "available", "--json"],
    capture_output=True, text=True, check=True,
).stdout)["devices"]

candidates = [
    device
    for runtime, devices in data.items() if "iOS" in runtime
    for device in devices if device.get("isAvailable")
]
if not candidates:
    sys.exit("no available iOS simulator found")

if not requested:
    # Unchanged default: the first available iPhone.
    for device in candidates:
        if "iPhone" in device["name"]:
            print(device["udid"]); break
    else:
        sys.exit("no available iPhone simulator found")
elif any(device["udid"] == requested for device in candidates):
    print(requested)
else:
    matches = [d for d in candidates if d["name"] == requested]
    if not matches:
        names = ", ".join(sorted({d["name"] for d in candidates}))
        sys.exit(f"no available simulator named {requested!r}. Available: {names}")
    # Newest runtime last in simctl output; take the newest match.
    print(matches[-1]["udid"])
')
echo "Using simulator $DEVICE ($(xcrun simctl list devices | grep "$DEVICE" | sed 's/^ *//;s/ (.*//'))"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

capture() {
  local appearance="$1"
  echo "==> $appearance"
  xcrun simctl boot "$DEVICE" 2>/dev/null || true
  xcrun simctl ui "$DEVICE" appearance "$appearance"

  xcodebuild \
    -project RingPromoter.xcodeproj \
    -scheme RingPromoter \
    -destination "id=$DEVICE" \
    -only-testing:RingPromoterUITests \
    -resultBundlePath "$WORK/$appearance.xcresult" \
    test > "$WORK/$appearance.log" 2>&1 || {
      # Export anyway. A single flaky test — the iPad simulator intermittently
      # answers "failed to launch <app>" — used to throw away the dozen good
      # screenshots the other tests had already taken, costing a 20-minute
      # re-run to recover them. Completeness is enforced where it belongs, by
      # the curation step in capture-app-store-screenshots.sh, which names the
      # exact screenshot it did not get.
      FAILED=1
      echo >&2
      echo "!! UI tests failed in $appearance mode — exporting the screenshots" >&2
      echo "!! that were taken anyway. Full log: $WORK/$appearance.log" >&2
      grep -E "^\s+CriticalPathUITests\.|Testing failed:" "$WORK/$appearance.log" >&2 || true
      echo >&2
    }

  xcrun xcresulttool export attachments \
    --path "$WORK/$appearance.xcresult" \
    --output-path "$WORK/$appearance-att" > /dev/null

  OUT="$OUT_ROOT/$appearance" PREFIX="$NAME_PREFIX" python3 - "$WORK/$appearance-att" <<'PY'
import json, os, pathlib, shutil, sys

source = pathlib.Path(sys.argv[1])
out = pathlib.Path(os.environ["OUT"])
if out.exists():
    shutil.rmtree(out)
out.mkdir(parents=True)

manifest = json.loads((source / "manifest.json").read_text())
count = 0
for test in manifest:
    for attachment in test.get("attachments", []):
        exported = source / attachment["exportedFileName"]
        if not exported.exists():
            continue
        # The manifest name is what the test called it plus a uniquifier
        # ("01-overview_0_<uuid>"); keep only the part the test chose.
        name = attachment.get("suggestedHumanReadableName") or exported.stem
        stem = name.split("_")[0].removesuffix(".png")
        shutil.copy(exported, out / f"{os.environ.get('PREFIX', '')}{stem}.png")
        count += 1
print(f"    {count} screenshots -> {out}")
PY
}

for appearance in $APPEARANCES; do
  capture "$appearance"
done
xcrun simctl ui "$DEVICE" appearance light

echo
echo "Done. Screenshots are in $OUT_ROOT/."
# Still a failure for anyone regenerating the documentation set; the App Store
# script tolerates it on purpose and lets curation decide.
exit "$FAILED"
