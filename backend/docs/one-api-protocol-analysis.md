# one-api Protocol Conversion Analysis

> Analyzing https://github.com/songquanpeng/one-api relay module for protocol conversion patterns to port to our project.

## one-api Architecture

one-api uses an **adapter pattern**: each provider has an `adaptor.go` + `main.go` + `model.go` + `constants.go`.

Key adapters relevant to us:
- `relay/adaptor/anthropic/` — Claude/Anthropic Messages API
- `relay/adaptor/gemini/` — Google Gemini API  
- `relay/adaptor/openai/` — OpenAI Chat Completions API (the "hub" format)

one-api's internal format = **OpenAI Chat Completions**. All providers convert TO/FROM OpenAI format.

Our project's direction is INVERSE: Claude is the API surface, OpenAI/Gemini are upstream providers.

## Key Differences Found

### 1. Claude Tool Choice Mapping (one-api has it, we're missing)

**one-api** (`relay/adaptor/anthropic/main.go` ConvertRequest):
```go
claudeToolChoice := struct {
    Type string `json:"type"`
    Name string `json:"name,omitempty"`
}{Type: "auto"} // default
if choice, ok := textRequest.ToolChoice.(map[string]any); ok {
    if function, ok := choice["function"].(map[string]any); ok {
        claudeToolChoice.Type = "tool"
        claudeToolChoice.Name = function["name"].(string)
    }
} else if toolChoiceType, ok := textRequest.ToolChoice.(string); ok {
    if toolChoiceType == "any" {
        claudeToolChoice.Type = toolChoiceType
    }
}
claudeRequest.ToolChoice = claudeToolChoice
```

**Our gap**: We handle basic tool_choice but miss the `any` type and `tool` with name.

### 2. Claude Streaming: Tool Call Argument Empty Object Fix (one-api has it)

**one-api** (`relay/adaptor/anthropic/main.go` StreamHandler):
```go
if len(lastToolCallChoice.Delta.ToolCalls) > 0 {
    lastArgs := &lastToolCallChoice.Delta.ToolCalls[len(lastToolCallChoice.Delta.ToolCalls)-1].Function
    if len(lastArgs.Arguments.(string)) == 0 {
        lastArgs.Arguments = "{}"  // Send empty object when no arguments
    }
}
```

**Our gap**: We don't emit `{}` for empty tool call arguments in streaming.

### 3. Claude Response: stop_sequence Handling (one-api has it)

**one-api** stopReason mapping:
```go
case "stop_sequence":
    return "stop"
```

**Our gap**: We don't handle `stop_sequence` as a stop reason.

### 4. Claude System Prompt: Separate Field (one-api has it)

**one-api** (`relay/adaptor/anthropic/main.go` ConvertRequest):
```go
if message.Role == "system" && claudeRequest.System == "" {
    claudeRequest.System = message.StringContent()
    continue
}
```

**Our status**: We handle this in our internal model. OK.

### 5. Claude anthropic-version Header (one-api has it)

**one-api** sends `anthropic-version: 2023-06-01` and `anthropic-beta: messages-2023-12-15`.

**Our gap**: We need to check if our handler forwards these properly.

### 6. Gemini Adapter: Safety Settings (one-api has it)

**one-api** converts OpenAI request to Gemini with safety settings.

**Our status**: Our Gemini converter exists but may be missing safety settings config.

### 7. Gemini Adapter: System Instruction Support (one-api has it)

**one-api** checks `IsModelSupportSystemInstruction(textRequest.Model)` and either uses `SystemInstruction` field or converts to user message.

**Our gap**: Need to verify our Gemini converter handles this.

### 8. Gemini Adapter: Dummy Model Message After System (one-api has it)

```go
if shouldAddDummyModelMessage {
    geminiRequest.Contents = append(geminiRequest.Contents, ChatContent{
        Role: "model",
        Parts: []Part{{Text: "Okay"}},
    })
}
```

