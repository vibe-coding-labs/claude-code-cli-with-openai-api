// Package types holds shared enums and classification helpers used by both the
// protocol converters and the HTTP handlers. It is dependency-free (no imports
// beyond stdlib) so that handler, converter, and database packages can all
// consume it without creating import cycles.
package types

import (
	"context"
	"errors"
	"strings"
)

// Session-interruption causes. These are the canonical values persisted to
// session_interruptions.interruption_cause and surfaced in the monitoring UI.
// Choose these constants over ad-hoc strings everywhere.
const (
	CauseClientDisconnect = "client_disconnect" // 客户端关流/取消 (Esc, stop, net drop)
	CauseClientTimeout    = "client_timeout"    // 客户端读超时 / ctx deadline exceeded
	CauseUpstreamStall    = "upstream_stall"    // 上游 stall 超时无数据
	CauseUpstreamError    = "upstream_error"    // 上游 4xx/5xx
	CauseRateLimit        = "rate_limit"        // 上游 429
	CauseAuthError        = "auth_error"        // 上游 401/403 (欠费 RELAY_101 等)
	CauseConversionError  = "conversion_error"  // SSE 解析 / orphan-bytes 失败
	CauseServerTermination = "server_termination" // 优雅停机 drain (SIGTERM), 不告警
	CauseUnknown          = "unknown"
)

// Interruption dimensions. A single field gates alerting and auto-restart:
// subjective events (user stopped) never page and never trigger a restart;
// infrastructure events (proxy/upstream fault) do.
const (
	DimensionSubjective     = "subjective"     // 用户主动停
	DimensionInfrastructure = "infrastructure" // 代理/上游故障
)

// Stages, mirroring database/proxy_errors.go request stages.
const (
	StageRequest    = "request"
	StageStreaming  = "streaming"
	StageConversion = "conversion"
	StageResponse   = "response"
)

// CauseDimension returns the dimension for a cause, so callers do not need to
// maintain a second mapping table.
func CauseDimension(cause string) string {
	switch cause {
	case CauseClientDisconnect, CauseClientTimeout:
		return DimensionSubjective
	case CauseUpstreamStall, CauseUpstreamError, CauseRateLimit, CauseAuthError,
		CauseConversionError, CauseServerTermination:
		return DimensionInfrastructure
	default:
		return DimensionInfrastructure // unknown defaults to observable/infrastructure
	}
}

// Error signatures we match on. Kept as distinctive substrings so we can
// classify error strings produced at the real signal points without importing
// the handler package (avoids an import cycle).
const (
	msgClientDisconnected = "client disconnected"
	msgUpstreamStalled    = "upstream stalled"
	msgOrphanBytes        = "orphan"
	msgSSEParse           = "sse" // SSE parse error
	msgRateLimit          = "429"
	msgAuth401            = "401"
	msgAuth403            = "403"
)

// Classify maps an error (and its context) to a cause + dimension.
//
// The context is authoritative for client-initiated terminations: a canceled
// context means the client closed the stream (client_disconnect), a deadline
// exceeded means the client-side read timed out (client_timeout). Error text is
// consulted only when the context is not the deciding signal. stage is not
// derived here — callers know where the interruption occurred and set it.
func Classify(err error, ctx context.Context) (cause string, dimension string) {
	// 1. Context signals are decisive for client-side termination.
	if ctx != nil {
		ctxErr := ctx.Err()
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return CauseClientTimeout, CauseDimension(CauseClientTimeout)
		}
		if errors.Is(ctxErr, context.Canceled) {
			return CauseClientDisconnect, CauseDimension(CauseClientDisconnect)
		}
	}

	if err == nil {
		return CauseUnknown, CauseDimension(CauseUnknown)
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, msgUpstreamStalled):
		return CauseUpstreamStall, CauseDimension(CauseUpstreamStall)
	case strings.Contains(msg, msgClientDisconnected):
		return CauseClientDisconnect, CauseDimension(CauseClientDisconnect)
	case strings.Contains(msg, msgRateLimit):
		return CauseRateLimit, CauseDimension(CauseRateLimit)
	case strings.Contains(msg, msgAuth401), strings.Contains(msg, msgAuth403):
		return CauseAuthError, CauseDimension(CauseAuthError)
	case strings.Contains(msg, msgOrphanBytes), strings.Contains(msg, msgSSEParse):
		return CauseConversionError, CauseDimension(CauseConversionError)
	}

	// Fall back: generic 4xx/5xx or anything else surfaces as upstream_error.
	return CauseUpstreamError, CauseDimension(CauseUpstreamError)
}
