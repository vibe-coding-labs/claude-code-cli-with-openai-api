package cmd

import (
	"strings"
	"testing"
)

// migrateCmd's RunE funcs hardcode the relative path "data/proxy.db", so we
// chdir into a scratch temp dir before invoking them — this keeps the test
// fully isolated from the real data/proxy.db (which backs the production
// systemd service) while still exercising the real code path.

func TestMigrateStatusAndUp(t *testing.T) {
	t.Chdir(t.TempDir())

	out := captureStdout(t, func() {
		if err := migrateStatusCmd.RunE(migrateStatusCmd, nil); err != nil {
			t.Fatalf("migrate status RunE() error = %v", err)
		}
	})
	// createTables() (invoked by InitDB) already runs all migrations, so by
	// the time `migrate status` first runs everything shows as Applied.
	if !strings.Contains(out, "VERSION") || !strings.Contains(out, "Applied") {
		t.Errorf("expected a migrations table with Applied entries, got:\n%s", out)
	}

	out = captureStdout(t, func() {
		if err := migrateUpCmd.RunE(migrateUpCmd, nil); err != nil {
			t.Fatalf("migrate up RunE() error = %v", err)
		}
	})
	if !strings.Contains(out, "Migrations completed successfully") {
		t.Errorf("expected success message, got:\n%s", out)
	}

	out = captureStdout(t, func() {
		if err := migrateStatusCmd.RunE(migrateStatusCmd, nil); err != nil {
			t.Fatalf("migrate status RunE() (post-up) error = %v", err)
		}
	})
	if !strings.Contains(out, "Applied") {
		t.Errorf("expected at least one Applied migration after 'up', got:\n%s", out)
	}
	if strings.Contains(out, "No migrations found") {
		t.Errorf("did not expect empty migrations table after 'up', got:\n%s", out)
	}
}

func TestMigrateRollbackInvalidVersion(t *testing.T) {
	t.Chdir(t.TempDir())

	err := migrateRollbackCmd.RunE(migrateRollbackCmd, []string{"not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-numeric version argument")
	}
	if !strings.Contains(err.Error(), "invalid version number") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMigrateRollbackAppliedVersion(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := migrateUpCmd.RunE(migrateUpCmd, nil); err != nil {
		t.Fatalf("migrate up RunE() error = %v", err)
	}

	// RollbackMigration is intentionally unimplemented (manual-only rollback
	// per its doc comment) — it always errors for an applied version. This
	// still exercises migrateRollbackCmd's error-propagation branch.
	err := migrateRollbackCmd.RunE(migrateRollbackCmd, []string{"1"})
	if err == nil {
		t.Fatal("expected RollbackMigration to report automatic rollback as unimplemented")
	}
	if !strings.Contains(err.Error(), "automatic rollback not implemented") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMigrateCommandsRegistered(t *testing.T) {
	uses := map[string]bool{}
	for _, c := range migrateCmd.Commands() {
		uses[strings.Fields(c.Use)[0]] = true
	}
	for _, want := range []string{"up", "status", "rollback"} {
		if !uses[want] {
			t.Errorf("expected migrate subcommand %q to be registered", want)
		}
	}

	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "migrate" {
			found = true
		}
	}
	if !found {
		t.Error("expected migrate command to be registered on rootCmd")
	}
}
