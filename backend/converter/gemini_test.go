package converter

import (
	"encoding/json"
	"testing"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

func TestNewGeminiConverter(t *testing.T) {
	g := NewGeminiConverter()
	if g == nil {
		t.Fatal("expected non-nil converter")
	}
}

func TestStripUnsupportedSchemaFields(t *testing.T) {
	if got := stripUnsupportedSchemaFields(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}

	schema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false, // unsupported, should be dropped
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"properties": map[string]interface{}{
			// "name" is an arbitrary user-defined parameter name, not a JSON
			// Schema keyword — it must survive even though it's absent from
			// geminiSchemaAllowed. See the regression test below for the bug
			// this used to trigger (every property silently dropped).
			"name": map[string]interface{}{
				"type":     "string",
				"$comment": "unsupported-at-nested-level-too",
			},
		},
		"items": []interface{}{
			map[string]interface{}{
				"type":     "string",
				"$comment": "dropped",
			},
			"not-a-map",
		},
	}

	got := stripUnsupportedSchemaFields(schema)
	if _, ok := got["additionalProperties"]; ok {
		t.Error("additionalProperties should have been stripped")
	}
	if _, ok := got["$schema"]; ok {
		t.Error("$schema should have been stripped")
	}
	props, ok := got["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to survive")
	}
	nameSchema, ok := props["name"].(map[string]interface{})
	if !ok {
		t.Fatal("expected nested name schema to survive")
	}
	if _, ok := nameSchema["$comment"]; ok {
		t.Error("nested $comment field is not in the Gemini allow-list and should be stripped")
	}
	if nameSchema["type"] != "string" {
		t.Errorf("expected nested type to survive, got %v", nameSchema["type"])
	}
	items, ok := got["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("expected items array of len 2, got %v", got["items"])
	}
	itemMap, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first item to be processed as map")
	}
	if _, ok := itemMap["$comment"]; ok {
		t.Error("$comment field inside items array element should be stripped")
	}
	if items[1] != "not-a-map" {
		t.Errorf("non-map array item should pass through unchanged, got %v", items[1])
	}
}

