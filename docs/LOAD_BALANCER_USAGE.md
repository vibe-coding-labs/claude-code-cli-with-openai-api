# 负载均衡器增强功能使用指南

## 概述

负载均衡器增强功能提供了以下核心能力:

1. **健康检查** - 自动检测节点健康状态
2. **熔断器** - 防止故障节点持续接收请求
3. **智能重试** - 使用指数退避策略自动重试失败请求
4. **实时监控** - 收集和展示详细的运行指标
5. **告警系统** - 在系统异常时及时通知

## 快速开始

### 1. 创建负载均衡器

通过前端界面或API创建负载均衡器时,系统会自动启用增强功能:

```bash
POST /api/load-balancers
{
  "name": "My Load Balancer",
  "strategy": "round_robin",
  "config_nodes": [
    {"config_id": "config-1", "weight": 1, "enabled": true},
    {"config_id": "config-2", "weight": 1, "enabled": true}
  ],
  "health_check_enabled": true,
  "health_check_interval": 30,
  "failure_threshold": 3,
  "recovery_threshold": 2,
  "circuit_breaker_enabled": true,
  "error_rate_threshold": 0.5,
  "max_retries": 3
}
```

### 2. 查看健康状态

```bash
GET /api/load-balancers/{id}/health
```

响应示例:
```json
{
  "load_balancer_id": "lb-123",
  "total_nodes": 2,
  "healthy_nodes": 2,
  "unhealthy_nodes": 0,
  "statuses": [
    {
      "config_id": "config-1",
      "status": "healthy",
      "last_check_time": "2024-01-20T10:30:00Z",
      "consecutive_successes": 10,
      "consecutive_failures": 0,
      "response_time_ms": 50
    }
  ]
}
```

### 3. 查看熔断器状态

```bash
GET /api/load-balancers/{id}/circuit-breakers
```

响应示例:
```json
{
  "load_balancer_id": "lb-123",
  "total_nodes": 2,
  "closed": 2,
  "open": 0,
  "half_open": 0,
  "states": [
    {
      "config_id": "config-1",
      "state": "closed",
      "failure_count": 0,
      "success_count": 100,
      "last_state_change": "2024-01-20T10:00:00Z"
    }
  ]
}
```

### 4. 查看统计数据

```bash
GET /api/load-balancers/{id}/stats/enhanced?window=24h
```

响应示例:
```json
{
  "load_balancer_id": "lb-123",
  "time_window": "24h",
  "total_requests": 10000,
  "success_requests": 9950,
  "failed_requests": 50,
  "avg_response_time_ms": 120.5,
  "p50_response_time_ms": 100,
  "p95_response_time_ms": 200,
  "p99_response_time_ms": 300,
  "error_rate": 0.005,
  "node_stats": [
    {
      "config_id": "config-1",
      "config_name": "Primary API",
      "health_status": "healthy",
      "circuit_breaker_state": "closed",
      "request_count": 5000,
      "success_rate": 0.99,
      "avg_response_time_ms": 115.2
    }
  ]
}
```

### 5. 查看请求日志

```bash
GET /api/load-balancers/{id}/logs?limit=100&offset=0
```

### 6. 查看告警

```bash
GET /api/load-balancers/{id}/alerts?acknowledged=false
```

响应示例:
```json
{
  "load_balancer_id": "lb-123",
  "unacknowledged_count": 2,
  "alerts": [
    {
      "id": "alert-1",
      "level": "warning",
      "type": "high_error_rate",
      "message": "Error rate (5.2%) exceeds threshold (5.0%)",
      "details": "In the last 5 minutes: 52 failed out of 1000 total requests",
      "acknowledged": false,
      "created_at": "2024-01-20T10:25:00Z"
    }
  ]
}
```

### 7. 确认告警

```bash
POST /api/alerts/{alert_id}/acknowledge
```

## 配置说明

### 健康检查配置

