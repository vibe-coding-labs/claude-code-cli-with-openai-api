package converter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ReactConverter handles ReAct XML tool calling fallback for models
// that do not support native function calling.
type ReactConverter struct{}

// reactSystemTemplate is injected into the system prompt to teach the model
// how to call tools using XML format.
const reactSystemTemplate = `

TOOL CALLING — YOU MUST USE THIS EXACT XML FORMAT

To call a tool, you MUST output this EXACT XML block — nothing else works:

<tool>
<name>TOOL_NAME</name>
<parameters>
{"param1": "value1"}
</parameters>
</tool>

You may call multiple tools by outputting multiple XML blocks.
You MUST use valid JSON for the parameters.
After receiving tool results, continue your response normally.

Available tools:
%s

IMPORTANT: When you need to call a tool, output the XML block directly. Do NOT wrap it in markdown or code fences.`

// ParsedToolCall represents a parsed tool call from XML response.
type ParsedToolCall struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

var (
	// toolCallRe matches complete XML tool call blocks
	toolCallRe = regexp.MustCompile(
		`(?s)<tool>\s*<name>\s*(.*?)\s*</name>\s*<parameters>\s*(.*?)\s*</parameters>\s*</tool>`,
	)
	// partialToolCallRe matches incomplete XML tool call blocks (for streaming detection)
	partialToolCallRe = regexp.MustCompile(
		`(?s)<tool>\s*<name>\s*(.*?)\s*<parameters>\s*(.*?)\s*$`,
	)
)

// BuildReactSystemPrompt injects XML tool descriptions into the system prompt.
func (r *ReactConverter) BuildReactSystemPrompt(tools []ToolDefinition, originalSystem string) string {
	var toolDescs []string
	for _, tool := range tools {
		schemaJSON, _ := json.Marshal(tool.Parameters)
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s\n  Schema: %s", tool.Name, tool.Description, string(schemaJSON)))
	}
	toolSection := strings.Join(toolDescs, "\n")
	xmlInstruction := fmt.Sprintf(reactSystemTemplate, toolSection)

	if originalSystem == "" {
		return xmlInstruction
	}
	return originalSystem + xmlInstruction
}

// ParseToolCallsFromResponse extracts XML tool calls from model response text.
// Returns: parsed tool calls, remaining text (non-tool content), whether any tools were found.
func (r *ReactConverter) ParseToolCallsFromResponse(text string) ([]ParsedToolCall, string, bool) {
	matches := toolCallRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, text, false
	}

	var calls []ParsedToolCall
	for _, match := range matches {
		name := strings.TrimSpace(match[1])
		paramsStr := strings.TrimSpace(match[2])

		var params map[string]interface{}
		if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
			params = map[string]interface{}{"raw": paramsStr}
		}

		calls = append(calls, ParsedToolCall{
			Name:       name,
			Parameters: params,
		})
	}

	remaining := toolCallRe.ReplaceAllString(text, "")
	remaining = strings.TrimSpace(remaining)

	return calls, remaining, true
}

// HasPartialToolCall checks if text contains an incomplete XML tool call (for streaming).
func (r *ReactConverter) HasPartialToolCall(text string) bool {
	return strings.Contains(text, "<tool>") && !strings.Contains(text, "</tool>")
}

// ConvertToolCallsToContentBlocks converts parsed tool calls to Claude content blocks.
func (r *ReactConverter) ConvertToolCallsToContentBlocks(calls []ParsedToolCall) []ContentBlock {
	var blocks []ContentBlock
	for i, call := range calls {
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    fmt.Sprintf("toolu_%016x", i),
			Name:  call.Name,
			Input: call.Parameters,
		})
	}
	return blocks
}

// ConvertToolResultsToXML converts Claude tool_result messages to text format for request injection.
func (r *ReactConverter) ConvertToolResultsToXML(toolUseID string, content string) string {
	return fmt.Sprintf("[Tool result for %s]: %s", toolUseID, content)
}

// NeedsReactFallback determines if the model needs ReAct XML fallback.
func NeedsReactFallback(modelName string) bool {
	nonFunctionModels := []string{
		"ollama", "llama", "mistral:7b", "mistral:13b",
		"qwen", "yi-", "deepseek-coder", "starcoder",
		"codellama", "phi-", "gemma",
	}

	lower := strings.ToLower(modelName)
	for _, prefix := range nonFunctionModels {
		if strings.Contains(lower, prefix) {
			return true
		}
	}

	if strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1") {
		return true
	}

	return false
}
