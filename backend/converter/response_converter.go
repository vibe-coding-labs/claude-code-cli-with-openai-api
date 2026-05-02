package converter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ConvertOpenAIToClaudeResponse converts OpenAI response to Claude format
// DEPRECATED: Use GlobalFactory.ConvertOpenAIToClaude instead
func ConvertOpenAIToClaudeResponse(openAIResp *models.OpenAIResponse, originalReq *models.ClaudeMessagesRequest) *models.ClaudeResponse {
	// Convert original request to JSON for factory
	reqBody, err := json.Marshal(originalReq)
	if err != nil {
		return legacyConvertOpenAIToClaude(openAIResp, originalReq)
	}

	// Parse the original request to get InternalRequest
	internalReq, err := GlobalFactory.ConvertClaudeToInternal(reqBody)
	if err != nil {
		return legacyConvertOpenAIToClaude(openAIResp, originalReq)
	}

	// Convert OpenAI response to JSON
	respBody, err := json.Marshal(openAIResp)
	if err != nil {
		return legacyConvertOpenAIToClaude(openAIResp, originalReq)
	}

	// Use factory to convert: OpenAI -> Internal -> Claude
	claudeBody, _, err := GlobalFactory.ConvertOpenAIToClaude(respBody, internalReq)
	if err != nil {
		return legacyConvertOpenAIToClaude(openAIResp, originalReq)
	}

	// Unmarshal to Claude response
	var claudeResp models.ClaudeResponse
	if err := json.Unmarshal(claudeBody, &claudeResp); err != nil {
		return legacyConvertOpenAIToClaude(openAIResp, originalReq)
	}

	claudeResp.Model = originalReq.Model
	return &claudeResp
}

// legacyConvertOpenAIToClaude is the original conversion logic as fallback
func legacyConvertOpenAIToClaude(openAIResp *models.OpenAIResponse, originalReq *models.ClaudeMessagesRequest) *models.ClaudeResponse {
	if openAIResp == nil {
		return &models.ClaudeResponse{
			ID:      "msg_empty",
			Type:    "message",
			Role:    models.RoleAssistant,
			Model:   originalReq.Model,
			Content: []models.ClaudeContentBlock{{Type: "text", Text: ""}},
			StopReason: "end_turn",
			Usage:   models.ClaudeUsage{},
		}
	}

	if len(openAIResp.Choices) == 0 {
		return &models.ClaudeResponse{
			ID:      openAIResp.ID,
			Type:    "message",
			Role:    models.RoleAssistant,
			Model:   originalReq.Model,
			Content: []models.ClaudeContentBlock{{Type: "text", Text: ""}},
			StopReason: "end_turn",
			Usage: models.ClaudeUsage{
				InputTokens:  openAIResp.Usage.PromptTokens,
				OutputTokens: openAIResp.Usage.CompletionTokens,
			},
		}
	}

	choice := openAIResp.Choices[0]
	message := choice.Message

	contentBlocks := []models.ClaudeContentBlock{}

	if message.ReasoningContent != "" {
		contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
			Type:     "thinking",
			Thinking: message.ReasoningContent,
		})
	}

	if message.Content != nil {
		if textContent, ok := message.Content.(string); ok && textContent != "" {
			contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
				Type: models.ContentText,
				Text: textContent,
			})
		}
	}

	for _, toolCall := range message.ToolCalls {
		if toolCall.Type == models.ToolFunction {
			var input map[string]interface{}
			_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &input)
			if input == nil {
				input = map[string]interface{}{
					"raw_arguments": toolCall.Function.Arguments,
				}
			}

			contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
				Type:  models.ContentToolUse,
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: input,
			})
		}
	}

	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
			Type: models.ContentText,
			Text: "",
		})
	}

	stopReason := models.StopEndTurn
	switch choice.FinishReason {
	case "length":
		stopReason = models.StopMaxTokens
	case "tool_calls", "function_call":
		stopReason = models.StopToolUse
	case "content_filter", "refusal", "content_filtered", "interrupt", models.StopSequence, "compaction":
		stopReason = models.StopEndTurn
	default:
		stopReason = models.StopEndTurn
	}

	usage := models.ClaudeUsage{
		InputTokens:  openAIResp.Usage.PromptTokens,
		OutputTokens: openAIResp.Usage.CompletionTokens,
	}

	if openAIResp.Usage.PromptTokensDetails != nil {
		usage.CacheReadInputTokens = openAIResp.Usage.PromptTokensDetails.CachedTokens
	}

	return &models.ClaudeResponse{
		ID:           openAIResp.ID,
		Type:         "message",
		Role:         models.RoleAssistant,
		Model:        originalReq.Model,
		Content:      contentBlocks,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage:        usage,
	}
}

