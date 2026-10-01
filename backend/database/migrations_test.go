package database

import (
	"strings"
	"testing"
)

func TestCreateMigrationsTable_Idempotent(t *testing.T) {
	ensureTestDB(t)
	if err := createMigrationsTable(); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := createMigrationsTable(); err != nil {
		t.Fatalf("second call should be idempotent: %v", err)
	}
}

func TestGetAppliedMigrations(t *testing.T) {
	ensureTestDB(t)
	applied, err := getAppliedMigrations()
	if err != nil {
		t.Fatalf("getAppliedMigrations: %v", err)
	}
	// createTables() already ran RunMigrations, so at least migration 1 should be present.
	if _, ok := applied[1]; !ok {
		t.Errorf("expected migration 1 to be recorded as applied, got %v", applied)
	}
}

func TestRecordMigration(t *testing.T) {
	ensureTestDB(t)
	_, _ = DB.Exec("DELETE FROM schema_migrations WHERE version = 99999")

	if err := recordMigration(99999, "test_migration"); err != nil {
		t.Fatalf("recordMigration: %v", err)
	}

	applied, err := getAppliedMigrations()
	if err != nil {
		t.Fatalf("getAppliedMigrations: %v", err)
	}
	if _, ok := applied[99999]; !ok {
		t.Error("expected recorded migration to show up as applied")
	}

	// Recording the same version twice should fail (primary key conflict) —
	// exercises the error path.
	if err := recordMigration(99999, "test_migration"); err == nil {
		t.Error("expected duplicate recordMigration to fail on primary key conflict")
	}

	_, _ = DB.Exec("DELETE FROM schema_migrations WHERE version = 99999")
}

func TestLoadMigrations(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least one embedded migration file")
	}
	// Must be sorted ascending by version.
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Version > migrations[i].Version {
			t.Fatalf("migrations not sorted: %d appears before %d", migrations[i-1].Version, migrations[i].Version)
		}
	}
	first := migrations[0]
	if first.Version != 1 {
		t.Errorf("expected first migration version 1, got %d", first.Version)
	}
	if first.Name == "" || first.Filename == "" || first.SQL == "" {
		t.Errorf("migration fields should be populated: %+v", first)
	}
}

func TestExtractUpMigration(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no markers returns full sql",
			input: "CREATE TABLE foo (id INTEGER);",
			want:  "CREATE TABLE foo (id INTEGER);",
		},
		{
			name: "up marker without down marker returns from up onward",
			input: "-- header\n-- UP Migration\nCREATE TABLE foo (id INTEGER);",
		},
		{
			name: "up and down markers isolate up section",
			input: "-- header\n-- UP Migration\nCREATE TABLE foo (id INTEGER);\n-- DOWN Migration\nDROP TABLE foo;",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractUpMigration(c.input)
			if c.want != "" && got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if strings.Contains(c.input, "DOWN Migration") && strings.Contains(got, "DROP TABLE") {
				t.Errorf("extractUpMigration leaked DOWN section: %q", got)
			}
		})
	}
}

func TestRunMigrations_NotInitialized(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()
	DB = nil

	if err := RunMigrations(); err == nil {
		t.Error("expected error when DB is nil")
	}
}

