#!/usr/bin/env bash
#
# Capture the screenshot sets App Store Connect requires, into the directory
# `deliver` uploads from.
#
# Apple accepts a fixed list of pixel sizes and rejects anything else at upload
# time, so the device matters: the documentation set in docs/screenshots is
# taken on whatever iPhone simulator is to hand (1206x2622 on an iPhone 16 Pro,
# which is not an accepted App Store size). This script pins the two sizes that
# are required while the app ships for iPhone *and* iPad:
#
#   6.9" iPhone   1320 x 2868   iPhone 17 Pro Max
#   13"   iPad    2064 x 2752   iPad Pro 13-inch
#
# Both sets land flat in fastlane/screenshots/<locale>/ — `deliver` infers the
# device from each image's dimensions, so one folder holds both.
#
#   ./scripts/capture-app-store-screenshots.sh                # capture + curate
#   ./scripts/capture-app-store-screenshots.sh --curate-only  # re-curate only
#
set -euo pipefail

cd "$(dirname "$0")/.."

LOCALE="${LOCALE:-en-GB}"
IPHONE="${IPHONE:-iPhone 17 Pro Max}"
IPAD="${IPAD:-iPad Pro 13-inch (M5)}"
DEST="fastlane/screenshots/$LOCALE"

CURATE_ONLY=0
[[ "${1:-}" == "--curate-only" ]] && CURATE_ONLY=1

# The UI tests attach 15 screenshots; App Store Connect takes at most 10 per
# display size. So the store set is chosen, not inherited — and the order here
# is the order shoppers see, because App Store Connect sorts by filename and
# the first two or three are what show in search results.
#
# Left out deliberately: 00-onboarding (an empty first-run screen is a weak
# opener), 03-promote-sheet (04-production-guard makes the same point harder),
# 11-maintenance, 14-ring-test-links and 15-test-kit-plan (detail screens that
# need their context explained).
CURATED=(
  01-overview          # the whole fleet, trouble pinned first
  02-app-detail        # per-ring versions, drift and gates
  11-rings-universe    # the fleet view, the most distinctive screen
  05-job-running       # steps and log appearing live
  04-production-guard  # password AND typed confirmation, submit disabled
  06-job-succeeded     # an explicit terminal state, per-step durations
  07-rollback-sheet    # no gate at all — incident response is never blocked
  10-history           # grouped by day, failures expandable
  08-activity          # the shared cross-app feed
  09-settings          # instances, security, what the server says it is
)

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Light appearance only: a mixed light/dark gallery reads as an accident rather
# than a choice. docs/screenshots still holds both appearances.
capture_for() {
  local device="$1" prefix="$2"
  echo
  echo "════ $prefix — $device ════"
  # A non-zero exit here means some test failed; the screenshots the rest of
  # the suite took are still exported, and the curation step below is what
  # actually decides whether the store set is complete. Swallowing it keeps one
  # flaky simulator launch from costing a 20-minute re-capture.
  APPEARANCES=light OUT_ROOT="$WORK/$prefix" \
    ./scripts/capture-screenshots.sh "$device" || true
  for f in "$WORK/$prefix/light/"*.png; do
    mv "$f" "$DEST/$prefix-$(basename "$f")"
  done
}

if (( CURATE_ONLY )); then
  echo "Curating the screenshots already in $DEST"
else
  rm -rf "$DEST"
  mkdir -p "$DEST"
  capture_for "$IPHONE" "iphone69"
  capture_for "$IPAD" "ipad13"
fi

echo
echo "════ curating ════"
DEST="$DEST" CURATED="${CURATED[*]}" python3 - <<'PY'
import os, pathlib, re, sys

dest = pathlib.Path(os.environ["DEST"])
curated = os.environ["CURATED"].split()
prefixes = sorted({m.group(1) for f in dest.glob("*.png")
                   if (m := re.match(r"(iphone69|ipad13)-", f.name))})
if not prefixes:
    sys.exit(f"no captured screenshots found in {dest}")

failed = False
for prefix in prefixes:
    # Undo any previous curation first, so re-running is safe. A curated file is
    # <prefix>-<order>-<slug>.png; an uncurated one is <prefix>-<stem>.png and
    # every stem the tests produce already begins with its own NN-. Telling the
    # two apart by regex alone is what broke before: "iphone69-00-onboarding"
    # matches <prefix>-NN- just as well as a curated name does.
    for f in list(dest.glob(f"{prefix}-*.png")):
        m = re.fullmatch(rf"{re.escape(prefix)}-\d\d-(\d\d-.+)\.png", f.name)
        if m:
            f.rename(dest / f"{prefix}-{m.group(1)}.png")

    targets, missing = {}, []
    for order, stem in enumerate(curated, start=1):
        src = dest / f"{prefix}-{stem}.png"
        if not src.exists():
            missing.append(stem)
            continue
        # Drop the test's own number: the curated order is the one that counts,
        # and keeping both produced names like "iphone69-01-01-overview.png".
        slug = re.sub(r"^\d+-", "", stem)
        targets[src] = dest / f"{prefix}-{order:02d}-{slug}.png"

    if missing:
        print(f"  {prefix}: MISSING {', '.join(missing)}")
        print(f"  {prefix}: either the UI test that takes them failed, or a name changed.")
        failed = True
        continue

    # Two-phase rename: a target can collide with another source's name, and a
    # one-phase rename would silently overwrite a screenshot.
    staged = {}
    for src, dst in targets.items():
        tmp = src.with_name(src.name + ".curating")
        src.rename(tmp)
        staged[tmp] = dst
    for tmp, dst in staged.items():
        tmp.rename(dst)

    kept = set(targets.values())
    for leftover in sorted(dest.glob(f"{prefix}-*.png")):
        if leftover not in kept:
            leftover.unlink()

    if len(kept) > 10:
        print(f"  {prefix}: {len(kept)} screenshots, App Store Connect allows 10")
        failed = True
    else:
        print(f"  {prefix}: kept {len(kept)}")

sys.exit(1 if failed else 0)
PY

echo
echo "════ captured ════"
# Apple rejects the upload, not the submission, when a size is wrong — so the
# sizes are printed and asserted here rather than discovered in fastlane.
python3 - "$DEST" <<'PY'
import pathlib, subprocess, sys
from collections import Counter

accepted = {
    (1320, 2868): '6.9" iPhone',
    (1290, 2796): '6.9" iPhone',
    (2064, 2752): '13" iPad',
    (2048, 2732): '13" iPad',
}

dest = pathlib.Path(sys.argv[1])
files = sorted(dest.glob("*.png"))
if not files:
    sys.exit("no screenshots were captured")

seen, bad = Counter(), []
for f in files:
    out = subprocess.run(["sips", "-g", "pixelWidth", "-g", "pixelHeight", str(f)],
                         capture_output=True, text=True).stdout
    dims = {k.strip(): int(v) for k, v in
            (line.split(":") for line in out.splitlines() if ":" in line and "pixel" in line)}
    size = (dims["pixelWidth"], dims["pixelHeight"])
    label = accepted.get(size)
    if label is None:
        bad.append(f"{f.name}: {size[0]}x{size[1]} is not an accepted App Store size")
    else:
        seen[label] += 1

for label, count in sorted(seen.items()):
    print(f"  {count:>2} x {label}")
if bad:
    sys.exit("\n".join(["", *bad]))

missing = {'6.9" iPhone', '13" iPad'} - set(seen)
if missing:
    sys.exit(f"missing required set(s): {', '.join(sorted(missing))}")
print(f"\nAll {len(files)} screenshots are an accepted size. Upload dir: {dest}")
PY
