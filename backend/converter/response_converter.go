package converter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ConvertOpenAIToClaudeResponse converts OpenAI response to Claude format
// DEPRECATED: Use GlobalFactory.ConvertOpenAIToClaude instead
func ConvertOpenAIToClaudeResponse(openAIResp *models.OpenAIResponse, originalReq *models.ClaudeMessagesRequest) *models.ClaudeResponse {
	// Convert original request to JSON for factory
	reqBody, err := json.Marshal(originalReq)
	if err != nil {
		// Fallback to legacy conversion
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

	// Ensure model is preserved from original request
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

	// Build Claude content blocks
	contentBlocks := []models.ClaudeContentBlock{}

	// Add reasoning/thinking content first (if present)
	if message.ReasoningContent != "" {
		contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
			Type:     "thinking",
			Thinking: message.ReasoningContent,
		})
	}

	// Add text content
	if message.Content != nil {
		if textContent, ok := message.Content.(string); ok && textContent != "" {
			contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
				Type: models.ContentText,
				Text: textContent,
			})
		}
	}

	// Add tool calls
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

	// Ensure at least one content block
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, models.ClaudeContentBlock{
			Type: models.ContentText,
			Text: "",
		})
	}

	// Map finish reason
	stopReason := models.StopEndTurn
	switch choice.FinishReason {
	case "length":
		stopReason = models.StopMaxTokens
	case "tool_calls", "function_call":
		stopReason = models.StopToolUse
	case "content_filter", "interrupt":
		stopReason = models.StopEndTurn
	default:
		stopReason = models.StopEndTurn
	}

	// Build usage
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

