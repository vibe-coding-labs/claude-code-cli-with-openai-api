package config

import (
	"os"
	"testing"
)

// configEnvKeys 列出 LoadConfig 会读取的所有环境变量，测试前后需要精确控制其存在性，
// 避免真实开发机上残留的环境变量（或其它测试的 t.Setenv）污染断言。
var configEnvKeys = []string{
	"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_BASE_URL", "AZURE_API_VERSION",
	"HOST", "PORT", "LOG_LEVEL", "MAX_TOKENS_LIMIT", "MIN_TOKENS_LIMIT",
	"REQUEST_TIMEOUT", "RETRY_COUNT", "BIG_MODEL", "SMALL_MODEL", "MIDDLE_MODEL",
	"ENABLE_REQUEST_LOGGING", "UPSTREAM_ENDPOINT",
}

// resetConfigEnv 清空所有相关环境变量并在测试结束后恢复原值，
// 保证 LoadConfig 的默认值断言不受宿主环境影响。
func resetConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range configEnvKeys {
		orig, had := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func(key, orig string, had bool) func() {
			return func() {
				if had {
					os.Setenv(key, orig)
				} else {
					os.Unsetenv(key)
				}
			}
		}(key, orig, had))
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	checks := map[string]struct {
		got  interface{}
		want interface{}
	}{
		"OpenAIBaseURL":        {cfg.OpenAIBaseURL, "https://api.openai.com/v1"},
		"Host":                 {cfg.Host, "0.0.0.0"},
		"Port":                 {cfg.Port, 54988},
		"LogLevel":             {cfg.LogLevel, "INFO"},
		"MaxTokensLimit":       {cfg.MaxTokensLimit, 4096},
		"MinTokensLimit":       {cfg.MinTokensLimit, 100},
		"RequestTimeout":       {cfg.RequestTimeout, 1800},
		"RetryCount":           {cfg.RetryCount, 20},
		"BigModel":             {cfg.BigModel, "gpt-4o"},
		"SmallModel":           {cfg.SmallModel, "gpt-4o-mini"},
		"EnableRequestLogging": {cfg.EnableRequestLogging, false},
		"UpstreamEndpoint":     {cfg.UpstreamEndpoint, "chat/completions"},
		"OpenAIAPIKey":         {cfg.OpenAIAPIKey, ""},
		"AnthropicAPIKey":      {cfg.AnthropicAPIKey, ""},
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", name, c.got, c.want)
		}
	}

	// MIDDLE_MODEL 未设置时应回落到 BigModel
	if cfg.MiddleModel != cfg.BigModel {
		t.Errorf("MiddleModel = %q, want fallback to BigModel %q", cfg.MiddleModel, cfg.BigModel)
	}

	if GlobalConfig != cfg {
		t.Errorf("GlobalConfig was not set to the loaded config")
	}
}

func TestLoadConfig_CustomEnvOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("OPENAI_API_KEY", "sk-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")
	t.Setenv("OPENAI_BASE_URL", "https://custom.example.com/v1")
	t.Setenv("AZURE_API_VERSION", "2024-05-01")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9999")
	t.Setenv("LOG_LEVEL", "DEBUG")
	t.Setenv("MAX_TOKENS_LIMIT", "8192")
	t.Setenv("MIN_TOKENS_LIMIT", "200")
	t.Setenv("REQUEST_TIMEOUT", "60")
	t.Setenv("RETRY_COUNT", "5")
	t.Setenv("BIG_MODEL", "big-model-x")
	t.Setenv("SMALL_MODEL", "small-model-x")
	t.Setenv("MIDDLE_MODEL", "middle-model-x")
	t.Setenv("ENABLE_REQUEST_LOGGING", "true")
	t.Setenv("UPSTREAM_ENDPOINT", "responses")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.OpenAIAPIKey != "sk-test-key" {
		t.Errorf("OpenAIAPIKey = %q", cfg.OpenAIAPIKey)
	}
	if cfg.AnthropicAPIKey != "anthropic-test-key" {
		t.Errorf("AnthropicAPIKey = %q", cfg.AnthropicAPIKey)
	}
	if cfg.OpenAIBaseURL != "https://custom.example.com/v1" {
		t.Errorf("OpenAIBaseURL = %q", cfg.OpenAIBaseURL)
	}
	if cfg.AzureAPIVersion != "2024-05-01" {
		t.Errorf("AzureAPIVersion = %q", cfg.AzureAPIVersion)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q", cfg.Host)
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.MaxTokensLimit != 8192 {
		t.Errorf("MaxTokensLimit = %d", cfg.MaxTokensLimit)
	}
	if cfg.MinTokensLimit != 200 {
		t.Errorf("MinTokensLimit = %d", cfg.MinTokensLimit)
	}
	if cfg.RequestTimeout != 60 {
		t.Errorf("RequestTimeout = %d", cfg.RequestTimeout)
	}
	if cfg.RetryCount != 5 {
		t.Errorf("RetryCount = %d", cfg.RetryCount)
	}
	if cfg.BigModel != "big-model-x" {
		t.Errorf("BigModel = %q", cfg.BigModel)
	}
	if cfg.SmallModel != "small-model-x" {
		t.Errorf("SmallModel = %q", cfg.SmallModel)
	}
	// 显式设置 MIDDLE_MODEL 时不应被 BigModel 覆盖
	if cfg.MiddleModel != "middle-model-x" {
		t.Errorf("MiddleModel = %q, want explicit middle-model-x", cfg.MiddleModel)
	}
	if !cfg.EnableRequestLogging {
		t.Errorf("EnableRequestLogging = false, want true")
	}
	if cfg.UpstreamEndpoint != "responses" {
		t.Errorf("UpstreamEndpoint = %q", cfg.UpstreamEndpoint)
	}
}

