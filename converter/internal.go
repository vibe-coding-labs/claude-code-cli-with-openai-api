package converter

// InternalRequest 是标准化的内部请求格式
// 所有外部格式（Claude/OpenAI）都先转换到这个格式，再转换到目标格式
type InternalRequest struct {
	Model           string                 `json:"model"`
	Messages        []InternalMessage      `json:"messages"`
	System          string                 `json:"system,omitempty"`
	MaxTokens       int                    `json:"max_tokens,omitempty"`
	Temperature     *float64               `json:"temperature,omitempty"`
	TopP            *float64               `json:"top_p,omitempty"`
	TopK            *int                   `json:"top_k,omitempty"`
	Stream          bool                   `json:"stream,omitempty"`
	StopSeqs        []string               `json:"stop_sequences,omitempty"`
	Tools           []ToolDefinition       `json:"tools,omitempty"`
	ToolChoice      interface{}            `json:"tool_choice,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Extra           map[string]interface{} `json:"extra,omitempty"` // 透传额外字段
	ReasoningEffort *string                `json:"reasoning_effort,omitempty"`
	BetaHeaders     []string               `json:"-"` // Beta headers for Claude API, not serialized
}

// InternalResponse 是标准化的内部响应格式
type InternalResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      *UsageInfo     `json:"usage,omitempty"`
}

// InternalMessage 表示对话中的单条消息
type InternalMessage struct {
	Role    string         `json:"role"` // user, assistant, tool
	Content []ContentBlock `json:"content"`
}

// ContentBlock 表示消息中的一个内容块
type ContentBlock struct {
	Type       string                 `json:"type"` // text, image, video, audio, tool_use, tool_result
	Text       string                 `json:"text,omitempty"`
	Source     *ImageSource           `json:"source,omitempty"`       // for image
	VideoSource *VideoSource          `json:"video_source,omitempty"` // for video
	AudioSource *AudioSource          `json:"audio_source,omitempty"` // for audio
	ID         string                 `json:"id,omitempty"`           // for tool_use
	Name       string                 `json:"name,omitempty"`         // for tool_use
	Input      map[string]interface{} `json:"input,omitempty"`        // for tool_use
	ToolUseID  string                 `json:"tool_use_id,omitempty"`  // for tool_result
	Content    string                 `json:"content,omitempty"`      // for tool_result
}

// ImageSource for image content blocks.
type ImageSource struct {
	Type      string `json:"type"`      // "base64" or "url"
	MediaType string `json:"media_type"` // e.g., "image/png"
	Data      string `json:"data"`       // base64 encoded data or URL
}

// VideoSource for video content blocks.
type VideoSource struct {
	Type      string `json:"type"`      // "base64" or "url"
	MediaType string `json:"media_type"` // e.g., "video/mp4"
	Data      string `json:"data"`       // base64 encoded data or URL
}

// AudioSource for audio content blocks.
type AudioSource struct {
	Type      string `json:"type"`      // "base64" or "url"
	MediaType string `json:"media_type"` // e.g., "audio/mpeg"
	Data      string `json:"data"`       // base64 encoded data or URL
}

// ToolDefinition 定义模型可调用的工具
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// UsageInfo 表示 Token 使用情况
type UsageInfo struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// StreamEvent 表示流式响应中的事件
type StreamEvent struct {
	Type         string            `json:"type"`
	Index        int               `json:"index,omitempty"`
	Text         string            `json:"text,omitempty"`
	Delta        *StreamDelta      `json:"delta,omitempty"`
	ContentBlock *ContentBlock     `json:"content_block,omitempty"`
	Message      *InternalResponse `json:"message,omitempty"` // For message_start events
	Usage        *UsageInfo        `json:"usage,omitempty"`
}

// StreamDelta 表示流式响应中的增量数据
type StreamDelta struct {
	Type       string `json:"type,omitempty"` // text_delta, input_json_delta
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

// FormatConverter 定义格式转换器接口
type FormatConverter interface {
	// ParseRequest 从外部格式解析为内部格式
	ParseRequest(body []byte) (*InternalRequest, error)
	// BuildRequest 从内部格式构建为外部格式
	BuildRequest(req *InternalRequest) ([]byte, error)
	// ParseResponse 从外部格式解析为内部格式
	ParseResponse(body []byte) (*InternalResponse, error)
	// BuildResponse 从内部格式构建为外部格式
	BuildResponse(resp *InternalResponse) ([]byte, error)
	// ParseStreamEvent 解析流式事件
	ParseStreamEvent(line []byte) (*StreamEvent, error)
	// BuildStreamEvent 构建流式事件（用于流式响应转换）
	BuildStreamEvent(event *StreamEvent) ([]byte, error)
}
