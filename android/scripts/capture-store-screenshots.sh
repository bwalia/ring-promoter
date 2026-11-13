#!/bin/sh
# Capture the Play Store phone screenshots from a running emulator.
#
# Drives demo mode, so the screenshots show the same data a reviewer sees when
# they tap "Explore in demo mode" — no real cluster, no redaction needed, and
# nothing that goes stale.
#
# Taps by label through tap.sh, so a layout change fails this loudly instead of
# quietly capturing the wrong screen.
#
#   ./scripts/capture-store-screenshots.sh [output-dir]
#
# Play wants 2–8 phone screenshots, at least 320px on the short edge. A Pixel
# emulator at 1080x2400 clears that with room to spare.
set -e
ADB="${ADB:-$HOME/Library/Android/sdk/platform-tools/adb}"
DIR="${1:-store/screenshots}"
HERE="$(cd "$(dirname "$0")" && pwd)"
PKG=org.fictionally.ringpromoter

mkdir -p "$DIR"

shot() {
  $ADB shell screencap -p /sdcard/shot.png
  $ADB pull /sdcard/shot.png "$DIR/$1.png" >/dev/null
  echo "captured $1"
}

tap() { "$HERE/tap.sh" "$1" >/dev/null; }

$ADB shell am force-stop "$PKG"
$ADB shell am start -n "$PKG/.MainActivity" >/dev/null
sleep 4

# Onboarding only appears before a control plane is chosen, so demo mode has
# to be entered if the app restored a previous session.
if $ADB shell cat /sdcard/ui.xml 2>/dev/null | grep -q "Explore in demo mode"; then
  tap "Explore in demo mode"
  sleep 3
fi

shot 01-overview

tap "Payments API"; sleep 3; shot 02-app-detail

tap "Promote"; sleep 2; shot 03-promote-sheet
$ADB shell input keyevent 4; sleep 2

tap "History"; sleep 3; shot 04-history
$ADB shell input keyevent 4; sleep 2

tap "Maintenance"; sleep 3; shot 05-maintenance
$ADB shell input keyevent 4; sleep 2
$ADB shell input keyevent 4; sleep 2

tap "Rings"; sleep 4; shot 06-rings

tap "Activity"; sleep 3; shot 07-activity

echo "Screenshots in $DIR"
