# 负载均衡器增强功能文档

## 概述

本文档描述了负载均衡器的增强功能，包括健康检查、熔断器、重试机制、监控和告警等核心特性。

## 核心功能

### 1. 健康检查机制

健康检查器定期检测配置节点的健康状态，及时发现和隔离故障节点。

**特性：**
- 定期健康检查（默认 30 秒间隔）
- 状态机管理（Unknown -> Healthy/Unhealthy）
- 失败阈值和恢复阈值
- 持久化健康状态到数据库

**状态转换：**
- 连续失败 >= 失败阈值（默认 3 次）→ 标记为 Unhealthy
- 连续成功 >= 恢复阈值（默认 2 次）→ 标记为 Healthy

### 2. 熔断器机制

熔断器防止故障节点持续接收请求，实现快速失败和自动恢复。

**状态：**
- **Closed（关闭）**：正常状态，请求正常通过
- **Open（开启）**：错误率超过阈值，拒绝所有请求
- **Half-Open（半开）**：超时后允许少量测试请求

**配置：**
- 错误率阈值：默认 50%
- 时间窗口：默认 60 秒
- 超时时间：默认 30 秒
- 半开测试请求数：默认 3 个

### 3. 请求重试机制

自动重试失败的请求，使用指数退避策略。

**特性：**
- 智能判断可重试错误
- 指数退避策略（初始 100ms，最大 5 秒）
- 重试时选择不同的健康节点
- 最大重试次数限制（默认 3 次）

**可重试错误：**
- 网络超时
- 连接错误
- HTTP 5xx 错误
- HTTP 429 错误（速率限制）

**不可重试错误：**
- HTTP 401（认证失败）
- HTTP 403（权限不足）
- HTTP 400（请求格式错误）

### 4. 监控和统计

实时收集和聚合系统运行指标。

**负载均衡器级别指标：**
- 总请求数
- 成功/失败请求数
- 平均响应时间
- P50/P95/P99 响应时间
- 错误率
- 活跃连接数

**配置节点级别指标：**
- 健康状态
- 请求分配数
- 成功率
- 平均响应时间
- 熔断器状态
- 当前权重

### 5. 告警机制

自动检测异常情况并生成告警。

**告警类型：**
- **Critical（严重）**：所有节点不健康
- **Warning（警告）**：健康节点数低于阈值、错误率超过阈值
- **Info（信息）**：熔断器状态变化

**告警功能：**
- 自动创建告警
- 告警查询和过滤
- 告警确认
- 未读告警计数

## API 端点

### 健康状态

```
GET /api/load-balancers/:id/health
获取负载均衡器的健康状态

GET /api/load-balancers/:id/health/:config_id
获取单个节点的健康状态

POST /api/load-balancers/:id/health/check
触发立即健康检查
```

### 熔断器

```
GET /api/load-balancers/:id/circuit-breakers
获取所有节点的熔断器状态

POST /api/load-balancers/:id/circuit-breakers/:config_id/reset
重置熔断器到关闭状态
```

### 统计数据

```
GET /api/load-balancers/:id/stats/enhanced?window=24h
获取增强统计数据
支持的时间窗口：1h, 24h, 7d, 30d
```

### 请求日志

```
GET /api/load-balancers/:id/logs?limit=100&offset=0
获取请求日志
```

### 告警

```
GET /api/load-balancers/:id/alerts?acknowledged=false
获取负载均衡器的告警

GET /api/alerts?level=critical&acknowledged=false
获取所有告警

POST /api/alerts/:id/acknowledge
确认告警
```

### 配置

```
GET /api/load-balancers/:id/config
获取负载均衡器配置

PUT /api/load-balancers/:id/config
更新负载均衡器配置
```

## 配置参数

### 健康检查配置

```json
{
  "health_check_enabled": true,
  "health_check_interval": 30,
  "failure_threshold": 3,
  "recovery_threshold": 2,
  "health_check_timeout": 5
}
```

### 重试配置

```json
{
  "max_retries": 3,
  "initial_retry_delay": 100,
  "max_retry_delay": 5000
}
```

### 熔断器配置

```json
{
  "circuit_breaker_enabled": true,
  "error_rate_threshold": 0.5,
  "circuit_breaker_window": 60,
  "circuit_breaker_timeout": 30,
  "half_open_requests": 3
}
```

### 告警配置

```json
{
  "alert_check_interval": 60,
  "error_rate_window": 5,
  "error_rate_threshold": 0.2,
  "min_healthy_nodes": 1
}
```

## 使用示例

### 创建负载均衡器

```bash
curl -X POST http://localhost:8080/api/load-balancers \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Load Balancer",
    "strategy": "weighted",
    "config_nodes": [
      {"config_id": "config-1", "weight": 70, "enabled": true},
      {"config_id": "config-2", "weight": 30, "enabled": true}
    ],
    "enabled": true
  }'
```

### 查询健康状态

```bash
curl http://localhost:8080/api/load-balancers/{id}/health
```

### 查询统计数据

```bash
curl http://localhost:8080/api/load-balancers/{id}/stats/enhanced?window=24h
```

### 查询告警

```bash
curl http://localhost:8080/api/load-balancers/{id}/alerts?acknowledged=false
```

## 性能优化

### 内存缓存

- 健康状态缓存在内存中，避免频繁查询数据库
- 配置节点信息预加载到内存

### 异步处理

- 健康检查异步执行，不阻塞请求
- 日志和统计数据异步记录
- 告警检查异步执行

### 连接池

- HTTP 连接池管理，避免频繁建立连接
- 数据库连接池优化

### 数据清理

- 自动清理 30 天前的请求日志
- 自动清理 90 天前的统计数据和告警

## 监控指标

### 系统级别

- P99 延迟 < 10ms
- 支持并发 1000+ 请求
- 内存使用稳定
- CPU 使用率低

### 负载均衡器级别

- 请求成功率
- 平均响应时间
- 错误率
- 健康节点数量

## 故障排查

### 所有节点不健康

1. 检查节点配置是否正确
2. 检查网络连接
3. 查看健康检查日志
4. 检查节点服务是否正常运行

### 熔断器频繁开启

1. 检查节点性能
2. 调整错误率阈值
3. 增加超时时间
4. 检查请求负载

### 请求失败率高

1. 查看请求日志
2. 检查重试配置
3. 查看节点健康状态
4. 检查熔断器状态

## 最佳实践

1. **合理设置阈值**：根据实际情况调整失败阈值和恢复阈值
2. **监控告警**：及时处理告警，避免服务中断
3. **定期检查**：定期查看统计数据和日志
4. **负载均衡**：合理分配权重，优化负载分配
5. **容量规划**：根据监控数据进行容量规划

## 未来改进

- [ ] 动态权重调整
- [ ] 更多负载均衡策略
- [ ] 更详细的监控指标
- [ ] Webhook 告警通知
- [ ] 性能分析工具
- [ ] 可视化监控面板
