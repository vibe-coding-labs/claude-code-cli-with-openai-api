# 用户系统使用说明

## 概述

本项目支持多用户管理，每个用户可以创建和管理自己的 API 配置和负载均衡器。系统基于 JWT 认证，提供完整的用户权限控制。

## 用户角色

### 管理员（admin）
- 拥有所有权限
- 可以创建、编辑、删除所有用户
- 可以查看所有用户的用量统计和日志
- 可以管理所有 API 配置和负载均衡器

### 普通用户（user）
- 只能查看和管理自己的资源
- 可以创建和管理自己的 API 配置
- 可以创建和管理自己的负载均衡器
- 可以查看自己的用量统计和日志

## 用户状态

### 启用（active）
- 用户可以正常登录
- 可以访问所有授权的功能
- API 配置和负载均衡器正常工作

### 禁用（disabled）
- 用户无法登录
- 所有 API 请求被拒绝（返回 403 Forbidden）
- 已创建的资源仍然保留，但无法访问

## 用户管理

### 初始化管理员用户

首次启动服务时，需要初始化管理员用户：

```bash
curl -X POST http://localhost:8083/api/auth/initialize \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "your-secure-password"
  }'
```

### 用户登录

```bash
curl -X POST http://localhost:8083/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "your-secure-password"
  }'
```

返回 JWT token，用于后续请求的认证。

### 创建用户（管理员）

```bash
curl -X POST http://localhost:8083/api/users \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{
    "username": "newuser",
    "password": "secure-password",
    "role": "user",
    "status": "active"
  }'
```

参数说明：
- `username`: 用户名（必填，3-50个字符）
- `password`: 密码（必填，最少6个字符）
- `role`: 角色（可选，默认为 `user`，可选值：`admin`, `user`）
- `status`: 状态（可选，默认为 `active`，可选值：`active`, `disabled`）

### 获取用户列表（管理员）

```bash
curl -X GET http://localhost:8083/api/users \
  -H "Authorization: Bearer <jwt-token>"
```

### 更新用户（管理员）

```bash
curl -X PUT http://localhost:8083/api/users/{id} \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{
    "username": "updated-username",
    "role": "admin",
    "status": "active"
  }'
```

### 重置用户密码（管理员）

```bash
curl -X PUT http://localhost:8083/api/users/{id}/password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{
    "password": "new-secure-password"
  }'
```

### 启用/禁用用户（管理员）

```bash
curl -X PUT http://localhost:8083/api/users/{id}/status \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{
    "status": "disabled"
  }'
```

### 删除用户（管理员）

```bash
curl -X DELETE http://localhost:8083/api/users/{id} \
  -H "Authorization: Bearer <jwt-token>"
```

**注意**：不能删除最后一个管理员用户。

## API 配置管理

### 创建 API 配置

普通用户创建的配置会自动关联到该用户：

```bash
curl -X POST http://localhost:8083/api/configs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{
    "name": "My Config",
    "openai_base_url": "https://api.openai.com/v1",
    "openai_api_key": "sk-...",
    "big_model": "gpt-4o",
    "middle_model": "gpt-4o",
    "small_model": "gpt-4o-mini"
  }'
```

### 获取用户配置列表

管理员可以查看所有配置，普通用户只能查看自己的配置：

```bash
# 管理员查看所有配置
curl -X GET http://localhost:8083/api/configs \
  -H "Authorization: Bearer <admin-jwt-token>"

# 普通用户查看自己的配置
curl -X GET http://localhost:8083/api/configs \
  -H "Authorization: Bearer <user-jwt-token>"
```

### 更新/删除配置

用户只能更新和删除自己创建的配置：

```bash
curl -X PUT http://localhost:8083/api/configs/{id} \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{...}'

curl -X DELETE http://localhost:8083/api/configs/{id} \
  -H "Authorization: Bearer <jwt-token>"
```

## 负载均衡器管理

### 创建负载均衡器

负载均衡器也会自动关联到创建用户：

```bash
curl -X POST http://localhost:8083/api/load-balancers \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -d '{
    "name": "My Load Balancer",
    "strategy": "round_robin",
    "config_nodes": [
      {"config_id": "config-1", "weight": 10, "enabled": true}
    ]
  }'
```

### 访问控制

- 管理员可以查看和管理所有负载均衡器
- 普通用户只能查看和管理自己创建的负载均衡器

## Token 统计

### 获取用户 Token 统计

```bash
curl -X GET "http://localhost:8083/api/users/{id}/stats?days=30" \
  -H "Authorization: Bearer <jwt-token>"
```

返回数据包含：
- 按模型分组的统计信息（请求数、输入 tokens、输出 tokens、总 tokens、错误数）
- 总计汇总行（model="TOTAL"）

