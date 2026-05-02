# litellm Protocol Conversion Analysis

> Analyzing https://github.com/BerriAI/litellm for protocol conversion patterns to port to our project.

## litellm Architecture

litellm uses a **config/transformer pattern**: each provider has a `BaseConfig` subclass with `transform_request()` and `transform_response()` methods.

Key components relevant to us:
- `litellm/llms/anthropic/chat/transformation.py` — AnthropicConfig: OpenAI→Claude request/response conversion
- `litellm/llms/anthropic/chat/handler.py` — ModelResponseIterator: Anthropic streaming→OpenAI chunk parsing
- `litellm/litellm_core_utils/prompt_templates/factory.py` — anthropic_messages_pt: OpenAI messages→Claude messages
- `litellm/litellm_core_utils/core_helpers.py` — map_finish_reason: stop reason mapping
- `litellm/llms/anthropic/common_utils.py` — Headers, OAuth, beta header management

Our project's direction is INVERSE of litellm: we expose Claude API surface, convert to OpenAI upstream. But many response conversion patterns are directly applicable.

## Key Differences Found

### 1. Finish Reason Mapping (litellm has more mappings)

**litellm** (`core_helpers.py:62-106`):
```python
"stop_sequence": "stop",
"end_turn": "stop",
"max_tokens": "length",
"tool_use": "tool_calls",
"refusal": "content_filter",
"compaction": "length",          # ← WE DON'T HAVE
"content_filtered": "content_filter",  # ← WE DON'T HAVE (Anthropic Sonnet 4)
```

**Our gap**: Missing `compaction` → `end_turn` and `content_filtered` → `end_turn` mappings.

### 2. Tool Call ID Sanitization (litellm has it)

**litellm** (`factory.py`): `_sanitize_anthropic_tool_use_id()` ensures IDs match `^[a-zA-Z0-9_-]+$` pattern.

**Our gap**: We normalize `call_` → `toolu_` but don't sanitize arbitrary characters.

### 3. Duplicate Tool Result Detection (litellm has it)

**litellm** (`factory.py:2362-2404`): Deduplicates tool results with same `tool_call_id` within contiguous blocks.

**Our gap**: Our `validateAndFixMessageSequence` handles orphaned and missing tool results but not duplicates.

### 4. Compaction Blocks (litellm has it)

**litellm**: Handles `compaction` content blocks and `compaction` stop reason.

**Our gap**: No handling for `compaction` blocks or stop reason.

### 5. JSON Mode / Response Format → Anthropic Tool (litellm has it)

**litellm** (`transformation.py:872-897`): Converts `response_format: {type: "json_schema"}` to an Anthropic tool definition with `RESPONSE_FORMAT_TOOL_NAME`.

**Our status**: We pass `response_format` through for Gemini but don't convert to Anthropic tool format for Claude output.

### 6. Server Tool Use (litellm has it)

**litellm**: Handles `server_tool_use` blocks (web_search, etc.) with IDs starting with `srvtoolu_`.

**Our gap**: We only handle `tool_use`, not `server_tool_use`.

### 7. Cache Control Passthrough (litellm has it)

**litellm**: Passes through `cache_control` on content blocks and system messages.

**Our gap**: No `cache_control` field in our ContentBlock model.

### 8. Prefix Prompt (litellm has it)

**litellm** (`transformation.py:2006-2027`): Handles assistant messages with `prefix: true`.

**Our gap**: Not handled.

### 9. Anthropic Beta Header Management (litellm has comprehensive)

**litellm**: Dynamic beta header management based on features used:
- `computer-use-2025-01-24` for computer tools
- `files-api-2025-04-14` for file IDs
- `mcp-client-2025-04-04` for MCP servers
- `structured-output-2025-09-25` for output_format
- `fast-mode-2026-02-01` for speed=fast
- etc.

**Our status**: We forward beta headers but don't dynamically add feature-specific ones.

