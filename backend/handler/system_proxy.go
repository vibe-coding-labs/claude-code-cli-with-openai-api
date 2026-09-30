package handler

import (
	"log"
	"strings"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// 服务级全局默认代理的 system_settings 键名。
// config 级 proxy_url 为空时回落到这里（见 client.ProxyConfigFromConfig）。
const (
	settingProxyURL      = "proxy_url"
	settingProxyType     = "proxy_type"
	settingProxyUsername = "proxy_username"
	// settingProxyPassword 是前端看到的明文键（GET 回显 / PUT 提交同一个键）；
	// 落库时统一加密为 settingProxyPasswordEncrypted。
	settingProxyPassword = "proxy_password"
	// settingProxyPasswordEncrypted 存储加密后的代理密码；只在库内使用，不出现在 API 响应。
	settingProxyPasswordEncrypted = "proxy_password_encrypted"
)

// ApplySystemSettings 在服务启动时把 system_settings 表中的全局默认代理
// 读入 h.config.SystemProxy*。必须在 database.InitDB + InitEncryption 之后、
// 路由对外服务之前调用。
//
// 不能放进 config.LoadConfig()：database 包 import config 包（models.go 的
// ToConfig），config 若再 import database 会形成循环依赖。handler 已持有
// *config.Config 且 import database，是读取的合适落点。
func (h *Handler) ApplySystemSettings() {
	if h.config == nil {
		return
	}

	urlVal, _ := database.GetSetting(settingProxyURL)
	typeVal, _ := database.GetSetting(settingProxyType)
	userVal, _ := database.GetSetting(settingProxyUsername)
	passEnc, _ := database.GetSetting(settingProxyPasswordEncrypted)

	pass := ""
	if strings.TrimSpace(passEnc) != "" {
		dec, err := database.DecryptAPIKey(passEnc)
		if err != nil {
			log.Printf("[system-proxy] failed to decrypt proxy_password_encrypted, using empty password: %v", err)
		} else {
			pass = dec
		}
	}

	h.config.SystemProxyURL = urlVal
	h.config.SystemProxyType = typeVal
	h.config.SystemProxyUsername = userVal
	h.config.SystemProxyPassword = pass

	if urlVal != "" {
		userLog := ""
		if userVal != "" {
			userLog = "username set"
		}
		log.Printf("[system-proxy] service-wide default proxy loaded: type=%s url=%s (%s)",
			typeValOrAuto(typeVal), urlVal, userLog)
	}
}

// typeValOrAuto 展示用：空 type 按 auto 推断。
func typeValOrAuto(t string) string {
	if strings.TrimSpace(t) == "" {
		return "auto"
	}
	return t
}
