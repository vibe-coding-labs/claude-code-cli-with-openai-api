package converter

import (
	"sync"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// geminiPlaceholderThoughtSignature is Google's documented fallback value
// for when a function call's real thought_signature isn't available.
// See: https://ai.google.dev/gemini-api/docs/thought-signatures
const geminiPlaceholderThoughtSignature = "skip_thought_signature_validator"

// thoughtSignatureCacheMaxEntries bounds memory use; this proxy is
// stateless per-request, so the cache only needs to bridge the gap between
// a tool_use being emitted to Claude Code and it being echoed back on the
// next turn of the same conversation.
const thoughtSignatureCacheMaxEntries = 20000

// thoughtSignatureCache maps a tool_call ID to the Gemini thought_signature
// that accompanied it, so it can be re-attached when Claude Code echoes
// the tool_use block back in conversation history on a later turn.
type thoughtSignatureCache struct {
	mu      sync.Mutex
	entries map[string]string
	order   []string
}

var globalThoughtSignatureCache = &thoughtSignatureCache{
	entries: make(map[string]string),
}

func (c *thoughtSignatureCache) remember(toolCallID, signature string) {
	if toolCallID == "" || signature == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[toolCallID]; !exists {
		if len(c.order) >= thoughtSignatureCacheMaxEntries {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, toolCallID)
	}
	c.entries[toolCallID] = signature
}

func (c *thoughtSignatureCache) lookup(toolCallID string) (string, bool) {
	if toolCallID == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sig, ok := c.entries[toolCallID]
	return sig, ok
}

// rememberThoughtSignature caches a Gemini thought_signature captured from
// an upstream tool_calls response, keyed by the tool call's stable ID.
func rememberThoughtSignature(toolCallID string, extra *models.OpenAIToolCallExtraContent) {
	if extra == nil || extra.Google == nil {
		return
	}
	globalThoughtSignatureCache.remember(toolCallID, extra.Google.ThoughtSignature)
}

// resolveThoughtSignatureExtraContent looks up the real thought_signature
// previously captured for toolCallID. When Gemini is the target provider
// and no real signature is known (e.g. history from a different session,
// or a provider that doesn't echo the value), Google's documented
// placeholder is used instead so Gemini doesn't reject the tool call.
func resolveThoughtSignatureExtraContent(toolCallID string, isGeminiProvider bool) *models.OpenAIToolCallExtraContent {
	if !isGeminiProvider {
		return nil
	}
	sig, ok := globalThoughtSignatureCache.lookup(toolCallID)
	if !ok || sig == "" {
		sig = geminiPlaceholderThoughtSignature
	}
	return &models.OpenAIToolCallExtraContent{
		Google: &models.OpenAIGoogleExtraContent{ThoughtSignature: sig},
	}
}