func TestGeminiParseRequest_InvalidJSON(t *testing.T) {
	g := NewGeminiConverter()
	_, err := g.ParseRequest([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGeminiParseRequest_Full(t *testing.T) {
	g := NewGeminiConverter()
	temp := 0.7
	topP := 0.9
	topK := 40
	reqJSON := `{
		"systemInstruction": {"parts": [{"text": "You are helpful."}, {"text": "Be concise."}]},
		"generationConfig": {
			"temperature": 0.7,
			"topP": 0.9,
			"topK": 40,
			"maxOutputTokens": 1024,
			"stopSequences": ["STOP"],
			"responseMimeType": "application/json"
		},
		"contents": [
			{"role": "user", "parts": [{"text": "hello"}]},
			{"role": "model", "parts": [{"text": "Okay"}]},
			{"role": "model", "parts": [{"text": "hi there"}]},
			{"role": "user", "parts": [{"inlineData": {"mimeType": "image/png", "data": "aW1nZGF0YQ=="}}]},
			{"role": "user", "parts": [{"inlineData": {"mimeType": "video/mp4", "data": "dmlkZGF0YQ=="}}]},
			{"role": "user", "parts": [{"inlineData": {"mimeType": "audio/mpeg", "data": "YXVkZGF0YQ=="}}]},
			{"role": "user", "parts": [{"inlineData": {"mimeType": "application/octet-stream", "data": "eHh4"}}]},
			{"role": "model", "parts": [{"functionCall": {"name": "get_weather", "args": {"city": "NYC"}}}]},
			{"role": "user", "parts": [{"functionResponse": {"name": "get_weather", "response": "sunny"}}]},
			{"role": "user", "parts": [{"functionResponse": {"name": "get_weather", "response": {"temp": 70}}}]}
		],
		"tools": [{"functionDeclarations": [{"name": "get_weather", "description": "gets weather", "parameters": {"type": "object", "additionalProperties": false, "properties": {"city": {"type": "string"}}}}]}],
		"toolConfig": {"functionCallingConfig": {"mode": "ANY"}}
	}`

	req, err := g.ParseRequest([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.System != "You are helpful.\nBe concise." {
		t.Errorf("unexpected system: %q", req.System)
	}
	if req.Temperature == nil || *req.Temperature != temp {
		t.Errorf("unexpected temperature: %v", req.Temperature)
	}
	if req.TopP == nil || *req.TopP != topP {
		t.Errorf("unexpected topP: %v", req.TopP)
	}
	if req.TopK == nil || *req.TopK != topK {
		t.Errorf("unexpected topK: %v", req.TopK)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("unexpected maxTokens: %d", req.MaxTokens)
	}
	if len(req.StopSeqs) != 1 || req.StopSeqs[0] != "STOP" {
		t.Errorf("unexpected stop seqs: %v", req.StopSeqs)
	}
	rf, ok := req.ResponseFormat.(map[string]interface{})
	if !ok || rf["type"] != "application/json" {
		t.Errorf("unexpected response format: %v", req.ResponseFormat)
	}

	// The dummy "Okay" model message must be skipped entirely.
	for _, m := range req.Messages {
		if m.Role == "assistant" && len(m.Content) == 1 && m.Content[0].Text == "Okay" {
			t.Error("dummy Okay message should have been skipped")
		}
	}

	// Count message types
	var gotImage, gotVideo, gotAudio, gotToolUse, gotToolResult int
	for _, m := range req.Messages {
		for _, cb := range m.Content {
			switch cb.Type {
			case "image":
				gotImage++
				if cb.Source == nil || cb.Source.MediaType != "image/png" {
					t.Errorf("bad image block: %+v", cb)
				}
			case "video":
				gotVideo++
				if cb.VideoSource == nil || cb.VideoSource.MediaType != "video/mp4" {
					t.Errorf("bad video block: %+v", cb)
				}
			case "audio":
				gotAudio++
				if cb.AudioSource == nil || cb.AudioSource.MediaType != "audio/mpeg" {
					t.Errorf("bad audio block: %+v", cb)
				}
			case "tool_use":
				gotToolUse++
				if cb.Name != "get_weather" || cb.ID != "call_get_weather" {
					t.Errorf("bad tool_use block: %+v", cb)
				}
			case "tool_result":
				gotToolResult++
				if cb.ToolUseID != "get_weather" {
					t.Errorf("bad tool_result block: %+v", cb)
				}
			}
		}
	}
	if gotImage != 1 {
		t.Errorf("expected 1 image block, got %d", gotImage)
	}
	if gotVideo != 1 {
		t.Errorf("expected 1 video block, got %d", gotVideo)
	}
	if gotAudio != 1 {
		t.Errorf("expected 1 audio block, got %d", gotAudio)
	}
	if gotToolUse != 1 {
		t.Errorf("expected 1 tool_use block, got %d", gotToolUse)
	}
	if gotToolResult != 2 {
		t.Errorf("expected 2 tool_result blocks, got %d", gotToolResult)
	}

	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Fatalf("unexpected tools: %+v", req.Tools)
	}
	if _, ok := req.Tools[0].Parameters["additionalProperties"]; ok {
		t.Error("tool parameters should have been stripped of unsupported fields")
	}

	if req.ToolChoice != "any" {
		t.Errorf("expected tool choice 'any', got %v", req.ToolChoice)
	}
}

func TestGeminiParseRequest_ToolConfigModes(t *testing.T) {
	g := NewGeminiConverter()
	cases := []struct {
		mode     string
		expected string
	}{
		{"NONE", "none"},
		{"AUTO", "auto"},
		{"SOMETHING_ELSE", "auto"},
	}
	for _, tc := range cases {
		reqJSON := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"toolConfig":{"functionCallingConfig":{"mode":"` + tc.mode + `"}}}`
		req, err := g.ParseRequest([]byte(reqJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.ToolChoice != tc.expected {
			t.Errorf("mode %s: expected tool choice %q, got %v", tc.mode, tc.expected, req.ToolChoice)
		}
	}
}

func TestGeminiParseRequest_NoSystemNoGenerationConfig(t *testing.T) {
	g := NewGeminiConverter()
	req, err := g.ParseRequest([]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.System != "" {
		t.Errorf("expected empty system, got %q", req.System)
	}
	if req.MaxTokens != 0 {
		t.Errorf("expected zero maxTokens, got %d", req.MaxTokens)
	}
}

func TestGeminiBuildRequest_Full(t *testing.T) {
	g := NewGeminiConverter()
	temp := 0.5
	topP := 0.8
	topK := 10
	req := &InternalRequest{
		System: "system prompt",
		Messages: []InternalMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}},
			{Role: "assistant", Content: []ContentBlock{
				{Type: "image", Source: &ImageSource{MediaType: "image/png", Data: "abc"}},
				{Type: "video", VideoSource: &VideoSource{MediaType: "video/mp4", Data: "def"}},
				{Type: "audio", AudioSource: &AudioSource{MediaType: "audio/mpeg", Data: "ghi"}},
				{Type: "tool_use", Name: "get_weather", Input: map[string]interface{}{"city": "NYC"}},
			}},
			{Role: "tool", Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "get_weather", Content: "sunny"},
			}},
			// Content blocks with nil sources must be skipped without panicking.
			{Role: "user", Content: []ContentBlock{
				{Type: "image"},
				{Type: "video"},
				{Type: "audio"},
				{Type: "unknown_type"},
			}},
		},
		Tools: []ToolDefinition{
			{Name: "get_weather", Description: "gets weather", Parameters: map[string]interface{}{"type": "object"}},
		},
		Temperature: &temp,
		TopP:        &topP,
		TopK:        &topK,
		MaxTokens:   512,
		StopSeqs:    []string{"END"},
		ResponseFormat: map[string]interface{}{
			"type": "json_object",
			"json_schema": map[string]interface{}{
				"schema": map[string]interface{}{"type": "object", "additionalProperties": false},
			},
		},
	}

	body, err := g.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var geminiReq models.GeminiRequest
	if err := json.Unmarshal(body, &geminiReq); err != nil {
		t.Fatalf("failed to unmarshal built request: %v", err)
	}

	if geminiReq.SystemInstruction == nil || geminiReq.SystemInstruction.Parts[0].Text != "system prompt" {
		t.Errorf("unexpected system instruction: %+v", geminiReq.SystemInstruction)
	}
	if len(geminiReq.SafetySettings) != 5 {
		t.Errorf("expected 5 default safety settings, got %d", len(geminiReq.SafetySettings))
	}
	if len(geminiReq.Contents) != 4 {
		t.Fatalf("expected 4 contents, got %d", len(geminiReq.Contents))
	}
	if geminiReq.Contents[0].Role != "user" {
		t.Errorf("unexpected role: %s", geminiReq.Contents[0].Role)
	}
	if geminiReq.Contents[1].Role != "model" {
		t.Errorf("unexpected role: %s", geminiReq.Contents[1].Role)
	}
	assistantParts := geminiReq.Contents[1].Parts
	if len(assistantParts) != 4 {
		t.Fatalf("expected 4 parts in assistant message, got %d", len(assistantParts))
	}
	if assistantParts[0].InlineData == nil || assistantParts[0].InlineData.MimeType != "image/png" {
		t.Errorf("bad image part: %+v", assistantParts[0])
	}
	if assistantParts[3].FunctionCall == nil || assistantParts[3].FunctionCall.Name != "get_weather" {
		t.Errorf("bad function call part: %+v", assistantParts[3])
	}
	toolContent := geminiReq.Contents[2]
	if toolContent.Role != "user" { // mapInternalRoleToGemini default for "tool"
		t.Errorf("unexpected tool role mapping: %s", toolContent.Role)
	}
	if len(toolContent.Parts) != 1 || toolContent.Parts[0].FunctionResponse == nil {
		t.Fatalf("expected function response part: %+v", toolContent.Parts)
	}

	// The content blocks with nil sources should produce zero parts (skipped).
	lastContent := geminiReq.Contents[3]
	if len(lastContent.Parts) != 0 {
		t.Errorf("expected nil-source blocks to be skipped, got %d parts", len(lastContent.Parts))
	}

	if len(geminiReq.Tools) != 1 || len(geminiReq.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("unexpected tools: %+v", geminiReq.Tools)
	}

	if geminiReq.GenerationConfig == nil {
		t.Fatal("expected generation config to be set")
	}
	if geminiReq.GenerationConfig.MaxOutputTokens != 512 {
		t.Errorf("unexpected maxOutputTokens: %d", geminiReq.GenerationConfig.MaxOutputTokens)
	}
	if geminiReq.GenerationConfig.ResponseMimeType != "application/json" {
		t.Errorf("unexpected mime type: %s", geminiReq.GenerationConfig.ResponseMimeType)
	}
	if _, ok := geminiReq.GenerationConfig.ResponseSchema["additionalProperties"]; ok {
		t.Error("response schema should have unsupported fields stripped")
	}
}

func TestGeminiBuildRequest_NoGenerationConfigWhenEmpty(t *testing.T) {
	g := NewGeminiConverter()
	req := &InternalRequest{
		Messages: []InternalMessage{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}},
	}
	body, err := g.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiReq models.GeminiRequest
	if err := json.Unmarshal(body, &geminiReq); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if geminiReq.GenerationConfig != nil {
		t.Errorf("expected nil generation config, got %+v", geminiReq.GenerationConfig)
	}
	if geminiReq.SystemInstruction != nil {
		t.Errorf("expected nil system instruction, got %+v", geminiReq.SystemInstruction)
	}
	if len(geminiReq.Tools) != 0 {
		t.Errorf("expected no tools, got %+v", geminiReq.Tools)
	}
}

func TestGeminiBuildRequest_ResponseFormatTextType(t *testing.T) {
	g := NewGeminiConverter()
	req := &InternalRequest{
		Messages:       []InternalMessage{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}},
		ResponseFormat: map[string]interface{}{"type": "text"},
	}
	body, err := g.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiReq models.GeminiRequest
	json.Unmarshal(body, &geminiReq)
	if geminiReq.GenerationConfig == nil || geminiReq.GenerationConfig.ResponseMimeType != "text/plain" {
		t.Errorf("unexpected generation config: %+v", geminiReq.GenerationConfig)
	}
}

func TestGeminiBuildRequest_ResponseFormatUnknownTypeNoOp(t *testing.T) {
	g := NewGeminiConverter()
	req := &InternalRequest{
		Messages:       []InternalMessage{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}},
		ResponseFormat: map[string]interface{}{"type": "unknown_format"},
	}
	body, err := g.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiReq models.GeminiRequest
	json.Unmarshal(body, &geminiReq)
	if geminiReq.GenerationConfig == nil {
		t.Fatal("generation config should still be allocated once response_format is processed")
	}
	if geminiReq.GenerationConfig.ResponseMimeType != "" {
		t.Errorf("expected no mime type mapping for unknown format, got %q", geminiReq.GenerationConfig.ResponseMimeType)
	}
}

func TestGeminiBuildRequest_ResponseFormatNotAMap(t *testing.T) {
	g := NewGeminiConverter()
	req := &InternalRequest{
		Messages:       []InternalMessage{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}},
		ResponseFormat: "not-a-map",
	}
	body, err := g.BuildRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiReq models.GeminiRequest
	json.Unmarshal(body, &geminiReq)
	if geminiReq.GenerationConfig != nil {
		t.Errorf("expected no generation config when response_format isn't a map, got %+v", geminiReq.GenerationConfig)
	}
}

func TestGeminiParseResponse_InvalidJSON(t *testing.T) {
	g := NewGeminiConverter()
	_, err := g.ParseResponse([]byte("not json"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGeminiParseResponse_NoCandidates(t *testing.T) {
	g := NewGeminiConverter()
	_, err := g.ParseResponse([]byte(`{"candidates":[]}`))
	if err == nil {
		t.Fatal("expected error for empty candidates")
	}
}

func TestGeminiParseResponse_FinishReasons(t *testing.T) {
	g := NewGeminiConverter()
	cases := []struct {
		finish   string
		expected string
	}{
		{"STOP", "end_turn"},
		{"MAX_TOKENS", "max_tokens"},
		{"SAFETY", "content_filter"},
		{"RECITATION", "content_filter"},
		{"OTHER", "end_turn"},
		{"", "end_turn"},
	}
	for _, tc := range cases {
		body := `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"` + tc.finish + `","index":0}]}`
		resp, err := g.ParseResponse([]byte(body))
		if err != nil {
			t.Fatalf("unexpected error for finish=%s: %v", tc.finish, err)
		}
		if resp.StopReason != tc.expected {
			t.Errorf("finish=%s: expected stop reason %s, got %s", tc.finish, tc.expected, resp.StopReason)
		}
	}
}

func TestGeminiParseResponse_ContentAndUsage(t *testing.T) {
	g := NewGeminiConverter()
	body := `{
		"candidates": [{
			"content": {"parts": [{"text": "hello"}, {"functionCall": {"name": "get_weather", "args": {"city": "NYC"}}}]},
			"finishReason": "STOP",
			"index": 2
		}],
		"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
	}`
	resp, err := g.ParseResponse([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "gemini-2" {
		t.Errorf("unexpected id: %s", resp.ID)
	}
	if resp.Role != "assistant" {
		t.Errorf("unexpected role: %s", resp.Role)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "hello" {
		t.Errorf("bad text block: %+v", resp.Content[0])
	}
	if resp.Content[1].Type != "tool_use" || resp.Content[1].Name != "get_weather" || resp.Content[1].ID != "call_get_weather" {
		t.Errorf("bad tool_use block: %+v", resp.Content[1])
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("bad usage: %+v", resp.Usage)
	}
}

func TestGeminiParseResponse_NoUsage(t *testing.T) {
	g := NewGeminiConverter()
	body := `{"candidates": [{"content": {"parts": [{"text": "hi"}]}, "index": 0}]}`
	resp, err := g.ParseResponse([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage != nil {
		t.Errorf("expected nil usage, got %+v", resp.Usage)
	}
}

func TestGeminiBuildResponse_StopReasons(t *testing.T) {
	g := NewGeminiConverter()
	cases := []struct {
		stop     string
		expected string
	}{
		{"max_tokens", "MAX_TOKENS"},
		{"tool_use", "STOP"},
		{"end_turn", "STOP"},
		{"", "STOP"},
	}
	for _, tc := range cases {
		resp := &InternalResponse{StopReason: tc.stop, Content: []ContentBlock{{Type: "text", Text: "hi"}}}
		body, err := g.BuildResponse(resp)
		if err != nil {
			t.Fatalf("unexpected error for stop=%s: %v", tc.stop, err)
		}
		var geminiResp models.GeminiResponse
		json.Unmarshal(body, &geminiResp)
		if geminiResp.Candidates[0].FinishReason != tc.expected {
			t.Errorf("stop=%s: expected finish reason %s, got %s", tc.stop, tc.expected, geminiResp.Candidates[0].FinishReason)
		}
	}
}

func TestGeminiBuildResponse_ContentAndUsage(t *testing.T) {
	g := NewGeminiConverter()
	resp := &InternalResponse{
		StopReason: "end_turn",
		Content: []ContentBlock{
			{Type: "text", Text: "hello"},
			{Type: "tool_use", Name: "get_weather", Input: map[string]interface{}{"city": "NYC"}},
			{Type: "unknown_type"}, // should be silently ignored
		},
		Usage: &UsageInfo{InputTokens: 10, OutputTokens: 5},
	}
	body, err := g.BuildResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	parts := geminiResp.Candidates[0].Content.Parts
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (unknown type skipped), got %d", len(parts))
	}
	if geminiResp.UsageMetadata == nil || geminiResp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("bad usage metadata: %+v", geminiResp.UsageMetadata)
	}
}

func TestGeminiBuildResponse_NoUsage(t *testing.T) {
	g := NewGeminiConverter()
	resp := &InternalResponse{StopReason: "end_turn", Content: []ContentBlock{{Type: "text", Text: "hi"}}}
	body, err := g.BuildResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiResp models.GeminiResponse
	json.Unmarshal(body, &geminiResp)
	if geminiResp.UsageMetadata != nil {
		t.Errorf("expected nil usage metadata, got %+v", geminiResp.UsageMetadata)
	}
}

func TestGeminiParseStreamEvent_InvalidJSON(t *testing.T) {
	g := NewGeminiConverter()
	_, err := g.ParseStreamEvent([]byte("not json"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGeminiParseStreamEvent_TextDelta(t *testing.T) {
	g := NewGeminiConverter()
	event, err := g.ParseStreamEvent([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Delta == nil || event.Delta.Type != "text_delta" || event.Delta.Text != "hi" {
		t.Errorf("unexpected delta: %+v", event.Delta)
	}
}

func TestGeminiParseStreamEvent_FunctionCallDelta(t *testing.T) {
	g := NewGeminiConverter()
	event, err := g.ParseStreamEvent([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"NYC"}}}]}}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Delta == nil || event.Delta.Type != "tool_call_delta" {
		t.Errorf("unexpected delta: %+v", event.Delta)
	}
}

// TestGeminiParseStreamEvent_FinishReasonWithoutParts is a regression test for
// a nil-pointer panic: Gemini's terminal stream chunk commonly carries only
// finishReason with an empty (or absent) parts array. Before the fix,
// event.Delta stayed nil and the code unconditionally wrote
// event.Delta.StopReason, panicking mid-stream.
func TestGeminiParseStreamEvent_FinishReasonWithoutParts(t *testing.T) {
	g := NewGeminiConverter()

	cases := []struct {
		name string
		body string
	}{
		{"empty parts array", `{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}`},
		{"no parts field", `{"candidates":[{"content":{},"finishReason":"MAX_TOKENS"}]}`},
		{"part with neither text nor functionCall", `{"candidates":[{"content":{"parts":[{}]},"finishReason":"SAFETY"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseStreamEvent panicked: %v", r)
				}
			}()
			event, err := g.ParseStreamEvent([]byte(tc.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if event.Delta == nil {
				t.Fatal("expected Delta to be allocated so StopReason could be recorded")
			}
		})
	}
}

func TestGeminiParseStreamEvent_FinishReasonMapping(t *testing.T) {
	g := NewGeminiConverter()
	cases := []struct {
		finish   string
		expected string
	}{
		{"MAX_TOKENS", "max_tokens"},
		{"STOP", "end_turn"},
		{"SAFETY", "content_filter"},
	}
	for _, tc := range cases {
		body := `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"` + tc.finish + `"}]}`
		event, err := g.ParseStreamEvent([]byte(body))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.Delta.StopReason != tc.expected {
			t.Errorf("finish=%s: expected stop reason %s, got %s", tc.finish, tc.expected, event.Delta.StopReason)
		}
	}
}

func TestGeminiParseStreamEvent_UnrecognizedFinishReasonLeavesStopReasonEmpty(t *testing.T) {
	g := NewGeminiConverter()
	event, err := g.ParseStreamEvent([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"OTHER"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Delta.StopReason != "" {
		t.Errorf("expected empty stop reason for unrecognized finish reason, got %q", event.Delta.StopReason)
	}
}

func TestGeminiParseStreamEvent_NoCandidates(t *testing.T) {
	g := NewGeminiConverter()
	event, err := g.ParseStreamEvent([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Delta != nil {
		t.Errorf("expected nil delta when no candidates present, got %+v", event.Delta)
	}
}

func TestGeminiParseStreamEvent_Usage(t *testing.T) {
	g := NewGeminiConverter()
	event, err := g.ParseStreamEvent([]byte(`{"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":7}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Usage == nil || event.Usage.InputTokens != 3 || event.Usage.OutputTokens != 7 {
		t.Errorf("unexpected usage: %+v", event.Usage)
	}
}

func TestGeminiBuildStreamEvent_TextAndStopReason(t *testing.T) {
	g := NewGeminiConverter()
	cases := []struct {
		stop     string
		expected string
	}{
		{"max_tokens", "MAX_TOKENS"},
		{"end_turn", "STOP"},
		{"content_filter", "SAFETY"},
		{"", ""},
	}
	for _, tc := range cases {
		event := &StreamEvent{Delta: &StreamDelta{Text: "hi", StopReason: tc.stop}}
		body, err := g.BuildStreamEvent(event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var geminiEvent models.GeminiStreamEvent
		json.Unmarshal(body, &geminiEvent)
		if geminiEvent.Candidates[0].FinishReason != tc.expected {
			t.Errorf("stop=%s: expected finish reason %s, got %s", tc.stop, tc.expected, geminiEvent.Candidates[0].FinishReason)
		}
		if len(geminiEvent.Candidates[0].Content.Parts) != 1 || geminiEvent.Candidates[0].Content.Parts[0].Text != "hi" {
			t.Errorf("unexpected parts: %+v", geminiEvent.Candidates[0].Content.Parts)
		}
	}
}

func TestGeminiBuildStreamEvent_NilDelta(t *testing.T) {
	g := NewGeminiConverter()
	event := &StreamEvent{}
	body, err := g.BuildStreamEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiEvent models.GeminiStreamEvent
	json.Unmarshal(body, &geminiEvent)
	if len(geminiEvent.Candidates[0].Content.Parts) != 0 {
		t.Errorf("expected no parts for nil delta, got %+v", geminiEvent.Candidates[0].Content.Parts)
	}
}

func TestGeminiBuildStreamEvent_EmptyTextNotEmitted(t *testing.T) {
	g := NewGeminiConverter()
	event := &StreamEvent{Delta: &StreamDelta{Text: "", StopReason: "end_turn"}}
	body, err := g.BuildStreamEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiEvent models.GeminiStreamEvent
	json.Unmarshal(body, &geminiEvent)
	if len(geminiEvent.Candidates[0].Content.Parts) != 0 {
		t.Errorf("expected no parts for empty text, got %+v", geminiEvent.Candidates[0].Content.Parts)
	}
	if geminiEvent.Candidates[0].FinishReason != "STOP" {
		t.Errorf("unexpected finish reason: %s", geminiEvent.Candidates[0].FinishReason)
	}
}

func TestGeminiBuildStreamEvent_Usage(t *testing.T) {
	g := NewGeminiConverter()
	event := &StreamEvent{Usage: &UsageInfo{InputTokens: 4, OutputTokens: 6}}
	body, err := g.BuildStreamEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var geminiEvent models.GeminiStreamEvent
	json.Unmarshal(body, &geminiEvent)
	if geminiEvent.UsageMetadata == nil || geminiEvent.UsageMetadata.PromptTokenCount != 4 || geminiEvent.UsageMetadata.CandidatesTokenCount != 6 {
		t.Errorf("unexpected usage metadata: %+v", geminiEvent.UsageMetadata)
	}
}

func TestMapGeminiRoleToInternal(t *testing.T) {
	cases := map[string]string{"user": "user", "model": "assistant", "weird": "user", "": "user"}
	for in, want := range cases {
		if got := mapGeminiRoleToInternal(in); got != want {
			t.Errorf("mapGeminiRoleToInternal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapInternalRoleToGemini(t *testing.T) {
	cases := map[string]string{"user": "user", "assistant": "model", "tool": "user", "": "user"}
	for in, want := range cases {
		if got := mapInternalRoleToGemini(in); got != want {
			t.Errorf("mapInternalRoleToGemini(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultGeminiSafetySettings(t *testing.T) {
	settings := defaultGeminiSafetySettings()
	if len(settings) != 5 {
		t.Fatalf("expected 5 settings, got %d", len(settings))
	}
	for _, s := range settings {
		if s.Threshold != "BLOCK_ONLY_HIGH" {
			t.Errorf("unexpected threshold: %s", s.Threshold)
		}
	}
}

func TestAddDummyModelMessage(t *testing.T) {
	contents := []models.GeminiContent{{Role: "user", Parts: []models.GeminiPart{{Text: "hi"}}}}
	addDummyModelMessage(&contents)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents after append, got %d", len(contents))
	}
	last := contents[1]
	if last.Role != "model" || len(last.Parts) != 1 || last.Parts[0].Text != "Okay" {
		t.Errorf("unexpected dummy message: %+v", last)
	}
}
