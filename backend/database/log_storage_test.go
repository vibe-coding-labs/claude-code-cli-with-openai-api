package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogStorage_StoreAndLoadBody(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	now := time.Now()

	path, err := storage.StoreBody(1, body, "req", now)
	if err != nil {
		t.Fatalf("StoreBody failed: %v", err)
	}
	if path == "" {
		t.Fatal("StoreBody returned empty path")
	}

	// Verify file exists
	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatalf("Body file not created at %s", fullPath)
	}

	// Load and verify
	loaded, err := storage.LoadBody(path)
	if err != nil {
		t.Fatalf("LoadBody failed: %v", err)
	}
	if loaded != body {
		t.Errorf("Loaded body mismatch: got %q, want %q", loaded, body)
	}
}

func TestLogStorage_StoreEmptyBody(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	path, err := storage.StoreBody(1, "", "req", time.Now())
	if err != nil {
		t.Fatalf("StoreBody with empty body failed: %v", err)
	}
	if path != "" {
		t.Errorf("StoreBody with empty body should return empty path, got %q", path)
	}
}

func TestLogStorage_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	_, err := storage.LoadBody("nonexistent/path.json.gz")
	if err == nil {
		t.Error("LoadBody should fail for non-existent file")
	}
}

func TestLogStorage_CleanOldDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	// Create an old directory (31 days ago)
	oldDate := time.Now().AddDate(0, 0, -31)
	oldDir := filepath.Join(tmpDir, oldDate.Format("2006"), oldDate.Format("01"), oldDate.Format("02"))
	os.MkdirAll(oldDir, 0755)
	os.WriteFile(filepath.Join(oldDir, "test.json.gz"), []byte("test"), 0644)

	// Create a recent directory (1 day ago)
	recentDate := time.Now().AddDate(0, 0, -1)
	recentDir := filepath.Join(tmpDir, recentDate.Format("2006"), recentDate.Format("01"), recentDate.Format("02"))
	os.MkdirAll(recentDir, 0755)
	os.WriteFile(filepath.Join(recentDir, "test.json.gz"), []byte("test"), 0644)

	removed, err := storage.CleanOldDirectories(30)
	if err != nil {
		t.Fatalf("CleanOldDirectories failed: %v", err)
	}
	if removed != 1 {
		t.Errorf("Expected 1 directory removed, got %d", removed)
	}

	// Old directory should be gone
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("Old directory should have been removed")
	}

	// Recent directory should still exist
	if _, err := os.Stat(recentDir); os.IsNotExist(err) {
		t.Error("Recent directory should still exist")
	}
}

func TestLogStorage_GetStorageSize(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	// Initially should be 0
	size, err := storage.GetStorageSize()
	if err != nil {
		t.Fatalf("GetStorageSize failed: %v", err)
	}

	// Store a body
	storage.StoreBody(1, "test content for size check", "req", time.Now())

	size2, err := storage.GetStorageSize()
	if err != nil {
		t.Fatalf("GetStorageSize after store failed: %v", err)
	}
	if size2 <= size {
		t.Errorf("Storage size should increase after storing a body: before=%d, after=%d", size, size2)
	}
}

func TestLogStorage_DeleteBodyFile(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	path, _ := storage.StoreBody(42, "delete me", "req", time.Now())
	if path == "" {
		t.Fatal("StoreBody returned empty path")
	}

	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatal("Body file should exist before delete")
	}

	if err := storage.DeleteBodyFile(path); err != nil {
		t.Fatalf("DeleteBodyFile failed: %v", err)
	}

	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Error("Body file should be deleted")
	}
}
