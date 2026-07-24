package converter

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

// DegenerateOutputDetector detects pseudo-tool-call markers and other
// degenerate output patterns that indicate the upstream model produced
// invalid output instead of a proper response.
//
// Some models (e.g., certain DeepSeek variants) emit structured pseudo-tags
// like </｜DSML｜invoke> or </｜DSML｜tool_calls> in their text output when
// they attempt to call tools but the API doesn't support structured tool_calls.
// These markers are meaningless to Claude Code CLI and should be treated as
// degenerate output that warrants a retry.
type DegenerateOutputDetector struct {
	patterns []*regexp.Regexp
	mu       sync.RWMutex
}

// defaultDegeneratePatterns are the built-in patterns for known degenerate output.
// Each pattern is designed for HIGH SPECIFICITY to avoid false positives:
// - Must contain the distinctive ｜DSML｜ or similar structured pseudo-tag markers
// - Only matches closing tags (</...>), not opening tags that might be legitimate XML
// - The full-width pipe ｜ (U+FF5C) is a strong signal — it's never used in legitimate
//   XML/HTML but is common in these model-generated pseudo-tags
var defaultDegeneratePatterns = []string{
	// DSML pseudo-tool-call closing tags (DeepSeek pattern)
	// The full-width pipe ｜ (U+FF5C) is the key discriminant
	`</｜DSML｜\w+>`,
	// DSML pseudo-tool-call opening tags — less common but also degenerate
	`<｜DSML｜\w+>`,
	// Variant with half-width pipe (some models use this)
	`</\|DSML\|\w+>`,
	`<\|DSML\|\w+>`,
}

// globalDetector is the singleton detector instance, initialized once.
var globalDetector *DegenerateOutputDetector
var globalDetectorOnce sync.Once

// GetDegenerateDetector returns the global detector instance.
func GetDegenerateDetector() *DegenerateOutputDetector {
	globalDetectorOnce.Do(func() {
		globalDetector = newDegenerateDetector()
	})
	return globalDetector
}

// newDegenerateDetector creates a detector with default + env-configured patterns.
func newDegenerateDetector() *DegenerateOutputDetector {
	d := &DegenerateOutputDetector{}

	// Compile default patterns
	for _, p := range defaultDegeneratePatterns {
		re, err := regexp.Compile(p)
		if err == nil {
			d.patterns = append(d.patterns, re)
		}
	}

	// Load additional patterns from environment variable
	// PROXY_DEGENERATE_PATTERNS="pattern1;pattern2;pattern3"
	// Patterns are separated by semicolons
	if extra := os.Getenv("PROXY_DEGENERATE_PATTERNS"); extra != "" {
		for _, p := range strings.Split(extra, ";") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			re, err := regexp.Compile(p)
			if err != nil {
				// Log but don't fail — bad pattern is skipped
				continue
			}
			d.patterns = append(d.patterns, re)
		}
	}

	return d
}

// IsDegenerate checks whether the given text contains degenerate output patterns.
// Returns true if any configured pattern matches, along with the first matching pattern.
func (d *DegenerateOutputDetector) IsDegenerate(text string) (bool, string) {
	if text == "" {
		return false, ""
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, re := range d.patterns {
		if re.MatchString(text) {
			return true, re.String()
		}
	}
	return false, ""
}

// AddPattern adds a new detection pattern at runtime (for testing).
func (d *DegenerateOutputDetector) AddPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.patterns = append(d.patterns, re)
	return nil
}