- `health_check_enabled`: 是否启用健康检查 (默认: true)
- `health_check_interval`: 健康检查间隔,单位秒 (默认: 30)
- `failure_threshold`: 连续失败多少次后标记为不健康 (默认: 3)
- `recovery_threshold`: 连续成功多少次后恢复为健康 (默认: 2)
- `health_check_timeout`: 健康检查超时时间,单位秒 (默认: 5)

### 重试配置

- `max_retries`: 最大重试次数 (默认: 3)
- `initial_retry_delay`: 初始重试延迟,单位毫秒 (默认: 100)
- `max_retry_delay`: 最大重试延迟,单位毫秒 (默认: 5000)

### 熔断器配置

- `circuit_breaker_enabled`: 是否启用熔断器 (默认: true)
- `error_rate_threshold`: 错误率阈值,0.0-1.0 (默认: 0.5)
- `circuit_breaker_window`: 统计窗口,单位秒 (默认: 60)
- `circuit_breaker_timeout`: 熔断超时时间,单位秒 (默认: 30)
- `half_open_requests`: 半开状态测试请求数 (默认: 3)

### 动态权重配置

- `dynamic_weight_enabled`: 是否启用动态权重 (默认: false)
- `weight_update_interval`: 权重更新间隔,单位秒 (默认: 300)

### 日志配置

- `log_level`: 日志级别 (minimal, standard, detailed) (默认: standard)

## 前端界面

### 负载均衡器详情页

访问 `/ui/load-balancers/{id}` 可以查看:

1. **概览** - 基本信息和配置
2. **配置节点** - 节点列表和权重
3. **健康状态** - 实时健康状态监控
4. **熔断器** - 熔断器状态监控
5. **告警** - 告警列表和确认
6. **统计信息** - 基础统计数据
7. **增强统计** - 详细的性能指标和图表
8. **请求日志** - 详细的请求日志

## 最佳实践

### 1. 健康检查配置

- 对于稳定的API,可以设置较长的检查间隔(60秒)
- 对于不稳定的API,建议设置较短的检查间隔(10-30秒)
- 失败阈值建议设置为3-5次,避免误判
- 恢复阈值建议设置为2-3次,确保节点真正恢复

### 2. 熔断器配置

- 错误率阈值建议设置为0.3-0.5(30%-50%)
- 统计窗口建议设置为60-120秒
- 熔断超时时间建议设置为30-60秒
- 半开状态测试请求数建议设置为3-5次

### 3. 重试配置

- 最大重试次数建议设置为2-3次
- 初始延迟建议设置为100-200毫秒
- 最大延迟建议设置为5-10秒

### 4. 监控和告警

- 定期查看健康状态和熔断器状态
- 及时处理告警,特别是critical级别的告警
- 定期清理已确认的告警
- 关注错误率和响应时间趋势

### 5. 性能优化

- 使用加权策略时,根据节点性能合理分配权重
- 启用动态权重可以自动优化负载分配
- 定期清理过期的日志和统计数据
- 监控P99延迟,确保不超过10ms

## 故障排查

### 所有节点不健康

1. 检查节点配置是否正确
2. 检查网络连接是否正常
3. 检查API密钥是否有效
4. 查看健康检查日志

### 熔断器频繁打开

1. 检查节点是否真的有问题
2. 调整错误率阈值
3. 增加统计窗口时间
4. 检查请求日志找出失败原因

### 请求失败率高

1. 查看请求日志找出失败模式
2. 检查重试配置是否合理
3. 检查节点健康状态
4. 查看熔断器状态

### 响应时间慢

1. 查看节点统计数据
2. 检查是否有节点拖慢整体性能
3. 考虑调整权重分配
4. 检查网络延迟

## API参考

完整的API文档请参考: [API文档](./API_PROTOCOL_CHECKLIST.md)

## 技术支持

如有问题,请查看:
- [设计文档](../.kiro/specs/load-balancer-enhancements/design.md)
- [需求文档](../.kiro/specs/load-balancer-enhancements/requirements.md)
- [任务列表](../.kiro/specs/load-balancer-enhancements/tasks.md)