### 10. Output Schema Filtering (litellm has it)

**litellm** (`transformation.py:252-363`): `filter_anthropic_output_schema()` removes unsupported JSON schema fields (maxItems, minItems, minimum, maximum, etc.) for Anthropic's output_format API, and adds constraint info to descriptions.

**Our gap**: Not implemented.

## Patterns where we are SUPERIOR to litellm

- **Tool name truncation**: litellm doesn't handle long tool names; we use SHA256 hash suffix
- **Tool call ID normalization**: litellm doesn't normalize `call_` → `toolu_`, `fc_` → `toolu_`
- **Provider-specific quirks**: We have provider detection and feature flags
- **Video/audio content**: litellm mainly handles images; we support video and audio content blocks

## Priority Actions

### P0 (Critical for Claude Code CLI compatibility):
1. Add `compaction` stop reason mapping
2. Add `content_filtered` stop reason mapping
3. Add tool call ID sanitization to `^[a-zA-Z0-9_-]+$`

### P1 (Important for protocol completeness):
4. Add duplicate tool result detection in message validation
5. Handle `server_tool_use` content blocks
6. Add `cache_control` passthrough on content blocks
7. Add `compaction` content block handling

### P2 (Nice to have):
8. Add prefix prompt handling
9. Add output schema filtering
10. Add dynamic beta header management
11. Add response_format → Anthropic tool conversion

## Implementation Status (2026-05-03)

### Completed (from one-api analysis):
- [x] Tool choice mapping (any→required, none→none)
- [x] Empty tool args {} in streaming
- [x] stop_sequence stop reason
- [x] Gemini safety settings
- [x] MaxTokens default 4096
- [x] Beta header forwarding
- [x] All 22 one-api patterns

### From litellm analysis — To Implement:
- [x] P0: compaction stop reason — added to `translateFinishReason()` and response_converter.go
- [x] P0: content_filtered stop reason — added alongside refusal/content_filter mappings
- [x] P0: Tool call ID sanitization — `sanitizeToolCallID()` in normalizer.go
- [x] P1: Duplicate tool result detection — added to validateAndFixMessageSequence (litellm sanitize_messages pattern)
- [x] P1: server_tool_use content blocks — added to claude.go switch case
- [x] P1: cache_control passthrough — added CacheControl field to ContentBlock and ClaudeContentBlock
- [x] P1: compaction content blocks — added to claude.go switch case

## Files Modified

- `converter/streaming_state.go` — Added `content_filter`, `refusal`, `content_filtered`, `compaction` to translateFinishReason
- `converter/response_converter.go` — Added same mappings to non-streaming finish reason conversion
- `converter/normalizer.go` — Added `sanitizeToolCallID()` for `^[a-zA-Z0-9_-]+$` pattern compliance
- `converter/claude.go` — Added `server_tool_use`, `_tool_result` subtypes, `compaction` content block, `cache_control` preservation
- `converter/internal.go` — Added `CacheControl` field to `ContentBlock` struct
- `converter/openai.go` — Added tool result deduplication in `validateAndFixMessageSequence`
- `models/constants.go` — Added `StopCompaction` and `StopContentFilter` constants

---

## Phase 3: Deep Analysis (2026-05-03)

### Additional patterns discovered via deeper litellm + one-api analysis:

### 12. Redacted Thinking Blocks (litellm has it)

**litellm** (`handler.py:677-693`): `ChatCompletionRedactedThinkingBlock` type with `data` field for opaque thinking content.

**Our status**: ✅ Added `BlockRedactedThinking` to streaming state machine, `case "redacted_thinking"` already in claude.go.

### 13. Citation Support (litellm has it)

**litellm** (`handler.py:630`): Handles `citation` deltas and `citations` on content blocks. Citations are preserved on text content blocks.

**Our status**: ✅ Added `Citations` field to `ClaudeContentBlock` and `ContentBlock`, preserved in claude.go parsing.

