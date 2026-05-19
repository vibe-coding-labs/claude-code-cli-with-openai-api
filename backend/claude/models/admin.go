package models

// Admin API 相关模型

// OrganizationResponse 组织响应
type OrganizationResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// OrganizationUsageResponse 组织使用情况响应
type OrganizationUsageResponse struct {
	Object string       `json:"object"`
	Data   []UsageEntry `json:"data"`
}

// UsageEntry 使用条目
type UsageEntry struct {
	Type          string  `json:"type"`
	CreditBalance float64 `json:"credit_balance,omitempty"`
	CreditsUsed   float64 `json:"credits_used,omitempty"`
	TokensUsed    int64   `json:"tokens_used,omitempty"`
	Period        string  `json:"period,omitempty"`
}

// ModelResponse 模型响应
type ModelResponse struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	ContextSize int    `json:"context_size,omitempty"`
}

// ListModelsResponse 列出模型响应
type ListModelsResponse struct {
	Object string          `json:"object"`
	Data   []ModelResponse `json:"data"`
}

// ErrorResponse API错误响应
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
