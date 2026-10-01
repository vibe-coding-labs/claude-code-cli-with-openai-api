package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestLoadConfigs_FileMissing(t *testing.T) {
	cm := &ConfigManager{filePath: filepath.Join(t.TempDir(), "sub", "configs.json")}
	if err := cm.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs() error = %v", err)
	}
	if len(cm.configs) != 0 {
		t.Errorf("expected empty configs map, got %d entries", len(cm.configs))
	}
	if _, err := os.Stat(filepath.Dir(cm.filePath)); err != nil {
		t.Errorf("expected config directory to be created: %v", err)
	}
}

func TestLoadConfigs_ValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configs.json")
	fileData := struct {
		Configs         map[string]*models.APIConfig `json:"configs"`
		DefaultConfigID string                        `json:"default_config_id"`
	}{
		Configs: map[string]*models.APIConfig{
			"id-1": {ID: "id-1", Name: "one"},
		},
		DefaultConfigID: "id-1",
	}
	data, _ := json.Marshal(fileData)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cm := &ConfigManager{filePath: path}
	if err := cm.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs() error = %v", err)
	}
	if len(cm.configs) != 1 || cm.configs["id-1"].Name != "one" {
		t.Errorf("configs not loaded correctly: %+v", cm.configs)
	}
	if cm.defaultConfigID != "id-1" {
		t.Errorf("defaultConfigID = %q, want id-1", cm.defaultConfigID)
	}
}

func TestLoadConfigs_NullConfigsBecomesEmptyMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configs.json")
	if err := os.WriteFile(path, []byte(`{"configs":null,"default_config_id":""}`), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cm := &ConfigManager{filePath: path}
	if err := cm.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs() error = %v", err)
	}
	if cm.configs == nil {
		t.Errorf("expected non-nil empty map when configs is JSON null")
	}
}

func TestLoadConfigs_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configs.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cm := &ConfigManager{filePath: path}
	if err := cm.LoadConfigs(); err == nil {
		t.Errorf("expected error for malformed JSON")
	}
}

func TestLoadConfigs_MkdirFailsWhenParentIsFile(t *testing.T) {
	// dir 组件本身是一个已存在的普通文件时，os.MkdirAll 必然失败
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	cm := &ConfigManager{filePath: filepath.Join(blocker, "sub", "configs.json")}
	if err := cm.LoadConfigs(); err == nil {
		t.Errorf("expected error when config directory cannot be created")
	}
}

func TestLoadConfigs_ReadFileFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, file permission checks are bypassed")
	}
	path := filepath.Join(t.TempDir(), "configs.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("failed to chmod fixture: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	cm := &ConfigManager{filePath: path}
	if err := cm.LoadConfigs(); err == nil {
		t.Errorf("expected error when config file cannot be read")
	}
}

func TestSaveConfigs_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configs.json")
	cm := &ConfigManager{
		filePath: path,
		configs: map[string]*models.APIConfig{
			"id-1": {ID: "id-1", Name: "one", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		defaultConfigID: "id-1",
	}
	if err := cm.SaveConfigs(); err != nil {
		t.Fatalf("SaveConfigs() error = %v", err)
	}

	reloaded := &ConfigManager{filePath: path}
	if err := reloaded.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs() after save error = %v", err)
	}
	if reloaded.defaultConfigID != "id-1" || reloaded.configs["id-1"].Name != "one" {
		t.Errorf("round-tripped data mismatch: %+v", reloaded.configs)
	}
}

func TestSaveConfigs_MkdirFailsWhenParentIsFile(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	cm := &ConfigManager{filePath: filepath.Join(blocker, "sub", "configs.json"), configs: map[string]*models.APIConfig{}}
	if err := cm.SaveConfigs(); err == nil {
		t.Errorf("expected error when config directory cannot be created")
	}
}

func TestSaveConfigs_WriteFailsWhenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	cm := &ConfigManager{filePath: dir, configs: map[string]*models.APIConfig{}}
	if err := cm.SaveConfigs(); err == nil {
		t.Errorf("expected error when filePath is a directory")
	}
}
