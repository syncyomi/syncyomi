#!/usr/bin/env bash
set -euo pipefail

SDK="${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}"
SDKMANAGER="$SDK/cmdline-tools/latest/bin/sdkmanager"
AVDMANAGER="$SDK/cmdline-tools/latest/bin/avdmanager"
SYSIMG="system-images;android-35;google_apis;x86_64"
E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="$E2E_DIR/.tools"
MAESTRO_VERSION="2.9.0"
EMULATOR_VERSION="37.1.11"
EMULATOR_BUILD="15917651"
EMULATOR_SHA1="1b1f78891abf8ec268264356e1365c25519e8379"

log() { printf '\e[1;34m[setup]\e[0m %s\n' "$*"; }
die() { printf '\e[1;31m[setup] ERROR:\e[0m %s\n' "$*" >&2; exit 1; }

[ -e /dev/kvm ] || die "/dev/kvm not present; emulator acceleration unavailable"
[ -r /dev/kvm ] && [ -w /dev/kvm ] || die "/dev/kvm not readable/writable by $USER"
[ -x "$SDKMANAGER" ] || die "sdkmanager not found at $SDKMANAGER"

log "accepting SDK licenses"
yes | "$SDKMANAGER" --licenses >/dev/null 2>&1 || true

if [ ! -d "$SDK/system-images/android-35/google_apis/x86_64" ]; then
    log "installing $SYSIMG (large download)"
    "$SDKMANAGER" "$SYSIMG"
else
    log "system image already installed"
fi

emulator_revision() { grep -s '^Pkg.Revision=' "$SDK/emulator/source.properties" | cut -d= -f2; }
emulator_registered() { [ "$(emulator_revision)" = "$EMULATOR_VERSION" ] && [ -f "$SDK/emulator/package.xml" ]; }
write_emulator_package_xml() {
    IFS=. read -r major minor micro <<<"$EMULATOR_VERSION"
    cat > "$SDK/emulator/package.xml" <<EOF
<?xml version="1.0" encoding="UTF-8" standalone="yes"?><ns2:repository xmlns:ns2="http://schemas.android.com/repository/android/common/02" xmlns:ns5="http://schemas.android.com/repository/android/generic/02"><localPackage path="emulator" obsolete="false"><type-details xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="ns5:genericDetailsType"/><revision><major>$major</major><minor>$minor</minor><micro>$micro</micro></revision><display-name>Android Emulator</display-name></localPackage></ns2:repository>
EOF
}
if emulator_registered; then
    log "emulator $EMULATOR_VERSION already installed"
else
    log "installing emulator $EMULATOR_VERSION (build $EMULATOR_BUILD)"
    tmp="$(mktemp -d)"
    curl -fsSL -o "$tmp/emulator.zip" "https://dl.google.com/android/repository/emulator-linux_x64-$EMULATOR_BUILD.zip"
    echo "$EMULATOR_SHA1  $tmp/emulator.zip" | sha1sum -c --quiet || die "emulator download does not match $EMULATOR_SHA1"
    unzip -q "$tmp/emulator.zip" -d "$tmp"
    rm -rf "$SDK/emulator"
    mv "$tmp/emulator" "$SDK/emulator"
    rm -rf "$tmp"
    write_emulator_package_xml
    emulator_registered || die "emulator $EMULATOR_VERSION did not install"
fi

export ANDROID_AVD_HOME="${ANDROID_AVD_HOME:-$HOME/.android/avd}"
AVD_HOME="$ANDROID_AVD_HOME"
mkdir -p "$AVD_HOME"

create_avd() {
    local name="$1" ini="$AVD_HOME/$1.ini"
    if [ -f "$ini" ]; then
        log "AVD $name already exists"
    else
        log "creating AVD $name"
        echo no | "$AVDMANAGER" create avd -n "$name" -k "$SYSIMG" -d pixel_6
    fi
    local avd_dir
    avd_dir="$(grep '^path=' "$ini" | head -1 | cut -d= -f2-)"
    [ -n "$avd_dir" ] && [ -d "$avd_dir" ] || die "AVD dir for $name not found (ini: $ini)"
    local cfg="$avd_dir/config.ini"
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
