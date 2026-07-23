package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ClaudeErrorResponse represents a Claude API compatible error response.
// Claude Code CLI uses error type to decide whether to retry automatically.
type ClaudeErrorResponse struct {
	Type  string            `json:"type"`
	Error ClaudeErrorDetail `json:"error"`
}

// ClaudeErrorDetail contains the error details.
type ClaudeErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Claude error types that affect retry behavior:
// - overloaded_error: Claude Code will automatically retry
// - api_error: Claude Code will NOT retry, session stops
// - authentication_error: Invalid API key
// - permission_error: Insufficient permissions
// - not_found_error: Resource not found
// - rate_limit_error: Rate limit exceeded (Claude Code may retry)
const (
	// ErrorTypeOverloaded triggers Claude Code automatic retry
	ErrorTypeOverloaded = "overloaded_error"
	// ErrorTypeAPI does NOT trigger Claude Code retry
	ErrorTypeAPI = "api_error"
	// ErrorTypeAuthentication for invalid API key
	ErrorTypeAuthentication = "authentication_error"
	// ErrorTypePermission for insufficient permissions
	ErrorTypePermission = "permission_error"
	// ErrorTypeNotFound for resource not found
	ErrorTypeNotFound = "not_found_error"
	// ErrorTypeRateLimit for rate limit exceeded
	ErrorTypeRateLimit = "rate_limit_error"
	// ErrorTypeInvalidRequest for bad request
	ErrorTypeInvalidRequest = "invalid_request_error"
)

// SendOverloadedError sends a Claude-compatible overloaded_error response.
// Claude Code will automatically retry when receiving this error type.
func SendOverloadedError(c *gin.Context, message string) {
	c.JSON(http.StatusServiceUnavailable, ClaudeErrorResponse{
		Type: "error",
		Error: ClaudeErrorDetail{
			Type:    ErrorTypeOverloaded,
			Message: message,
		},
	})
}

// SendAPIError sends a Claude-compatible api_error response.
// Claude Code will NOT retry when receiving this error type.
func SendAPIError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, ClaudeErrorResponse{
		Type: "error",
		Error: ClaudeErrorDetail{
			Type:    ErrorTypeAPI,
			Message: message,
		},
	})
}

// SendAuthenticationError sends an authentication_error response.
func SendAuthenticationError(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, ClaudeErrorResponse{
		Type: "error",
		Error: ClaudeErrorDetail{
			Type:    ErrorTypeAuthentication,
			Message: message,
		},
	})
}
