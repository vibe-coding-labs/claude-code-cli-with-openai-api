package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
)

// captureStdout redirects fd 1 (the real OS stdout) for the duration of fn and
// returns what was written. A plain `os.Stdout = w` reassignment is not enough
// here: github.com/fatih/color caches `color.Output` as the *os.File pointing
// at the original stdout fd once, at package-init time, so color.Print* calls
// (used throughout cmd/'s CLI output) would keep writing to the real terminal
// instead of any later-reassigned os.Stdout variable. Redirecting the
// underlying fd via dup2 catches both plain fmt.Print* and color.Print*.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	stdoutFd := int(os.Stdout.Fd())
	savedFd, err := syscall.Dup(stdoutFd)
	if err != nil {
		t.Fatalf("failed to dup stdout fd: %v", err)
	}
	if err := syscall.Dup2(int(w.Fd()), stdoutFd); err != nil {
		t.Fatalf("failed to redirect stdout fd: %v", err)
	}

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	if err := syscall.Dup2(savedFd, stdoutFd); err != nil {
		t.Fatalf("failed to restore stdout fd: %v", err)
	}
	syscall.Close(savedFd)

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	r.Close()
	return buf.String()
}

func TestPrintVersion(t *testing.T) {
	origVersion, origBuildTime, origGitCommit := Version, BuildTime, GitCommit
	Version = "9.9.9"
	BuildTime = "2026-01-01T00:00:00Z"
	GitCommit = "deadbeef"
	defer func() {
		Version, BuildTime, GitCommit = origVersion, origBuildTime, origGitCommit
	}()

	out := captureStdout(t, printVersion)

	for _, want := range []string{"9.9.9", "2026-01-01T00:00:00Z", "deadbeef", "Version:", "Build Time:", "Git Commit:"} {
		if !strings.Contains(out, want) {
			t.Errorf("printVersion() output missing %q, got:\n%s", want, out)
		}
	}
}

func TestVersionCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected version subcommand to be registered on rootCmd")
	}
}

func TestVersionCommandRun(t *testing.T) {
	out := captureStdout(t, func() {
		versionCmd.Run(versionCmd, nil)
	})
	if !strings.Contains(out, "Version:") {
		t.Errorf("versionCmd.Run() output missing version info, got:\n%s", out)
	}
}
