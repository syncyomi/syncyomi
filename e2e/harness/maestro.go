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

type flowStep struct {
	Kind   string
	Status string
	Depth  int
	Error  string
}

type flowVerdict struct {
	passed    bool
	retryable bool
	reason    string
}

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

func E2ERoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

func RepoRoot() string {
	return filepath.Dir(E2ERoot())
}

func maestroBin() string {
	return filepath.Join(E2ERoot(), ".tools", "maestro", "bin", "maestro")
}

func FlowPath(name string) string {
	return filepath.Join(E2ERoot(), "flows", name)
}
