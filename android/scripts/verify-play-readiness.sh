#!/bin/sh
# Check everything Play will refuse the upload for, before the build runs.
#
# The same idea as ios/scripts/verify-app-store-readiness.sh: each of these
# fails for a reason that would otherwise be found by the Play Console after a
# bundle already exists, or by a reviewer a day later.
#
#   ./scripts/verify-play-readiness.sh
#
# Checks only what can be checked from this repository. Creating the Play app
# entry, accepting the Play Console terms, and owning the upload key are the
# operator's, and cannot be automated from here.
set -e
cd "$(dirname "$0")/.."

fail=0
note() { printf '  %s\n' "$1"; }
bad() { printf 'FAIL  %s\n' "$1"; fail=1; }
ok() { printf 'ok    %s\n' "$1"; }

printf '\nRing Promoter — Play readiness\n\n'

# The package name is permanent once published, so it must match everywhere.
gradle_id=$(sed -n 's/.*applicationId = "\([^"]*\)".*/\1/p' app/build.gradle.kts | head -1)
fastlane_id=$(sed -n 's/.*package_name("\([^"]*\)").*/\1/p' fastlane/Appfile | head -1)
if [ -z "$gradle_id" ]; then
  bad "no applicationId in app/build.gradle.kts"
elif [ "$gradle_id" != "$fastlane_id" ]; then
  bad "applicationId ($gradle_id) and fastlane package_name ($fastlane_id) disagree"
else
  ok "package name $gradle_id"
  note "permanent once published: it can never be changed, reused or freed"
fi

# Play wants 2-8 phone screenshots.
shots=$(ls store/screenshots/*.png 2>/dev/null | wc -l | tr -d ' ')
if [ "$shots" -lt 2 ]; then
  bad "$shots phone screenshots — Play wants at least 2 (scripts/capture-store-screenshots.sh)"
elif [ "$shots" -gt 8 ]; then
  bad "$shots phone screenshots — Play accepts at most 8"
else
  ok "$shots phone screenshots"
fi

[ -f store/feature-graphic.png ] \
  && ok "feature graphic" \
  || bad "no store/feature-graphic.png (scripts/make-feature-graphic.py)"

[ -f app/src/main/res/drawable/ic_launcher_foreground.xml ] \
  && ok "launcher icon" \
  || bad "no launcher icon"

# An unsigned bundle cannot be uploaded at all.
if [ -n "${RP_ANDROID_KEYSTORE:-}" ] || [ -f keystore.properties ]; then
  ok "upload key configured"
else
  bad "no upload key — set keystore.properties or RP_ANDROID_KEYSTORE (CI loads it from Vault)"
fi

# The data-safety form is answered in the Console, but it has to be true of the
# build. This is the check that keeps it honest.
if grep -rqE 'com\.google\.firebase|google-services|analytics' app/build.gradle.kts; then
  bad "an analytics or Firebase dependency is present — the Data safety form says no data is collected"
else
  ok "no analytics or telemetry dependency"
fi

printf '\n'
if [ "$fail" -ne 0 ]; then
  printf 'Not ready.\n\n'
  exit 1
fi
printf 'Ready to build.\n\n'
