#!/usr/bin/env bash
#
# locate-toolchain.sh — find a JDK and the Android SDK on whatever machine
# this is running on.
#
#   CI:    ./ci/locate-toolchain.sh           # appends to $GITHUB_ENV
#   local: eval "$(./ci/locate-toolchain.sh)" # exports to shell
#
# Both ci.yml and android-release.yml used to hardcode
# /Applications/Android Studio.app/Contents/jbr/Contents/Home. That path is
# real on a developer's laptop and absent on the self-hosted runner, so the
# first Android job ever to reach the runner died in half a second with
# "JAVA_HOME is set to an invalid directory" — and android-release.yml, which
# had never run, carried the same line and would have failed identically at
# the first step of a Play release.
#
# Searching beats asserting here: the two machines genuinely differ, and the
# failure when neither has a toolchain should say what was looked for rather
# than name one path and stop.
#
# Mirrors load-android-vault-secrets.sh: one place to look when either
# platform's build stops finding its tools.
#
set -euo pipefail

emit() {
  if [ -n "${GITHUB_ENV:-}" ]; then
    printf '%s=%s\n' "$1" "$2" >> "$GITHUB_ENV"
    printf 'using %s=%s\n' "$1" "$2" >&2
  else
    # Quoted: these paths contain spaces ("Android Studio.app"), and this form
    # is meant to be eval'd.
    printf 'export %s=%q\n' "$1" "$2"
  fi
}

# A directory is a usable JDK only if it can actually compile. Checking for
# bin/java alone matches the /usr/bin stub that has no runtime behind it.
is_jdk() {
  [ -n "$1" ] && [ -x "$1/bin/javac" ] && [ -x "$1/bin/java" ]
}

find_jdk() {
  # Already set and real wins: a runner that has deliberately configured one
  # should keep it.
  if is_jdk "${JAVA_HOME:-}"; then
    printf '%s' "$JAVA_HOME"
    return 0
  fi

  # macOS keeps a registry of installed JDKs. AGP 9.x wants 17 at minimum;
  # prefer 21, which is what the Gradle toolchain resolves to.
  if [ -x /usr/libexec/java_home ]; then
    for v in 21 17; do
      if home=$(/usr/libexec/java_home -v "$v" 2>/dev/null) && is_jdk "$home"; then
        printf '%s' "$home"
        return 0
      fi
    done
    if home=$(/usr/libexec/java_home 2>/dev/null) && is_jdk "$home"; then
      printf '%s' "$home"
      return 0
    fi
  fi

  for candidate in \
    "/Applications/Android Studio.app/Contents/jbr/Contents/Home" \
    "$HOME/Applications/Android Studio.app/Contents/jbr/Contents/Home" \
    /opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home \
    /opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
    /opt/homebrew/opt/openjdk/libexec/openjdk.jdk/Contents/Home \
    /usr/local/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home \
    /usr/lib/jvm/temurin-21-jdk-amd64 \
    /usr/lib/jvm/temurin-17-jdk-amd64
  do
    if is_jdk "$candidate"; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  return 1
}

find_sdk() {
  for candidate in \
    "${ANDROID_HOME:-}" \
    "${ANDROID_SDK_ROOT:-}" \
    "$HOME/Library/Android/sdk" \
    "$HOME/Android/Sdk" \
    /usr/local/share/android-sdk \
    /usr/local/lib/android/sdk \
    /opt/android-sdk
  do
    # platform-tools alone can exist in a half-installed SDK; a usable one has
    # the licence directory Gradle checks before it will download anything.
    if [ -n "$candidate" ] && [ -d "$candidate/licenses" ]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  # local.properties is not committed, but if someone has set one on this
  # machine it is authoritative for this checkout.
  if [ -f local.properties ]; then
    dir=$(sed -n 's/^sdk\.dir=//p' local.properties | head -1)
    if [ -n "$dir" ] && [ -d "$dir" ]; then
      printf '%s' "$dir"
      return 0
    fi
  fi
  return 1
}

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if jdk=$(find_jdk); then
  emit JAVA_HOME "$jdk"
  # To stderr: stdout carries the export lines and has to stay eval-able.
  "$jdk/bin/java" -version 2>&1 | sed 's/^/  /' >&2
else
  cat >&2 <<'EOF'
ERROR: no JDK found.

Looked at: $JAVA_HOME, /usr/libexec/java_home (21, 17, default),
Android Studio's bundled JBR, Homebrew openjdk, /usr/lib/jvm/temurin-*.

Install a JDK 17 or 21 on this runner, or add actions/setup-java to the
workflow so the job provisions its own.
EOF
  exit 1
fi

if sdk=$(find_sdk); then
  emit ANDROID_HOME "$sdk"
  emit ANDROID_SDK_ROOT "$sdk"
else
  cat >&2 <<'EOF'
ERROR: no Android SDK found.

Looked at: $ANDROID_HOME, $ANDROID_SDK_ROOT, ~/Library/Android/sdk,
~/Android/Sdk, /usr/local/share/android-sdk, /usr/local/lib/android/sdk,
/opt/android-sdk, and sdk.dir in local.properties.

The Android Gradle Plugin cannot configure the project without it. Install
the command line tools on this runner, or run the Android jobs on a
GitHub-hosted runner, whose images ship the SDK.
EOF
  exit 1
fi