### 统计字段说明

- `model`: 模型名称或 "TOTAL"（总计）
- `total_requests`: 总请求数
- `input_tokens`: 输入 tokens
- `output_tokens`: 输出 tokens
- `total_tokens`: 总 tokens
- `error_count`: 错误数

## 请求日志

### 获取用户请求日志

```bash
curl -X GET "http://localhost:8083/api/users/{id}/logs?page=1&page_size=20" \
  -H "Authorization: Bearer <jwt-token>"
```

### 筛选参数

- `status`: 筛选状态（`success`, `error`）
- `model`: 筛选模型
- `search`: 搜索摘要内容
- `start_time`: 开始时间（RFC3339 或 YYYY-MM-DD 格式）
- `end_time`: 结束时间（RFC3339 或 YYYY-MM-DD 格式）
- `sort_by`: 排序字段（`created_at`, `duration_ms`, `total_tokens`）
- `sort_order`: 排序方向（`asc`, `desc`）
- `page`: 页码（从1开始）
- `page_size`: 每页数量

示例：

```bash
curl -X GET "http://localhost:8083/api/users/{id}/logs?status=success&model=gpt-4o&start_time=2026-01-01&end_time=2026-01-31&page=1&page_size=20" \
  -H "Authorization: Bearer <jwt-token>"
```

## Web 界面使用

### 访问管理界面

1. 访问 `http://localhost:8083/ui`
2. 使用用户名和密码登录
3. 根据角色访问相应功能

### 用户管理页面（管理员）

- 查看用户列表
- 创建新用户
- 编辑用户信息
- 重置用户密码
- 启用/禁用用户
- 删除用户

### 用户用量页面

- 查看用户 Token 统计
- 查看用户请求日志
- 按模型、状态、时间范围筛选日志
- 查看请求详情

## 安全建议

1. **密码策略**
   - 使用强密码（至少8个字符，包含大小写字母、数字和特殊字符）
   - 定期更换密码
   - 不要使用默认密码

2. **用户管理**
   - 只授予必要的权限
   - 定期审查用户列表
   - 及时禁用不再使用的账户
   - 保持至少一个活跃的管理员账户

3. **JWT Token**
   - Token 有效期默认为 24 小时
   - 不要在客户端长时间保存 token
   - 使用 HTTPS 保护 token 传输

4. **资源隔离**
   - 普通用户只能访问自己的资源
   - 管理员可以访问所有资源
   - 禁用用户的资源仍然保留但无法访问

## 常见问题

### Q: 忘记管理员密码怎么办？

A: 需要直接访问数据库重置密码：

```bash
sqlite3 data/proxy.db
UPDATE users SET password = '<bcrypt-hash>' WHERE username = 'admin';
```

### Q: 如何禁用所有普通用户？

A: 使用 SQL 批量更新：

```bash
sqlite3 data/proxy.db
UPDATE users SET status = 'disabled' WHERE role = 'user';
```

### Q: 用户被禁用后，他们的 API 配置还能用吗？

A: 不能。被禁用的用户无法登录，也无法通过 API Key 访问任何资源。

### Q: 如何查看某个用户的所有 API 配置？

A: 使用数据库查询：

```bash
sqlite3 data/proxy.db
SELECT * FROM api_configs WHERE user_id = <user-id>;
```

### Q: 如何备份用户数据？

A: 备份整个数据库文件：

```bash
cp data/proxy.db data/proxy.db.backup
```

## 数据库迁移

用户系统相关的数据库迁移：

- `017_add_user_role_status.sql`: 添加角色和状态字段
- `018_add_user_id_to_api_configs.sql`: 为 API 配置添加 user_id
- `019_add_user_id_to_load_balancers.sql`: 为负载均衡器添加 user_id
- `020_add_user_id_to_request_logs.sql`: 为请求日志添加 user_id
- `021_add_user_id_to_token_stats.sql`: 为 Token 统计添加 user_id
- `022_add_user_indexes.sql`: 添加用户相关索引
- `023_add_username_unique_index.sql`: 添加用户名唯一索引
- `024_add_composite_indexes.sql`: 添加复合索引优化查询

## 总结

用户系统提供了完整的用户管理功能，包括：

- ✅ 多用户支持（管理员/普通用户）
- ✅ 用户状态管理（启用/禁用）
- ✅ 资源隔离（用户只能访问自己的资源）
- ✅ JWT 认证
- ✅ 完整的 API 接口
- ✅ Web 管理界面
- ✅ Token 统计和日志查询
- ✅ 权限控制

通过合理使用用户系统，可以实现多租户管理和资源隔离，提高系统的安全性和可管理性。
