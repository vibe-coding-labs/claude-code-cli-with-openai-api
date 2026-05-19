package models

import "time"

// Skills API 相关模型

// CreateSkillRequest 创建技能请求
type CreateSkillRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Instructions string                 `json:"instructions"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// SkillResponse 技能响应
type SkillResponse struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Instructions string                 `json:"instructions"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// ListSkillsResponse 列出技能响应
type ListSkillsResponse struct {
	Data    []SkillResponse `json:"data"`
	HasMore bool            `json:"has_more"`
	FirstID string          `json:"first_id,omitempty"`
	LastID  string          `json:"last_id,omitempty"`
}

// CreateSkillVersionRequest 创建技能版本请求
type CreateSkillVersionRequest struct {
	Instructions string                 `json:"instructions"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// SkillVersionResponse 技能版本响应
type SkillVersionResponse struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	SkillID      string                 `json:"skill_id"`
	Version      int                    `json:"version"`
	Instructions string                 `json:"instructions"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	IsActive     bool                   `json:"is_active"`
}

// ListSkillVersionsResponse 列出技能版本响应
type ListSkillVersionsResponse struct {
	Data    []SkillVersionResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
	FirstID string                 `json:"first_id,omitempty"`
	LastID  string                 `json:"last_id,omitempty"`
}

// ListSkillsParams 列出技能参数
type ListSkillsParams struct {
	Limit    int    `json:"limit,omitempty"`
	BeforeID string `json:"before_id,omitempty"`
	AfterID  string `json:"after_id,omitempty"`
}

// ListSkillVersionsParams 列出技能版本参数
type ListSkillVersionsParams struct {
	Limit    int    `json:"limit,omitempty"`
	BeforeID string `json:"before_id,omitempty"`
	AfterID  string `json:"after_id,omitempty"`
}