**Our gap**: We may need this for Gemini compatibility.

### 9. Gemini: Response Format / JSON Schema (one-api has it)

```go
if textRequest.ResponseFormat != nil {
    if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
        geminiRequest.GenerationConfig.ResponseMimeType = mimeType
    }
    if textRequest.ResponseFormat.JsonSchema != nil {
        geminiRequest.GenerationConfig.ResponseSchema = textRequest.ResponseFormat.JsonSchema.Schema
    }
}
```

**Our gap**: Response format conversion for Gemini.

### 10. Reasoning Content (new field)

**one-api** model has:
```go
type Message struct {
    ReasoningContent any `json:"reasoning_content,omitempty"`
}
```

**Our status**: We handle this. OK.

### 11. Tool ID Generation for Gemini (one-api generates UUIDs)

**one-api**: `fmt.Sprintf("call_%s", random.GetUUID())` for Gemini tool calls that don't have IDs.

**Our status**: We have `NormalizeToolCallID` that generates IDs. OK.

### 12. Claude legacy model name mapping (one-api has it)

```go
if claudeRequest.Model == "claude-instant-1" {
    claudeRequest.Model = "claude-instant-1.1"
}
```

**Our gap**: Minor, but we don't do legacy model name mapping.

### 13. MaxTokens default (one-api sets 4096)

```go
if claudeRequest.MaxTokens == 0 {
    claudeRequest.MaxTokens = 4096
}
```

**Our status**: Our handler does this. OK.

### 14. Streaming: content_block_start Tool Use Detection

**one-api** handles tool_use in `content_block_start`:
```go
case "content_block_start":
    if claudeResponse.ContentBlock.Type == "tool_use" {
        tools = append(tools, model.Tool{
            Id:   claudeResponse.ContentBlock.Id,
            Type: "function",
            Function: model.Function{
                Name:      claudeResponse.ContentBlock.Name,
                Arguments: "",
            },
        })
    }
```

**Our status**: We handle this. OK.

### 15. Claude → OpenAI: Multiple Content Blocks Response

**one-api** iterates all content blocks for tool_use:
```go
for _, v := range claudeResponse.Content {
    if v.Type == "tool_use" {
        args, _ := json.Marshal(v.Input)
        tools = append(tools, model.Tool{...})
    }
}
```

But uses only first block for text: `responseText = claudeResponse.Content[0].Text`

**Our gap**: Our legacy converter uses only first text content. The new factory converter handles all blocks.

## Priority Actions

Based on the analysis, here are the gaps to fill (ordered by impact):

1. **P0**: Tool choice mapping (`any`, `tool` with name) — affects Claude Code CLI tool routing
2. **P0**: Empty tool args `{}` in streaming — causes parsing errors in some clients
3. **P1**: `stop_sequence` stop reason handling — completeness
4. **P1**: Gemini safety settings — Gemini API will reject without these
5. **P1**: Gemini system instruction support — required for system prompts
6. **P1**: Gemini dummy model message after system — Gemini API requirement
7. **P1**: Gemini response format / JSON schema — structured output support
8. **P2**: Anthropic-beta header forwarding — prompt caching support
9. **P2**: Legacy model name mapping — backward compatibility

## Implementation Status (2026-05-03)

