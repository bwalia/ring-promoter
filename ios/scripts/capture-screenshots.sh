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
set -euo pipefail

cd "$(dirname "$0")/.."

DEVICE="${1:-}"
if [[ -z "$DEVICE" ]]; then
  DEVICE=$(xcrun simctl list devices available --json \
    | python3 -c '
import json, sys
data = json.load(sys.stdin)["devices"]
for runtime, devices in data.items():
    if "iOS" not in runtime:
        continue
    for device in devices:
        if device["isAvailable"] and "iPhone" in device["name"]:
            print(device["udid"]); raise SystemExit
raise SystemExit("no available iPhone simulator found")')
fi
echo "Using simulator $DEVICE"

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
      echo "UI tests failed in $appearance mode; see $WORK/$appearance.log" >&2
      tail -40 "$WORK/$appearance.log" >&2
      return 1
    }

  xcrun xcresulttool export attachments \
    --path "$WORK/$appearance.xcresult" \
    --output-path "$WORK/$appearance-att" > /dev/null

  OUT="docs/screenshots/$appearance" python3 - "$WORK/$appearance-att" <<'PY'
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
        shutil.copy(exported, out / f"{stem}.png")
        count += 1
print(f"    {count} screenshots -> {out}")
PY
}

capture light
capture dark
xcrun simctl ui "$DEVICE" appearance light

echo
echo "Done. Screenshots are in docs/screenshots/."
