package models

// GeminiRequest represents a Google Gemini API request
type GeminiRequest struct {
	Contents       []GeminiContent       `json:"contents"`
	SystemInstruction *GeminiContent     `json:"systemInstruction,omitempty"`
	Tools          []GeminiTool          `json:"tools,omitempty"`
	ToolConfig     *GeminiToolConfig     `json:"toolConfig,omitempty"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings []GeminiSafetySetting `json:"safetySettings,omitempty"`
}

// GeminiContent represents content in a Gemini request/response
type GeminiContent struct {
	Role  string      `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of content (text, inline data, or function call)
type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// GeminiInlineData represents inline data (e.g., image, video)
type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// GeminiFunctionCall represents a function call from the model
type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// GeminiFunctionResponse represents a function call response
type GeminiFunctionResponse struct {
	Name   string      `json:"name"`
	Response interface{} `json:"response"`
}

// GeminiTool represents a tool (function declaration) available to the model
type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// GeminiFunctionDeclaration represents a function that can be called
type GeminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GeminiToolConfig represents tool configuration
type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// GeminiFunctionCallingConfig represents function calling configuration
type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// GeminiGenerationConfig represents generation configuration
type GeminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

// GeminiSafetySetting represents safety settings
type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiResponse represents a Gemini API response
type GeminiResponse struct {
	Candidates     []GeminiCandidate     `json:"candidates"`
	UsageMetadata  *GeminiUsageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *GeminiPromptFeedback `json:"promptFeedback,omitempty"`
}

// GeminiCandidate represents a response candidate
type GeminiCandidate struct {
	Content      GeminiContent       `json:"content"`
	FinishReason string              `json:"finishReason,omitempty"`
	Index        int                 `json:"index"`
	SafetyRatings []GeminiSafetyRating `json:"safetyRatings,omitempty"`
}

// GeminiUsageMetadata represents token usage metadata
type GeminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
}

// GeminiPromptFeedback represents prompt feedback
type GeminiPromptFeedback struct {
	BlockReason   string              `json:"blockReason,omitempty"`
	SafetyRatings []GeminiSafetyRating `json:"safetyRatings,omitempty"`
}

// GeminiSafetyRating represents a safety rating
type GeminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Blocked     bool   `json:"blocked,omitempty"`
}

// GeminiStreamEvent represents a streaming event
type GeminiStreamEvent struct {
	Candidates     []GeminiCandidate     `json:"candidates,omitempty"`
	UsageMetadata  *GeminiUsageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *GeminiPromptFeedback `json:"promptFeedback,omitempty"`
}
