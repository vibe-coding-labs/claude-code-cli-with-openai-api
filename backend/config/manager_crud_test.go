package config

import (
	"path/filepath"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// newTestManager 绕过 GetConfigManager 的进程级单例，构造一个指向临时文件的独立
// ConfigManager，使每个测试彼此隔离，互不污染磁盘状态或单例内存。
func newTestManager(t *testing.T) *ConfigManager {
	t.Helper()
	return &ConfigManager{
		configs:  make(map[string]*models.APIConfig),
		filePath: filepath.Join(t.TempDir(), "configs.json"),
	}
}

func validCreateReq() *models.APIConfigRequest {
	return &models.APIConfigRequest{
		Name:          "test-config",
		OpenAIAPIKey:  "sk-test",
		OpenAIBaseURL: "https://api.openai.com/v1",
		BigModel:      "gpt-4o",
	}
}

func TestCreateConfig(t *testing.T) {
	t.Run("creates config with generated id and anthropic key", func(t *testing.T) {
		cm := newTestManager(t)
		cfg, err := cm.CreateConfig(validCreateReq())
		if err != nil {
			t.Fatalf("CreateConfig() error = %v", err)
		}
		if cfg.ID == "" {
			t.Errorf("expected generated ID")
		}
		if cfg.AnthropicAPIKey == "" {
			t.Errorf("expected auto-generated AnthropicAPIKey")
		}
		if !cfg.Enabled {
			t.Errorf("expected Enabled = true")
		}
		// 第一个创建的配置应自动成为默认配置
		if cm.defaultConfigID != cfg.ID {
			t.Errorf("expected first config to become default, got defaultConfigID=%q", cm.defaultConfigID)
		}
	})

	t.Run("keeps explicit anthropic key", func(t *testing.T) {
		cm := newTestManager(t)
		req := validCreateReq()
		req.AnthropicAPIKey = "explicit-key"
		cfg, err := cm.CreateConfig(req)
		if err != nil {
			t.Fatalf("CreateConfig() error = %v", err)
		}
		if cfg.AnthropicAPIKey != "explicit-key" {
			t.Errorf("AnthropicAPIKey = %q, want explicit-key", cfg.AnthropicAPIKey)
		}
	})

	// 回归测试：CreateConfig 此前完全没有把 req.SupportedModels 拷贝进新建的
	// config，导致创建时传的 supported_models 被静默丢弃，必须靠一次额外的
	// UpdateConfig 才能补上。
	t.Run("copies supported models from request", func(t *testing.T) {
		cm := newTestManager(t)
		req := validCreateReq()
		req.SupportedModels = []string{"gpt-4o", "gpt-4o-mini"}
		cfg, err := cm.CreateConfig(req)
		if err != nil {
			t.Fatalf("CreateConfig() error = %v", err)
		}
		if len(cfg.SupportedModels) != 2 {
			t.Errorf("SupportedModels = %v, want [gpt-4o gpt-4o-mini]", cfg.SupportedModels)
		}
	})

	t.Run("second config does not override default", func(t *testing.T) {
		cm := newTestManager(t)
		first, _ := cm.CreateConfig(validCreateReq())
		req2 := validCreateReq()
		req2.Name = "second"
		second, err := cm.CreateConfig(req2)
		if err != nil {
			t.Fatalf("CreateConfig() error = %v", err)
		}
		if cm.defaultConfigID != first.ID {
			t.Errorf("expected default to remain first config, got %q (second=%q)", cm.defaultConfigID, second.ID)
		}
	})

	t.Run("rejects missing name", func(t *testing.T) {
		cm := newTestManager(t)
		req := validCreateReq()
		req.Name = ""
		if _, err := cm.CreateConfig(req); err == nil {
			t.Errorf("expected error for missing name")
		}
	})

	t.Run("rejects missing openai api key", func(t *testing.T) {
		cm := newTestManager(t)
		req := validCreateReq()
		req.OpenAIAPIKey = ""
		if _, err := cm.CreateConfig(req); err == nil {
			t.Errorf("expected error for missing openai api key")
		}
	})

	t.Run("rejects missing openai base url", func(t *testing.T) {
		cm := newTestManager(t)
		req := validCreateReq()
		req.OpenAIBaseURL = ""
		if _, err := cm.CreateConfig(req); err == nil {
			t.Errorf("expected error for missing openai base url")
		}
	})

	t.Run("propagates save failure", func(t *testing.T) {
		cm := newTestManager(t)
		// 把 filePath 指向一个目录而非文件，写入时必然失败
		cm.filePath = t.TempDir()
		if _, err := cm.CreateConfig(validCreateReq()); err == nil {
			t.Errorf("expected error when SaveConfigs fails")
		}
	})
}

func TestGetConfig(t *testing.T) {
	cm := newTestManager(t)
	created, _ := cm.CreateConfig(validCreateReq())

	t.Run("found", func(t *testing.T) {
		got, err := cm.GetConfig(created.ID)
		if err != nil {
			t.Fatalf("GetConfig() error = %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("got ID %q, want %q", got.ID, created.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := cm.GetConfig("does-not-exist"); err == nil {
			t.Errorf("expected error for missing config")
		}
	})
}

func TestGetAllConfigs(t *testing.T) {
	cm := newTestManager(t)

	if got := cm.GetAllConfigs(); len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}

	cm.CreateConfig(validCreateReq())
	req2 := validCreateReq()
	req2.Name = "second"
	cm.CreateConfig(req2)

	got := cm.GetAllConfigs()
	if len(got) != 2 {
		t.Errorf("expected 2 configs, got %d", len(got))
	}
}

func TestUpdateConfig(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		cm := newTestManager(t)
		if _, err := cm.UpdateConfig("missing", &models.APIConfigRequest{}); err == nil {
			t.Errorf("expected error for missing config")
		}
	})

	t.Run("updates provided fields and preserves omitted ones", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())
		originalSmallModel := created.SmallModel

		update := &models.APIConfigRequest{
			Name:             "renamed",
			Description:      "desc",
			OpenAIAPIKey:     "sk-new",
			OpenAIBaseURL:    "https://new.example.com/v1",
			BigModel:         "gpt-5",
			MiddleModel:      "gpt-5-mid",
			SmallModel:       "",
			MaxTokensLimit:   8192,
			RequestTimeout:   120,
			RetryCount:       3,
			RetryBackoffBase: 2.5,
			RetryBackoffMax:  30,
			AnthropicAPIKey:  "new-anthropic-key",
			SupportedModels:  []string{"gpt-5"},
			Enabled:          false,
		}
		got, err := cm.UpdateConfig(created.ID, update)
		if err != nil {
			t.Fatalf("UpdateConfig() error = %v", err)
		}
		if got.Name != "renamed" {
			t.Errorf("Name = %q", got.Name)
		}
		if got.OpenAIAPIKey != "sk-new" {
			t.Errorf("OpenAIAPIKey = %q", got.OpenAIAPIKey)
		}
		if got.BigModel != "gpt-5" {
			t.Errorf("BigModel = %q", got.BigModel)
		}
		if got.MaxTokensLimit != 8192 {
			t.Errorf("MaxTokensLimit = %d", got.MaxTokensLimit)
		}
		if got.RetryBackoffBase != 2.5 {
			t.Errorf("RetryBackoffBase = %v", got.RetryBackoffBase)
		}
		if len(got.SupportedModels) != 1 || got.SupportedModels[0] != "gpt-5" {
			t.Errorf("SupportedModels = %v", got.SupportedModels)
		}
		// SmallModel 留空 ("") 应保持原值不被清空
		if got.SmallModel != originalSmallModel {
			t.Errorf("SmallModel = %q, want preserved %q", got.SmallModel, originalSmallModel)
		}
		// Enabled 是布尔值，即便传 false 也必须生效（不像其它字段那样用零值跳过）
		if got.Enabled {
			t.Errorf("Enabled = true, want false to take effect")
		}
	})

	t.Run("empty supported models list is ignored, not clearing existing", func(t *testing.T) {
		cm := newTestManager(t)
		req := validCreateReq()
		req.SupportedModels = []string{"gpt-4o", "gpt-4o-mini"}
		created, _ := cm.CreateConfig(req)

		got, err := cm.UpdateConfig(created.ID, &models.APIConfigRequest{Enabled: true})
		if err != nil {
			t.Fatalf("UpdateConfig() error = %v", err)
		}
		if len(got.SupportedModels) != 2 {
			t.Errorf("expected SupportedModels preserved, got %v", got.SupportedModels)
		}
	})

	t.Run("small model is updated when explicitly provided", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())

		got, err := cm.UpdateConfig(created.ID, &models.APIConfigRequest{SmallModel: "gpt-4o-mini-new", Enabled: true})
		if err != nil {
			t.Fatalf("UpdateConfig() error = %v", err)
		}
		if got.SmallModel != "gpt-4o-mini-new" {
			t.Errorf("SmallModel = %q, want gpt-4o-mini-new", got.SmallModel)
		}
	})

	t.Run("propagates save failure", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())
		cm.filePath = t.TempDir()
		if _, err := cm.UpdateConfig(created.ID, &models.APIConfigRequest{Enabled: true}); err == nil {
			t.Errorf("expected error when SaveConfigs fails")
		}
	})
}

