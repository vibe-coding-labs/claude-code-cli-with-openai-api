package models

// Messages API 相关模型

// MessagesRequest 消息请求模型
type MessagesRequest struct {
	Model                  string                `json:"model"`
	Messages               []Message             `json:"messages"`
	System                 interface{}           `json:"system,omitempty"`
	MaxTokens              int                   `json:"max_tokens"`
	Temperature            float64               `json:"temperature,omitempty"`
	TopP                   *float64              `json:"top_p,omitempty"`
	TopK                   *int                  `json:"top_k,omitempty"`
	Stream                 bool                  `json:"stream,omitempty"`
	StopSequences          []string              `json:"stop_sequences,omitempty"`
	Metadata               *MessageMetadata      `json:"metadata,omitempty"`
	Tools                  []Tool                `json:"tools,omitempty"`
	ToolChoice             interface{}           `json:"tool_choice,omitempty"`
	DisableParallelToolUse *bool                 `json:"disable_parallel_tool_use,omitempty"`
	ContextManagement      *ContextManagement    `json:"context_management,omitempty"`
	Thinking               *ThinkingConfig       `json:"thinking,omitempty"`
	Container              interface{}           `json:"container,omitempty"`
	MCPServers             []MCPServerDefinition `json:"mcp_servers,omitempty"`
	ServiceTier            string                `json:"service_tier,omitempty"`
}

// Message 消息模型
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// MessageMetadata 消息元数据
type MessageMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// ThinkingConfig 思考配置
type ThinkingConfig struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // 思考token预算
}

// ContextManagement 上下文管理配置
type ContextManagement struct {
	ClearFunctionResults bool `json:"clear_function_results,omitempty"`
}

// MCPServerDefinition MCP服务器定义
type MCPServerDefinition struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

// Tool 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
	Type        string                 `json:"type,omitempty"` // "client" or "server"
}

// ContentBlock 内容块
type ContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Source    interface{}            `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"` // for thinking blocks
}

// MessagesResponse 消息响应模型
type MessagesResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Usage 使用量统计
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// CountTokensRequest 计数tokens请求
type CountTokensRequest struct {
	Model    string      `json:"model"`
	Messages []Message   `json:"messages"`
	System   interface{} `json:"system,omitempty"`
	Tools    []Tool      `json:"tools,omitempty"`
}

// CountTokensResponse 计数tokens响应
type CountTokensResponse struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// StreamEvent 流式事件
type StreamEvent struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index,omitempty"`
	Delta        *ContentBlock          `json:"delta,omitempty"`
	ContentBlock *ContentBlock          `json:"content_block,omitempty"`
	Message      *MessagesResponse      `json:"message,omitempty"`
	Usage        *Usage                 `json:"usage,omitempty"`
	Error        map[string]interface{} `json:"error,omitempty"`
}
