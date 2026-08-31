package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// RunFlow executes a Maestro flow YAML against this emulator, writing Maestro
// output under artifactDir. flowEnv values are exposed to the flow as ${KEY}.
func (e *Emulator) RunFlow(ctx context.Context, flowPath, artifactDir string, flowEnv map[string]string) error {
	bin := maestroBin()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("maestro not installed at %s — run e2e/scripts/setup-env.sh", bin)
	}
	outDir := filepath.Join(artifactDir, "maestro", filepath.Base(flowPath)+"-"+e.AVD)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	args := []string{"--device", e.Serial, "test", flowPath, "--debug-output", outDir}
	for k, v := range flowEnv {
		args = append(args, "-e", k+"="+v)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "MAESTRO_CLI_NO_ANALYTICS=1", "MAESTRO_CLI_ANALYSIS_NOTIFICATION_DISABLED=true")
	out, err := cmd.CombinedOutput()
	logPath := filepath.Join(outDir, "maestro.log")
	_ = os.WriteFile(logPath, out, 0o644)
	if err != nil {
		return fmt.Errorf("maestro flow %s on %s failed (log: %s): %w", filepath.Base(flowPath), e.AVD, logPath, err)
	}
	return nil
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
