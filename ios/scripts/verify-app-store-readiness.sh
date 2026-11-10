#!/usr/bin/env bash
#
# Everything that has to be true before an App Store submission, checked in one
# place so it is not re-remembered per release.
#
# Each check exists because getting it wrong costs a round trip with App Review
# (days), or an upload rejection (hours). Run it before `fastlane ios release`;
# .github/workflows/ios-release.yml runs it for the app_store target too.
#
#   ./scripts/verify-app-store-readiness.sh            # offline checks
#   ./scripts/verify-app-store-readiness.sh --online   # also fetch the public URLs
#
set -uo pipefail

cd "$(dirname "$0")/.."

ONLINE=0
[[ "${1:-}" == "--online" ]] && ONLINE=1

PASS=0 FAIL=0 WARN=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; PASS=$((PASS+1)); }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$1"; FAIL=$((FAIL+1)); }
warn() { printf '  \033[33m!\033[0m %s\n' "$1"; WARN=$((WARN+1)); }
head_() { printf '\n\033[1m%s\033[0m\n' "$1"; }

PLIST=Config/RingPromoter-Info.plist
ENTS=Config/RingPromoter.entitlements
PROJ=RingPromoter.xcodeproj/project.pbxproj
MANIFEST=RingPromoter/PrivacyInfo.xcprivacy
META=fastlane/metadata
WEB=../web

# ─────────────────────────────────────────────────────────────────────────────
head_ "Branding (environment)"
# The publisher details are read from the environment at build time — by the
# web legal pages and by fastlane — so this repository can be released under a
# different company without a code change. They are checked here because an
# unset one is not a cosmetic problem: a privacy policy naming the wrong
# publisher, or a support address that bounces, is a Guideline 1.5 rejection,
# and App Review opens both URLs and writes to the address.
#
# None of these is secret; all of them end up published. CI loads them from
# Vault (secret/ring-promoter/config) with ci/load-branding-from-vault.sh;
# web/.env.example documents them for local work.
for var in PUBLISHER CONTACT_EMAIL JURISDICTION POSTAL_ADDRESS \
           REVIEW_FIRST_NAME REVIEW_LAST_NAME REVIEW_PHONE; do
  if [[ -n "${!var:-}" ]]; then
    ok "$var is set"
  else
    bad "$var is not set — see web/.env.example"
  fi
done
# REVIEW_EMAIL falls back to CONTACT_EMAIL, COPYRIGHT to "<year> <publisher>".
for var in REVIEW_EMAIL COPYRIGHT APP_NAME BUNDLE_ID GITHUB_URL SITE_URL; do
  [[ -n "${!var:-}" ]] && echo "  · $var overridden: ${!var}"
done

# Nothing should still carry the old sentinels; they were replaced by the
# environment lookups above and a surviving one means a stale copy somewhere.
if grep -rqs "FILL_IN_" "$WEB/src" "$META" 2>/dev/null; then
  bad "FILL_IN_ placeholders still present:"
  grep -rlos "FILL_IN_" "$WEB/src" "$META" 2>/dev/null | sed 's/^/      /'
else
  ok "no FILL_IN_ placeholders left in the web sources or store metadata"
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Privacy manifest"
if [[ ! -f "$MANIFEST" ]]; then
  bad "$MANIFEST is missing — uploads get ITMS-91053 'Missing API declaration'"
elif ! plutil -lint "$MANIFEST" >/dev/null 2>&1; then
  bad "$MANIFEST is not a valid plist"
else
  ok "$MANIFEST exists and lints"
  plutil -extract NSPrivacyTracking raw -o - "$MANIFEST" 2>/dev/null | grep -qx false \
    && ok "NSPrivacyTracking is false" \
    || bad "NSPrivacyTracking must be present and false (the app does no tracking)"

  # Required-reason APIs: anything matched here must be declared, or Apple's
  # scanner mails ITMS-91053 after the upload. Extend both sides together.
  declared=$(plutil -convert json -o - "$MANIFEST" 2>/dev/null)
  check_api() { # pattern, category, human name
    if grep -rqsE "$1" --include="*.swift" RingPromoter/ RingPromoterWidget/ SharedWidget/; then
      if printf '%s' "$declared" | grep -q "$2"; then
        ok "$3 is used and declared ($2)"
      else
        bad "$3 is used in Swift but $2 is not declared in the manifest"
      fi
    elif printf '%s' "$declared" | grep -q "$2"; then
      warn "$2 is declared but no $3 use was found — remove the declaration"
    fi
  }
  check_api 'UserDefaults|@AppStorage'                      NSPrivacyAccessedAPICategoryUserDefaults   "UserDefaults"
  check_api '\.systemUptime|mach_absolute_time'             NSPrivacyAccessedAPICategorySystemBootTime "system boot time"
  check_api 'volumeAvailableCapacity|systemFreeSize'        NSPrivacyAccessedAPICategoryDiskSpace      "disk space"
  check_api 'contentModificationDate|\.creationDate|NSURLContentModificationDateKey' \
                                                            NSPrivacyAccessedAPICategoryFileTimestamp  "file timestamps"
  check_api 'activeInputModes'                              NSPrivacyAccessedAPICategoryActiveKeyboards "active keyboards"
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Info.plist"
plutil -lint "$PLIST" >/dev/null 2>&1 && ok "$PLIST lints" || bad "$PLIST is not a valid plist"
# Unanswered export compliance leaves the build at "Missing Compliance" and
# un-submittable until somebody answers the questionnaire by hand.
if plutil -extract ITSAppUsesNonExemptEncryption raw -o - "$PLIST" >/dev/null 2>&1; then
  ok "ITSAppUsesNonExemptEncryption is declared ($(plutil -extract ITSAppUsesNonExemptEncryption raw -o - "$PLIST"))"
