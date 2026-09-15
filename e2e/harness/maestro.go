package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const maxFlowAttempts = 3

// RunFlow executes a Maestro flow YAML against this emulator, writing Maestro
// output under artifactDir/maestro/<flow>-<avd>/attempt-N/. flowEnv values are
// exposed to the flow as ${KEY}.
//
// The verdict comes from Maestro's per-command metadata (commands.json in the
// debug output) rather than the exit status or the transcript: the CLI prints
// no FAILED marker for a step that dies with an IOException, and on
// resource-starved runners it sometimes crashes at teardown after every step
// ran. A failure at or before launchApp left the app untouched, so it is
// retried once the adb transport is healthy again; a later failure is not
// (steps are not idempotent) and fails loudly.
func (e *Emulator) RunFlow(ctx context.Context, flowPath, artifactDir string, flowEnv map[string]string) error {
	bin := maestroBin()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("maestro not installed at %s — run e2e/scripts/setup-env.sh", bin)
	}
	topLevel, err := flowTopLevelCommands(flowPath)
	if err != nil {
		return err
	}
	flowName := strings.TrimSuffix(filepath.Base(flowPath), filepath.Ext(flowPath))
	outDir := filepath.Join(artifactDir, "maestro", filepath.Base(flowPath)+"-"+e.AVD)

	var lastErr error
	for attempt := 1; attempt <= maxFlowAttempts; attempt++ {
		if err := e.WaitForDevice(ctx); err != nil {
			return err
		}
		attemptDir := filepath.Join(outDir, fmt.Sprintf("attempt-%d", attempt))
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			return err
		}
		args := []string{"--device", e.Serial, "test", flowPath, "--debug-output", attemptDir}
		for k, v := range flowEnv {
			args = append(args, "-e", k+"="+v)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "MAESTRO_CLI_NO_ANALYTICS=1", "MAESTRO_CLI_ANALYSIS_NOTIFICATION_DISABLED=true")
		out, runErr := cmd.CombinedOutput()
		_ = os.WriteFile(filepath.Join(attemptDir, "maestro.log"), out, 0o644)
		if runErr == nil {
			return nil
		}
		steps, err := readFlowSteps(attemptDir, flowName)
		if err != nil {
			return err
		}
		v := judgeFlow(steps, topLevel)
		if v.passed {
			return nil
		}
		lastErr = fmt.Errorf("maestro flow %s on %s attempt %d: %s (debug output: %s): %w",
			filepath.Base(flowPath), e.AVD, attempt, v.reason, attemptDir, runErr)
		if !v.retryable || ctx.Err() != nil {
			return lastErr
		}
	}
	return lastErr
}

// flowStep is one entry of Maestro's commands.json.
type flowStep struct {
	Kind   string // e.g. launchAppCommand, tapOnElement
	Status string // COMPLETED, SKIPPED, FAILED, ...
	Depth  int    // 0 for top-level commands, 1+ for runFlow children
	Error  string
}

type flowVerdict struct {
	passed    bool
	retryable bool
	reason    string
}

// judgeFlow classifies a non-zero Maestro exit from the recorded steps.
// topLevel is the number of top-level commands in the flow YAML; Maestro
// records two synthetic depth-0 entries (defineVariables, applyConfiguration)
// before them, which is how a transcript cut short is told apart from a
// teardown crash after a complete run.
func judgeFlow(steps []flowStep, topLevel int) flowVerdict {
	if len(steps) == 0 {
		return flowVerdict{retryable: true, reason: "no step ran"}
	}
	launched := false
	recorded := 0
	for _, s := range steps {
		switch s.Status {
		case "COMPLETED", "SKIPPED":
		default:
			reason := fmt.Sprintf("%s %s", s.Kind, s.Status)
			if s.Error != "" {
				reason += ": " + s.Error
			}
			return flowVerdict{retryable: !launched, reason: reason}
		}
		if s.Depth == 0 {
			recorded++
		}
		if s.Kind == "launchAppCommand" {
			launched = true
		}
	}
	if want := topLevel + 2; recorded < want {
		return flowVerdict{
			retryable: !launched,
			reason:    fmt.Sprintf("flow stopped after %d of %d top-level steps with no failure recorded", recorded-2, topLevel),
		}
	}
	return flowVerdict{passed: true}
}

// readFlowSteps loads commands.json from a --debug-output directory. It is
// absent when Maestro died before running the first command.
func readFlowSteps(debugDir, flowName string) ([]flowStep, error) {
	matches, err := filepath.Glob(filepath.Join(debugDir, ".maestro", "tests", "*", flowName, "commands.json"))
	if err != nil || len(matches) == 0 {
		return nil, err
	}
	sort.Strings(matches)
	return parseFlowSteps(matches[len(matches)-1])
}

func parseFlowSteps(path string) ([]flowStep, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Command  map[string]json.RawMessage `json:"command"`
		Metadata struct {
			Status string `json:"status"`
			Depth  int    `json:"depth"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	steps := make([]flowStep, 0, len(raw))
	for _, r := range raw {
		s := flowStep{Status: r.Metadata.Status, Depth: r.Metadata.Depth}
		for kind := range r.Command {
			s.Kind = kind
		}
		if r.Metadata.Error != nil {
			s.Error = r.Metadata.Error.Message
		}
		steps = append(steps, s)
	}
	return steps, nil
}

// flowTopLevelCommands counts the top-level commands of a flow YAML: the "- "
// list items after the "---" document separator. The flows in e2e/flows are
// plain lists, so a line scan is enough.
func flowTopLevelCommands(flowPath string) (int, error) {
	f, err := os.Open(flowPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	inBody := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.TrimSpace(line) == "---":
			inBody = true
		case inBody && strings.HasPrefix(line, "- "):
			n++
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fmt.Errorf("%s: no top-level commands found", flowPath)
	}
	return n, nil
}

// E2ERoot is the absolute path of the e2e/ directory, resolved from this source file.
func E2ERoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

// RepoRoot is the syncyomi repository root.
func RepoRoot() string {
	return filepath.Dir(E2ERoot())
}

func maestroBin() string {
	return filepath.Join(E2ERoot(), ".tools", "maestro", "bin", "maestro")
}

// FlowPath resolves a flow YAML by name from e2e/flows.
func FlowPath(name string) string {
	return filepath.Join(E2ERoot(), "flows", name)
}