### 14. System Message Cache Control (litellm has it)

**litellm** (`factory.py:90-122`): Preserves `cache_control` on system message blocks, supports array format system messages with per-block cache markers.

**Our status**: ✅ Extended system message parsing to extract `cache_control` markers into metadata.

### 15. Compaction Streaming Delta (litellm has it)

**litellm** (`handler.py:636-640`): Handles `compaction_delta` type in streaming content blocks with `content` field.

**Our status**: ✅ Added `BlockCompaction` to ContentBlockType enum.

### 16. Claude 4.6+ output_config (litellm has it)

**litellm** (`transformation.py:1100-1130`): Maps `reasoning_effort` to `output_config: {effort: "low"|"medium"|"high"}` for Claude 4.6+ models, separate from `thinking.budget_tokens`.

**Our status**: ✅ Added `OutputConfig` field to `ClaudeMessagesRequest`.

### 17. Signature Field in Thinking Blocks (litellm has it)

**litellm** (`handler.py:632-634`): Preserves `signature` field in thinking blocks for Anthropic's signature-based thinking verification.

**Our status**: ✅ Added `Signature` field to `ClaudeContentBlock` and `ContentBlock`, preserved in claude.go parsing.

### 18. Web Search Options → Anthropic Tool (litellm has it)

**litellm** (`transformation.py:1127-1130`): Maps `web_search_options` from OpenAI to Anthropic's hosted web search tool format.

**Our status**: ✅ Hosted tools (web_search, computer_use, text_editor, bash) converted to function tools for OpenAI compatibility. server_tool_use and tool_result subtypes handled in claude.go.

### Phase 3 Implementation Status

- [x] #12: BlockRedactedThinking in streaming_state.go
- [x] #13: Citations field on ClaudeContentBlock and ContentBlock
- [x] #14: System message cache_control preservation
- [x] #15: BlockCompaction in streaming_state.go
- [x] #16: OutputConfig field on ClaudeMessagesRequest
- [x] #17: Signature field in thinking blocks (P2)
- [x] #18: Web search options mapping (P2)
- `models/claude.go` — Added `CacheControl` field to `ClaudeContentBlock` struct
- `handler/handler.go` — Updated `appendDefaultBetaHeaders` to remove outdated headers

---

## Phase 4: Additional Patterns (2026-05-03)

### 19. Advisor Block Stripping (litellm has it)

**litellm** (`common_utils.py:664-760`): `strip_advisor_blocks_from_messages()` removes `server_tool_use(name='advisor')` and `advisor_tool_result` blocks from conversation history when the advisor tool is not in the tools array. Prevents Anthropic 400 `invalid_request_error`.

**Our status**: ✅ Added `stripAdvisorBlocks()` in claude.go, called during request parsing.

### 20. Dynamic Beta Header Management (litellm has it)

**litellm** (`transformation.py:1365-1380`): `_ensure_beta_header()` dynamically adds feature-specific beta headers based on request content (speed=fast → fast-mode, output_config → structured-output, advisor tool → advisor-tool).

**Our status**: ✅ Added `ensureBetaHeader()` and `detectBetaHeadersFromRequest()` in handler.go.

### 21. Signature and Data Fields for Thinking Blocks (litellm has it)

**litellm** (`handler.py:636-643`): Preserves `signature` field in thinking blocks and `data` field in redacted_thinking blocks.

**Our status**: ✅ Added `Signature` and `Data` fields to both `ClaudeContentBlock` and `ContentBlock`.

### Phase 4 Implementation Status

- [x] #19: Advisor block stripping — stripAdvisorBlocks() in claude.go
- [x] #20: Dynamic beta header management — ensureBetaHeader() + detectBetaHeadersFromRequest() in handler.go
- [x] #21: Signature/Data fields for thinking blocks — ClaudeContentBlock + ContentBlock

### Total: 43 protocol patterns implemented