func TestLoadConfig_InvalidIntAndBoolFallBackToDefault(t *testing.T) {
	resetConfigEnv(t)

	// 非法的数字/布尔字符串应当静默回落到默认值，而不是返回错误
	t.Setenv("MAX_TOKENS_LIMIT", "not-a-number")
	t.Setenv("ENABLE_REQUEST_LOGGING", "not-a-bool")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}
	if cfg.MaxTokensLimit != 4096 {
		t.Errorf("MaxTokensLimit = %d, want default 4096 on parse failure", cfg.MaxTokensLimit)
	}
	if cfg.EnableRequestLogging {
		t.Errorf("EnableRequestLogging = true, want default false on parse failure")
	}
}

func TestLoadConfig_CustomHeaders(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("CUSTOM_HEADER_X_MY_HEADER", "hello")
	t.Setenv("CUSTOM_HEADER_AUTHORIZATION", "Bearer xyz")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if got := cfg.CustomHeaders["X-MY-HEADER"]; got != "hello" {
		t.Errorf("CustomHeaders[X-MY-HEADER] = %q, want %q", got, "hello")
	}
	if got := cfg.CustomHeaders["AUTHORIZATION"]; got != "Bearer xyz" {
		t.Errorf("CustomHeaders[AUTHORIZATION] = %q, want %q", got, "Bearer xyz")
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"empty key", "", false},
		{"missing sk- prefix", "abc123", false},
		{"valid prefix", "sk-abc123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{OpenAIAPIKey: tt.key}
			if got := c.ValidateAPIKey(); got != tt.want {
				t.Errorf("ValidateAPIKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateClientAPIKey(t *testing.T) {
	t.Run("no anthropic key configured skips validation", func(t *testing.T) {
		c := &Config{AnthropicAPIKey: ""}
		if !c.ValidateClientAPIKey("anything") {
			t.Errorf("expected true when AnthropicAPIKey is empty")
		}
	})

	t.Run("matching key is valid", func(t *testing.T) {
		c := &Config{AnthropicAPIKey: "secret-key"}
		if !c.ValidateClientAPIKey("secret-key") {
			t.Errorf("expected true for matching key")
		}
	})

	t.Run("mismatched key is invalid", func(t *testing.T) {
		c := &Config{AnthropicAPIKey: "secret-key"}
		if c.ValidateClientAPIKey("wrong-key") {
			t.Errorf("expected false for mismatched key")
		}
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_ENV_STR", "value")
		if got := getEnvOrDefault("TEST_ENV_STR", "default"); got != "value" {
			t.Errorf("got %q, want %q", got, "value")
		}
	})
	t.Run("returns default when unset", func(t *testing.T) {
		os.Unsetenv("TEST_ENV_STR_UNSET")
		if got := getEnvOrDefault("TEST_ENV_STR_UNSET", "default"); got != "default" {
			t.Errorf("got %q, want %q", got, "default")
		}
	})
}

func TestGetEnvAsInt(t *testing.T) {
	t.Run("returns parsed value", func(t *testing.T) {
		t.Setenv("TEST_ENV_INT", "42")
		if got := getEnvAsInt("TEST_ENV_INT", 1); got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})
	t.Run("returns default when unset", func(t *testing.T) {
		os.Unsetenv("TEST_ENV_INT_UNSET")
		if got := getEnvAsInt("TEST_ENV_INT_UNSET", 7); got != 7 {
			t.Errorf("got %d, want 7", got)
		}
	})
	t.Run("returns default on parse error", func(t *testing.T) {
		t.Setenv("TEST_ENV_INT_BAD", "not-an-int")
		if got := getEnvAsInt("TEST_ENV_INT_BAD", 3); got != 3 {
			t.Errorf("got %d, want 3", got)
		}
	})
}

func TestGetEnvAsBool(t *testing.T) {
	t.Run("returns parsed true", func(t *testing.T) {
		t.Setenv("TEST_ENV_BOOL", "true")
		if got := getEnvAsBool("TEST_ENV_BOOL", false); !got {
			t.Errorf("got false, want true")
		}
	})
	t.Run("returns default when unset", func(t *testing.T) {
		os.Unsetenv("TEST_ENV_BOOL_UNSET")
		if got := getEnvAsBool("TEST_ENV_BOOL_UNSET", true); !got {
			t.Errorf("got false, want default true")
		}
	})
	t.Run("returns default on parse error", func(t *testing.T) {
		t.Setenv("TEST_ENV_BOOL_BAD", "not-a-bool")
		if got := getEnvAsBool("TEST_ENV_BOOL_BAD", true); !got {
			t.Errorf("got false, want default true on parse failure")
		}
	})
}
