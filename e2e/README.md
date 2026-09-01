# SyncYomi E2E suite

End-to-end tests for sync protocol v2 across real clients: two Android
TachiyomiSY apps on headless emulators, and Suwayomi-Server, all talking to a
real SyncYomi server booted from this repo.

## Requirements

- Linux with KVM (`/dev/kvm` accessible)
- Android SDK at `~/Android/Sdk` (or `$ANDROID_SDK_ROOT`) with cmdline-tools
- Go, JDK 17+ (Maestro), `adb` on PATH
- A TachiyomiSY debug APK (`./gradlew :app:assembleDebug` in that repo)
- For Suwayomi scenarios: a Suwayomi-Server shadowJar (`./gradlew :server:shadowJar`)

## Setup (once)

```sh
e2e/scripts/setup-env.sh   # emulator + system image + AVDs + Maestro
e2e/scripts/doctor.sh      # verify everything
```

## Run

```sh
e2e/scripts/run-e2e.sh                 # whole suite
e2e/scripts/run-e2e.sh -run TestS1     # one scenario
```

Env vars:

| Var | Default | Meaning |
|---|---|---|
| `E2E_TACHIYOMI_APK` | `~/projects/TachiyomiSY/app/build/outputs/apk/debug/app-x86_64-debug.apk` | APK under test |
| `E2E_SUWAYOMI_JAR` | unset | Suwayomi shadowJar (Suwayomi scenarios skip without it) |
| `E2E_KEEP` | unset | `1` leaves emulators running after the suite for debugging |

## How it works

- `harness/` — Go orchestration: boots the server (fresh data dir, HTTP
  onboarding, generated API key), two AVDs (`syncE2E-a/b`, headless), installs
  the APK, seeds sync settings by writing the app's SharedPreferences via
  `adb run-as` (host is `http://10.0.2.2:<port>`, the emulator's alias for this
  machine), and asserts on three oracles: the decoded `/api/sync/v2/snapshot`
  protobuf, the pulled `tachiyomi.db`, and Suwayomi's GraphQL API.
- Library seeding goes **through the sync protocol**: a `SyntheticClient` in the
  harness pushes a generated fixture backup to the server via `/api/sync/v2/merge`,
  and the device pulls it on its first sync. Divergent per-device state (for merge
  scenarios) is staged against throwaway servers on ports 8791/8792 before the
  devices are re-pointed at the main server. In-app backup restore is not used —
  it crashes with SQLITE_BUSY on current builds
  (jobobby04/TachiyomiSY#1634); `flows/restore_backup.yaml` stays around for when
  that is fixed.
- Sync triggers: the debug-only `SyncTriggerReceiver` broadcast
  (`<pkg>.TRIGGER_SYNC`, debug source set of TachiyomiSY) for most scenarios;
  S1 exercises the real "Sync now" settings UI via Maestro once per run.
- `flows/` — the few Maestro UI flows that can't be replaced by adb.
- `scenarios/` — the tests (`//go:build e2e`), one fresh server per test,
  emulators booted once per run.
- Failures dump logcat, prefs, server snapshot/devices into
  `artifacts/<run>/<test>/`.

## Scenarios

| Test | What it proves |
|---|---|
| S1 pairing (UI) | Two devices pair with a seeded server through the real "Sync now" settings UI |
| S2 bidirectional merge | Disjoint libraries staged on separate servers converge to the union |
| S3 read progress | Mark-as-read via the real UI reaches the server and the other device, and re-syncs don't regress it |
| S4 category tombstone | `X-Sync-Deleted-Categories` removes the category everywhere with no resurrection |
| S5 conflict | Concurrent chapter-read (device) and category-move (remote, higher version) both survive |
| S6 stale cursor | A device several generations behind converges without duplicates |
| S7 Android⇄Suwayomi | Both directions through the server, asserted via Suwayomi GraphQL |
| S8 soak (skipped with `-short`) | The scrubbed real-library fixture stays byte-stable across repeated syncs |
| S10 category rename (UI) | A rename keeps the category uid, doesn't duplicate, and reaches the other device |
| S11 category reorder | A remote position swap lands on the device and survives its next push |
| S12 create + assign (UI) | A category created and populated through the real UI reaches server and peer |
| S13 device tombstone (UI) | Deleting a category on-device sends `X-Sync-Deleted-Categories`; no resurrection |
| S14 Suwayomi core convergence | Suwayomi applies read progress, category rename/membership and tombstones, and pushes its own edits back |
| S15 cross-platform deep sync | Android UI edits (read + category assign) reach Suwayomi; Suwayomi edits come back to Android |

Device-originated *drag* reorder is a known gap: the drag handle has no
accessibility label, so Maestro can't grip it; S11 covers reorder propagation
from the server side instead.

v1-fallback (S9) is not covered: this server always speaks v2, so the 404-probe
path needs an old server build — test it manually when touching the fallback code.

## CI

`.github/workflows/e2e.yml` runs the whole suite on GitHub-hosted runners
(KVM-accelerated emulators): nightly, on demand via *Run workflow* (with
overridable TachiyomiSY/Suwayomi refs), and on PRs touching sync-relevant paths
(`internal/`, `proto/`, `e2e/`). The APK and shadowJar are built in parallel
jobs from the `feat/syncyomi-v2` fork branches with Gradle caching; the
emulator, system image, AVDs and Maestro are cached between runs. On failure
the run uploads `e2e/artifacts/` (logcat, Maestro screenshots, server logs) as
a workflow artifact. Expect ~25–30 min warm, ~45 min on cold caches.

## Ports

8790 (SyncYomi server), 4568 (Suwayomi), 5554/5556 (emulators). The doctor
warns if any are taken.