// ConvertOpenAIStreamingToClaude converts OpenAI streaming response to Claude format
// Returns the collected content and token usage information
func ConvertOpenAIStreamingToClaude(c *gin.Context, reader io.Reader, originalReq *models.ClaudeMessagesRequest, ctx context.Context) *StreamingResult {
	state := &StreamingState{
		messageID:        fmt.Sprintf("msg_%s", uuid.New().String()[:24]),
		textBlockIndex:   0,
		toolBlockCounter: 0,
		currentToolCalls: make(map[int]*ToolCallState),
		finalStopReason:  models.StopEndTurn,
		usage: models.ClaudeUsage{
			InputTokens:  0,
			OutputTokens: 0,
		},
		hasStartedTextBlock:    false,
		hasFinishedTextBlock:   false,
		hasStartedThinkingBlock: false,
		hasFinishedThinkingBlock: false,
	}
	var collectedContent strings.Builder

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("X-Accel-Buffering", "no")

	// Send initial message_start event (NO content_block_start yet — wait for actual content)
	sendSSE(c, models.EventMessageStart, map[string]interface{}{
		"type": models.EventMessageStart,
		"message": map[string]interface{}{
			"id":            state.messageID,
			"type":          "message",
			"role":          models.RoleAssistant,
			"model":         originalReq.Model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})

	sendSSE(c, models.EventPing, map[string]interface{}{
		"type": models.EventPing,
	})

	// Start heartbeat to keep connection alive (ping every 5 seconds)
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
			// Check if context is cancelled (client disconnected)
			select {
			case <-ctx.Done():
				errChan <- fmt.Errorf("client disconnected")
				return
			default:
			}

			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			if strings.HasPrefix(line, "data:") {
				line = "data: " + strings.TrimPrefix(line, "data:")
			}

			if strings.HasPrefix(line, "data: ") {
				chunkData := strings.TrimPrefix(line, "data: ")
				if strings.TrimSpace(chunkData) == "[DONE]" {
					return
				}

				var chunk models.OpenAIResponse
				if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
					continue
				}

				// Extract usage information with mutex protection
				if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
					state.mu.Lock()
					state.usage.InputTokens = chunk.Usage.PromptTokens
					state.usage.OutputTokens = chunk.Usage.CompletionTokens
					if chunk.Usage.PromptTokensDetails != nil {
						state.usage.CacheReadInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
					}
					state.mu.Unlock()
				}

				if len(chunk.Choices) == 0 {
					continue
				}

				choice := chunk.Choices[0]
				delta := choice.Delta

				if delta == nil {
					continue
				}

				// Handle reasoning/thinking delta (for DeepSeek/gpt-5.x)
				if delta.ReasoningContent != "" {
					// First time seeing reasoning content — send thinking block start
					if !state.hasStartedThinkingBlock {
						state.mu.Lock()
						state.hasStartedThinkingBlock = true
						state.mu.Unlock()
						sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
							"type":  models.EventContentBlockStart,
							"index": 0,
							"content_block": map[string]interface{}{
								"type":     "thinking",
								"thinking": "",
							},
						})
					}
					sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
						"type":  models.EventContentBlockDelta,
						"index": 0,
						"delta": map[string]interface{}{
							"type":     "thinking_delta",
							"thinking": delta.ReasoningContent,
						},
					})
				}

				// Handle text delta
				if delta.Content != nil {
					if textContent, ok := delta.Content.(string); ok && textContent != "" {
						// Close thinking block if it was started and text is now arriving
						if state.hasStartedThinkingBlock && !state.hasFinishedThinkingBlock {
							state.mu.Lock()
							state.hasFinishedThinkingBlock = true
							// Text block goes after thinking block
							state.textBlockIndex = 1
							state.mu.Unlock()
							// Send content_block_stop for thinking block
							sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
								"type":  models.EventContentBlockStop,
								"index": 0,
							})
						}

						// Start text block on first text content
						if !state.hasStartedTextBlock {
							state.mu.Lock()
							state.hasStartedTextBlock = true
							textIdx := state.textBlockIndex
							state.mu.Unlock()
							sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
								"type":  models.EventContentBlockStart,
								"index": textIdx,
								"content_block": map[string]interface{}{
									"type": models.ContentText,
									"text": "",
								},
							})
						}

						// Collect content for logging
						collectedContent.WriteString(textContent)

						state.mu.Lock()
						textIdx := state.textBlockIndex
						state.mu.Unlock()
						sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
							"type":  models.EventContentBlockDelta,
							"index": textIdx,
							"delta": map[string]interface{}{
								"type": models.DeltaText,
								"text": textContent,
							},
						})
					}
				}

				// Handle tool call deltas
				if len(delta.ToolCalls) > 0 {
					// Ensure text block is started and closed before tool blocks
					if !state.hasStartedTextBlock && !state.hasStartedThinkingBlock {
						state.mu.Lock()
						state.hasStartedTextBlock = true
						textIdx := state.textBlockIndex
						state.mu.Unlock()
						sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
							"type":  models.EventContentBlockStart,
							"index": textIdx,
							"content_block": map[string]interface{}{
								"type": models.ContentText,
								"text": "",
							},
						})
					}

					for _, tcDelta := range delta.ToolCalls {
						processToolCallDelta(c, state, &tcDelta)
					}
				}

				// Handle finish reason
				if choice.FinishReason != "" {
					state.mu.Lock()
					switch choice.FinishReason {
					case "length":
						state.finalStopReason = models.StopMaxTokens
					case "tool_calls", "function_call":
						state.finalStopReason = models.StopToolUse
					case "content_filter", "interrupt":
						state.finalStopReason = models.StopEndTurn
					default:
						state.finalStopReason = models.StopEndTurn
					}
					state.mu.Unlock()
					return
				}
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

	// Close thinking block if still open
	if state.hasStartedThinkingBlock && !state.hasFinishedThinkingBlock {
		sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
			"type":  models.EventContentBlockStop,
			"index": 0,
		})
		state.hasFinishedThinkingBlock = true
		if !state.hasStartedTextBlock {
			state.textBlockIndex = 1
		}
	}

	// Close text block if started and not yet closed
	if state.hasStartedTextBlock && !state.hasFinishedTextBlock {
		state.hasFinishedTextBlock = true
		sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
			"type":  models.EventContentBlockStop,
			"index": state.textBlockIndex,
		})
	}

	// Send final events (with mutex protection for reading state)
	state.mu.Lock()
	finalStopReason := state.finalStopReason
	usage := state.usage
	currentToolCalls := make(map[int]*ToolCallState)
	for k, v := range state.currentToolCalls {
		currentToolCalls[k] = v
	}
	state.mu.Unlock()

	for _, toolData := range currentToolCalls {
		if toolData.Started {
			sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
				"type":  models.EventContentBlockStop,
				"index": toolData.ClaudeIndex,
			})
		}
	}

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

	sendSSE(c, models.EventMessageStop, map[string]interface{}{
		"type": models.EventMessageStop,
	})

	// Collect tool calls for session saving
	toolCalls := []map[string]interface{}{}
	for _, toolData := range currentToolCalls {
		if toolData.Started && toolData.ID != "" {
			var input map[string]interface{}
			if toolData.ArgsBuffer != "" {
				_ = json.Unmarshal([]byte(toolData.ArgsBuffer), &input)
			}
			if input == nil {
				input = map[string]interface{}{}
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"type":  "tool_use",
				"id":    toolData.ID,
				"name":  toolData.Name,
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
		ToolCalls:    toolCalls,
	}
}

func processToolCallDelta(c *gin.Context, state *StreamingState, tcDelta *models.OpenAIToolCall) {
	tcIndex := tcDelta.Index

	state.mu.Lock()
	// Initialize tool call tracking
	if _, exists := state.currentToolCalls[tcIndex]; !exists {
		state.currentToolCalls[tcIndex] = &ToolCallState{
			ID:          "",
			Name:        "",
			ArgsBuffer:  "",
			ClaudeIndex: 0,
			Started:     false,
		}
	}

	toolCall := state.currentToolCalls[tcIndex]

	if tcDelta.ID != "" {
		toolCall.ID = tcDelta.ID
	}

	if tcDelta.Function.Name != "" {
		toolCall.Name = tcDelta.Function.Name
	}

	// Start content block when we have complete initial data
	shouldStartBlock := toolCall.ID != "" && toolCall.Name != "" && !toolCall.Started
	var claudeIndex int
	var toolID, toolName string
	if shouldStartBlock {
		state.toolBlockCounter++
		toolCall.ClaudeIndex = state.textBlockIndex + state.toolBlockCounter
		toolCall.Started = true
		claudeIndex = toolCall.ClaudeIndex
		toolID = toolCall.ID
		toolName = toolCall.Name
	}

	var shouldProcessArgs bool
	var deltaArgs string
	if tcDelta.Function.Arguments != "" && toolCall.Started {
		toolCall.ArgsBuffer += tcDelta.Function.Arguments
		shouldProcessArgs = true
		deltaArgs = tcDelta.Function.Arguments
		claudeIndex = toolCall.ClaudeIndex
	}

	state.mu.Unlock()

	if shouldStartBlock {
		sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
			"type":  models.EventContentBlockStart,
			"index": claudeIndex,
			"content_block": map[string]interface{}{
				"type":  models.ContentToolUse,
				"id":    toolID,
				"name":  toolName,
				"input": map[string]interface{}{},
			},
		})
	}

	if shouldProcessArgs {
		sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
			"type":  models.EventContentBlockDelta,
			"index": claudeIndex,
			"delta": map[string]interface{}{
				"type":         models.DeltaInputJSON,
				"partial_json": deltaArgs,
			},
		})
	}
}
