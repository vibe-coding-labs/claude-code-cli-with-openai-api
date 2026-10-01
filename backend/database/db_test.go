package database

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInitDB_RealFile exercises InitDB against a real on-disk sqlite file
// (InitTestDB only ever uses :memory:, so InitDB itself is otherwise never
// invoked by the test suite).
func TestInitDB_RealFile(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "test.db")

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer DB.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected db file to exist: %v", err)
	}

	// Tables should exist and be queryable.
	if _, err := DB.Exec("SELECT COUNT(*) FROM api_configs"); err != nil {
		t.Fatalf("api_configs table missing: %v", err)
	}
	if _, err := DB.Exec("SELECT COUNT(*) FROM users"); err != nil {
		t.Fatalf("users table missing: %v", err)
	}

	// Re-running InitDB on the same path should be idempotent (CREATE TABLE
	// IF NOT EXISTS, ALTER TABLE guarded by contains("duplicate column name")).
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("second InitDB call should be idempotent, got error: %v", err)
	}
	defer DB.Close()
}

// TestInitDB_UnwritableDir verifies InitDB surfaces an error instead of
// panicking when the directory cannot be created (parent path is a file).
// TestInitDB_PingFailure points dbPath at a directory: sql.Open succeeds
// lazily, but Ping fails because sqlite cannot open a directory as a
// database file — exercises InitDB's Ping error-wrapping branch.
func TestInitDB_PingFailure(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	dir := t.TempDir()
	asDir := filepath.Join(dir, "iamadir")
	if err := os.Mkdir(asDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := InitDB(asDir); err == nil {
		t.Fatal("expected InitDB to fail when dbPath is a directory")
	}
}

// TestCreateTables_ExecFailure forces the first DB.Exec in createTables to
// fail by pointing the global DB at an already-closed connection.
func TestCreateTables_ExecFailure(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	if err := testDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := createTables(); err == nil {
		t.Fatal("expected createTables to fail against a closed DB")
	}
}

func TestInitDB_UnwritableDir(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// blocker is a file, so MkdirAll(blocker/sub) must fail.
	badPath := filepath.Join(blocker, "sub", "test.db")

	if err := InitDB(badPath); err == nil {
		t.Fatal("expected InitDB to fail when data directory cannot be created")
	}
}

// TestColumnExists checks existing/non-existing columns and tables.
// TestCreateIndexes_SkipsMissingColumn exercises the columnsReady=false
// continue-branch: drop a column (and its dependent index) from an
// otherwise fully-migrated schema, then verify createIndexes() skips
// re-creating that index instead of erroring.
func TestCreateIndexes_SkipsMissingColumn(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()

	if _, err := DB.Exec("DROP INDEX IF EXISTS idx_api_configs_user_id"); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if _, err := DB.Exec("ALTER TABLE api_configs DROP COLUMN user_id"); err != nil {
		t.Fatalf("drop column: %v", err)
	}

	if err := createIndexes(); err != nil {
		t.Fatalf("createIndexes should skip missing-column index, got error: %v", err)
	}

	exists, err := columnExists("api_configs", "user_id")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if exists {
		t.Fatal("test setup invariant broken: user_id column should be absent")
	}
}

// TestColumnExists_QueryError exercises the DB.Query error branch by
// running against an already-closed connection.
func TestColumnExists_QueryError(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	if err := testDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := columnExists("api_configs", "id"); err == nil {
		t.Error("expected error querying a closed database")
	}
}

func TestColumnExists(t *testing.T) {
	ensureTestDB(t)

	exists, err := columnExists("api_configs", "id")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !exists {
		t.Error("expected api_configs.id to exist")
	}

	exists, err = columnExists("api_configs", "definitely_not_a_column")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if exists {
		t.Error("expected missing column to report false")
	}

	// PRAGMA table_info on a non-existent table returns zero rows, not an error.
	exists, err = columnExists("no_such_table_xyz", "id")
	if err != nil {
		t.Fatalf("columnExists on missing table should not error: %v", err)
	}
	if exists {
		t.Error("expected columnExists on missing table to be false")
	}
}

// TestCreateIndexes_SkipsMissingColumns exercises the columnsReady=false
// continue-branch: create a fresh DB, drop down to a state where an indexed
// column check fails, and ensure createIndexes doesn't error.
func TestCreateIndexes_Idempotent(t *testing.T) {
	ensureTestDB(t)
	if err := createIndexes(); err != nil {
		t.Fatalf("createIndexes should be idempotent: %v", err)
	}
}

// TestRunMigrations_Idempotent covers the "duplicate column name" swallow
// branch of contains() by invoking the legacy ALTER-TABLE migration runner
// twice against the same schema.
func TestRunMigrations_Idempotent(t *testing.T) {
	ensureTestDB(t)
	if err := runMigrations(); err != nil {
		t.Fatalf("first runMigrations: %v", err)
	}
	if err := runMigrations(); err != nil {
		t.Fatalf("second runMigrations (idempotent) should not error: %v", err)
	}
}

func TestContainsAndHasSubstring(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"", "", true},
		{"abc", "", true},
		{"", "abc", false},
		{"abc", "abcd", false},
		{"abc", "abc", true},
		{"duplicate column name: foo", "duplicate column name", true},
		{"hello world", "wor", true},
		{"hello world", "xyz", false},
		{"hello", "Hello", false}, // case sensitive
		{"aaab", "aab", true},
	}
	for _, c := range cases {
		if got := contains(c.s, c.sub); got != c.want {
			t.Errorf("contains(%q, %q) = %v, want %v", c.s, c.sub, got, c.want)
		}
	}

	if !hasSubstring("abcdef", "cde") {
		t.Error("hasSubstring should find middle substring")
	}
	if hasSubstring("abc", "xyz") {
		t.Error("hasSubstring should not find absent substring")
	}
	if !hasSubstring("abc", "") {
		t.Error("hasSubstring with empty substr should be true")
	}
}

