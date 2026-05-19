package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ProxyDetectResult struct {
	Found    bool   `json:"found"`
	HTTPURL  string `json:"http_url,omitempty"`
	SOCKSURL string `json:"socks_url,omitempty"`
	Name     string `json:"name,omitempty"`
	Port     int    `json:"port,omitempty"`
}

func (h *Handler) DetectProxy(c *gin.Context) {
	result := detectLocalProxy()
	c.JSON(http.StatusOK, result)
}

func detectLocalProxy() []ProxyDetectResult {
	var results []ProxyDetectResult

	// Common proxy software and their default ports
	type proxyCandidate struct {
		name         string
		httpPort     int
		socksPort    int
		apiPort      int
		apiPath      string
		apiRespCheck func(map[string]interface{}) bool
	}

	candidates := []proxyCandidate{
		{
			name:     "Clash",
			httpPort: 7890,
			socksPort: 7891,
			apiPort:  9090,
			apiPath:  "/version",
			apiRespCheck: func(resp map[string]interface{}) bool {
				// Clash API returns {"meta":true|false,"version":"..."}
				_, ok := resp["version"]
				return ok
			},
		},
		{
			name:     "Clash Verge",
			httpPort: 7897,
			socksPort: 7898,
			apiPort:  9097,
			apiPath:  "/version",
			apiRespCheck: func(resp map[string]interface{}) bool {
				_, ok := resp["version"]
				return ok
			},
		},
		{
			name:     "V2rayN",
			httpPort: 10809,
			socksPort: 10808,
		},
		{
			name:     "Shadowsocks",
			httpPort: 1087,
			socksPort: 1086,
		},
		{
			name:     "Surge",
			httpPort: 6152,
			socksPort: 6153,
		},
	}

	for _, candidate := range candidates {
		// Try API endpoint first (for Clash-like clients)
		if candidate.apiPort > 0 {
			if verifyProxyAPI(candidate.apiPort, candidate.apiPath, candidate.apiRespCheck) {
				r := ProxyDetectResult{
					Found: true,
					Name:  candidate.name,
					Port:  candidate.httpPort,
				}
				if isPortOpen(candidate.httpPort) {
					r.HTTPURL = fmt.Sprintf("http://127.0.0.1:%d", candidate.httpPort)
				}
				if isPortOpen(candidate.socksPort) {
					r.SOCKSURL = fmt.Sprintf("socks5://127.0.0.1:%d", candidate.socksPort)
				}
				results = append(results, r)
				continue
			}
		}

		// Fallback: just check if the HTTP proxy port is open
		if isPortOpen(candidate.httpPort) {
			results = append(results, ProxyDetectResult{
				Found:   true,
				HTTPURL: fmt.Sprintf("http://127.0.0.1:%d", candidate.httpPort),
				Name:    candidate.name,
				Port:    candidate.httpPort,
			})
		}
	}

	return results
}

func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func verifyProxyAPI(port int, path string, check func(map[string]interface{}) bool) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	if check == nil {
		return true
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}

	return check(body)
}

// Read Clash config to get actual ports (if available)
func readClashConfigPorts(configPath string) (httpPort, socksPort int) {
	// This is a best-effort function
	return 0, 0
}

// parseYAMLPorts is a minimal YAML port parser for Clash config
func parseYAMLPorts(data string) (httpPort, socksPort int) {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "port:") {
			fmt.Sscanf(strings.TrimPrefix(line, "port:"), "%d", &httpPort)
		}
		if strings.HasPrefix(line, "socks-port:") {
			fmt.Sscanf(strings.TrimPrefix(line, "socks-port:"), "%d", &socksPort)
		}
	}
	return
}