func TestDeleteConfig(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		cm := newTestManager(t)
		if err := cm.DeleteConfig("missing"); err == nil {
			t.Errorf("expected error for missing config")
		}
	})

	t.Run("cannot delete default config", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())
		if err := cm.DeleteConfig(created.ID); err == nil {
			t.Errorf("expected error deleting the default config")
		}
	})

	t.Run("deletes non-default config", func(t *testing.T) {
		cm := newTestManager(t)
		cm.CreateConfig(validCreateReq())
		req2 := validCreateReq()
		req2.Name = "second"
		second, _ := cm.CreateConfig(req2)

		if err := cm.DeleteConfig(second.ID); err != nil {
			t.Fatalf("DeleteConfig() error = %v", err)
		}
		if _, err := cm.GetConfig(second.ID); err == nil {
			t.Errorf("expected config to be gone after delete")
		}
	})

	t.Run("propagates save failure", func(t *testing.T) {
		cm := newTestManager(t)
		cm.CreateConfig(validCreateReq())
		req2 := validCreateReq()
		req2.Name = "second"
		second, _ := cm.CreateConfig(req2)

		cm.filePath = t.TempDir()
		if err := cm.DeleteConfig(second.ID); err == nil {
			t.Errorf("expected error when SaveConfigs fails")
		}
	})
}

