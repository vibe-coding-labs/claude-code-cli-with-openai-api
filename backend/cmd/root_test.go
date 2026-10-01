package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestShouldDefaultToServer(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"bare invocation", []string{"prog"}, true},
		{"empty args slice", []string{}, true},
		{"empty first arg", []string{"prog", ""}, true},
		{"explicit server subcommand", []string{"prog", "server"}, false},
		{"explicit version subcommand", []string{"prog", "version"}, false},
		{"explicit config subcommand", []string{"prog", "config"}, false},
		{"explicit migrate subcommand", []string{"prog", "migrate", "status"}, false},
		{"long help flag", []string{"prog", "--help"}, false},
		{"short help flag", []string{"prog", "-h"}, false},
		{"long version flag", []string{"prog", "--version"}, false},
		{"short version flag", []string{"prog", "-v"}, false},
		{"unrelated flag defaults to server", []string{"prog", "--port", "8080"}, true},
		{"another unrelated flag", []string{"prog", "--log-level", "debug"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldDefaultToServer(tt.args); got != tt.want {
				t.Errorf("shouldDefaultToServer(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestExecuteInsertsServerArg exercises Execute()'s os.Args rewriting logic
// directly (without actually running cobra), confirming "server" is inserted
// at index 1 and trailing flags are preserved in order.
func TestExecuteInsertsServerArg(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"prog", "--port", "9"}
	if shouldDefaultToServer(os.Args) {
		args := make([]string, 0, len(os.Args)+1)
		args = append(args, os.Args[0], "server")
		args = append(args, os.Args[1:]...)
		os.Args = args
	}

	want := []string{"prog", "server", "--port", "9"}
	if len(os.Args) != len(want) {
		t.Fatalf("os.Args = %v, want %v", os.Args, want)
	}
	for i := range want {
		if os.Args[i] != want[i] {
			t.Fatalf("os.Args = %v, want %v", os.Args, want)
		}
	}
}

// TestExecuteWithVersionArg drives the real Execute() entrypoint with a safe
// explicit subcommand ("version"), confirming shouldDefaultToServer correctly
// leaves os.Args untouched and rootCmd.Execute() runs the version command
// successfully (no os.Exit is reached on this path).
func TestExecuteWithVersionArg(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"claude-with-openai-api", "version"}

	out := captureStdout(t, func() {
		Execute()
	})
	if !strings.Contains(out, "Version:") {
		t.Errorf("Execute() with 'version' arg produced unexpected output:\n%s", out)
	}
}

// TestExecuteWithHelpFlag drives Execute() with "--help", confirming
// shouldDefaultToServer treats it as an explicit root-level flag (not
// defaulted into "server") and cobra prints usage successfully.
func TestExecuteWithHelpFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"claude-with-openai-api", "--help"}

	out := captureStdout(t, func() {
		Execute()
	})
	if !strings.Contains(out, "Use \"claude-with-openai-api [command] --help\"") {
		t.Errorf("Execute() with '--help' produced unexpected output:\n%s", out)
	}
}

// TestExecuteInvalidCommandExits verifies Execute()'s error path (unknown
// subcommand -> print error -> os.Exit(1)) via a subprocess, since os.Exit
// cannot be safely exercised in-process without killing the test binary.
func TestExecuteInvalidCommandExits(t *testing.T) {
	if os.Getenv("ROOT_TEST_EXECUTE_INVALID") == "1" {
		os.Args = []string{"claude-with-openai-api", "bogus-command"}
		Execute()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteInvalidCommandExits")
	cmd.Env = append(os.Environ(), "ROOT_TEST_EXECUTE_INVALID=1")
	out, err := cmd.CombinedOutput()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected subprocess to exit with error, got err=%v output=%s", err, out)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d, output:\n%s", exitErr.ExitCode(), out)
	}
	if !strings.Contains(string(out), "unknown command") {
		t.Errorf("expected 'unknown command' in output, got:\n%s", out)
	}
}
