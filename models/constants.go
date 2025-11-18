package models

const (
	// Roles
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"

	// Content types
	ContentText       = "text"
	ContentImage      = "image"
	ContentToolUse    = "tool_use"
	ContentToolResult = "tool_result"

	// Tool types
	ToolFunction = "function"

	// Stop reasons
	StopEndTurn   = "end_turn"
	StopMaxTokens = "max_tokens"
	StopToolUse   = "tool_use"

	// Event types
	EventMessageStart      = "message_start"
	EventMessageDelta      = "message_delta"
	EventMessageStop       = "message_stop"
	EventContentBlockStart = "content_block_start"
	EventContentBlockDelta = "content_block_delta"
	EventContentBlockStop  = "content_block_stop"
	EventPing              = "ping"

	// Delta types
	DeltaText      = "text_delta"
	DeltaInputJSON = "input_json_delta"
)
