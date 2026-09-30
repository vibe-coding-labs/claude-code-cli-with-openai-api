package converter

import (
	"github.com/gin-gonic/gin"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// Gin context keys the handler stashes before streaming so the converter can
// attribute interruption events without re-plumbing config/session everywhere.
const (
	ginKeyConfigID  = "cfg_id"
	ginKeyConfigName = "cfg_name"
	ginKeySessionID  = "session_id"
)

// SetInterruptionContext records per-request attribution on the gin context.
// The handler calls this once it resolves the config + session, before any
// streaming; the converter's EmitInterruption then reads these back.
func SetInterruptionContext(c *gin.Context, configID, configName, sessionID string) {
	if configID != "" {
		c.Set(ginKeyConfigID, configID)
	}
	if configName != "" {
		c.Set(ginKeyConfigName, configName)
	}
	if sessionID != "" {
		c.Set(ginKeySessionID, sessionID)
	}
}

// EmitInterruption queues a session-interruption event (async, non-blocking).
// It is the single instrumentation point for both the converter and the handler.
//
// config/session come from the gin keys set by SetInterruptionContext; model is
// passed explicitly (the caller knows the authoritative value — e.g. state.model
// in the converter, openAIReq.Model in the handler). client_ip comes from gin.
func EmitInterruption(c *gin.Context, cause, dimension, stage, detail string, durationMs int64, model string) {
	if c == nil {
		return
	}
	database.LogInterruptionAsync(&database.SessionInterruption{
		ConfigID:          c.GetString(ginKeyConfigID),
		ConfigName:        c.GetString(ginKeyConfigName),
		SessionID:         c.GetString(ginKeySessionID),
		ClientIP:          c.ClientIP(),
		Model:             model,
		InterruptionCause: cause,
		Dimension:         dimension,
		Stage:             stage,
		Detail:            detail,
		DurationMs:        durationMs,
	})
}
