//go:build e2e

package scenarios

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
)

const (
	fixtureAManga    = 5
	fixtureAChapters = 3

	mainPort   = 8790
	stagePortA = 8791
	stagePortB = 8792
)

// TestS1_FirstSyncPairingUI: the server holds a seeded library; devices A and B
// pair with it through the real settings UI and both end up with that library.
func TestS1_FirstSyncPairingUI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	seedServer(t, ctx, srv, "E2E Alpha")

	resetApp(t, ctx, emuA, srv)
	resetApp(t, ctx, emuB, srv)
	syncViaUI(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)
	syncViaUI(t, ctx, emuB, srv)
	awaitLibrary(t, ctx, emuB, fixtureAManga)

	wantTitles := harness.FixtureTitles("E2E Alpha", fixtureAManga)
	for _, e := range []*harness.Emulator{emuA, emuB} {
		got, err := libraryTitles(t, ctx, e)
		if err != nil {
			t.Fatalf("read %s library: %v", e.AVD, err)
		}
		if !reflect.DeepEqual(got, wantTitles) {
			t.Errorf("%s library = %v, want %v", e.AVD, got, wantTitles)
		}
	}

	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatalf("server snapshot: %v", err)
	}
	if got := harness.SnapshotTitles(snap); !reflect.DeepEqual(got, wantTitles) {
		t.Errorf("server snapshot titles = %v, want %v", got, wantTitles)
	}

	devices, err := srv.Devices(ctx)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	real := 0
	for _, d := range devices {
		if d.DeviceName == "e2e-synthetic" {
			continue
		}
		real++
		if d.Protocol != "v2" {
			t.Errorf("device %s used protocol %q, want v2", d.DeviceName, d.Protocol)
		}
		if d.LastEvent != "SYNC_SUCCESS" {
			t.Errorf("device %s last event %q, want SYNC_SUCCESS", d.DeviceName, d.LastEvent)
		}
	}
	if real != 2 {
		t.Errorf("server saw %d real devices, want 2", real)
	}
}

// TestS2_BidirectionalMerge: A and B build disjoint libraries against separate
// staging servers, then meet on one server and converge to the union.
func TestS2_BidirectionalMerge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	stageA := startServer(t, stagePortA)
	stageB := startServer(t, stagePortB)
	main := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, main, emuA, emuB)

	seedServer(t, ctx, stageA, "E2E Alpha")
	seedServer(t, ctx, stageB, "E2E Beta")

	resetApp(t, ctx, emuA, stageA)
	resetApp(t, ctx, emuB, stageB)
	syncViaBroadcast(t, ctx, emuA, stageA)
	awaitLibrary(t, ctx, emuA, fixtureAManga)
	syncViaBroadcast(t, ctx, emuB, stageB)
	awaitLibrary(t, ctx, emuB, fixtureAManga)

	repoint(t, ctx, emuA, main)
	repoint(t, ctx, emuB, main)
	syncViaBroadcast(t, ctx, emuA, main)
	syncViaBroadcast(t, ctx, emuB, main)
	awaitLibrary(t, ctx, emuB, 2*fixtureAManga)
	syncViaBroadcast(t, ctx, emuA, main)
	awaitLibrary(t, ctx, emuA, 2*fixtureAManga)

	union := append(harness.FixtureTitles("E2E Alpha", fixtureAManga),
		harness.FixtureTitles("E2E Beta", fixtureAManga)...)

	for _, e := range []*harness.Emulator{emuA, emuB} {
		got, err := libraryTitles(t, ctx, e)
		if err != nil {
			t.Fatalf("read %s library: %v", e.AVD, err)
		}
		if !sameStringSet(got, union) {
			t.Errorf("%s library = %v, want union %v", e.AVD, got, union)
		}
		if len(got) != len(union) {
			t.Errorf("%s library has %d entries, want %d (duplicates?)", e.AVD, len(got), len(union))
		}
	}

	snap, err := main.Snapshot(ctx)
	if err != nil {
		t.Fatalf("server snapshot: %v", err)
	}
	if got := harness.SnapshotTitles(snap); !sameStringSet(got, union) {
		t.Errorf("server snapshot = %v, want union", got)
	}
}

// syncViaUI drives the Sync now button, then waits for the server to record the
// sync AND for the app to finish applying the response (the pref write is the
// last step of a client sync — stopping the app before it lands loses data).
func syncViaUI(t *testing.T, ctx context.Context, e *harness.Emulator, srv *harness.SyncServer) {
	t.Helper()
	prev, _ := e.LastSyncTimestamp(ctx)
	start := time.Now()
	if err := e.RunFlow(ctx, harness.FlowPath("sync_now.yaml"), artifactDir, nil); err != nil {
		t.Fatalf("sync_now flow on %s: %v", e.AVD, err)
	}
	awaitSync(t, ctx, e, srv, prev, start)
}

// syncViaBroadcast triggers sync via the debug receiver (no UI).
func syncViaBroadcast(t *testing.T, ctx context.Context, e *harness.Emulator, srv *harness.SyncServer) {
	t.Helper()
	if err := e.LaunchApp(ctx); err != nil {
		t.Fatalf("launch app on %s: %v", e.AVD, err)
	}
	prev, _ := e.LastSyncTimestamp(ctx)
	start := time.Now()
	if err := e.TriggerSyncBroadcast(ctx); err != nil {
		t.Fatalf("trigger sync on %s: %v", e.AVD, err)
	}
	awaitSync(t, ctx, e, srv, prev, start)
}

func awaitSync(t *testing.T, ctx context.Context, e *harness.Emulator, srv *harness.SyncServer, prevClientTS int64, start time.Time) {
	t.Helper()
	if _, err := srv.WaitForDeviceSync(ctx, start, 90*time.Second); err != nil {
		t.Fatalf("sync from %s never reached the server: %v", e.AVD, err)
	}
	if err := e.WaitForClientSync(ctx, prevClientTS, 60*time.Second); err != nil {
		t.Fatalf("sync on %s never finished applying: %v", e.AVD, err)
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, s := range a {
		set[s]++
	}
	for _, s := range b {
		set[s]--
	}
	for _, n := range set {
		if n != 0 {
			return false
		}
	}
	return true
}
