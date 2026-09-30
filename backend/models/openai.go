package models

import "encoding/json"

// OpenAI API Models

type OpenAIRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"` // o1/o3 models
	Temperature         float64         `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	Stop                interface{}     `json:"stop,omitempty"` // string or []string
	Tools               []OpenAITool    `json:"tools,omitempty"`
	ToolChoice          interface{}     `json:"tool_choice,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"` // o1/o3 models reasoning effort: low, medium, high
	ResponseFormat      interface{}     `json:"response_format,omitempty"`  // structured output support
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type OpenAIMessage struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"` // DeepSeek/gpt-5.x thinking content
	ReasoningDetails interface{}      `json:"reasoning_details,omitempty"` // GPT-5.x/o1-pro reasoning array
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type OpenAIMessageContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *OpenAIImageURL `json:"image_url,omitempty"`
}

type OpenAIImageURL struct {
	URL string `json:"url"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type OpenAIToolCall struct {
	ID           string                      `json:"id"`
	Type         string                      `json:"type"`
	Function     OpenAIFunctionCall          `json:"function"`
	Index        int                         `json:"index,omitempty"`
	ExtraContent *OpenAIToolCallExtraContent `json:"extra_content,omitempty"`
}

// OpenAIToolCallExtraContent carries provider-specific extensions riding
// alongside the standard OpenAI tool_calls wire format. Gemini's
// OpenAI-compatibility endpoint uses this to transport its thought
// signature (see https://ai.google.dev/gemini-api/docs/thought-signatures);
// generic OpenAI clients ignore it, which is why it must be captured and
// re-attached explicitly rather than relying on pass-through.
type OpenAIToolCallExtraContent struct {
	Google *OpenAIGoogleExtraContent `json:"google,omitempty"`
}

type OpenAIGoogleExtraContent struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []OpenAIChoice  `json:"choices"`
	Usage   OpenAIUsage     `json:"usage"`
	Error   *OpenAIAPIError `json:"error,omitempty"`
}

type OpenAIAPIError struct {
	Message string         `json:"message"`
	Type    string         `json:"type"`
	Code    FlexibleString `json:"code,omitempty"`
}

// FlexibleString unmarshals a JSON string OR number into a Go string.
// Some upstream relays send a numeric error.code (e.g. the raw HTTP status
// 403) even though the OpenAI error schema specifies a string, which used to
// make json.Unmarshal fail on the entire chunk with "cannot unmarshal number
// into Go struct field ...code of type string" — silently discarding the
// upstream's actual diagnostic message (e.g. an insufficient-balance error)
// instead of surfacing it to the client.
type FlexibleString string

func (f *FlexibleString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexibleString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexibleString(n.String())
		return nil
	}
	return json.Unmarshal(data, (*string)(f))
}

type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      OpenAIMessage  `json:"message,omitempty"`
	Delta        *OpenAIMessage `json:"delta,omitempty"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}
