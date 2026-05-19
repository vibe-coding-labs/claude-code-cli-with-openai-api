package models

import "time"

// Files API 相关模型

// CreateFileRequest 创建文件请求
type CreateFileRequest struct {
	File     []byte `json:"file"`
	Purpose  string `json:"purpose"`
	FileName string `json:"file_name"`
}

// FileResponse 文件响应
type FileResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	FileName  string    `json:"file_name"`
	Purpose   string    `json:"purpose"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	FileSize  int64     `json:"file_size"`
	Status    string    `json:"status"`
}

// ListFilesResponse 列出文件响应
type ListFilesResponse struct {
	Data    []FileResponse `json:"data"`
	HasMore bool           `json:"has_more"`
	FirstID string         `json:"first_id,omitempty"`
	LastID  string         `json:"last_id,omitempty"`
}

// ListFilesParams 列出文件参数
type ListFilesParams struct {
	Purpose  string `json:"purpose,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	BeforeID string `json:"before_id,omitempty"`
	AfterID  string `json:"after_id,omitempty"`
}

// FileContentResponse 文件内容响应
type FileContentResponse struct {
	Content []byte `json:"content"`
}