else
  bad "ITSAppUsesNonExemptEncryption is missing — every upload will ask for export compliance"
fi
# A permission prompt with no usage string is an immediate crash on first use.
if grep -rqs "LAContext\|LocalAuthentication" --include="*.swift" RingPromoter/; then
  plutil -extract NSFaceIDUsageDescription raw -o - "$PLIST" >/dev/null 2>&1 \
    && ok "NSFaceIDUsageDescription is present (the app uses LocalAuthentication)" \
    || bad "the app uses LocalAuthentication but NSFaceIDUsageDescription is missing"
fi
# An ATS exception invites a review question this app has no reason to answer.
if plutil -extract NSAppTransportSecurity raw -o - "$PLIST" >/dev/null 2>&1; then
  warn "NSAppTransportSecurity is set — be ready to justify the exception in review notes"
else
  ok "no App Transport Security exceptions"
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Push notifications"
if grep -q "registerForRemoteNotifications" -r --include="*.swift" RingPromoter/; then
  grep -q "aps-environment" "$ENTS" \
    && ok "aps-environment is declared (the app registers for APNs)" \
    || bad "the app calls registerForRemoteNotifications() but $ENTS has no aps-environment — registration fails with a visible error"
  for want in development production; do
    grep -q "APS_ENVIRONMENT = $want;" "$PROJ" \
      && ok "APS_ENVIRONMENT = $want is set in the project" \
      || bad "APS_ENVIRONMENT = $want is not set — the entitlement expands to an empty string"
  done
  grep -q "ensure_push!" fastlane/Fastfile \
    && ok "fastlane enables the Push Notifications capability on the App ID" \
    || warn "fastlane does not enable Push Notifications — the profile may lack aps-environment"
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "App icon"
ICON=RingPromoter/Resources/Assets.xcassets/AppIcon.appiconset/AppIcon.png
if [[ ! -f "$ICON" ]]; then
  bad "$ICON is missing"
else
  read -r w h a <<<"$(sips -g pixelWidth -g pixelHeight -g hasAlpha "$ICON" 2>/dev/null \
    | awk '/pixelWidth/{w=$2} /pixelHeight/{h=$2} /hasAlpha/{a=$2} END{print w, h, a}')"
  [[ "$w" == 1024 && "$h" == 1024 ]] && ok "icon is 1024x1024" || bad "icon is ${w}x${h}, must be 1024x1024"
  # Apple rejects an app icon with an alpha channel at upload time.
  [[ "$a" == "no" ]] && ok "icon has no alpha channel" || bad "icon has an alpha channel — Apple rejects that"
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Screenshots"
FAMILY=$(sed -n 's/.*TARGETED_DEVICE_FAMILY = "\{0,1\}\([0-9,]*\)"\{0,1\};.*/\1/p' "$PROJ" | head -1)
LOCALE="${LOCALE:-en-GB}"
SHOTS="fastlane/screenshots/$LOCALE"
echo "  device family $FAMILY, locale $LOCALE"
if [[ ! -d "$SHOTS" ]] || ! compgen -G "$SHOTS/*.png" >/dev/null; then
  bad "no screenshots in $SHOTS — run ./scripts/capture-app-store-screenshots.sh"
else
  python3 - "$SHOTS" "$FAMILY" <<'PY'
import pathlib, subprocess, sys
from collections import Counter

accepted = {(1320, 2868): '6.9" iPhone', (1290, 2796): '6.9" iPhone',
            (2064, 2752): '13" iPad',    (2048, 2732): '13" iPad'}
shots, family = pathlib.Path(sys.argv[1]), sys.argv[2]
required = {'6.9" iPhone'} | ({'13" iPad'} if "2" in family.split(",") else set())

seen, bad = Counter(), []
for f in sorted(shots.glob("*.png")):
    out = subprocess.run(["sips", "-g", "pixelWidth", "-g", "pixelHeight", str(f)],
                         capture_output=True, text=True).stdout
    d = {k.strip(): int(v) for k, v in
         (l.split(":") for l in out.splitlines() if ":" in l and "pixel" in l)}
    size = (d["pixelWidth"], d["pixelHeight"])
    if size in accepted:
        seen[accepted[size]] += 1
    else:
        bad.append(f"{f.name} is {size[0]}x{size[1]}, not an accepted App Store size")

