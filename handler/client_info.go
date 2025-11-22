package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// GetClientIP extracts the real client IP from the request
// Handles X-Forwarded-For, X-Real-IP headers and falls back to RemoteAddr
func GetClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header first
	forwarded := c.GetHeader("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can be a comma-separated list
		// The first one is the original client
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Check X-Real-IP header
	realIP := c.GetHeader("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	// Remove port if present
	ip := c.ClientIP()
	return ip
}

// GetUserAgent extracts the User-Agent from the request
func GetUserAgent(c *gin.Context) string {
	ua := c.GetHeader("User-Agent")
	if ua == "" {
		return "unknown"
	}
	// Truncate if too long (max 500 chars)
	if len(ua) > 500 {
		return ua[:500]
	}
	return ua
}

// GetClientInfo extracts both IP and User-Agent from the request
func GetClientInfo(c *gin.Context) (string, string) {
	return GetClientIP(c), GetUserAgent(c)
}
