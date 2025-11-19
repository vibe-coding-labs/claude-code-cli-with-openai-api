# Claude-to-OpenAI API Proxy 使用指南

## 快速开始

### 1. 启动服务器

使用iFlow API配置快速启动：

```bash
./start-with-iflow.sh
```

或者使用自定义配置：

```bash
OPENAI_API_KEY=your-api-key \
OPENAI_BASE_URL=https://api.openai.com/v1 \
BIG_MODEL=gpt-4o \
MIDDLE_MODEL=gpt-4o \
SMALL_MODEL=gpt-4o-mini \
./claude-with-openai-api server
```

### 2. 访问管理界面

启动成功后，访问: http://localhost:10086/ui

管理界面提供以下功能：
- **配置管理**: 创建、编辑、删除多个API配置
- **详情页面**: 查看每个配置的详细信息和统计数据
- **Token统计**: 查看输入/输出Token消耗
- **请求日志**: 查看最近的API请求历史
- **在线测试**: 测试API配置是否正常工作

## 核心功能

### 配置管理

#### 创建新配置

1. 点击"新建配置"按钮
2. 填写配置信息：
   - **名称**: 配置的名称，如"iFlow API"
   - **描述**: 可选的配置描述
   - **API Key**: OpenAI兼容的API密钥（加密存储）
   - **Base URL**: API端点地址
   - **模型映射**:
     - 大模型 (Opus): 映射到高级模型
     - 中模型 (Sonnet): 映射到中级模型
     - 小模型 (Haiku): 映射到快速模型
   - **最大Token限制**: 默认4096
   - **请求超时**: 默认90秒

#### 查看配置详情

点击配置列表中的"详情"按钮，进入详情页面：

**概览标签页**:
- 配置基本信息
- 使用统计（最近30天）
  - 总请求数
  - 成功率
  - Token消耗
  - 平均响应时间

**请求日志标签页**:
- 最近50条请求记录
- 包含时间、模型、状态、Token使用、耗时等信息

#### 在线测试

在详情页点击"在线测试"按钮：
- 系统会发送一个测试请求
- 显示响应时间和Token使用情况
- 测试结果会被记录到统计数据中

#### 编辑配置

在详情页点击"编辑"按钮可以修改配置：
- API Key留空则保持不变
- 其他字段可以更新

#### 删除配置

点击"删除"按钮删除配置：
- 删除操作会同时删除相关的统计数据和日志
- 此操作不可恢复

### Token统计

系统自动记录每个请求的Token使用：

- **输入Token**: 请求中的Token数量
- **输出Token**: 响应中的Token数量
- **总Token**: 输入+输出的总和

统计数据按天聚合，可以查看：
- 总请求数
- 成功/失败请求数
- Token总消耗
- 平均响应时间

### 数据安全

- **API Key加密**: 所有API Key使用AES-256-GCM加密存储
- **加密密钥**: 通过`ENCRYPTION_KEY`环境变量设置
- **掩码显示**: 界面上只显示部分API Key（前8位+后4位）

## API端点

### 管理API

- `GET /api/configs` - 获取所有配置
- `GET /api/configs/:id` - 获取指定配置
- `POST /api/configs` - 创建新配置
- `PUT /api/configs/:id` - 更新配置
- `DELETE /api/configs/:id` - 删除配置
- `GET /api/configs/:id/stats?days=30` - 获取统计数据
- `GET /api/configs/:id/logs?limit=50` - 获取请求日志
- `POST /api/configs/:id/test` - 测试配置

### Claude API

- `POST /v1/messages` - 使用默认配置发送消息
- `POST /v1/messages/count_tokens` - 计算Token数量
- `GET /health` - 健康检查
- `GET /test-connection` - 测试上游连接

## 数据库

系统使用SQLite数据库存储：

**位置**: `./data/proxy.db` （可通过`DB_PATH`环境变量配置）

**表结构**:

1. `api_configs` - API配置
   - 存储配置信息（加密的API Key）
   - 模型映射
   - 超时设置等

2. `token_stats` - Token统计（按天聚合）
   - 每个配置每天的统计数据
   - 请求数、成功数、错误数
   - Token消耗汇总

3. `request_logs` - 请求日志
   - 每个请求的详细记录
   - Token使用、响应时间、错误信息

## 使用示例

### 示例1: 配置iFlow API

```json
{
  "name": "iFlow API",
  "description": "心流开放平台API配置",
  "openai_api_key": "sk-d35ba48d260a51054982b9a6794ca2d9",
  "openai_base_url": "https://apis.iflow.cn/v1",
  "big_model": "tstars2.0",
  "middle_model": "qwen3-coder",
  "small_model": "qwen3-coder",
  "max_tokens_limit": 4096,
  "request_timeout": 90
}
```

### 示例2: 通过代理调用Claude API

```bash
curl -X POST http://localhost:10086/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: any-value" \
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

### 示例3: 使用Claude Code CLI

```bash
# 配置环境变量
export ANTHROPIC_BASE_URL=http://localhost:10086
export ANTHROPIC_API_KEY="any-value"

# 使用Claude
claude
```

## 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `OPENAI_API_KEY` | OpenAI API密钥 | 必填 |
| `OPENAI_BASE_URL` | API端点 | `https://api.openai.com/v1` |
| `BIG_MODEL` | Opus映射模型 | `gpt-4o` |
| `MIDDLE_MODEL` | Sonnet映射模型 | `gpt-4o` |
| `SMALL_MODEL` | Haiku映射模型 | `gpt-4o-mini` |
| `PORT` | 服务器端口 | `10086` |
| `HOST` | 服务器地址 | `0.0.0.0` |
| `LOG_LEVEL` | 日志级别 | `INFO` |
| `DB_PATH` | 数据库路径 | `./data/proxy.db` |
| `ENCRYPTION_KEY` | 加密密钥 | 自动生成 |
| `ANTHROPIC_API_KEY` | 客户端验证密钥 | 空（禁用验证） |

## 故障排除

### 数据库相关

**问题**: 数据库文件无法创建

**解决**: 确保`data`目录存在且有写权限

```bash
mkdir -p data
chmod 755 data
```

### API Key相关

**问题**: API Key解密失败

**解决**: 确保`ENCRYPTION_KEY`环境变量保持一致，不要更改

### 测试失败

**问题**: 在线测试失败

**解决方案**:
1. 检查Base URL是否正确
2. 检查API Key是否有效
3. 检查模型名称是否正确
4. 查看错误日志了解详细信息

## 安全建议

1. **生产环境**: 务必设置强密码的`ENCRYPTION_KEY`
2. **API Key**: 定期轮换API密钥
3. **访问控制**: 在生产环境中添加访问限制
4. **日志管理**: 定期清理旧日志数据

## 性能优化

1. **数据库**: SQLite设置为单连接模式，适合中小规模使用
2. **日志限制**: 默认只保留最近50条日志，可通过参数调整
3. **统计聚合**: 按天聚合统计数据，减少存储空间

## 更多信息

- [iFlow 官方文档](https://platform.iflow.cn/docs)
- [iFlow 模型列表](https://platform.iflow.cn/models)
- [项目README](README.md)
