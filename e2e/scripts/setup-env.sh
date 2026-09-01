#!/usr/bin/env bash
# Idempotent environment setup for the SyncYomi E2E suite.
# Installs the Android emulator, a system image, two AVDs, and a pinned Maestro CLI.
set -euo pipefail

SDK="${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}"
SDKMANAGER="$SDK/cmdline-tools/latest/bin/sdkmanager"
AVDMANAGER="$SDK/cmdline-tools/latest/bin/avdmanager"
SYSIMG="system-images;android-35;google_apis;x86_64"
E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="$E2E_DIR/.tools"
MAESTRO_VERSION="2.9.0"

log() { printf '\e[1;34m[setup]\e[0m %s\n' "$*"; }
die() { printf '\e[1;31m[setup] ERROR:\e[0m %s\n' "$*" >&2; exit 1; }

[ -e /dev/kvm ] || die "/dev/kvm not present; emulator acceleration unavailable"
[ -r /dev/kvm ] && [ -w /dev/kvm ] || die "/dev/kvm not readable/writable by $USER"
[ -x "$SDKMANAGER" ] || die "sdkmanager not found at $SDKMANAGER"

log "accepting SDK licenses"
yes | "$SDKMANAGER" --licenses >/dev/null 2>&1 || true

if [ ! -x "$SDK/emulator/emulator" ]; then
    log "installing emulator package"
    "$SDKMANAGER" "emulator"
else
    log "emulator already installed"
fi

if [ ! -d "$SDK/system-images/android-35/google_apis/x86_64" ]; then
    log "installing $SYSIMG (large download)"
    "$SDKMANAGER" "$SYSIMG"
else
    log "system image already installed"
fi

AVD_HOME="${ANDROID_AVD_HOME:-$HOME/.android/avd}"

create_avd() {
    local name="$1" ini="$AVD_HOME/$1.ini"
    if [ -f "$ini" ]; then
        log "AVD $name already exists"
    else
        log "creating AVD $name"
        echo no | "$AVDMANAGER" create avd -n "$name" -k "$SYSIMG" -d pixel_6
    fi
    # avdmanager records the actual AVD directory in the .ini — don't assume it.
    local avd_dir
    avd_dir="$(grep '^path=' "$ini" | head -1 | cut -d= -f2-)"
    [ -n "$avd_dir" ] && [ -d "$avd_dir" ] || die "AVD dir for $name not found (ini: $ini)"
    local cfg="$avd_dir/config.ini"
    # Idempotent config pinning: drop any prior value, append ours.
    for kv in "hw.ramSize=2048" "disk.dataPartition.size=6G" "hw.keyboard=yes" "hw.gpu.enabled=yes" "hw.gpu.mode=swiftshader_indirect"; do
        local key="${kv%%=*}"
        grep -v "^$key" "$cfg" > "$cfg.tmp" && mv "$cfg.tmp" "$cfg"
        echo "$kv" >> "$cfg"
    done
}
create_avd syncE2E-a
create_avd syncE2E-b

if [ ! -x "$TOOLS_DIR/maestro/bin/maestro" ]; then
    log "installing Maestro CLI $MAESTRO_VERSION into $TOOLS_DIR"
    mkdir -p "$TOOLS_DIR"
    tmp="$(mktemp -d)"
    curl -fsSL -o "$tmp/maestro.zip" \
        "https://github.com/mobile-dev-inc/Maestro/releases/download/cli-$MAESTRO_VERSION/maestro.zip"
    unzip -q "$tmp/maestro.zip" -d "$tmp"
    rm -rf "$TOOLS_DIR/maestro"
    mv "$tmp/maestro" "$TOOLS_DIR/maestro"
    rm -rf "$tmp"
else
    log "Maestro already installed ($("$TOOLS_DIR/maestro/bin/maestro" --version 2>/dev/null || echo unknown))"
fi

log "done — run doctor.sh to verify"
