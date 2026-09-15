package harness

import (
	"path/filepath"
	"strings"
	"testing"
)

func loadSteps(t *testing.T, name string) []flowStep {
	t.Helper()
	steps, err := parseFlowSteps(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

func TestJudgeFlow(t *testing.T) {
	const markRead, syncNow = 7, 8

	failedAfterLaunch := loadSteps(t, "commands_pass.json")
	failedAfterLaunch[5].Status = "FAILED"
	failedAfterLaunch[5].Error = "Element not found: Text matching regex: More"
	failedAfterLaunch = failedAfterLaunch[:6]

	tests := []struct {
		name      string
		steps     []flowStep
		topLevel  int
		passed    bool
		retryable bool
		reason    string
	}{
		{"offline at launchApp", loadSteps(t, "commands_offline_at_launch.json"), markRead, false, true, "launchAppCommand FAILED: Command failed (host:transport:emulator-5554): device offline"},
		{"Wait tapped then offline at launchApp", loadSteps(t, "commands_wait_then_offline.json"), markRead, false, true, "launchAppCommand FAILED"},
		{"all steps completed (teardown crash)", loadSteps(t, "commands_pass.json"), syncNow, true, false, ""},
		{"transcript cut short after launch", loadSteps(t, "commands_pass.json"), syncNow + 1, false, false, "flow stopped after 8 of 9 top-level steps"},
		{"failed after launch", failedAfterLaunch, syncNow, false, false, "tapOnElement FAILED: Element not found"},
		{"transport dropped during first step", loadSteps(t, "commands_killed_before_launch.json"), markRead, false, true, "runFlowCommand RUNNING"},
		{"nothing recorded", nil, markRead, false, true, "no step ran"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := judgeFlow(tt.steps, tt.topLevel)
			if v.passed != tt.passed || v.retryable != tt.retryable {
				t.Fatalf("got passed=%v retryable=%v (%s), want passed=%v retryable=%v", v.passed, v.retryable, v.reason, tt.passed, tt.retryable)
			}
			if !strings.Contains(v.reason, tt.reason) {
				t.Fatalf("reason %q does not contain %q", v.reason, tt.reason)
			}
		})
	}
}

func TestParseFlowSteps(t *testing.T) {
	steps := loadSteps(t, "commands_wait_then_offline.json")
	want := []flowStep{
		{Kind: "defineVariablesCommand", Status: "COMPLETED"},
		{Kind: "applyConfigurationCommand", Status: "COMPLETED"},
		{Kind: "runFlowCommand", Status: "COMPLETED"},
		{Kind: "tapOnElement", Status: "COMPLETED", Depth: 1},
		{Kind: "launchAppCommand", Status: "FAILED", Error: "Command failed (host:transport:emulator-5554): device offline"},
	}
	if len(steps) != len(want) {
		t.Fatalf("got %d steps, want %d", len(steps), len(want))
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Errorf("step %d = %+v, want %+v", i, steps[i], want[i])
		}
	}
}

func TestFlowTopLevelCommands(t *testing.T) {
	flows, err := filepath.Glob(FlowPath("*.yaml"))
	if err != nil || len(flows) == 0 {
		t.Fatalf("no flows found: %v", err)
	}
	for _, f := range flows {
		if n, err := flowTopLevelCommands(f); err != nil || n == 0 {
			t.Errorf("%s: n=%d err=%v", filepath.Base(f), n, err)
		}
	}
	for name, want := range map[string]int{"mark_read.yaml": 7, "sync_now.yaml": 8} {
		if n, _ := flowTopLevelCommands(FlowPath(name)); n != want {
			t.Errorf("%s: got %d top-level commands, want %d", name, n, want)
		}
	}
}