for label, n in sorted(seen.items()):
    print(f"  \033[32m✓\033[0m {n} x {label}")
    # App Store Connect takes at most 10 per display size and rejects the
    # upload above that, so it is cheaper to catch here than in deliver.
    if n > 10:
        bad.append(f"{n} {label} screenshots, App Store Connect allows 10")
for line in bad:
    print(f"  \033[31m✗\033[0m {line}")
for label in sorted(required - set(seen)):
    print(f"  \033[31m✗\033[0m no {label} screenshots, but the app ships for that device")
sys.exit(1 if bad or (required - set(seen)) else 0)
PY
  if [[ $? -eq 0 ]]; then PASS=$((PASS+1)); else FAIL=$((FAIL+1)); fi
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Store metadata"
python3 - "$META" "${LOCALE:-en-GB}" <<'PY'
import pathlib, sys
meta, locale = pathlib.Path(sys.argv[1]), sys.argv[2]
limits = {"name.txt": 30, "subtitle.txt": 30, "keywords.txt": 100,
          "promotional_text.txt": 170, "description.txt": 4000,
          "release_notes.txt": 4000}
required = list(limits) + ["support_url.txt", "privacy_url.txt"]
problems = []
d = meta / locale
if not d.is_dir():
    problems.append(f"{d} does not exist — it must match the App Store Connect primary language")
else:
    for name in required:
        f = d / name
        if not f.is_file() or not f.read_text().strip():
            problems.append(f"{locale}/{name} is missing or empty")
            continue
        n, lim = len(f.read_text().rstrip("\n")), limits.get(name)
        if lim and n > lim:
            problems.append(f"{locale}/{name} is {n} characters, limit is {lim}")
notes = meta / "review_information" / "notes.txt"
if not notes.is_file() or len(notes.read_text().strip()) < 200:
    problems.append("review_information/notes.txt is missing or too short — "
                    "a reviewer with no server needs the demo-mode walkthrough")
for p in problems:
    print(f"  \033[31m✗\033[0m {p}")
if not problems:
    print("  \033[32m✓\033[0m metadata present and within every character limit")
sys.exit(1 if problems else 0)
PY
if [[ $? -eq 0 ]]; then PASS=$((PASS+1)); else FAIL=$((FAIL+1)); fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Public pages"
for page in privacy terms support; do
  [[ -f "$WEB/src/app/$page/page.tsx" ]] \
    && ok "web/src/app/$page/page.tsx exists" \
    || bad "web/src/app/$page/page.tsx is missing — the listing links to /$page"
done
grep -q "for page in privacy terms support" ../.github/workflows/deploy-pages.yml 2>/dev/null \
  && ok "deploy-pages.yml publishes the legal pages" \
  || bad "deploy-pages.yml does not copy the legal pages into the published site"

if (( ONLINE )); then
  for url in \
    https://ring-promoter-pages.fictionally.org/privacy \
    https://ring-promoter-pages.fictionally.org/terms \
    https://ring-promoter-pages.fictionally.org/support; do
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 "$url")
    [[ "$code" == 200 ]] && ok "$url → 200" || bad "$url → $code (App Review opens this)"
  done
else
  warn "public URLs not fetched — re-run with --online once the site has deployed"
fi

# ─────────────────────────────────────────────────────────────────────────────
head_ "Shipping code"
if grep -rqsE "TODO|FIXME|XXX|Lorem ipsum" --include="*.swift" RingPromoter/ RingPromoterWidget/ SharedWidget/; then
  warn "TODO/FIXME markers in shipping code — check none are user-visible"
else
  ok "no TODO/FIXME markers in shipping code"
fi
V=$(sed -n 's/.*MARKETING_VERSION = \([^;]*\);.*/\1/p' "$PROJ" | head -1)
[[ -n "$V" ]] && ok "MARKETING_VERSION is $V" || bad "MARKETING_VERSION is not set"

# ─────────────────────────────────────────────────────────────────────────────
printf '\n\033[1m%s\033[0m\n' "────────────────────────────────"
printf '  %d passed, %d failed, %d to check by hand\n' "$PASS" "$FAIL" "$WARN"
if (( FAIL )); then
  printf '\n\033[31mNot ready to submit.\033[0m Fix the ✗ items above.\n'
  exit 1
fi
printf '\n\033[32mAutomated checks pass.\033[0m Still done by hand in App Store Connect:\n'
printf '  · App Privacy → "Data Not Collected"\n'
printf '  · Age rating → 4+\n'
printf '  · Sign-in required → No\n'
printf '  · Content rights → contains no third-party content\n'
printf '  See docs/APP-STORE-RELEASE.md for the full list.\n'
