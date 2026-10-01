package converter

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// TestGeminiThoughtSignature_CaptureAndRestore reproduces the reported bug:
// Gemini rejects a tool_use echoed back in history without its original
// thought_signature. This verifies the signature captured from an upstream
// non-streaming response is re-attached when that tool_use is later sent
// back as conversation history.
func TestGeminiThoughtSignature_CaptureAndRestore(t *testing.T) {
	cfg := &config.Config{OpenAIBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/"}
	c := NewOpenAIConverter(cfg)

	body := []byte(`{
		"id": "chatcmpl-1",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"tool_calls": [{
					"id": "call_sig_test_1",
					"type": "function",
					"function": {"name": "Bash", "arguments": "{\"command\":\"ls\"}"},
					"extra_content": {"google": {"thought_signature": "real-signature-xyz"}}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 1, "completion_tokens": 1}
	}`)

	resp, err := c.ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].ID != "call_sig_test_1" {
		t.Fatalf("unexpected parsed content: %+v", resp.Content)
	}

	// Simulate Claude Code echoing this tool_use back in the next turn.
	req := &InternalRequest{
		Model: "gemini-3-pro",
		Messages: []InternalMessage{
			{Role: "assistant", Content: []ContentBlock{{
				Type: "tool_use", ID: "call_sig_test_1", Name: "Bash",
				Input: map[string]interface{}{"command": "ls"},
			}}},
		},
	}
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out.Messages) == 0 || len(out.Messages[0].ToolCalls) != 1 {
		t.Fatalf("expected first message with 1 tool call, got %+v", out.Messages)
	}
	tc := out.Messages[0].ToolCalls[0]
	if tc.ExtraContent == nil || tc.ExtraContent.Google == nil {
		t.Fatalf("expected extra_content.google to be set")
	}
	if got := tc.ExtraContent.Google.ThoughtSignature; got != "real-signature-xyz" {
		t.Errorf("expected the real captured signature, got %q", got)
	}
}

// TestGeminiThoughtSignature_PlaceholderFallback verifies that when no
// signature was ever captured for a tool_call_id (e.g. cache miss across
// restarts, or a synthetic ID), the outgoing request to Gemini still gets
// Google's documented placeholder rather than nothing at all.
func TestGeminiThoughtSignature_PlaceholderFallback(t *testing.T) {
	cfg := &config.Config{OpenAIBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/"}
	c := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "gemini-3-pro",
		Messages: []InternalMessage{
			{Role: "assistant", Content: []ContentBlock{{
				Type: "tool_use", ID: "call_never_seen_before", Name: "Bash",
				Input: map[string]interface{}{"command": "ls"},
			}}},
		},
	}
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	tc := out.Messages[0].ToolCalls[0]
	if tc.ExtraContent == nil || tc.ExtraContent.Google == nil {
		t.Fatalf("expected extra_content.google to be set even without a cached signature")
	}
	if got := tc.ExtraContent.Google.ThoughtSignature; got != geminiPlaceholderThoughtSignature {
		t.Errorf("expected placeholder signature %q, got %q", geminiPlaceholderThoughtSignature, got)
	}
}

// TestGeminiThoughtSignature_NotAttachedForNonGeminiProviders verifies the
// extra_content extension isn't injected when the upstream isn't Gemini,
// since it's a Gemini-specific, non-standard OpenAI field.
func TestGeminiThoughtSignature_NotAttachedForNonGeminiProviders(t *testing.T) {
	cfg := &config.Config{OpenAIBaseURL: "https://api.openai.com/v1"}
	c := NewOpenAIConverter(cfg)

	req := &InternalRequest{
		Model: "gpt-4o",
		Messages: []InternalMessage{
			{Role: "assistant", Content: []ContentBlock{{
				Type: "tool_use", ID: "call_openai_1", Name: "Bash",
				Input: map[string]interface{}{"command": "ls"},
			}}},
		},
	}
	data, err := c.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var out models.OpenAIRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	tc := out.Messages[0].ToolCalls[0]
	if tc.ExtraContent != nil {
		t.Errorf("expected no extra_content for a non-Gemini provider, got %+v", tc.ExtraContent)
	}
}

