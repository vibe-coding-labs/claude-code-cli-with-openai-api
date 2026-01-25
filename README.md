# Claude-to-OpenAI API Proxy

一个高性能的 Go 语言 API 代理服务器，可以将 Claude API 请求转换为 OpenAI 兼容格式，支持任意 OpenAI 兼容的 LLM 服务。

## ✨ 特性

- 🔄 **完整的 API 转换** - 支持 Claude Messages API 到 OpenAI Chat Completion API 的双向转换
- 🚀 **流式响应支持** - 完整支持 Server-Sent Events (SSE) 流式输出
- 🔑 **多配置管理** - 支持多个独立的 API 配置，通过不同的 API Key 区分
- ⚖️ **智能负载均衡** - 支持多种负载均衡策略（轮询、随机、权重、最少连接）
- 🏥 **健康检查** - 自动检测配置节点的健康状态，及时发现和隔离故障节点
- 🔄 **故障转移** - 自动切换到健康节点，确保服务连续性
- 🔁 **智能重试** - 使用指数退避策略自动重试失败的请求
- 🛡️ **熔断器保护** - 防止故障节点持续接收请求，避免资源浪费和级联故障
- 📊 **详细的请求日志** - 记录完整的请求/响应体、Token 统计和性能指标
- 📈 **实时监控** - 提供详细的运行指标和可视化监控图表
- 🚨 **告警机制** - 在系统异常时及时通知管理员
- 🖥️ **Web 管理界面** - 现代化的 React 管理界面，支持 OpenAI API 配置管理、负载均衡器管理、在线测试和日志查看
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

### 负载均衡器管理
- ✅ 创建、编辑、删除负载均衡器
- ✅ 支持多种负载均衡策略（轮询、随机、权重、最少连接）
- ✅ 配置健康检查参数（间隔、超时、阈值）
- ✅ 配置重试策略（最大重试次数、退避延迟）
- ✅ 配置熔断器参数（错误率阈值、时间窗口）
- ✅ 实时查看健康状态和熔断器状态
- ✅ 查看实时监控图表（请求趋势、成功率、响应时间）
- ✅ 查看请求日志和告警通知

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
- ✅ 平均响应时间、P50/P95/P99响应时间
- ✅ 错误统计和错误率
- ✅ 节点级别的统计数据

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

### 负载均衡器管理 API
- `GET /api/load-balancers` - 获取所有负载均衡器
- `GET /api/load-balancers/:id` - 获取指定负载均衡器
- `POST /api/load-balancers` - 创建新负载均衡器
- `PUT /api/load-balancers/:id` - 更新负载均衡器
- `DELETE /api/load-balancers/:id` - 删除负载均衡器
- `GET /api/load-balancers/:id/health` - 获取健康状态
- `GET /api/load-balancers/:id/circuit-breaker` - 获取熔断器状态
- `GET /api/load-balancers/:id/stats` - 获取统计数据
- `GET /api/load-balancers/:id/logs` - 获取请求日志
- `GET /api/load-balancers/:id/alerts` - 获取告警列表
- `POST /api/alerts/:id/acknowledge` - 确认告警

### 统计和日志 API
- `GET /api/configs/:id/stats` - 获取统计信息
- `GET /api/configs/:id/logs` - 获取请求日志

### 健康检查
- `GET /health` - 服务健康状态

## 🚀 生产部署

详细的生产部署指南请参考 [DEPLOYMENT.md](DEPLOYMENT.md)

快速部署步骤：

```bash
# 1. 复制模板文件
cp deploy-prod.sh.template deploy-prod.sh
cp k8s/deployment.yaml.template k8s/deployment.yaml
cp k8s/ingress.yaml.template k8s/ingress.yaml

# 2. 编辑配置文件，填入你的实际信息
# 3. 执行部署
./deploy-prod.sh
```

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

- ✅ API Key 加密存储（AES-256-GCM）
- ✅ 基于 API Key 的请求认证
- ✅ 配置级别的访问控制
- ✅ 无效 API Key 自动拒绝（401 Unauthorized）
- ✅ JWT 认证保护管理界面
- ✅ 敏感数据文件自动排除版本控制

### 安全最佳实践

1. **环境变量**: 使用 `env.example` 创建你自己的 `.env` 文件，不要提交真实的 API 密钥
2. **数据库权限**: 确保 `data/proxy.db` 文件权限正确（建议 600）
3. **生产部署**: 始终使用 HTTPS
4. **访问控制**: 在生产环境中配置防火墙规则
5. **密码策略**: 使用强密码保护管理界面

更多安全信息请参阅 [SECURITY.md](SECURITY.md)

## 📄 许可证

[MIT License](LICENSE)

## 🙏 致谢

本项目参考了社区中优秀的 Claude API 代理实现，并在此基础上进行了 Go 语言重写和功能增强。


## ⚖️ 负载均衡器

