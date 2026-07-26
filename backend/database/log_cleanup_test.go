package database

import (
	"testing"
)

// TestGetDatabaseSizeInfo requires a database connection to be initialized.
// This test is skipped when DB is nil (running in isolation without test DB setup).
func TestGetDatabaseSizeInfo(t *testing.T) {
	if DB == nil {
		t.Skip("Skipping: DB not initialized (requires test database setup)")
	}
	info, err := GetDatabaseSizeInfo()
	if err != nil {
		t.Fatalf("GetDatabaseSizeInfo failed: %v", err)
	}
	if info == nil {
		t.Fatal("GetDatabaseSizeInfo returned nil")
	}
	if info.TotalPages <= 0 {
		t.Errorf("TotalPages should be > 0, got %d", info.TotalPages)
	}
	if info.PageSize <= 0 {
		t.Errorf("PageSize should be > 0, got %d", info.PageSize)
	}
}

// TestDeleteOldestRequestLogs requires a database connection.
func TestDeleteOldestRequestLogs(t *testing.T) {
	if DB == nil {
		t.Skip("Skipping: DB not initialized (requires test database setup)")
	}
	deleted, err := DeleteOldestRequestLogs(10)
	if err != nil {
		t.Fatalf("DeleteOldestRequestLogs failed: %v", err)
	}
	_ = deleted
}

// TestGetRequestLogCountByAge requires a database connection.
func TestGetRequestLogCountByAge(t *testing.T) {
	if DB == nil {
		t.Skip("Skipping: DB not initialized (requires test database setup)")
	}
	count, err := GetRequestLogCountByAge(30)
	if err != nil {
		t.Fatalf("GetRequestLogCountByAge failed: %v", err)
	}
	_ = count
}

func TestGetAvailableDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()
	space := getAvailableDiskSpace(tmpDir)
	// On Linux, should return a positive value
	if space < 0 {
		t.Logf("getAvailableDiskSpace returned -1 (platform may not support statfs)")
	}
}

// TestVacuumInto_InvalidPath requires a database connection to be initialized.
func TestVacuumInto_InvalidPath(t *testing.T) {
	if DB == nil {
		t.Skip("Skipping: DB not initialized (requires test database setup)")
	}
	// VacuumInto should fail gracefully with an invalid path
	_, err := VacuumInto("/nonexistent/path/to/db.sqlite")
	if err == nil {
		t.Error("VacuumInto should fail for non-existent database path")
	}
}

// TestMigrateInlineBodiesToFiles requires a database connection.
func TestMigrateInlineBodiesToFiles(t *testing.T) {
	if DB == nil {
		t.Skip("Skipping: DB not initialized (requires test database setup)")
	}
	migrated, err := MigrateInlineBodiesToFiles(100)
	if err != nil {
		t.Fatalf("MigrateInlineBodiesToFiles failed: %v", err)
	}
	if migrated != 0 {
		t.Errorf("Expected 0 migrated rows on empty table, got %d", migrated)
	}
}
