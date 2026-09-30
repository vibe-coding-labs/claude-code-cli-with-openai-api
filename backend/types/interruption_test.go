package types

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The error strings below are the exact messages produced at the real signal
// points (handler.go:523, response_converter.go:321, stall_detector.go:15,
// responses.go:579). Locking them here guards the taxonomy against drift.
func TestClassifyByErrorText(t *testing.T) {
	cases := []struct {
		name string
		err  error
		wantCause     string
		wantDimension string
	}{
		{"client disconnect (converter)", errors.New("client disconnected"), CauseClientDisconnect, DimensionSubjective},
		{"client disconnect before request", errors.New("client disconnected before request"), CauseClientDisconnect, DimensionSubjective},
		{"upstream stalled", errors.New("upstream stalled: no data received within stall timeout"), CauseUpstreamStall, DimensionInfrastructure},
		{"rate limit", errors.New("429 rate limit exceeded"), CauseRateLimit, DimensionInfrastructure},
		{"auth 401", errors.New(`401 {"error":{"type":"authentication_error"}}`), CauseAuthError, DimensionInfrastructure},
		{"auth 403 quota RELAY_101", errors.New(`403 {"error":{"type":"api_error","message":"余额或额度不足：余额或订阅额度不足，请充值 [RELAY_101]"}}`), CauseAuthError, DimensionInfrastructure},
		{"orphan bytes", errors.New("orphan bytes detected in SSE stream"), CauseConversionError, DimensionInfrastructure},
		{"generic upstream 500", errors.New("upstream returned 500 internal server error"), CauseUpstreamError, DimensionInfrastructure},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cause, dim := Classify(c.err, context.Background())
			if cause != c.wantCause {
				t.Errorf("Classify cause = %q, want %q", cause, c.wantCause)
			}
			if dim != c.wantDimension {
				t.Errorf("Classify dimension = %q, want %q", dim, c.wantDimension)
			}
		})
	}
}

// Context is decisive over error text for client-side termination.
func TestClassifyContextDecisive(t *testing.T) {
	// A canceled context wins even if the error text is generic.
	cause, dim := Classify(errors.New("some random error"), ctxCanceled())
	if cause != CauseClientDisconnect {
		t.Errorf("canceled ctx -> cause = %q, want client_disconnect", cause)
	}
	if dim != DimensionSubjective {
		t.Errorf("canceled ctx -> dimension = %q, want subjective", dim)
	}

	cause, dim = Classify(errors.New("some random error"), ctxDeadline())
	if cause != CauseClientTimeout {
		t.Errorf("deadline ctx -> cause = %q, want client_timeout", cause)
	}
	if dim != DimensionSubjective {
		t.Errorf("deadline ctx -> dimension = %q, want subjective", dim)
	}
}

func TestClassifyNilErrNoCtx(t *testing.T) {
	cause, dim := Classify(nil, nil)
	if cause != CauseUnknown {
		t.Errorf("nil err -> cause = %q, want unknown", cause)
	}
	if dim != DimensionInfrastructure {
		t.Errorf("nil err -> dimension = %q, want infrastructure (default)", dim)
	}
}

// CauseDimension is a pure mapping; spot-check a few non-obvious rows.
func TestCauseDimensionMapping(t *testing.T) {
	checks := map[string]string{
		CauseClientDisconnect: DimensionSubjective,
		CauseClientTimeout:    DimensionSubjective,
		CauseUpstreamStall:    DimensionInfrastructure,
		CauseRateLimit:        DimensionInfrastructure,
		CauseAuthError:        DimensionInfrastructure,
		CauseServerTermination: DimensionInfrastructure,
	}
	for cause, wantDim := range checks {
		if got := CauseDimension(cause); got != wantDim {
			t.Errorf("CauseDimension(%s) = %q, want %q", cause, got, wantDim)
		}
	}
}

func ctxCanceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func ctxDeadline() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}
