package client

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
	"golang.org/x/net/proxy"
)

// ProxyConfig carries the effective proxy settings for a transport.
// It is derived from config.Config with the system-wide fallback already
// applied (see ProxyConfigFromConfig).
type ProxyConfig struct {
	ProxyURL      string // 可为空：空则回落环境变量（ProxFromEnvironment，与现状一致）
	ProxyType     string // auto/http/https/socks5/socks5h；auto 按 proxy_url scheme 推断
	ProxyUsername string // 可选代理认证用户名
	ProxyPassword string // 可选代理认证密码
}

// ProxyConfigFromConfig extracts the effective proxy settings for one request
// path: per-config proxy takes priority, falling back to the service-wide
// system default proxy when the config-level proxy_url is empty.
func ProxyConfigFromConfig(cfg *config.Config) ProxyConfig {
	pc := ProxyConfig{
		ProxyURL:      cfg.ProxyURL,
		ProxyType:     cfg.ProxyType,
		ProxyUsername: cfg.ProxyUsername,
		ProxyPassword: cfg.ProxyPassword,
	}
	if pc.ProxyURL == "" && cfg.SystemProxyURL != "" {
		pc = ProxyConfig{
			ProxyURL:      cfg.SystemProxyURL,
			ProxyType:     cfg.SystemProxyType,
			ProxyUsername: cfg.SystemProxyUsername,
			ProxyPassword: cfg.SystemProxyPassword,
		}
	}
	return pc
}

// resolveProxyType decides which proxy client to build. 'auto' (or empty)
// infers from the proxy_url scheme: socks5/socks5h -> SOCKS5, everything else
// (http/https) -> HTTP proxy. An explicit value overrides the inference.
func resolveProxyType(proxyURL, proxyType string) string {
	t := strings.ToLower(strings.TrimSpace(proxyType))
	if t != "" && t != "auto" {
		if t == "http" || t == "https" || t == "socks5" || t == "socks5h" {
			return t
		}
		return "http" // 未知 type 兜底为 HTTP
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return "http"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h":
		return parsed.Scheme
	default:
		return "http"
	}
}

// BuildTransportProxy returns the pieces an http.Transport needs to route
// through the configured intermediate proxy:
//
//   - HTTP/HTTPS proxy: a Proxy func (stdlib auto-CONNECT for https targets;
//     credentials become Proxy-Authorization). dialCtx is nil -> transport
//     uses its default dialer.
//   - SOCKS5: Proxy is nil and a custom DialContext is returned that performs
//     the SOCKS5 handshake (x/net/proxy). Domain names are resolved by the
//     proxy server (FQDN addressing), matching both socks5 and socks5h.
//
// With no proxy configured it falls back to http.ProxyFromEnvironment,
// preserving the pre-existing behavior.
func BuildTransportProxy(pc ProxyConfig) (proxyFunc func(*http.Request) (*url.URL, error), dialCtx func(ctx context.Context, network, addr string) (net.Conn, error)) {
	if strings.TrimSpace(pc.ProxyURL) == "" {
		return http.ProxyFromEnvironment, nil
	}

	parsed, err := url.Parse(pc.ProxyURL)
	if err != nil {
		utils.GetLogger().Warn("[proxy] invalid proxy_url %q, falling back to environment proxy: %v", pc.ProxyURL, err)
		return http.ProxyFromEnvironment, nil
	}

	switch resolveProxyType(pc.ProxyURL, pc.ProxyType) {
	case "socks5", "socks5h":
		// Credentials ride in the URL userinfo; x/net/proxy extracts them for
		// the SOCKS5 username/password authentication handshake.
		if pc.ProxyUsername != "" || pc.ProxyPassword != "" {
			parsed.User = url.UserPassword(pc.ProxyUsername, pc.ProxyPassword)
		}
		// tlsHandshakeTimeout governs the TLS session above the tunnel, so the
		// underlying TCP dial to the SOCKS server gets its own generous cap.
		dialer, err := proxy.FromURL(parsed, &net.Dialer{Timeout: 60 * time.Second})
		if err != nil {
			utils.GetLogger().Warn("[proxy] failed to build SOCKS5 dialer for %q, falling back to environment proxy: %v", pc.ProxyURL, err)
			return http.ProxyFromEnvironment, nil
		}
		dialCtx = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				return cd.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		}
		return nil, dialCtx

	default: // http / https
		if pc.ProxyUsername != "" || pc.ProxyPassword != "" {
			parsed.User = url.UserPassword(pc.ProxyUsername, pc.ProxyPassword)
		}
		// Go's stdlib transport sends Proxy-Authorization from the URL userinfo
		// and performs an automatic CONNECT tunnel for https targets.
		return http.ProxyURL(parsed), nil
	}
}
