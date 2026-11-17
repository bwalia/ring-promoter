#!/usr/bin/env bash
#
# verify-mac-release-readiness.sh — everything that has to be true before the
# Mac app can be archived for the App Store, checked before a build rather
# than after one.
#
# This exists because of android-release.yml, which was written, committed, and
# would have failed on the first step of its first real release because nobody
# had run it. A release lane that has never run is a guess. This script is the
# part of the guess that can be checked from a laptop.
#
#   ./scripts/verify-mac-release-readiness.sh            # report and exit 1 on a blocker
#   ./scripts/verify-mac-release-readiness.sh --warn     # report only, always exit 0
#
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

WARN_ONLY=0
[ "${1:-}" = "--warn" ] && WARN_ONLY=1

blockers=0
note() { printf '  %s\n' "$1"; }
ok()   { printf '  \033[32mok\033[0m    %s\n' "$1"; }
bad()  { printf '  \033[31mBLOCK\033[0m %s\n' "$1"; blockers=$((blockers + 1)); }
warn() { printf '  \033[33mwarn\033[0m  %s\n' "$1"; }

echo "Ring Promoter — macOS App Store readiness"
echo

echo "Signing"
if security find-identity -v -p codesigning 2>/dev/null | grep -q "Apple Distribution"; then
  ok "an Apple Distribution certificate is installed"
else
  bad "no Apple Distribution certificate. The App Store will not take a build"
  note "signed with Apple Development. CI mints one from the App Store Connect"
  note "API key (see ios/fastlane/Fastfile, lane :prepare_signing); locally it"
  note "has to be downloaded from developer.apple.com."
fi

if security find-identity -v 2>/dev/null | grep -q "3rd Party Mac Developer Installer\|Apple Distribution"; then
  ok "an installer-capable identity is present"
else
  warn "no Mac installer certificate seen — the .pkg step needs one"
fi

profiles="$HOME/Library/MobileDevice/Provisioning Profiles"
if [ -d "$profiles" ] && ls "$profiles"/*.provisionprofile >/dev/null 2>&1; then
  ok "a macOS provisioning profile is installed"
else
  bad "no macOS provisioning profile installed"
  note "also why the app falls back to the legacy keychain — see Keychain.swift"
fi

echo
echo "The app"
if [ -f RingPromoterMac/Resources/Assets.xcassets/AppIcon.appiconset/Contents.json ]; then
  count=$(ls RingPromoterMac/Resources/Assets.xcassets/AppIcon.appiconset/*.png 2>/dev/null | wc -l | tr -d ' ')
  if [ "$count" -ge 10 ]; then
    ok "app icon present ($count sizes)"
  else
    bad "app icon has only $count sizes; macOS wants 10"
  fi
else
  bad "no AppIcon.appiconset"
fi

if grep -q "ITSAppUsesNonExemptEncryption" project.yml 2>/dev/null; then
  ok "export-compliance key declared"
else
  bad "Info.plist has no ITSAppUsesNonExemptEncryption"
  note "every upload is held for a manual answer without it."
fi

# Checked in the built bundle, not in project.yml. The first version of this
# check read the spec and reported a blocker that did not exist: xcodegen picks
# the manifest up from the shared source path without being told to.
built=$(find "$HOME/Library/Developer/Xcode/DerivedData/RingPromoterMac-"*/Build/Products \
  -name "PrivacyInfo.xcprivacy" 2>/dev/null | head -1)
if [ -n "$built" ]; then
  ok "privacy manifest ships in the bundle"
elif [ -f ../ios/RingPromoter/PrivacyInfo.xcprivacy ]; then
  warn "privacy manifest exists but no build to confirm it ships — build first"
else
  bad "no privacy manifest in the shared sources"
fi

echo
echo "Release plumbing"
if [ -f ../.github/workflows/macos-release.yml ]; then
  ok "a macOS release workflow exists"
  warn "it has never run — treat its first run as the test"
else
  bad "no macOS release workflow"
fi

echo
echo "Only you can do these — they need the Apple account:"
note "1. An App Store Connect record for the macOS platform of"
note "   org.fictionally.ringpromoter. A native Mac app is NOT automatically"
note "   covered by the iOS record; check whether it can be added as a platform"
note "   or needs its own."
note "2. Accept any pending agreements. Nobody else may do this on your behalf."
note "3. Screenshots at Mac sizes, and a description for the macOS listing."
note "4. Press Submit for Review."
echo

if [ "$blockers" -gt 0 ]; then
  echo "$blockers blocker(s)."
  [ "$WARN_ONLY" -eq 1 ] && exit 0
  exit 1
fi
echo "No blockers found."