func TestSetDefaultConfig(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		cm := newTestManager(t)
		if err := cm.SetDefaultConfig("missing"); err == nil {
			t.Errorf("expected error for missing config")
		}
	})

	t.Run("sets default", func(t *testing.T) {
		cm := newTestManager(t)
		first, _ := cm.CreateConfig(validCreateReq())
		req2 := validCreateReq()
		req2.Name = "second"
		second, _ := cm.CreateConfig(req2)

		if err := cm.SetDefaultConfig(second.ID); err != nil {
			t.Fatalf("SetDefaultConfig() error = %v", err)
		}
		if cm.defaultConfigID != second.ID {
			t.Errorf("defaultConfigID = %q, want %q", cm.defaultConfigID, second.ID)
		}
		_ = first
	})

	t.Run("propagates save failure", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())
		cm.filePath = t.TempDir()
		if err := cm.SetDefaultConfig(created.ID); err == nil {
			t.Errorf("expected error when SaveConfigs fails")
		}
	})
}

func TestGetDefaultConfig(t *testing.T) {
	t.Run("no default set", func(t *testing.T) {
		cm := newTestManager(t)
		if _, err := cm.GetDefaultConfig(); err == nil {
			t.Errorf("expected error when no default is set")
		}
	})

	t.Run("default id set but config missing (orphaned)", func(t *testing.T) {
		cm := newTestManager(t)
		cm.defaultConfigID = "ghost-id"
		if _, err := cm.GetDefaultConfig(); err == nil {
			t.Errorf("expected error for orphaned default id")
		}
	})

	t.Run("returns default config", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())
		got, err := cm.GetDefaultConfig()
		if err != nil {
			t.Fatalf("GetDefaultConfig() error = %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("got ID %q, want %q", got.ID, created.ID)
		}
	})
}

func TestGetConfigByAnthropicKey(t *testing.T) {
	cm := newTestManager(t)
	req := validCreateReq()
	req.AnthropicAPIKey = "find-me"
	created, _ := cm.CreateConfig(req)

	t.Run("found", func(t *testing.T) {
		got, err := cm.GetConfigByAnthropicKey("find-me")
		if err != nil {
			t.Fatalf("GetConfigByAnthropicKey() error = %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("got ID %q, want %q", got.ID, created.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := cm.GetConfigByAnthropicKey("does-not-exist"); err == nil {
			t.Errorf("expected error for unknown anthropic key")
		}
	})
}

// TestUpdateTestStatus 覆盖对 config/manager_crud.go 中一个真实 bug 的修复：
// 此前 UpdateTestStatus 接收 status/errorMsg 参数却完全不写回 LastTestStatus/
// LastTestError/LastTestedAt 字段，导致 handler/config_testing.go 报告的测试结果
// 从未持久化，前端永远看不到"测试成功/失败"的状态。
func TestUpdateTestStatus(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		cm := newTestManager(t)
		if err := cm.UpdateTestStatus("missing", "success", ""); err == nil {
			t.Errorf("expected error for missing config")
		}
	})

	t.Run("persists success status", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())

		if err := cm.UpdateTestStatus(created.ID, "success", ""); err != nil {
			t.Fatalf("UpdateTestStatus() error = %v", err)
		}
		got, _ := cm.GetConfig(created.ID)
		if got.LastTestStatus != "success" {
			t.Errorf("LastTestStatus = %q, want success", got.LastTestStatus)
		}
		if got.LastTestError != "" {
			t.Errorf("LastTestError = %q, want empty", got.LastTestError)
		}
		if got.LastTestedAt == nil {
			t.Errorf("LastTestedAt was not set")
		}
	})

	t.Run("persists failure status with error message", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())

		if err := cm.UpdateTestStatus(created.ID, "failed", "upstream timeout"); err != nil {
			t.Fatalf("UpdateTestStatus() error = %v", err)
		}
		got, _ := cm.GetConfig(created.ID)
		if got.LastTestStatus != "failed" {
			t.Errorf("LastTestStatus = %q, want failed", got.LastTestStatus)
		}
		if got.LastTestError != "upstream timeout" {
			t.Errorf("LastTestError = %q, want %q", got.LastTestError, "upstream timeout")
		}
	})

	t.Run("propagates save failure", func(t *testing.T) {
		cm := newTestManager(t)
		created, _ := cm.CreateConfig(validCreateReq())
		cm.filePath = t.TempDir()
		if err := cm.UpdateTestStatus(created.ID, "success", ""); err == nil {
			t.Errorf("expected error when SaveConfigs fails")
		}
	})
}
