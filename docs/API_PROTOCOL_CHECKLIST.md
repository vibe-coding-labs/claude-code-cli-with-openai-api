# Anthropic API 协议实现检查清单

## Request 字段实现状态

### 核心字段
- [x] `model` (string, required) - 模型名称
- [x] `messages` (array, required) - 对话消息数组
- [x] `max_tokens` (integer, required) - 最大生成 token 数

### 可选参数
- [x] `system` (string|array) - 系统提示词
- [x] `temperature` (number, 0-1) - 温度参数
- [x] `top_p` (number, 0-1) - Top-p 采样
- [x] `top_k` (integer) - Top-k 采样
- [x] `stop_sequences` (array of strings) - 停止序列
- [x] `stream` (boolean) - 是否流式响应
- [x] `metadata` (object) - 元数据
  - [x] `user_id` (string) - 用户ID

### 工具相关
- [x] `tools` (array) - 工具定义数组
  - [x] `name` (string) - 工具名称
  - [x] `description` (string) - 工具描述
  - [x] `input_schema` (object) - 输入 JSON Schema
- [x] `tool_choice` (object) - 工具选择策略
  - [x] `type` (string) - "auto" | "any" | "tool"
  - [x] `name` (string) - 工具名称（当 type="tool"）
  - [ ] `disable_parallel_tool_use` (boolean) - 禁用并行工具调用

### Beta 功能
- [x] `thinking` (object) - 扩展思维功能
  - [x] `type` (string) - "enabled"
  - [x] `budget_tokens` (integer) - 思维 token 预算

### Claude Code 特有
- [x] `context_management` (object) - 上下文管理

## Response 字段实现状态

### 非流式响应
- [x] `id` (string) - 消息ID
- [x] `type` (string) - "message"
- [x] `role` (string) - "assistant"
- [x] `model` (string) - 使用的模型
- [x] `content` (array) - 内容块数组
- [x] `stop_reason` (string) - 停止原因
- [x] `stop_sequence` (string|null) - 停止序列
- [x] `usage` (object) - Token 使用情况
  - [x] `input_tokens` (integer)
  - [x] `output_tokens` (integer)
  - [x] `cache_creation_input_tokens` (integer)
  - [x] `cache_read_input_tokens` (integer)

### Content Block 类型
- [x] `text` - 文本内容
  - [x] `type` = "text"
  - [x] `text` (string)
- [x] `image` - 图片内容
  - [x] `type` = "image"
  - [x] `source` (object)
- [x] `tool_use` - 工具使用
  - [x] `type` = "tool_use"
  - [x] `id` (string)
  - [x] `name` (string)
  - [x] `input` (object)
- [x] `tool_result` - 工具结果
  - [x] `type` = "tool_result"
  - [x] `tool_use_id` (string)
  - [x] `content` (string|array)
  - [x] `is_error` (boolean)

### 流式响应事件
- [x] `message_start` - 消息开始
- [x] `content_block_start` - 内容块开始
- [x] `content_block_delta` - 内容块增量
  - [x] `type` = "content_block_delta"
  - [x] `index` (integer)
  - [x] `delta` (object)
    - [x] `type` = "text_delta" | "input_json_delta"
    - [x] `text` (string) - 文本增量
    - [x] `partial_json` (string) - JSON 增量
- [x] `content_block_stop` - 内容块结束
- [x] `message_delta` - 消息增量
  - [x] `delta` (object)
    - [x] `stop_reason` (string)
    - [x] `stop_sequence` (string|null)
  - [x] `usage` (object)
- [x] `message_stop` - 消息结束
- [ ] `ping` - 心跳事件（可选）
- [x] `error` - 错误事件

## Stop Reasons
- [x] `end_turn` - 模型自然结束
- [x] `max_tokens` - 达到最大 token 限制
- [x] `stop_sequence` - 遇到停止序列
- [x] `tool_use` - 需要执行工具

## Error Types
- [x] `invalid_request_error` - 无效请求
- [x] `authentication_error` - 认证错误
- [x] `permission_error` - 权限错误
- [x] `not_found_error` - 未找到资源
- [x] `rate_limit_error` - 速率限制
- [x] `api_error` - API 错误
- [x] `overloaded_error` - 服务过载

## 待实现功能

### 高优先级
1. [ ] `disable_parallel_tool_use` 字段支持
2. [ ] `ping` 事件支持（流式）
3. [ ] 更精确的 token 计数
4. [ ] 完整的图片内容支持

### 中优先级
1. [ ] 多模态内容完整支持
2. [ ] 工具并行调用控制
3. [ ] 更详细的错误分类

### 低优先级
1. [ ] 请求重试机制
2. [ ] 速率限制实现
3. [ ] 缓存策略优化

## 测试覆盖

### 已测试
- [x] 基本文本对话
- [x] 流式响应
- [x] 工具调用
- [x] 连接测试请求

### 待测试
- [ ] 多轮对话
- [ ] 图片输入
- [ ] 并行工具调用
- [ ] 思维链功能
- [ ] 各种错误场景

## 参考文档
- Anthropic Messages API: https://docs.anthropic.com/en/api/messages
- Claude Code CLI: https://docs.anthropic.com/en/docs/build-with-claude/claude-code