func TestInitTestDB_GetDB(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}

	if testDB.GetDB() == nil {
		t.Fatal("GetDB returned nil")
	}
	if err := testDB.GetDB().Ping(); err != nil {
		t.Fatalf("GetDB() connection should be live: %v", err)
	}

	if err := testDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := testDB.GetDB().Ping(); err == nil {
		t.Error("expected Ping to fail after Close")
	}
}

func TestTestDB_Close_NilDB(t *testing.T) {
	tdb := &TestDB{}
	if err := tdb.Close(); err != nil {
		t.Errorf("Close on zero-value TestDB should be a no-op, got %v", err)
	}
}

func TestIsInitializedAndCloseDB(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	DB = nil
	if IsInitialized() {
		t.Error("IsInitialized should be false when DB is nil")
	}
	if err := CloseDB(); err != nil {
		t.Errorf("CloseDB on nil DB should be a no-op, got %v", err)
	}

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	if !IsInitialized() {
		t.Error("IsInitialized should be true after InitTestDB")
	}
	if err := CloseDB(); err != nil {
		t.Errorf("CloseDB should succeed: %v", err)
	}
	_ = testDB // underlying *sql.DB already closed via CloseDB/DB
}

// TestRunMigrationsLegacy_NonDuplicateColumnError exercises the legacy
// runMigrations() "warn and continue" branch for a real (non duplicate
// column) Exec error, by dropping one of its hardcoded ALTER TABLE targets
// beforehand so the statement fails with "no such table" instead.
func TestRunMigrationsLegacy_NonDuplicateColumnError(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()

	if _, err := DB.Exec("DROP TABLE load_balancers"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	// Must not return an error: non-duplicate-column failures are logged and
	// skipped, not propagated.
	if err := runMigrations(); err != nil {
		t.Fatalf("runMigrations should swallow non-duplicate-column errors, got: %v", err)
	}
}

// TestInitDB_ReadonlyFilePingFailure exercises another real-world trigger of
// InitDB's Ping error-wrap branch: a pre-existing, permission-read-only db
// file. (Note: WAL-mode Ping itself requires write access here, so this
// still lands on the Ping branch rather than reaching createTables — that
// branch appears unreachable in practice, since anything that would make
// createTables fail post-Ping would already fail Ping first.)
func TestInitDB_ReadonlyFilePingFailure(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "readonly.db")
	if err := os.WriteFile(dbPath, nil, 0444); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := InitDB(dbPath)
	// Only close the new connection if InitDB actually got far enough to
	// replace the global DB — if Ping failed first (e.g. WAL's -wal/-shm
	// file creation blocked by the read-only dir), DB is still prevDB and
	// must NOT be closed here.
	if DB != nil && DB != prevDB {
		defer DB.Close()
	}
	if err == nil {
		t.Fatal("expected InitDB to fail against a read-only database file")
	}
}
