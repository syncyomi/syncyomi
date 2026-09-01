#!/usr/bin/env bash
# Verifies the E2E environment. Exit 0 = ready to run.
set -uo pipefail

SDK="${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}"
E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(dirname "$E2E_DIR")"
APK="${E2E_TACHIYOMI_APK:-$HOME/projects/TachiyomiSY/app/build/outputs/apk/debug/app-universal-debug.apk}"
JAR="${E2E_SUWAYOMI_JAR:-}"

fail=0
ok()   { printf '  \e[1;32mOK\e[0m   %s\n' "$*"; }
bad()  { printf '  \e[1;31mFAIL\e[0m %s\n' "$*"; fail=1; }
warn() { printf '  \e[1;33mWARN\e[0m %s\n' "$*"; }

echo "SyncYomi E2E doctor"

[ -r /dev/kvm ] && [ -w /dev/kvm ] && ok "/dev/kvm accessible" || bad "/dev/kvm not accessible"
command -v adb >/dev/null && ok "adb: $(adb --version | head -1)" || bad "adb not on PATH"
[ -x "$SDK/emulator/emulator" ] && ok "emulator: $("$SDK/emulator/emulator" -version 2>/dev/null | head -1)" || bad "emulator not installed (run setup-env.sh)"
[ -d "$SDK/system-images/android-35/google_apis/x86_64" ] && ok "system image android-35 google_apis x86_64" || bad "system image missing (run setup-env.sh)"
AVD_HOME="${ANDROID_AVD_HOME:-$HOME/.android/avd}"
for avd in syncE2E-a syncE2E-b; do
    [ -f "$AVD_HOME/$avd.ini" ] && ok "AVD $avd" || bad "AVD $avd missing (run setup-env.sh)"
done
[ -x "$E2E_DIR/.tools/maestro/bin/maestro" ] && ok "maestro: $("$E2E_DIR/.tools/maestro/bin/maestro" --version 2>/dev/null)" || bad "maestro missing (run setup-env.sh)"
command -v java >/dev/null && ok "java: $(java -version 2>&1 | head -1)" || bad "java not on PATH (maestro needs 17+)"
command -v go >/dev/null && ok "go: $(go version)" || bad "go not on PATH"
[ -d "$REPO_DIR/web/dist" ] && ok "web/dist present (server embeddable)" || bad "web/dist missing — run: cd web && pnpm install && pnpm build"

[ -f "$APK" ] && ok "TachiyomiSY APK: $APK" || warn "APK not found at $APK (set E2E_TACHIYOMI_APK or build :app:assembleDebug)"
if [ -n "$JAR" ]; then
    [ -f "$JAR" ] && ok "Suwayomi jar: $JAR" || warn "Suwayomi jar not found at $JAR"
else
    warn "E2E_SUWAYOMI_JAR not set (only needed for Suwayomi scenarios)"
fi

for port in 8790 8791 8792 4568 5554 5556; do
    if ss -ltn "sport = :$port" 2>/dev/null | grep -q ":$port"; then
        warn "port $port in use (suite expects it free)"
    else
        ok "port $port free"
    fi
done

exit $fail
