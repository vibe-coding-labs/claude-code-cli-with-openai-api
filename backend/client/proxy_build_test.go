package client

import (
	"net/http"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

func TestProxyConfigFromConfig(t *testing.T) {
	t.Run("config-level proxy takes priority", func(t *testing.T) {
		cfg := &config.Config{
			ProxyURL:        "http://config-proxy:8080",
			ProxyType:       "http",
			ProxyUsername:   "u1",
			ProxyPassword:   "p1",
			SystemProxyURL:  "http://system-proxy:9090",
			SystemProxyType: "socks5",
		}
		pc := ProxyConfigFromConfig(cfg)
		if pc.ProxyURL != "http://config-proxy:8080" || pc.ProxyType != "http" || pc.ProxyUsername != "u1" || pc.ProxyPassword != "p1" {
			t.Errorf("expected config-level proxy to win, got %+v", pc)
		}
	})

	t.Run("falls back to system proxy when config proxy empty", func(t *testing.T) {
		cfg := &config.Config{
			SystemProxyURL:      "http://system-proxy:9090",
			SystemProxyType:     "socks5",
			SystemProxyUsername: "su",
			SystemProxyPassword: "sp",
		}
		pc := ProxyConfigFromConfig(cfg)
		if pc.ProxyURL != "http://system-proxy:9090" || pc.ProxyType != "socks5" || pc.ProxyUsername != "su" || pc.ProxyPassword != "sp" {
			t.Errorf("expected system-level fallback, got %+v", pc)
		}
	})

	t.Run("both empty yields empty config", func(t *testing.T) {
		cfg := &config.Config{}
		pc := ProxyConfigFromConfig(cfg)
		if pc.ProxyURL != "" {
			t.Errorf("expected empty proxy URL, got %+v", pc)
		}
	})
}

func TestResolveProxyType(t *testing.T) {
	tests := []struct {
		name      string
		proxyURL  string
		proxyType string
		want      string
	}{
		{"explicit http", "http://x:1", "http", "http"},
		{"explicit https", "http://x:1", "https", "https"},
		{"explicit socks5", "http://x:1", "socks5", "socks5"},
		{"explicit socks5h", "http://x:1", "socks5h", "socks5h"},
		{"unknown explicit type falls back to http", "http://x:1", "carrier-pigeon", "http"},
		{"auto infers socks5 from scheme", "socks5://x:1", "auto", "socks5"},
		{"auto infers socks5h from scheme", "socks5h://x:1", "auto", "socks5h"},
		{"auto infers http from http scheme", "http://x:1", "auto", "http"},
		{"auto infers http from https scheme", "https://x:1", "", "http"},
		{"empty type defaults to auto-inference", "socks5://x:1", "", "socks5"},
		{"invalid url with explicit auto falls back to http", "://bad-url", "auto", "http"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveProxyType(tt.proxyURL, tt.proxyType); got != tt.want {
				t.Errorf("resolveProxyType(%q, %q) = %q, want %q", tt.proxyURL, tt.proxyType, got, tt.want)
			}
		})
	}
}

func TestBuildTransportProxy_NoProxyFallsBackToEnvironment(t *testing.T) {
	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{})
	if dialCtx != nil {
		t.Errorf("expected nil dialCtx when no proxy configured")
	}
	if proxyFunc == nil {
		t.Fatalf("expected non-nil proxyFunc (http.ProxyFromEnvironment)")
	}
}

func TestBuildTransportProxy_InvalidURLFallsBackToEnvironment(t *testing.T) {
	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{ProxyURL: "://not-a-valid-url"})
	if dialCtx != nil {
		t.Errorf("expected nil dialCtx on invalid proxy URL")
	}
	if proxyFunc == nil {
		t.Fatalf("expected fallback proxyFunc on invalid URL")
	}
}

func TestBuildTransportProxy_HTTPProxyWithCredentials(t *testing.T) {
	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{
		ProxyURL:      "http://proxy.example.com:8080",
		ProxyType:     "http",
		ProxyUsername: "alice",
		ProxyPassword: "secret",
	})
	if dialCtx != nil {
		t.Errorf("expected nil dialCtx for HTTP proxy (uses stdlib Proxy func instead)")
	}
	if proxyFunc == nil {
		t.Fatalf("expected non-nil proxyFunc for HTTP proxy")
	}
	req, _ := http.NewRequest("GET", "https://api.example.com/v1/chat", nil)
	resolved, err := proxyFunc(req)
	if err != nil {
		t.Fatalf("proxyFunc returned error: %v", err)
	}
	if resolved == nil || resolved.Host != "proxy.example.com:8080" {
		t.Errorf("expected resolved proxy host proxy.example.com:8080, got %v", resolved)
	}
	if resolved.User == nil {
		t.Fatalf("expected userinfo to carry credentials")
	}
	if user := resolved.User.Username(); user != "alice" {
		t.Errorf("expected username alice, got %q", user)
	}
	if pass, _ := resolved.User.Password(); pass != "secret" {
		t.Errorf("expected password secret, got %q", pass)
	}
}

func TestBuildTransportProxy_SOCKS5WithCredentials(t *testing.T) {
	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{
		ProxyURL:      "socks5://proxy.example.com:1080",
		ProxyType:     "socks5",
		ProxyUsername: "bob",
		ProxyPassword: "hunter2",
	})
	if proxyFunc != nil {
		t.Errorf("expected nil proxyFunc for SOCKS5 (uses custom DialContext instead)")
	}
	if dialCtx == nil {
		t.Fatalf("expected non-nil dialCtx for SOCKS5 proxy")
	}
}

func TestBuildTransportProxy_SOCKS5NoCredentials(t *testing.T) {
	proxyFunc, dialCtx := BuildTransportProxy(ProxyConfig{
		ProxyURL:  "socks5h://proxy.example.com:1080",
		ProxyType: "auto",
	})
	if proxyFunc != nil {
		t.Errorf("expected nil proxyFunc for SOCKS5")
	}
	if dialCtx == nil {
		t.Fatalf("expected non-nil dialCtx for SOCKS5 proxy")
	}
}
