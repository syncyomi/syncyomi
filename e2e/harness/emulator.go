package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const bootTimeout = 180 * time.Second

type Emulator struct {
	AVD    string
	Serial string
	Port   int

	cmd     *exec.Cmd
	logFile *os.File
}

func sdkRoot() string {
	if v := os.Getenv("ANDROID_SDK_ROOT"); v != "" {
		return v
	}
	if v := os.Getenv("ANDROID_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Android", "Sdk")
}

func StartEmulator(ctx context.Context, avd string, port int, artifactDir string, wipeData bool) (*Emulator, error) {
	e := &Emulator{AVD: avd, Serial: fmt.Sprintf("emulator-%d", port), Port: port}

	logPath := filepath.Join(artifactDir, fmt.Sprintf("emulator-%s.log", avd))
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	e.logFile = logFile

	args := []string{
		"-avd", avd,
		"-port", fmt.Sprint(port),
		"-no-window", "-no-audio", "-no-boot-anim", "-no-snapshot",
		"-gpu", "swiftshader_indirect",
	}
	if wipeData {
		args = append(args, "-wipe-data")
	}
	cmd := exec.CommandContext(ctx, filepath.Join(sdkRoot(), "emulator", "emulator"), args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start emulator %s: %w", avd, err)
	}
	e.cmd = cmd

	if err := e.waitBooted(ctx); err != nil {
		e.Stop()
		if !wipeData {
			return StartEmulator(ctx, avd, port, artifactDir, true)
		}
		return nil, err
	}
	e.settle(ctx)
	return e, nil
}

func (e *Emulator) settle(ctx context.Context) {
	for _, key := range []string{"window_animation_scale", "transition_animation_scale", "animator_duration_scale"} {
		_, _ = e.Adb(ctx, "shell", "settings", "put", "global", key, "0")
	}
	_, _ = e.Adb(ctx, "logcat", "-G", "32M")
	time.Sleep(10 * time.Second)
}

func (e *Emulator) waitBooted(ctx context.Context) error {
	deadline := time.Now().Add(bootTimeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		booted, _ := e.adbOnce(ctx, "shell", "getprop", "sys.boot_completed")
		if strings.TrimSpace(booted) == "1" {
			if out, err := e.adbOnce(ctx, "shell", "pm", "path", "android"); err == nil && strings.Contains(out, "package:") {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("emulator %s not booted after %s", e.AVD, bootTimeout)
}

func adbCommand(ctx context.Context, serial string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "adb", append([]string{"-s", serial}, args...)...)
}

func (e *Emulator) Adb(ctx context.Context, args ...string) (string, error) {
	out, err := e.adbOnce(ctx, args...)
	if err != nil && e.isTransportError(out) {
		if werr := e.WaitForDevice(ctx); werr == nil {
			out, err = e.adbOnce(ctx, args...)
		}
	}
	return out, err
}

func (e *Emulator) adbOnce(ctx context.Context, args ...string) (string, error) {
	out, err := adbCommand(ctx, e.Serial, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

func (e *Emulator) isTransportError(out string) bool {
	return strings.Contains(out, "device offline") ||
		strings.Contains(out, fmt.Sprintf("device '%s' not found", e.Serial))
}

const deviceReadyTimeout = 30 * time.Second

func (e *Emulator) WaitForDevice(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, deviceReadyTimeout)
	defer cancel()
	_ = adbCommand(ctx, e.Serial, "wait-for-device").Run()
	healthy := 0
	for healthy < 2 {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: adb transport did not recover within %s", e.AVD, deviceReadyTimeout)
		}
		state, _ := adbCommand(ctx, e.Serial, "get-state").CombinedOutput()
		if strings.TrimSpace(string(state)) == "device" {
			if err := adbCommand(ctx, e.Serial, "shell", "true").Run(); err == nil {
				healthy++
				time.Sleep(time.Second)
				continue
			}
		}
		healthy = 0
		time.Sleep(time.Second)
	}
	return nil
}

func (e *Emulator) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = e.adbOnce(ctx, "emu", "kill")
	if e.cmd != nil && e.cmd.Process != nil {
		done := make(chan struct{})
		go func() { _ = e.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = e.cmd.Process.Kill()
		}
	}
	if e.logFile != nil {
		e.logFile.Close()
	}
}
