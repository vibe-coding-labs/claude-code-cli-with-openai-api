package models

import "time"

// Batch API 相关模型

// CreateBatchRequest 创建批处理请求
type CreateBatchRequest struct {
	Requests []BatchMessageRequest `json:"requests"`
}

// BatchMessageRequest 批处理消息请求
type BatchMessageRequest struct {
	CustomID        string           `json:"custom_id"`
	MessagesRequest *MessagesRequest `json:"params"`
}

// BatchResponse 批处理响应
type BatchResponse struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"`
	ProcessingStatus  string             `json:"processing_status"`
	RequestCounts     BatchRequestCounts `json:"request_counts"`
	EndedAt           *time.Time         `json:"ended_at,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	ExpiresAt         time.Time          `json:"expires_at"`
	ArchivedAt        *time.Time         `json:"archived_at,omitempty"`
	CancelInitiatedAt *time.Time         `json:"cancel_initiated_at,omitempty"`
	ResultsURL        string             `json:"results_url,omitempty"`
}

// BatchRequestCounts 批处理请求计数
type BatchRequestCounts struct {
	Processing int `json:"processing"`
	Succeeded  int `json:"succeeded"`
	Errored    int `json:"errored"`
	Canceled   int `json:"canceled"`
	Expired    int `json:"expired"`
	Total      int `json:"total"`
}

// ListBatchesResponse 列出批处理响应
type ListBatchesResponse struct {
	Data    []BatchResponse `json:"data"`
	HasMore bool            `json:"has_more"`
	FirstID string          `json:"first_id,omitempty"`
	LastID  string          `json:"last_id,omitempty"`
}

// BatchResultsResponse 批处理结果响应
type BatchResultsResponse struct {
	Results []BatchResult `json:"results"`
}

// BatchResult 批处理单个结果
type BatchResult struct {
	CustomID string                 `json:"custom_id"`
	Type     string                 `json:"type"`
	Result   *MessagesResponse      `json:"result,omitempty"`
	Error    map[string]interface{} `json:"error,omitempty"`
}

// ListBatchesParams 列出批处理参数
type ListBatchesParams struct {
	Limit    int    `json:"limit,omitempty"`
	BeforeID string `json:"before_id,omitempty"`
	AfterID  string `json:"after_id,omitempty"`
}
