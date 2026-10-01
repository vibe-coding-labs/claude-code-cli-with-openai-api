package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDBPathFromEnv(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})

	os.Setenv("DB_PATH", "/tmp/custom/proxy.db")
	if got := resolveDBPath(); got != "/tmp/custom/proxy.db" {
		t.Errorf("resolveDBPath() = %q, want %q", got, "/tmp/custom/proxy.db")
	}
}

func TestResolveDBPathFallsBackToRelative(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})
	os.Unsetenv("DB_PATH")

	// No data/proxy.db exists next to the test binary, so resolveDBPath must
	// fall back to "./data/proxy.db".
	want := filepath.Join(".", "data", "proxy.db")
	if got := resolveDBPath(); got != want {
		t.Errorf("resolveDBPath() = %q, want %q", got, want)
	}
}

func TestResolveDBPathFindsCandidateNextToExecutable(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})
	os.Unsetenv("DB_PATH")

	exePath, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable() unavailable: %v", err)
	}
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil || realPath == "" {
		realPath = exePath
	}
	dataDir := filepath.Join(filepath.Dir(realPath), "data")
	candidate := filepath.Join(dataDir, "proxy.db")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Skipf("cannot create data dir next to test binary: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("x"), 0o644); err != nil {
		t.Skipf("cannot write candidate db next to test binary: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	if got := resolveDBPath(); got != candidate {
		t.Errorf("resolveDBPath() = %q, want %q", got, candidate)
	}
}

func TestResetPasswordCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "reset-password" {
			found = true
		}
	}
	if !found {
		t.Error("expected reset-password command to be registered on rootCmd")
	}
}

// withStdin temporarily replaces os.Stdin with r for the duration of fn.
func withStdin(t *testing.T, r *os.File, fn func()) {
	t.Helper()
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()
	fn()
}

func TestRunResetPasswordDatabaseInitError(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})

	// A regular file used as a path component makes os.MkdirAll (inside
	// database.InitDB) fail, exercising runResetPassword's DB-init error
	// branch without ever touching a real terminal or the production DB.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	os.Setenv("DB_PATH", filepath.Join(blocker, "sub", "proxy.db"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	w.Close()

	var runErr error
	captureStdout(t, func() {
		withStdin(t, r, func() {
			runErr = runResetPassword(resetPasswordCmd, nil)
		})
	})
	r.Close()

	if runErr == nil {
		t.Fatal("expected error when database directory cannot be created")
	}
}

func TestRunResetPasswordUsernameTooShort(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})
	os.Setenv("DB_PATH", filepath.Join(t.TempDir(), "proxy.db"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString("ab\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	w.Close()

	var runErr error
	captureStdout(t, func() {
		withStdin(t, r, func() {
			runErr = runResetPassword(resetPasswordCmd, nil)
		})
	})
	r.Close()

	if runErr == nil || !strings.Contains(runErr.Error(), "username too short") {
		t.Fatalf("expected 'username too short' error, got: %v", runErr)
	}
}

func TestRunResetPasswordReadUsernameEOF(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})
	os.Setenv("DB_PATH", filepath.Join(t.TempDir(), "proxy.db"))

	// Closed pipe: the first ReadString('\n') fails immediately with EOF.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	w.Close()

	var runErr error
	captureStdout(t, func() {
		withStdin(t, r, func() {
			runErr = runResetPassword(resetPasswordCmd, nil)
		})
	})
	r.Close()

	if runErr == nil || !strings.Contains(runErr.Error(), "failed to read input") {
		t.Fatalf("expected 'failed to read input' error, got: %v", runErr)
	}
}

func TestRunResetPasswordPasswordReadFailsOnNonTTY(t *testing.T) {
	orig, had := os.LookupEnv("DB_PATH")
	t.Cleanup(func() {
		if had {
			os.Setenv("DB_PATH", orig)
		} else {
			os.Unsetenv("DB_PATH")
		}
	})
	os.Setenv("DB_PATH", filepath.Join(t.TempDir(), "proxy.db"))

	// A valid username followed by EOF: term.ReadPassword requires a real
	// TTY, which a pipe is not, so it errors out immediately — this is the
	// furthest runResetPassword can be driven without a pseudo-terminal.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString("validuser\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	w.Close()

	var runErr error
	captureStdout(t, func() {
		withStdin(t, r, func() {
			runErr = runResetPassword(resetPasswordCmd, nil)
		})
	})
	r.Close()

	if runErr == nil || !strings.Contains(runErr.Error(), "failed to read password") {
		t.Fatalf("expected 'failed to read password' error, got: %v", runErr)
	}
}
