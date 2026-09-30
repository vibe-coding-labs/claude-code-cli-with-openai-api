package converter

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/types"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// panicToError converts a recovered panic value into an error that preserves the
// stack trace. A bare "%v" would tell you a panic happened but not *where* —
// with the stack, an incident log line becomes actionable for post-mortem.
func panicToError(r interface{}) error {
	return fmt.Errorf("conversion panic: %v\n%s", r, debug.Stack())
}

// guardStreamPanic is the defer body shared by the streaming scanner goroutines.
// It handles a panic value `r` that the *caller's* deferred function captured via
// recover(): logs it with its stack trace, records a conversion-error
// interruption, and pushes the error onto errChan so the main select loop turns
// it into a well-formed SSE error (the client stays on the protocol instead of
// hanging). It returns true if a panic was recovered.
//
// CRITICAL — callers MUST call recover() directly in their deferred body and pass
// the value in here, NOT call recover() yourself. Go's recover() only returns the
// panic value if it runs in the same frame the panic is unwinding (the deferred
// closure). Calling recover() from a helper that the deferred closure merely calls
// yields nil, so the panic would escape uncaught and crash the goroutine — an
// entire class of silent robustness regression. See the call sites for the shape.
//
// Callers must also close `done` ONLY when guardStreamPanic reports false: on a
// panic it pushes to errChan and the main select must take the error path
// deterministically, not race with a normal completion.
func guardStreamPanic(c *gin.Context, errChan chan error, stage, model string, r interface{}) (recovered bool) {
	if r != nil {
		err := panicToError(r)
		utils.GetLogger().Error("[converter] recovered panic in %s: %v", stage, err)
		// Conversion panics are infra failures (malformed upstream data we failed
		// to handle defensively) — surface them on the interruption panel so they
		// are visible, not just a line buried in logs.
		EmitInterruption(c, types.CauseConversionError, types.DimensionInfrastructure, stage,
			fmt.Sprintf("conversion panic: %v", r), 0, model)
		errChan <- err
		return true
	}
	return false
}

// guardConversionPanic wraps a single-shot conversion entry point so a panic on
// malformed input degrades to a fallback instead of crashing the whole proxy
// process. It logs the panic with its stack, records a conversion-error
// interruption, then returns the result of fallback().
func guardConversionPanic[T any](c *gin.Context, model string, fallback func() T, f func() T) (out T) {
	defer func() {
		if r := recover(); r != nil {
			err := panicToError(r)
			utils.GetLogger().Error("[converter] recovered conversion panic (falling back): %v", err)
			EmitInterruption(c, types.CauseConversionError, types.DimensionInfrastructure, types.StageConversion,
				fmt.Sprintf("conversion panic: %v", r), 0, model)
			out = fallback()
		}
	}()
	return f()
}

// logConversionFallback records when a factory conversion returns an error and
// the proxy silently degrades to the legacy fallback. Before this, such paths
// returned the fallback with no log line at all — making a factory regression
// invisible until users complained. The operation name locates the exact stage.
func logConversionFallback(op string, err error) {
	utils.GetLogger().Error("[converter] %s failed, falling back to legacy conversion: %v", op, err)
}