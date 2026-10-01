package database

import "testing"

func TestSettingsCRUD(t *testing.T) {
	ensureTestDB(t)
	_, _ = DB.Exec("DELETE FROM system_settings WHERE key = 'test_setting_key'")

	// Not found path.
	if _, err := GetSetting("test_setting_key"); err == nil {
		t.Error("expected error for missing setting")
	}

	// Create.
	if err := SetSetting("test_setting_key", "v1", "tester"); err != nil {
		t.Fatalf("SetSetting create: %v", err)
	}
	got, err := GetSetting("test_setting_key")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "v1" {
		t.Errorf("got %q, want v1", got)
	}

	// Update (ON CONFLICT path).
	if err := SetSetting("test_setting_key", "v2", "tester2"); err != nil {
		t.Fatalf("SetSetting update: %v", err)
	}
	got, err = GetSetting("test_setting_key")
	if err != nil {
		t.Fatalf("GetSetting after update: %v", err)
	}
	if got != "v2" {
		t.Errorf("got %q, want v2", got)
	}

	all, err := GetAllSettings()
	if err != nil {
		t.Fatalf("GetAllSettings: %v", err)
	}
	if all["test_setting_key"] != "v2" {
		t.Errorf("GetAllSettings missing updated key: %v", all)
	}

	_, _ = DB.Exec("DELETE FROM system_settings WHERE key = 'test_setting_key'")
}

func TestGetAllSettings_Empty(t *testing.T) {
	// Use an isolated fresh DB rather than wiping the shared global one,
	// since system_settings carries package-wide defaults other tests rely on.
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	defer testDB.Close()
	_, _ = DB.Exec("DELETE FROM system_settings")

	all, err := GetAllSettings()
	if err != nil {
		t.Fatalf("GetAllSettings: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty map, got %v", all)
	}
}

// TestSettings_DBErrors exercises the real-error (non ErrNoRows) branches of
// GetSetting/SetSetting/GetAllSettings by operating on an already-closed
// database connection, isolated from the shared global DB.
func TestSettings_DBErrors(t *testing.T) {
	prevDB := DB
	defer func() { DB = prevDB }()

	testDB, err := InitTestDB()
	if err != nil {
		t.Fatalf("InitTestDB: %v", err)
	}
	if err := testDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// DB now points at a closed *sql.DB; every operation below must fail.

	if _, err := GetSetting("anything"); err == nil {
		t.Error("expected error querying a closed database")
	}
	if err := SetSetting("k", "v", "tester"); err == nil {
		t.Error("expected error writing to a closed database")
	}
	if _, err := GetAllSettings(); err == nil {
		t.Error("expected error listing settings on a closed database")
	}
}