// TestGeminiThoughtSignature_CapturedDuringStreaming verifies the streaming
// SSE path (ConvertOpenAIStreamingToClaude) also captures extra_content's
// thought_signature, keyed by the normalized tool_use ID Claude Code will
// see and later echo back (NormalizeToolCallID turns "call_abc123" into
// "toolu_abc123").
func TestGeminiThoughtSignature_CapturedDuringStreaming(t *testing.T) {
	id := "chatcmpl-sig-stream"
	model := "gemini-3-pro"

	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_stream_sig_1",
							"type":  "function",
							"function": map[string]interface{}{
								"name":      "Bash",
								"arguments": `{"command":"ls"}`,
							},
							"extra_content": map[string]interface{}{
								"google": map[string]interface{}{
									"thought_signature": "stream-signature-abc",
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(chunk)
	sse := "data: " + string(data) + "\n\n" +
		openAIChunk(id, model, map[string]interface{}{}, "tool_calls")

	_, result := runStreamingTestWithDone(t, sse, "claude-sonnet-4-6")
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool_calls count = %d, want 1", len(result.ToolCalls))
	}
	toolID, _ := result.ToolCalls[0]["id"].(string)
	if toolID != "toolu_stream_sig_1" {
		t.Fatalf("expected normalized tool id toolu_stream_sig_1, got %q", toolID)
	}

	sig, ok := globalThoughtSignatureCache.lookup(toolID)
	if !ok || sig != "stream-signature-abc" {
		t.Errorf("expected cached signature stream-signature-abc for %q, got %q (ok=%v)", toolID, sig, ok)
	}
}

// TestThoughtSignatureCache_RememberIgnoresEmptyKeyOrValue covers remember's
// early-return guard: neither an empty tool call ID nor an empty signature
// should create an entry.
func TestThoughtSignatureCache_RememberIgnoresEmptyKeyOrValue(t *testing.T) {
	c := &thoughtSignatureCache{entries: make(map[string]string)}
	c.remember("", "some-signature")
	c.remember("some-id", "")
	if len(c.entries) != 0 {
		t.Errorf("expected no entries to be recorded, got %d", len(c.entries))
	}
}

// TestThoughtSignatureCache_LookupEmptyID covers lookup's early-return guard
// for an empty tool call ID.
func TestThoughtSignatureCache_LookupEmptyID(t *testing.T) {
	c := &thoughtSignatureCache{entries: make(map[string]string)}
	if sig, ok := c.lookup(""); ok || sig != "" {
		t.Errorf("expected (\"\", false) for empty ID, got (%q, %v)", sig, ok)
	}
}

// TestThoughtSignatureCache_UpdateExistingEntryDoesNotEvict proves that
// re-remembering an already-cached ID with a new signature just updates the
// value in place — it must not consume an eviction slot or touch c.order,
// since the ID is already tracked there.
func TestThoughtSignatureCache_UpdateExistingEntryDoesNotEvict(t *testing.T) {
	c := &thoughtSignatureCache{entries: make(map[string]string)}
	c.remember("toolu_1", "sig-v1")
	c.remember("toolu_1", "sig-v2")

	if len(c.order) != 1 {
		t.Fatalf("expected order to have exactly 1 entry after updating an existing key, got %d", len(c.order))
	}
	sig, ok := c.lookup("toolu_1")
	if !ok || sig != "sig-v2" {
		t.Errorf("expected updated signature sig-v2, got %q (ok=%v)", sig, ok)
	}
}

// TestThoughtSignatureCache_EvictsOldestBeyondMaxEntries proves the cache is
// bounded: once thoughtSignatureCacheMaxEntries distinct IDs are recorded,
// inserting one more evicts the single oldest entry (FIFO), preventing
// unbounded memory growth across a long-running proxy process.
func TestThoughtSignatureCache_EvictsOldestBeyondMaxEntries(t *testing.T) {
	c := &thoughtSignatureCache{entries: make(map[string]string)}

	for i := 0; i < thoughtSignatureCacheMaxEntries; i++ {
		c.remember(fmt.Sprintf("toolu_%d", i), fmt.Sprintf("sig-%d", i))
	}
	if len(c.entries) != thoughtSignatureCacheMaxEntries {
		t.Fatalf("expected %d entries after filling to capacity, got %d", thoughtSignatureCacheMaxEntries, len(c.entries))
	}
	if _, ok := c.lookup("toolu_0"); !ok {
		t.Fatal("expected the oldest entry to still be present right at capacity")
	}

	// One more insert should evict exactly the oldest entry (toolu_0).
	c.remember("toolu_overflow", "sig-overflow")

	if len(c.entries) != thoughtSignatureCacheMaxEntries {
		t.Fatalf("expected cache to stay bounded at %d entries, got %d", thoughtSignatureCacheMaxEntries, len(c.entries))
	}
	if _, ok := c.lookup("toolu_0"); ok {
		t.Error("expected the oldest entry (toolu_0) to have been evicted")
	}
	if _, ok := c.lookup("toolu_1"); !ok {
		t.Error("expected the second-oldest entry (toolu_1) to still be present")
	}
	if sig, ok := c.lookup("toolu_overflow"); !ok || sig != "sig-overflow" {
		t.Errorf("expected the newly inserted entry to be present, got %q (ok=%v)", sig, ok)
	}
}
