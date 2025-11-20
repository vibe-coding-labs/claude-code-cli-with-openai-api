# 用户系统使用指南

## 概述

系统已成功集成用户认证系统，所有管理API现在都需要登录后才能访问（Claude API除外）。

## 主要功能

### 1. 用户认证系统

#### 首次运行
- 系统首次运行时会自动重定向到初始化页面
- 在初始化页面设置管理员用户名和密码
- 用户名要求：3-50个字符
- 密码要求：至少6个字符

#### 登录
- 访问 `http://localhost:8083/ui/` 会自动重定向到登录页面
- 使用设置的用户名和密码登录
- 登录成功后会获得JWT token，有效期7天

#### 忘记密码
如果忘记密码，可以使用命令行重置：

```bash
./claude-code-cli-with-openai-api reset-password
```

此命令会：
1. 删除所有用户数据
2. 允许重新初始化系统
3. 需要通过Web界面重新设置用户名和密码

### 2. 配置详情页面优化

#### 页面结构
配置详情页面现在使用标签页组织：
- **详情**：查看配置的所有信息
- **请求日志**：查看和管理请求日志（带高级筛选功能）
- **在线测试**：跳转到独立测试页面

#### 主要改进
1. **移除顶部在线测试按钮**：改为标签页形式
2. **编辑功能**：点击"编辑配置"跳转到独立编辑页面
3. **更新API Token**：在详情标签页的Token卡片中，点击图标按钮即可刷新
4. **Claude CLI配置示例**：自动根据实际服务器端口和配置生成正确的示例

### 3. 独立页面

#### 编辑页面
- URL: `/ui/configs/{id}/edit`
- 提供完整的表单编辑功能
- 保存后自动返回详情页

#### 在线测试页面
- URL: `/ui/configs/{id}/test`
- 独立的测试页面，不再使用弹窗
- 提供完整的测试结果展示

### 4. 请求日志增强功能

#### 筛选功能
- **状态筛选**：成功/失败
- **模型筛选**：根据实际使用的模型动态生成
- **搜索**：在请求摘要和响应预览中搜索关键词

#### 排序功能
- 按时间排序
- 按耗时排序
- 按Token数排序
- 升序/降序切换

#### 分页功能
- 每页显示记录数可调整（20/50/100）
- 快速跳转到指定页
- 显示总记录数和总页数

#### 其他功能
- **刷新**：手动刷新日志列表
- **清空日志**：删除所有日志记录（需二次确认）
- **日志详情**：点击查看完整的请求和响应体
- **统计横幅**：在日志页面顶部显示最近30天的统计数据

### 5. Claude CLI配置示例修正

配置详情页面现在会根据实际情况生成正确的示例：

```bash
# 环境变量方式
export ANTHROPIC_BASE_URL=http://localhost:8083
export ANTHROPIC_API_KEY="你的-API-Token"

# 或直接在命令中使用
ANTHROPIC_BASE_URL=http://localhost:8083 \
ANTHROPIC_API_KEY="你的-API-Token" \
claude
```

**重要提示**：
- URL中的端口会根据实际运行端口自动调整
- API Token是配置特有的，用于识别使用哪个配置

## 安全性

### 认证保护
- 所有管理API都需要JWT认证
- Claude API端点（`/v1/*`, `/proxy/*`）使用独立的API Key认证
- Token在7天后自动过期

### 密码安全
- 密码使用bcrypt加密存储
- 不会在任何API响应中暴露明文密码

## 技术实现

### 后端
- JWT认证（使用golang-jwt/jwt/v5）
- Bcrypt密码加密
- SQLite用户表
- 认证中间件自动保护管理API

### 前端
- React Router路由守卫
- Axios拦截器自动附加Token
- LocalStorage存储用户会话
- 401错误自动重定向到登录页

### API端点

#### 认证相关
- `GET /api/auth/initialized` - 检查系统是否已初始化
- `POST /api/auth/initialize` - 初始化系统（创建首个用户）
- `POST /api/auth/login` - 用户登录

#### OpenAI API 配置管理（需要认证）
- `GET /api/configs` - 获取所有配置
- `GET /api/configs/:id` - 获取单个配置
- `POST /api/configs` - 创建配置
- `PUT /api/configs/:id` - 更新配置
- `DELETE /api/configs/:id` - 删除配置
- `POST /api/configs/:id/renew-key` - 更新API Token

#### 日志管理（需要认证）
- `GET /api/configs/:id/logs` - 获取日志（支持分页、筛选、排序）
  - 查询参数：
    - `page`: 页码（默认1）
    - `page_size`: 每页大小（默认20，最大100）
    - `status`: 状态筛选（success/error）
    - `model`: 模型筛选
    - `search`: 搜索关键词
    - `sort_by`: 排序字段（created_at/duration_ms/total_tokens）
    - `sort_order`: 排序方向（asc/desc）
- `DELETE /api/configs/:id/logs` - 清空配置的所有日志
- `GET /api/configs/:id/logs/:log_id` - 获取单条日志详情
- `GET /api/configs/:id/models` - 获取配置使用过的模型列表

#### Claude API（无需管理认证）
- `POST /v1/messages` - Claude消息API
- `POST /proxy/:id/v1/messages` - 指定配置的消息API

## 使用流程

### 首次使用
1. 启动服务器：`./claude-code-cli-with-openai-api server`
2. 访问：`http://localhost:8083/ui/`
3. 自动跳转到初始化页面
4. 设置管理员账户
5. 登录成功，开始使用

### 日常使用
1. 访问：`http://localhost:8083/ui/`
2. 如未登录，会跳转到登录页
3. 登录后查看和管理配置
4. 使用配置的API Token调用Claude API

### 密码重置
1. 执行：`./claude-code-cli-with-openai-api reset-password`
2. 确认操作
3. 访问Web界面重新初始化

## 注意事项

1. **首次运行**：必须完成初始化才能使用系统
2. **密码保管**：请妥善保管密码，重置会清除所有用户数据
3. **Token有效期**：JWT Token 7天后过期，需要重新登录
4. **端口配置**：Claude CLI配置示例会根据实际端口自动生成
5. **日志清空**：清空日志操作不可恢复，请谨慎操作

## 故障排除

### 无法登录
- 检查用户名和密码是否正确
- 如忘记密码，使用`reset-password`命令重置

### Token过期
- 重新登录即可获得新Token

### 页面无法访问
- 检查服务器是否正在运行
- 检查端口是否正确（默认8083）

### Claude API调用失败
- 检查API Token是否正确
- 检查配置是否启用
- Claude API不需要管理员登录，只需要正确的API Token
