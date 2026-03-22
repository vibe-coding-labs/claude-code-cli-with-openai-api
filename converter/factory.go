package converter

import (
	"fmt"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

// ConverterType 表示转换器类型
type ConverterType string

const (
	// ConverterTypeClaude Claude API 转换器
	ConverterTypeClaude ConverterType = "claude"
	// ConverterTypeOpenAI OpenAI API 转换器
	ConverterTypeOpenAI ConverterType = "openai"
	// ConverterTypeGemini Gemini API 转换器
	ConverterTypeGemini ConverterType = "gemini"
)

// ConverterFactory 转换器工厂，管理所有转换器实例
type ConverterFactory struct {
	claudeConverter *ClaudeConverter
	openAIConverter *OpenAIConverter
	geminiConverter *GeminiConverter
}

// NewConverterFactory 创建新的转换器工厂
func NewConverterFactory() *ConverterFactory {
	return &ConverterFactory{
		claudeConverter: NewClaudeConverter(),
		openAIConverter: NewOpenAIConverter(nil),
		geminiConverter: NewGeminiConverter(),
	}
}

// GetClaudeConverter 获取 Claude 转换器
func (f *ConverterFactory) GetClaudeConverter() *ClaudeConverter {
	return f.claudeConverter
}

// GetOpenAIConverter 获取 OpenAI 转换器
func (f *ConverterFactory) GetOpenAIConverter() *OpenAIConverter {
	return f.openAIConverter
}

// GetGeminiConverter 获取 Gemini 转换器
func (f *ConverterFactory) GetGeminiConverter() *GeminiConverter {
	return f.geminiConverter
}

// SetOpenAIConfig 设置 OpenAI 转换器的配置
func (f *ConverterFactory) SetOpenAIConfig(cfg *config.Config) {
	f.openAIConverter.SetConfig(cfg)
}

// ConvertClaudeToInternal 将 Claude 请求转换为内部格式
func (f *ConverterFactory) ConvertClaudeToInternal(body []byte) (*InternalRequest, error) {
	return f.claudeConverter.ParseRequest(body)
}

// ConvertInternalToOpenAI 将内部格式转换为 OpenAI 请求
func (f *ConverterFactory) ConvertInternalToOpenAI(req *InternalRequest) ([]byte, error) {
	return f.openAIConverter.BuildRequest(req)
}

// ConvertOpenAIToInternal 将 OpenAI 响应转换为内部格式
func (f *ConverterFactory) ConvertOpenAIToInternal(body []byte) (*InternalResponse, error) {
	return f.openAIConverter.ParseResponse(body)
}

// ConvertInternalToClaude 将内部格式转换为 Claude 响应
func (f *ConverterFactory) ConvertInternalToClaude(resp *InternalResponse) ([]byte, error) {
	return f.claudeConverter.BuildResponse(resp)
}

// ConvertClaudeToOpenAI 直接转换 Claude 请求为 OpenAI 请求（完整流程）
func (f *ConverterFactory) ConvertClaudeToOpenAI(body []byte, cfg *config.Config) ([]byte, *InternalRequest, error) {
	// 保存配置
	if cfg != nil {
		f.SetOpenAIConfig(cfg)
	}

	// Claude -> Internal
	internalReq, err := f.ConvertClaudeToInternal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse claude request: %w", err)
	}

	// Internal -> OpenAI
	openAIBody, err := f.ConvertInternalToOpenAI(internalReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build openai request: %w", err)
	}

	return openAIBody, internalReq, nil
}

// ConvertOpenAIToClaude 直接转换 OpenAI 响应为 Claude 响应（完整流程）
func (f *ConverterFactory) ConvertOpenAIToClaude(body []byte, originalReq *InternalRequest) ([]byte, *InternalResponse, error) {
	// OpenAI -> Internal
	internalResp, err := f.ConvertOpenAIToInternal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse openai response: %w", err)
	}

	// 设置原始请求中的模型
	if originalReq != nil {
		internalResp.Model = originalReq.Model
	}

	// Internal -> Claude
	claudeBody, err := f.ConvertInternalToClaude(internalResp)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build claude response: %w", err)
	}

	return claudeBody, internalResp, nil
}

// GlobalFactory 全局转换器工厂实例
var GlobalFactory = NewConverterFactory()