// StreamingResult holds the result of a streaming conversion
type StreamingResult struct {
	Content      string
	InputTokens  int
	OutputTokens int
	StopReason   string
	ToolCalls    []map[string]interface{}
}

// ConvertOpenAIStreamingToClaude converts OpenAI streaming response to Claude format.
//
// This is a complete rewrite porting litellm's AnthropicStreamWrapper state machine.
// Key architectural changes from the old implementation:
//   - Text block is started eagerly (before any content), matching litellm exactly
//   - Block type transitions detected via shouldStartNewBlock()
//   - Uses event queue to ensure proper SSE event ordering (close → open → delta)
//   - Handles tool_use and thinking blocks with proper sequential transitions
func ConvertOpenAIStreamingToClaude(c *gin.Context, reader io.Reader, originalReq *models.ClaudeMessagesRequest, ctx context.Context) *StreamingResult {
	return ConvertOpenAIStreamingToClaudeWithMapping(c, reader, originalReq, ctx, nil)
}

// ConvertOpenAIStreamingToClaudeWithMapping converts OpenAI streaming response to Claude format with tool name mapping.
func ConvertOpenAIStreamingToClaudeWithMapping(c *gin.Context, reader io.Reader, originalReq *models.ClaudeMessagesRequest, ctx context.Context, toolNameMapping map[string]string) *StreamingResult {
	state := newStreamingState(originalReq.Model, toolNameMapping)
	var collectedContent strings.Builder

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("X-Accel-Buffering", "no")

	// Emit initial events (litellm: sent_first_chunk + sent_content_block_start)
	emitMessageStart(c, state)
	emitPing(c)

	// Start heartbeat to keep connection alive
	heartbeatStop := StartHeartbeat(c, ctx, 5*time.Second)
	defer StopHeartbeat(heartbeatStop)

	// Process streaming chunks with 1MB buffer for large tool call arguments
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	done := make(chan bool, 1)
	errChan := make(chan error, 1)

	// Read from stream in a goroutine
	go func() {
		defer close(done)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				errChan <- fmt.Errorf("client disconnected")
				return
			default:
			}

			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, ":") {
				continue
			}

			var chunkData string
			if strings.HasPrefix(trimmed, "data: ") {
				chunkData = strings.TrimPrefix(trimmed, "data: ")
			} else if strings.HasPrefix(trimmed, "data:") {
				chunkData = strings.TrimPrefix(trimmed, "data:")
			} else {
				continue
			}

			if strings.TrimSpace(chunkData) == "[DONE]" {
				return
			}

			var chunk models.OpenAIResponse
			if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
				continue
			}
			state.updateUsage(&chunk)

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := &chunk.Choices[0]
			delta := choice.Delta
			if delta == nil {
				continue
			}

			// --- litellm state machine core ---
			hasContent := delta.Content != nil || delta.ReasoningContent != "" || len(delta.ToolCalls) > 0

			if hasContent {
					emittedToolArgsInTransition := false

					if !state.sentContentBlockStart {
						// Lazy start: start first content block only when content arrives
						initType, initBlockData := state.detectBlockType(delta)
						state.currentBlockType = initType
						if initBlockData != nil {
							state.currentBlockStart = initBlockData
						}
						emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart)
						state.sentContentBlockStart = true
					} else {
						// Block transitions only when a block was already established on a previous chunk
						shouldStart, newType, blockStartData := state.shouldStartNewBlock(choice)
						if shouldStart {
							emitContentBlockStop(c, state.currentBlockIndex)
							state.currentBlockIndex++
							state.currentBlockType = newType
							if blockStartData != nil {
								state.currentBlockStart = blockStartData
							}
							emitContentBlockStart(c, state.currentBlockIndex, state.currentBlockStart)

							// litellm pattern: if trigger chunk has tool arguments, emit them
							// after content_block_start (some providers send args with name/ID)
							if state.currentBlockType == BlockToolUse && len(delta.ToolCalls) > 0 {
								for _, tc := range delta.ToolCalls {
									idx := tc.Index
									if state.toolCalls[idx] == nil {
										state.toolCalls[idx] = &toolCallInfo{}
									}
									if tc.ID != "" {
										state.toolCalls[idx].id = NormalizeToolCallID(tc.ID)
									}
									toolName := state.restoreToolName(tc.Function.Name)
									if toolName != "" {
										state.toolCalls[idx].name = toolName
									}
									if tc.Function.Arguments != "" {
										state.toolCalls[idx].argsBuffer += tc.Function.Arguments
										emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, tc.Function.Arguments)
										emittedToolArgsInTransition = true
									}
								}
							}
						}
					}

					switch state.currentBlockType {
					case BlockText:
					if delta.Content != nil {
						if textContent, ok := delta.Content.(string); ok && textContent != "" {
							collectedContent.WriteString(textContent)
							emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaText, textContent)
						}
					}
				case BlockToolUse:
					if !emittedToolArgsInTransition && len(delta.ToolCalls) > 0 {
						tc := delta.ToolCalls[0]
						idx := tc.Index
						if state.toolCalls[idx] == nil {
							state.toolCalls[idx] = &toolCallInfo{}
						}
						if tc.ID != "" {
							state.toolCalls[idx].id = NormalizeToolCallID(tc.ID)
						}
						if tc.Function.Name != "" {
							state.toolCalls[idx].name = state.restoreToolName(tc.Function.Name)
						}
						if tc.Function.Arguments != "" {
							state.toolCalls[idx].argsBuffer += tc.Function.Arguments
							emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, tc.Function.Arguments)
						}
					}
				case BlockThinking:
					if delta.ReasoningContent != "" {
						emitContentBlockDelta(c, state.currentBlockIndex, "thinking_delta", delta.ReasoningContent)
					}
				}
			}

			if choice.FinishReason != "" {
				state.mu.Lock()
				state.finalStopReason = translateFinishReason(choice.FinishReason)
				state.mu.Unlock()
				if !state.sentContentBlockFinish {
					// one-api pattern: emit {} for tool_use blocks that never received arguments
					emitEmptyToolArgsIfNeeded(c, state)
					state.sentContentBlockFinish = true
					emitContentBlockStop(c, state.currentBlockIndex)
				}
				emitMessageDelta(c, state)
				state.emittedMessageDelta = true
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("scanner error: %w", err)
		}
	}()

	// Wait for completion or cancellation
	select {
	case <-done:
		// Normal completion
	case err := <-errChan:
		if strings.Contains(err.Error(), "client disconnected") {
			sendSSEError(c, "cancelled", "Request was cancelled by client")
			return nil
		}
		errorMsg := err.Error()
		classifiedError := client.ClassifyOpenAIError(errorMsg)
		sendSSEError(c, "api_error", fmt.Sprintf("Streaming error: %s", classifiedError))
		return nil
	case <-ctx.Done():
		sendSSEError(c, "cancelled", "Request was cancelled by client")
		return nil
	case <-time.After(5 * time.Minute):
		sendSSEError(c, "api_error", "Streaming timeout")
		return nil
	}

	// If content_block_finish was never sent (stream ended without finish_reason),
	// close the current block
	if !state.sentContentBlockFinish {
		// one-api pattern: emit {} for tool_use blocks that never received arguments
		emitEmptyToolArgsIfNeeded(c, state)
		emitContentBlockStop(c, state.currentBlockIndex)
		state.sentContentBlockFinish = true
	}

	// If no stop reason was set, default to end_turn
	state.mu.Lock()
	if state.finalStopReason == "" {
		state.finalStopReason = models.StopEndTurn
	}
	finalStopReason := state.finalStopReason
	usage := state.usage
	toolCalls := make(map[int]*toolCallInfo)
	for k, v := range state.toolCalls {
		toolCalls[k] = v
	}
	state.mu.Unlock()

	// Emit final message_delta if not already sent (finish_reason path already emits it)
	if !state.emittedMessageDelta {
		usageData := map[string]interface{}{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		}
		if usage.CacheReadInputTokens > 0 {
			usageData["cache_read_input_tokens"] = usage.CacheReadInputTokens
		}
		sendSSE(c, models.EventMessageDelta, map[string]interface{}{
			"type": models.EventMessageDelta,
			"delta": map[string]interface{}{
				"stop_reason":   finalStopReason,
				"stop_sequence": nil,
			},
			"usage": usageData,
		})
	}

	// Emit message_stop (litellm: sent_last_message)
	state.sentLastMessage = true
	sendSSE(c, models.EventMessageStop, map[string]interface{}{
		"type": models.EventMessageStop,
	})

		// Collect tool calls for session saving (sorted by OpenAI index for deterministic order)
		resultToolCalls := []map[string]interface{}{}
		toolIndices := make([]int, 0, len(toolCalls))
		for idx := range toolCalls {
			toolIndices = append(toolIndices, idx)
		}
		sort.Ints(toolIndices)
		for _, idx := range toolIndices {
			toolData := toolCalls[idx]
			if toolData.id != "" {
				var input map[string]interface{}
				if toolData.argsBuffer != "" {
					_ = json.Unmarshal([]byte(toolData.argsBuffer), &input)
				}
				if input == nil {
					input = map[string]interface{}{}
				}
				resultToolCalls = append(resultToolCalls, map[string]interface{}{
					"type":  "tool_use",
					"id":    toolData.id,
					"name":  toolData.name,
					"input": input,
				})
			}
		}

	c.Writer.Flush()

	return &StreamingResult{
		Content:      collectedContent.String(),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		StopReason:   string(finalStopReason),
		ToolCalls:    resultToolCalls,
	}
}

