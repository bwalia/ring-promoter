#!/bin/sh
# Tap the on-screen element whose text contains $1, then optionally screenshot
# to $2.
#
# Taps by label rather than by coordinate, so a layout change makes the
# walkthrough fail loudly instead of silently tapping empty space and
# "passing". Retries for a few seconds because a tap and the recomposition it
# causes are not synchronous — the previous version of this script raced them
# and reported success against a stale screen.
set -e
ADB="$HOME/Library/Android/sdk/platform-tools/adb"
OUT_DIR="$(dirname "$0")"

label="$1"
shot="$2"
tries=${3:-25}

i=0
while [ "$i" -lt "$tries" ]; do
  $ADB shell uiautomator dump /sdcard/ui.xml >/dev/null 2>&1 || true
  xml=$($ADB shell cat /sdcard/ui.xml 2>/dev/null || true)
  coords=$(printf '%s' "$xml" | tr '>' '\n' | grep -F "text=\"$label" | head -1 |
    grep -oE 'bounds="\[[0-9]+,[0-9]+\]\[[0-9]+,[0-9]+\]"' |
    grep -oE '[0-9]+' | tr '\n' ' ')
  [ -n "$coords" ] && break
  i=$((i + 1))
done

if [ -z "$coords" ]; then
  echo "NOT FOUND after $tries tries: $label"
  exit 1
fi

set -- $coords
x=$(( ($1 + $3) / 2 ))
y=$(( ($2 + $4) / 2 ))
$ADB shell input tap "$x" "$y"
echo "tapped '$label' at $x,$y"

if [ -n "$shot" ]; then
  # Let the tap's recomposition land before capturing.
  $ADB shell uiautomator dump /sdcard/ui.xml >/dev/null 2>&1 || true
  $ADB shell uiautomator dump /sdcard/ui.xml >/dev/null 2>&1 || true
  $ADB shell screencap -p /sdcard/shot.png >/dev/null 2>&1
  $ADB pull /sdcard/shot.png "$OUT_DIR/$shot" >/dev/null 2>&1
  echo "captured $shot"
fi
