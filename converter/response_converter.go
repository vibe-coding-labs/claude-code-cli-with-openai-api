package converter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/client"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/models"
)

// ConvertOpenAIToClaudeResponse converts OpenAI response to Claude format
func ConvertOpenAIToClaudeResponse(openAIResp *models.OpenAIResponse, originalReq *models.ClaudeMessagesRequest) *models.ClaudeResponse {
	if len(openAIResp.Choices) == 0 {
		return nil
	}

	choice := openAIResp.Choices[0]
	message := choice.Message

	// Build Claude content blocks
	contentBlocks := []models.ClaudeContentBlock{}

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

// StreamingState holds the state for streaming conversion
type StreamingState struct {
	mu               sync.Mutex
	messageID        string
	textBlockIndex   int
	toolBlockCounter int
	currentToolCalls map[int]*ToolCallState
	finalStopReason  string
	usage            models.ClaudeUsage
}

type ToolCallState struct {
	ID          string
	Name        string
	ArgsBuffer  string
	JSONSent    bool
	ClaudeIndex int
	Started     bool
}

// ConvertOpenAIStreamingToClaude converts OpenAI streaming response to Claude format
func ConvertOpenAIStreamingToClaude(c *gin.Context, reader io.Reader, originalReq *models.ClaudeMessagesRequest, ctx context.Context) {
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
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "*")

	// Send initial events
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

	sendSSE(c, models.EventContentBlockStart, map[string]interface{}{
		"type":  models.EventContentBlockStart,
		"index": 0,
		"content_block": map[string]interface{}{
			"type": models.ContentText,
			"text": "",
		},
	})

	sendSSE(c, models.EventPing, map[string]interface{}{
		"type": models.EventPing,
	})

	// Process streaming chunks
	scanner := bufio.NewScanner(reader)
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

			if strings.HasPrefix(line, "data: ") {
				chunkData := strings.TrimPrefix(line, "data: ")
				if strings.TrimSpace(chunkData) == "[DONE]" {
					return
				}

				var chunk models.OpenAIResponse
				if err := json.Unmarshal([]byte(chunkData), &chunk); err != nil {
					// Log parsing error but continue
					continue
				}

				// Extract usage information with mutex protection
				// Usage information can appear in any chunk when stream_options.include_usage is true
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

				// Handle text delta
				if delta.Content != nil {
					if textContent, ok := delta.Content.(string); ok && textContent != "" {
						sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
							"type":  models.EventContentBlockDelta,
							"index": state.textBlockIndex,
							"delta": map[string]interface{}{
								"type": models.DeltaText,
								"text": textContent,
							},
						})
					}
				}

				// Handle tool call deltas
				if len(delta.ToolCalls) > 0 {
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
		// Handle error (including cancellation)
		if strings.Contains(err.Error(), "client disconnected") {
			sendSSEError(c, "cancelled", "Request was cancelled by client")
			return
		}
		// Classify error message for better error handling
		errorMsg := err.Error()
		classifiedError := client.ClassifyOpenAIError(errorMsg)
		sendSSEError(c, "api_error", fmt.Sprintf("Streaming error: %s", classifiedError))
		return
	case <-ctx.Done():
		// Client disconnected
		sendSSEError(c, "cancelled", "Request was cancelled by client")
		return
	case <-time.After(5 * time.Minute):
		// Timeout
		sendSSEError(c, "api_error", "Streaming timeout")
		return
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

	sendSSE(c, models.EventContentBlockStop, map[string]interface{}{
		"type":  models.EventContentBlockStop,
		"index": state.textBlockIndex,
	})

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
}

func sendSSEError(c *gin.Context, errorType, message string) {
	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	}
	sendSSE(c, "error", errorEvent)
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
			JSONSent:    false,
			ClaudeIndex: 0,
			Started:     false,
		}
	}

	toolCall := state.currentToolCalls[tcIndex]

	// Update tool call ID
	if tcDelta.ID != "" {
		toolCall.ID = tcDelta.ID
	}

	// Update function name
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

	// Handle function arguments
	var shouldProcessArgs bool
	var argsBuffer string
	var jsonSent bool
	if tcDelta.Function.Arguments != "" && toolCall.Started {
		toolCall.ArgsBuffer += tcDelta.Function.Arguments
		shouldProcessArgs = true
		argsBuffer = toolCall.ArgsBuffer
		jsonSent = toolCall.JSONSent
		claudeIndex = toolCall.ClaudeIndex
	}

	state.mu.Unlock()

	// Send content block start event (outside of lock)
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

	// Process function arguments (outside of lock)
	if shouldProcessArgs {
		// Try to parse complete JSON
		var testJSON interface{}
		if err := json.Unmarshal([]byte(argsBuffer), &testJSON); err == nil {
			if !jsonSent {
				state.mu.Lock()
				state.currentToolCalls[tcIndex].JSONSent = true
				state.mu.Unlock()

				sendSSE(c, models.EventContentBlockDelta, map[string]interface{}{
					"type":  models.EventContentBlockDelta,
					"index": claudeIndex,
					"delta": map[string]interface{}{
						"type":         models.DeltaInputJSON,
						"partial_json": argsBuffer,
					},
				})
			}
		}
	}
}

func sendSSE(c *gin.Context, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData))))
	c.Writer.Flush()
}