// --- SSE event emitters (litellm pattern) ---

func emitMessageStart(c *gin.Context, state *StreamingState) {
	sendSSE(c, models.EventMessageStart, map[string]interface{}{
		"type": models.EventMessageStart,
		"message": map[string]interface{}{
			"id":            state.messageID,
			"type":          "message",
			"role":          models.RoleAssistant,
			"model":         state.model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":              0,
				"output_tokens":             0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
			},
		},
	})
	state.sentFirstChunk = true
}

func emitPing(c *gin.Context) {
	sendSSE(c, models.EventPing, map[string]interface{}{
		"type": models.EventPing,
	})
}

func emitContentBlockStart(c *gin.Context, index int, blockData map[string]interface{}) {
	sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
		"type":          models.EventContentBlockStart,
		"index":         index,
		"content_block": blockData,
	})
}

func emitContentBlockStop(c *gin.Context, index int) {
	sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
		"type":  models.EventContentBlockStop,
		"index": index,
	})
}

func emitContentBlockDelta(c *gin.Context, index int, deltaType string, text string) {
	delta := map[string]interface{}{
		"type": deltaType,
	}
	switch deltaType {
	case models.DeltaInputJSON:
		delta["partial_json"] = text
	case "thinking_delta":
		delta["thinking"] = text
	default:
		delta["text"] = text
	}
	sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
		"type":  models.EventContentBlockDelta,
		"index": index,
		"delta": delta,
	})
}

func emitMessageDelta(c *gin.Context, state *StreamingState) {
	state.mu.Lock()
	stopReason := state.finalStopReason
	usage := state.usage
	state.mu.Unlock()

	usageData := map[string]interface{}{
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 {
		usageData["cache_read_input_tokens"] = usage.CacheReadInputTokens
	}

	sendSSE(c, models.EventMessageDelta, map[string]interface{}{
		"type": models.EventMessageDelta,
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usageData,
	})
}


// emitEmptyToolArgsIfNeeded emits a partial_json delta with "{}" for the current
// tool_use block if it never received any argument deltas (one-api pattern).
// This prevents client-side parsing errors when tool calls have no arguments.
func emitEmptyToolArgsIfNeeded(c *gin.Context, state *StreamingState) {
	if state.currentBlockType != BlockToolUse {
		return
	}
	// Check if any tool call has received args
	needsEmptyArgs := true
	for _, tc := range state.toolCalls {
		if tc.argsBuffer != "" {
			needsEmptyArgs = false
			break
		}
	}
	if needsEmptyArgs {
		emitContentBlockDelta(c, state.currentBlockIndex, models.DeltaInputJSON, "{}")
	}
}
