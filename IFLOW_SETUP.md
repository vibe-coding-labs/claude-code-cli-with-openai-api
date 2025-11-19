# iFlow API 配置指南

本文档说明如何配置代理服务器使用 iFlow API。

## iFlow API 信息

- **Base URL**: `https://apis.iflow.cn/v1`
- **API Key**: 在 [iFlow 官网](https://iflow.cn/?open=setting) 获取
- **文档**: https://platform.iflow.cn/docs
- **模型列表**: https://platform.iflow.cn/models

## 支持的模型

根据 iFlow 平台，以下是部分可用模型（需要复制页面上的实际模型ID）：

| 模型名称 | 模型ID | 上下文 | 最大输出 | 特点 |
|---------|--------|--------|---------|------|
| TStars-2.0 | `tstars2.0` | 128K | 64K | 淘宝星辰大模型 |
| Qwen3-Coder-Plus | `qwen3-coder` | 1M | 64K | 代码生成，支持 Claude Code |
| Qwen3-Max | 待确认 | 256K | 32K | 智能体编程 |
| DeepSeek-V3 | 待确认 | 128K | 32K | 推理能力强 |
| DeepSeek-R1 | 待确认 | 128K | 32K | 推理模型 |
| Kimi-K2 | 待确认 | 128K | 64K | Agent 能力 |
| GLM-4.6 | 待确认 | 200K | 128K | 支持 thinking |

> **注意**: 模型ID需要从 https://platform.iflow.cn/models 页面点击"复制模型 ID"获取

## 配置步骤

### 1. 创建 .env 文件

```bash
cp env.example .env
```

### 2. 编辑 .env 文件

```env
# Required: iFlow API Key
OPENAI_API_KEY=sk-your-iflow-api-key

# API Configuration
OPENAI_BASE_URL=https://apis.iflow.cn/v1

# Model Configuration (根据需要选择)
BIG_MODEL=tstars2.0           # Claude opus 映射
MIDDLE_MODEL=qwen3-coder      # Claude sonnet 映射
SMALL_MODEL=qwen3-coder       # Claude haiku 映射

# Security: 留空表示不验证客户端 API key
ANTHROPIC_API_KEY=

# Server Settings
HOST=0.0.0.0
PORT=10086
LOG_LEVEL=INFO
```

### 3. 启动服务器

```bash
# 使用 .env 文件
./claude-with-openai-api

# 或者使用环境变量
OPENAI_API_KEY=sk-xxx OPENAI_BASE_URL=https://apis.iflow.cn/v1 ./claude-with-openai-api
```

## 测试验证

### 方法 1: 使用测试脚本

```bash
# 先启动服务器
./claude-with-openai-api

# 在另一个终端运行测试
./test-iflow-api.sh
```

### 方法 2: 手动测试

```bash
# 测试健康检查
curl http://localhost:10086/health

# 测试连接
curl http://localhost:10086/test-connection

# 测试 Claude API 格式转换
curl -X POST http://localhost:10086/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 100,
    "messages": [
      {
        "role": "user",
        "content": "你好"
      }
    ]
  }'
```

## 使用 Claude Code CLI

```bash
# 设置环境变量
export ANTHROPIC_BASE_URL=http://localhost:10086
export ANTHROPIC_API_KEY="any-value"

# 使用 Claude Code CLI
claude
```

## 模型映射规则

代理服务器会根据 Claude 模型名称自动映射到配置的 iFlow 模型：

| Claude 模型请求 | 映射规则 | 环境变量 |
|----------------|---------|---------|
| 包含 "opus" | BIG_MODEL | 默认: tstars2.0 |
| 包含 "sonnet" | MIDDLE_MODEL | 默认: qwen3-coder |
| 包含 "haiku" | SMALL_MODEL | 默认: qwen3-coder |

## API 示例代码

### 直接调用 iFlow API (OpenAI 格式)

```bash
curl -X POST https://apis.iflow.cn/v1/chat/completions \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tstars2.0",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "max_tokens": 100
  }'
```

### 通过代理调用 (Claude 格式)

```bash
curl -X POST http://localhost:10086/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 100,
    "messages": [
      {"role": "user", "content": "你好"}
    ]
  }'
```

## 常见问题

### Q: 模型不支持错误 (Model not support)

**解决方案**:
1. 确认模型ID是否正确（需要从官网复制）
2. 检查 API Key 是否有该模型的使用权限
3. 访问 https://platform.iflow.cn/models 确认最新的模型ID

### Q: 如何获取正确的模型ID？

访问 https://platform.iflow.cn/models，点击每个模型卡片上的"复制模型 ID"按钮。

### Q: 返回 null 响应

这通常是因为上游 API 返回的响应格式与预期不符。检查：
1. 模型ID是否正确
2. 请求参数是否符合要求
3. API Key 是否有效

## 参考链接

- [iFlow 官方文档](https://platform.iflow.cn/docs)
- [iFlow 模型库](https://platform.iflow.cn/models)
- [Claude Code 文档](https://docs.anthropic.com/en/docs/claude-code/overview)