func TestRunMigrations_AlreadyUpToDate(t *testing.T) {
	ensureTestDB(t)
	// createTables() already applied every embedded migration, so a second
	// run should find zero pending migrations (pendingCount == 0 branch).
	if err := RunMigrations(); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

func TestGetMigrationStatus(t *testing.T) {
	ensureTestDB(t)

	statuses, err := GetMigrationStatus()
	if err != nil {
		t.Fatalf("GetMigrationStatus: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected migration statuses")
	}
	foundApplied := false
	for _, m := range statuses {
		if m.AppliedAt != nil {
			foundApplied = true
			break
		}
	}
	if !foundApplied {
		t.Error("expected at least one migration marked as applied")
	}
}

func TestGetMigrationStatus_NotInitialized(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()
	DB = nil

	if _, err := GetMigrationStatus(); err == nil {
		t.Error("expected error when DB is nil")
	}
}

func TestRollbackMigration(t *testing.T) {
	ensureTestDB(t)

	// Unapplied / unknown version.
	if err := RollbackMigration(999999); err == nil {
		t.Error("expected error rolling back a migration that was never applied")
	}

	// Applied version: rollback is intentionally unimplemented and must
	// return an explanatory error rather than mutate the schema.
	if err := RollbackMigration(1); err == nil {
		t.Error("expected RollbackMigration(1) to return the not-implemented error")
	}
}

func TestRollbackMigration_NotInitialized(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()
	DB = nil

	if err := RollbackMigration(1); err == nil {
		t.Error("expected error when DB is nil")
	}
}

// TestRollbackMigration_AppliedButFileMissing covers the "migration not
// found" branch: mark a bogus version as applied without a corresponding
// embedded .sql file.
func TestRollbackMigration_AppliedButFileMissing(t *testing.T) {
	ensureTestDB(t)
	_, _ = DB.Exec("DELETE FROM schema_migrations WHERE version = 88888")
	if err := recordMigration(88888, "phantom"); err != nil {
		t.Fatalf("recordMigration: %v", err)
	}
	defer DB.Exec("DELETE FROM schema_migrations WHERE version = 88888")

	err := RollbackMigration(88888)
	if err == nil {
		t.Fatal("expected error for migration with no matching file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// TestMigrations_ClosedDBErrors exercises the internal DB error-wrapping
// branches of createMigrationsTable/getAppliedMigrations/RunMigrations/
// GetMigrationStatus/RollbackMigration by pointing DB at an already-closed
// connection (non-nil, so the `DB == nil` guards are bypassed).
func TestMigrations_ClosedDBErrors(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	if err := testDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := createMigrationsTable(); err == nil {
		t.Error("createMigrationsTable: expected error on closed DB")
	}
	if _, err := getAppliedMigrations(); err == nil {
		t.Error("getAppliedMigrations: expected error on closed DB")
	}
	if err := RunMigrations(); err == nil {
		t.Error("RunMigrations: expected error on closed DB")
	}
	if _, err := GetMigrationStatus(); err == nil {
		t.Error("GetMigrationStatus: expected error on closed DB")
	}
	if err := RollbackMigration(1); err == nil {
		t.Error("RollbackMigration: expected error on closed DB")
	}
}

// TestRunMigrations_AppliedMigrationsQueryError and
// TestGetMigrationStatus_AppliedMigrationsQueryError cover the
// getAppliedMigrations error branch as seen from RunMigrations/
// GetMigrationStatus specifically (distinct call sites from
// TestMigrations_ClosedDBErrors, where createMigrationsTable fails first and
// getAppliedMigrations is never reached). Dropping applied_at leaves
// "CREATE TABLE IF NOT EXISTS schema_migrations" a no-op (table already
// exists) while the SELECT in getAppliedMigrations fails on the missing
// column.
func TestRunMigrations_AppliedMigrationsQueryError(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()

	if _, err := DB.Exec("ALTER TABLE schema_migrations DROP COLUMN applied_at"); err != nil {
		t.Fatalf("drop column: %v", err)
	}

	if err := RunMigrations(); err == nil {
		t.Fatal("expected RunMigrations to fail when schema_migrations is missing applied_at")
	}
}

func TestGetMigrationStatus_AppliedMigrationsQueryError(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()

	if _, err := DB.Exec("ALTER TABLE schema_migrations DROP COLUMN applied_at"); err != nil {
		t.Fatalf("drop column: %v", err)
	}

	if _, err := GetMigrationStatus(); err == nil {
		t.Fatal("expected GetMigrationStatus to fail when schema_migrations is missing applied_at")
	}
}

// TestRunMigrations_ExecFailureRollsBack replays an already-applied,
// non-idempotent ALTER TABLE migration (027_add_reasoning_effort, which
// lacks an IF NOT EXISTS guard) by deleting its schema_migrations row
// without reverting the actual column. RunMigrations must then fail on
// tx.Exec (duplicate column), roll back the transaction, and leave the
// migration unrecorded.
func TestRunMigrations_ExecFailureRollsBack(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()

	if _, err := DB.Exec("DELETE FROM schema_migrations WHERE version = 27"); err != nil {
		t.Fatalf("setup delete: %v", err)
	}

	err = RunMigrations()
	if err == nil {
		t.Fatal("expected RunMigrations to fail replaying a non-idempotent ALTER TABLE")
	}
	if !strings.Contains(err.Error(), "failed to execute migration 27") {
		t.Errorf("expected migration 27 exec failure, got: %v", err)
	}

	applied, err := getAppliedMigrations()
	if err != nil {
		t.Fatalf("getAppliedMigrations: %v", err)
	}
	if _, ok := applied[27]; ok {
		t.Error("migration 27 should not be recorded as applied after rollback")
	}
}

