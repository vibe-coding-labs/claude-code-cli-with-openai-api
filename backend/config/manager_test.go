package config

import (
	"os"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestToConfigFromAPIConfig(t *testing.T) {
	ac := &models.APIConfig{
		OpenAIAPIKey:     "sk-x",
		AnthropicAPIKey:  "anthropic-x",
		OpenAIBaseURL:    "https://example.com/v1",
		AzureAPIVersion:  "2024-01-01",
		MaxTokensLimit:   1234,
		MinTokensLimit:   56,
		RequestTimeout:   99,
		RetryCount:       7,
		RetryBackoffBase: 1.5,
		RetryBackoffMax:  45,
		BigModel:         "big",
		MiddleModel:      "middle",
		SmallModel:       "small",
		CustomHeaders:    map[string]string{"X-Foo": "bar"},
	}

	cfg := ToConfigFromAPIConfig(ac)

	if cfg.OpenAIAPIKey != ac.OpenAIAPIKey || cfg.AnthropicAPIKey != ac.AnthropicAPIKey {
		t.Errorf("api keys not copied correctly: %+v", cfg)
	}
	if cfg.OpenAIBaseURL != ac.OpenAIBaseURL || cfg.AzureAPIVersion != ac.AzureAPIVersion {
		t.Errorf("base url / azure version not copied correctly: %+v", cfg)
	}
	if cfg.MaxTokensLimit != ac.MaxTokensLimit || cfg.MinTokensLimit != ac.MinTokensLimit {
		t.Errorf("token limits not copied correctly: %+v", cfg)
	}
	if cfg.RequestTimeout != ac.RequestTimeout || cfg.RetryCount != ac.RetryCount {
		t.Errorf("timeout/retry not copied correctly: %+v", cfg)
	}
	if cfg.RetryBackoffBase != ac.RetryBackoffBase || cfg.RetryBackoffMax != ac.RetryBackoffMax {
		t.Errorf("retry backoff not copied correctly: %+v", cfg)
	}
	if cfg.BigModel != ac.BigModel || cfg.MiddleModel != ac.MiddleModel || cfg.SmallModel != ac.SmallModel {
		t.Errorf("models not copied correctly: %+v", cfg)
	}
	if cfg.CustomHeaders["X-Foo"] != "bar" {
		t.Errorf("custom headers not copied correctly: %+v", cfg.CustomHeaders)
	}
	// 这些字段由 ToConfigFromAPIConfig 硬编码为固定值，用于向后兼容旧调用方
	if cfg.Host != "0.0.0.0" || cfg.Port != 10086 || cfg.LogLevel != "INFO" {
		t.Errorf("hardcoded defaults changed unexpectedly: host=%q port=%d level=%q", cfg.Host, cfg.Port, cfg.LogLevel)
	}
}

// TestGetConfigManager_SingletonAndDefaultPath 是本进程内唯一一次调用
// GetConfigManager 的测试：该函数用 sync.Once 实现真正的单例，一旦在某个
// go test 进程里被调用过一次，之后无论传入什么参数都会返回同一个实例、
// 忽略后续参数——因此不可能在同一测试二进制里分别覆盖"显式传路径"和
// "使用默认路径"两条分支，只能二选一。这里选择覆盖生产环境实际在用的
// 调用方式（handler/config_handler.go 不传参，走默认 "configs.json"）。
// 为避免在仓库 config/ 目录下残留 configs.json，测试内切换 cwd 到临时目录。
func TestGetConfigManager_SingletonAndDefaultPath(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}
	t.Cleanup(func() { os.Chdir(origWD) })

	cm1 := GetConfigManager()
	if cm1 == nil {
		t.Fatalf("expected non-nil ConfigManager")
	}
	if cm1.filePath != "configs.json" {
		t.Errorf("filePath = %q, want default configs.json", cm1.filePath)
	}
	// LoadConfigs 在文件不存在时只初始化内存中的空 map，并不会落盘，
	// 所以这里只能断言内存状态，不能断言 cwd 下出现了 configs.json 文件。
	if cm1.configs == nil {
		t.Errorf("expected configs map to be initialized")
	}

	// 单例已创建后，无论传什么参数都应返回同一个实例，忽略新参数
	cm2 := GetConfigManager("ignored-path.json")
	if cm2 != cm1 {
		t.Errorf("expected same singleton instance regardless of new arguments")
	}
}