### Completed
- [x] **P0**: Tool choice mapping — `any` → `required`, `none` → `none`, `tool` with `name` → `function` (fixed: was incorrectly mapping `any` → `auto`)
- [x] **P0**: Empty tool args `{}` in streaming — `emitEmptyToolArgsIfNeeded()` in `response_converter.go`
- [x] **P1**: `stop_sequence` stop reason — added to `translateFinishReason()` and legacy converter, plus `models.StopSequence` constant
- [x] **P1**: Gemini safety settings — `defaultGeminiSafetySettings()` in `gemini.go` (BLOCK_ONLY_HIGH threshold)
- [x] **P1**: Gemini system instruction — already handled via `systemInstruction` field
- [x] **P1**: Gemini dummy model message — skip logic in `ParseRequest`, `addDummyModelMessage()` helper in `gemini.go`
- [x] **P1**: Gemini response format / JSON schema — `ResponseMimeType` + `ResponseSchema` in `GeminiGenerationConfig`, mapped from `InternalRequest.ResponseFormat`
- [x] **P1**: Gemini TopK in generation config — added to `BuildRequest`
- [x] **P2**: Anthropic-beta header forwarding — already handled in `handler.go`
- [x] **P1**: MaxTokens default 4096 — set default before validation in both streaming/non-streaming paths (one-api pattern)
- [x] **P1**: Error response format — already correct Claude format (`type: "error"`, `error.type`, `error.message`)
- [x] **P1**: Tool arg parsing resilience — already handled (empty map on parse failure, raw_arguments in legacy path)
- [x] **P1**: Content alongside ToolCalls — already handled (block type switching prevents mixing)

### Remaining (intentionally skipped)
- Legacy model name mapping (`claude-instant-1` → `claude-instant-1.1`, `claude-2` → `claude-2.1`) — these models are deprecated, minimal impact
- `parallel_tool_calls` passthrough — Claude API doesn't support this field, not needed

## Deep Comparison with one-api Source Code (2026-05-03 Session 2)

Compared our entire `converter/` package line-by-line against one-api's `relay/adaptor/anthropic/main.go` and `relay/model/`.

### one-api patterns we already handle (confirmed ✅):
- Stop reason mapping: `end_turn→stop`, `stop_sequence→stop`, `max_tokens→length`, `tool_use→tool_calls`
- System message extraction to separate field
- Tool message role conversion: `tool→user` with `tool_result` content type
- Image content base64/URL conversion
- Tool call argument parsing with json.Unmarshal error tolerance
- MaxTokens default 4096
- Multiple content block iteration for tool_use extraction
- Streaming tool call assembly from `content_block_start` + `content_block_delta`

### Patterns where we are SUPERIOR to one-api:
- **Reasoning/thinking content**: one-api only has `ReasoningContent` field; we handle thinking blocks, redacted_thinking, reasoning_details
- **Tool call ID normalization**: one-api doesn't normalize; we normalize `call_→toolu_`, `fc_→toolu_`
- **Tool name truncation**: one-api doesn't handle long tool names; we use SHA256 hash suffix
- **Message sequence validation**: one-api doesn't validate; we have `validateAndFixMessageSequence()`
- **Video/audio content**: one-api only handles images; we support video and audio content blocks
- **Provider-specific quirks**: We have provider detection and feature flags
- **Cache token tracking**: Both handle it, but we also track `CacheCreationInputTokens`

### Bug found and fixed this session:
- **tool_choice mapping**: `any` was incorrectly mapped to `auto` instead of `required`. `none` was missing.
  - Claude `any` = "must use a tool" → OpenAI `required`
  - Claude `none` = "must not use tools" → OpenAI `none`

## Files Modified

- `converter/openai.go` — Tool choice fix (`any`→`required`, `none`→`none`), ResponseFormat parsing
- `converter/openai_test.go` — E2E tool_choice mapping tests (4 test cases)
- `converter/response_converter.go` — Empty tool args fix (`emitEmptyToolArgsIfNeeded`)
- `converter/streaming_state.go` — `stop_sequence` in `translateFinishReason`
- `converter/gemini.go` — Safety settings, response format, dummy message, TopK
- `converter/internal.go` — ResponseFormat field in InternalRequest
- `models/constants.go` — `StopSequence` constant
- `models/gemini.go` — `ResponseMimeType`, `ResponseSchema` in `GeminiGenerationConfig`
- `models/openai.go` — `ResponseFormat` in `OpenAIRequest`
- `handler/handler.go` — MaxTokens default 4096 in streaming/non-streaming paths
- `docs/superpowers/plans/2026-05-03-one-api-protocol-compatibility.md` — This plan document