### 功能概述

负载均衡器允许您将请求分配到多个 API 配置节点，提供高可用性和容错能力。

### 负载均衡策略

1. **轮询（Round Robin）**
   - 按顺序依次选择节点
   - 适合节点性能相近的场景

2. **随机（Random）**
   - 随机选择节点
   - 简单高效，适合大多数场景

3. **加权轮询（Weighted Round Robin）**
   - 根据权重分配请求
   - 适合节点性能不同的场景
   - 支持动态权重调整

4. **最少连接（Least Connections）**
   - 选择当前连接数最少的节点
   - 适合请求处理时间差异大的场景

### 健康检查

系统会定期检查配置节点的健康状态：

- **检查间隔**：默认30秒（可配置10-300秒）
- **失败阈值**：连续失败3次标记为不健康
- **恢复阈值**：连续成功2次标记为健康
- **超时时间**：默认5秒

不健康的节点会自动从负载均衡池中移除，恢复后自动加入。

### 故障转移

当请求失败时，系统会自动：

1. 选择另一个健康节点
2. 重试请求
3. 记录故障转移日志

### 请求重试

支持智能重试机制：

- **最大重试次数**：默认3次（可配置0-10次）
- **退避策略**：指数退避（初始100ms，最大5秒）
- **可重试错误**：网络超时、连接错误、HTTP 5xx、429错误
- **不可重试错误**：401、403、400、404错误

### 熔断器

防止故障节点持续接收请求：

- **错误率阈值**：默认50%（可配置0.0-1.0）
- **时间窗口**：默认60秒
- **熔断超时**：默认30秒
- **半开测试**：默认3个请求

熔断器状态：
- **Closed**：正常状态，允许所有请求
- **Open**：熔断状态，拒绝所有请求
- **Half-Open**：半开状态，允许少量测试请求

### 监控和告警

#### 实时监控指标

- 总请求数、成功率、错误率
- 平均响应时间、P50/P95/P99响应时间
- 活跃连接数
- 节点健康状态和熔断器状态

#### 告警级别

- **Critical**：所有节点不健康
- **Warning**：健康节点数低于阈值、错误率超过阈值
- **Info**：熔断器状态变化

### 性能指标

- **P99延迟**：< 10ms（负载均衡器引入的额外延迟）
- **吞吐量**：> 1000 req/s
- **并发支持**：1000+ 并发请求

### 使用示例

#### 创建负载均衡器

```bash
curl -X POST http://localhost:8083/api/load-balancers \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production LB",
    "strategy": "weighted_round_robin",
    "config_nodes": [
      {"config_id": "config-1", "weight": 10, "enabled": true},
      {"config_id": "config-2", "weight": 5, "enabled": true}
    ],
    "health_check_enabled": true,
    "health_check_interval": 30,
    "max_retries": 3,
    "circuit_breaker_enabled": true
  }'
```

#### 查看健康状态

```bash
curl http://localhost:8083/api/load-balancers/{lb_id}/health
```

#### 查看统计数据

```bash
curl http://localhost:8083/api/load-balancers/{lb_id}/stats?window=1h
```

### 配置参数

| 参数 | 默认值 | 范围 | 说明 |
|------|--------|------|------|
| health_check_enabled | true | - | 是否启用健康检查 |
| health_check_interval | 30 | 10-300秒 | 健康检查间隔 |
| failure_threshold | 3 | 1-10 | 失败阈值 |
| recovery_threshold | 2 | 1-10 | 恢复阈值 |
| health_check_timeout | 5 | 1-30秒 | 健康检查超时 |
| max_retries | 3 | 0-10 | 最大重试次数 |
| initial_retry_delay | 100 | 10-1000ms | 初始重试延迟 |
| max_retry_delay | 5000 | 100-10000ms | 最大重试延迟 |
| circuit_breaker_enabled | true | - | 是否启用熔断器 |
| error_rate_threshold | 0.5 | 0.0-1.0 | 错误率阈值 |
| circuit_breaker_window | 60 | 10-300秒 | 熔断器时间窗口 |
| circuit_breaker_timeout | 30 | 10-300秒 | 熔断器超时 |
| half_open_requests | 3 | 1-10 | 半开状态测试请求数 |
| dynamic_weight_enabled | false | - | 是否启用动态权重 |
| log_level | standard | minimal/standard/detailed | 日志详细级别 |

## 📚 文档

- [负载均衡器增强功能文档](./docs/LOAD_BALANCER_ENHANCEMENTS.md)
- [负载均衡器使用指南](./docs/LOAD_BALANCER_USAGE.md)
- [运维手册](./docs/OPERATIONS_MANUAL.md)
- [性能测试报告](./docs/PERFORMANCE_TEST_REPORT.md)
- [测试覆盖率报告](./docs/TEST_COVERAGE_REPORT.md)
- [部署指南](./DEPLOYMENT.md)

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License
