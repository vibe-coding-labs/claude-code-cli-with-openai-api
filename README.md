# Claude-to-OpenAI API Proxy

一个高性能的 Go 语言 API 代理服务器，可以将 Claude API 请求转换为 OpenAI 兼容格式，支持任意 OpenAI 兼容的 LLM 服务。

## ✨ 特性

- 🔄 **完整的 API 转换** - 支持 Claude Messages API 到 OpenAI Chat Completion API 的双向转换
- 🚀 **流式响应支持** - 完整支持 Server-Sent Events (SSE) 流式输出
- 🔑 **多配置管理** - 支持多个独立的 API 配置，通过不同的 API Key 区分
- 📊 **详细的请求日志** - 记录完整的请求/响应体、Token 统计和性能指标
- 🖥️ **Web 管理界面** - 现代化的 React 管理界面，支持 OpenAI API 配置管理、在线测试和日志查看
- 🔐 **安全认证** - 基于 API Key 的身份验证，确保服务安全
- 💾 **SQLite 存储** - 轻量级数据库，支持配置、统计和日志持久化

## 🚀 快速开始

### 1. 构建项目

```bash
go build -o claude-with-openai-api
```

### 2. 配置环境变量

创建 `.env` 文件：

```bash
# 服务器配置
PORT=8083
HOST=0.0.0.0
LOG_LEVEL=INFO

# 数据库配置
DB_PATH=./data/proxy.db

# 默认 OpenAI API 配置（首次启动后建议通过 Web UI 管理）
OPENAI_API_KEY=your-openai-api-key
OPENAI_BASE_URL=https://api.openai.com/v1
BIG_MODEL=gpt-4o
MIDDLE_MODEL=gpt-4o
SMALL_MODEL=gpt-4o-mini
```

### 3. 启动服务

```bash
./claude-with-openai-api server
```

服务启动后访问：
- **API 端点**: `http://localhost:8083`
- **管理界面**: `http://localhost:8083/ui`
- **健康检查**: `http://localhost:8083/health`

## 📖 使用方法

### 方式一：通过 Web UI 管理（推荐）

1. 访问 `http://localhost:8083/ui`
2. 创建新的 API 配置
3. 记录生成的 Anthropic API Key
4. 配置 Claude CLI：

```bash
export ANTHROPIC_BASE_URL=http://localhost:8083
export ANTHROPIC_API_KEY="your-anthropic-api-key"  # 从 Web UI 获取
claude -p "Hello"
```

### 方式二：使用 API

```bash
curl -X POST http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-anthropic-api-key" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, Claude!"}
    ]
  }'
```

## 🎯 Web UI 功能

### OpenAI API 配置管理
- ✅ 创建、编辑、删除 API 配置
- ✅ 为每个配置生成独立的 Anthropic API Key
- ✅ 一键更新 API Key（Renew Key）
- ✅ 启用/禁用配置

### 在线测试
- ✅ 独立的测试页面（`/ui/configs/:id/test`）
- ✅ 自定义模型、Max Tokens、Temperature
- ✅ 实时查看响应结果和 Token 统计

### 请求日志
- ✅ 详细的请求/响应记录
- ✅ 支持 URL 路由切换标签（`?tab=logs`）
- ✅ 查看完整的 JSON 请求体和响应体
- ✅ Token 统计和性能分析

### 统计分析
- ✅ 请求总数、成功率
- ✅ Token 使用统计（输入/输出/总计）
- ✅ 平均响应时间
- ✅ 错误统计

## 🔧 API 端点

### Claude API 兼容端点
- `POST /v1/messages` - 创建消息（需要有效的 Anthropic API Key）
- `POST /v1/messages/count_tokens` - 计算 Token 数量
- `GET /v1/admin/me` - 获取用户信息（Claude CLI 兼容）

### OpenAI API 配置管理 API
- `GET /api/configs` - 获取所有配置
- `GET /api/configs/:id` - 获取指定配置
- `POST /api/configs` - 创建新配置
- `PUT /api/configs/:id` - 更新配置
- `DELETE /api/configs/:id` - 删除配置
- `POST /api/configs/:id/renew-key` - 更新 API Key
- `POST /api/configs/:id/test` - 测试配置

### 统计和日志 API
- `GET /api/configs/:id/stats` - 获取统计信息
- `GET /api/configs/:id/logs` - 获取请求日志

### 健康检查
- `GET /health` - 服务健康状态

## 🛠️ 开发

### 项目结构

```
├── cmd/            # 命令行入口
├── config/         # 系统配置
├── converter/      # API 格式转换
├── database/       # 数据库操作
├── handler/        # HTTP 处理器
├── models/         # 数据模型
├── client/         # OpenAI 客户端
├── utils/          # 工具函数
├── frontend/       # React 管理界面
└── main.go         # 程序入口
```

### 构建前端

```bash
cd frontend
npm install
npm run build
```

### 运行测试

```bash
go test ./...
```

## 📝 配置说明

### 模型映射

代理会自动将 Claude 模型映射到配置的 OpenAI 模型：

- `claude-3-opus-*` → `big_model`
- `claude-3-5-sonnet-*` → `middle_model`
- `claude-3-*-haiku-*` → `small_model`

### 数据库

使用 SQLite 存储：
- API 配置（加密存储 OpenAI API Key）
- Token 使用统计
- 详细的请求日志

数据库文件默认位置：`./data/proxy.db`

## 🔐 安全特性

- ✅ API Key 加密存储
- ✅ 基于 API Key 的请求认证
- ✅ 配置级别的访问控制
- ✅ 无效 API Key 自动拒绝（401 Unauthorized）

## 📄 许可证

[MIT License](LICENSE)

## 🙏 致谢

本项目参考了社区中优秀的 Claude API 代理实现，并在此基础上进行了 Go 语言重写和功能增强。
