# 负载均衡器增强功能 - 运维手册

## 目录

1. [系统概述](#系统概述)
2. [日常运维](#日常运维)
3. [监控和告警](#监控和告警)
4. [故障排查](#故障排查)
5. [性能调优](#性能调优)
6. [备份和恢复](#备份和恢复)
7. [升级和回滚](#升级和回滚)
8. [安全最佳实践](#安全最佳实践)

---

## 系统概述

负载均衡器增强功能提供了以下核心能力：

- **健康检查**：自动检测配置节点的健康状态
- **故障转移**：自动切换到健康节点
- **请求重试**：使用指数退避策略自动重试失败的请求
- **熔断器**：防止故障节点持续接收请求
- **监控统计**：提供详细的运行指标和可视化监控
- **告警机制**：在系统异常时及时通知管理员

### 系统架构

```
客户端请求 → 负载均衡器处理器 → 选择器 → 配置节点
                ↓
            监控器 → 统计收集器 → 数据库
                ↓
            告警管理器
```

---

## 日常运维

### 启动服务

```bash
# 启动服务
./claude-code-cli-with-openai-api server

# 使用自定义端口
./claude-code-cli-with-openai-api server --port 8080

# 使用自定义数据库
./claude-code-cli-with-openai-api server --db /path/to/database.db
```

### 停止服务

```bash
# 优雅关闭（等待所有请求完成）
kill -SIGTERM <pid>

# 强制关闭
kill -SIGKILL <pid>
```

### 查看服务状态

```bash
# 检查服务是否运行
ps aux | grep claude-code-cli-with-openai-api

# 查看服务日志
tail -f server.log

# 查看实时指标
curl http://localhost:8080/api/load-balancers/{lb_id}/stats
```

### 配置管理

#### 查看负载均衡器配置

```bash
# 通过API查看
curl http://localhost:8080/api/load-balancers/{lb_id}

# 通过前端界面
访问 http://localhost:3000/load-balancers/{lb_id}
```

#### 更新配置

```bash
# 更新健康检查配置
curl -X PUT http://localhost:8080/api/load-balancers/{lb_id} \
  -H "Content-Type: application/json" \
  -d '{
    "health_check_enabled": true,
    "health_check_interval": 30,
    "failure_threshold": 3,
    "recovery_threshold": 2
  }'
```

#### 配置参数说明

| 参数 | 默认值 | 范围 | 说明 |
|------|--------|------|------|
| health_check_enabled | true | - | 是否启用健康检查 |
| health_check_interval | 30 | 10-300秒 | 健康检查间隔 |
| failure_threshold | 3 | 1-10 | 失败阈值 |
| recovery_threshold | 2 | 1-10 | 恢复阈值 |
| max_retries | 3 | 0-10 | 最大重试次数 |
| circuit_breaker_enabled | true | - | 是否启用熔断器 |
| error_rate_threshold | 0.5 | 0.0-1.0 | 错误率阈值 |

---

## 监控和告警

### 监控指标

#### 负载均衡器级别指标

- **总请求数**：通过负载均衡器的总请求数
- **成功请求数**：成功处理的请求数
- **失败请求数**：失败的请求数
- **平均响应时间**：所有请求的平均响应时间
- **P50/P95/P99响应时间**：响应时间的百分位数
- **错误率**：失败请求占总请求的比例
- **活跃连接数**：当前活跃的连接数

#### 配置节点级别指标

- **健康状态**：healthy, unhealthy, unknown
- **熔断器状态**：closed, open, half_open
- **请求分配数**：分配到该节点的请求数
- **成功率**：该节点的请求成功率
- **平均响应时间**：该节点的平均响应时间
- **当前权重**：动态权重（如果启用）

### 查看监控数据

#### 通过API

```bash
# 获取实时指标
curl http://localhost:8080/api/load-balancers/{lb_id}/stats

# 获取历史统计（1小时）
curl http://localhost:8080/api/load-balancers/{lb_id}/stats?window=1h

# 获取健康状态
curl http://localhost:8080/api/load-balancers/{lb_id}/health

# 获取熔断器状态
curl http://localhost:8080/api/load-balancers/{lb_id}/circuit-breaker
```

#### 通过前端界面

访问负载均衡器详情页面：
```
http://localhost:3000/load-balancers/{lb_id}
```

可以查看：
- 实时监控图表
- 健康状态面板
- 熔断器状态面板
- 请求日志列表
- 告警通知列表

### 告警规则

系统会在以下情况下生成告警：

#### 严重级别（Critical）

- **所有节点不健康**：负载均衡器的所有配置节点都处于unhealthy状态
  - 影响：服务完全不可用
  - 处理：立即检查所有节点，恢复至少一个节点

#### 警告级别（Warning）

- **健康节点数低于阈值**：健康节点数量低于配置的最小阈值
  - 影响：服务可用性降低，容错能力下降
  - 处理：检查不健康的节点，尽快恢复

- **错误率超过阈值**：5分钟内错误率超过20%
  - 影响：服务质量下降
  - 处理：检查日志，分析错误原因

#### 信息级别（Info）

- **熔断器状态变化**：配置节点的熔断器状态变为open
  - 影响：该节点暂时不接收请求
  - 处理：监控节点恢复情况

### 查看告警

```bash
# 获取未确认的告警
curl http://localhost:8080/api/load-balancers/{lb_id}/alerts?acknowledged=false

# 确认告警
curl -X POST http://localhost:8080/api/alerts/{alert_id}/acknowledge
```

---

## 故障排查

### 常见问题

#### 1. 所有节点都不健康

**症状**：
- 告警显示所有节点不健康
- 请求全部失败

**排查步骤**：

1. 检查节点是否真的不可用：
```bash
# 手动测试节点
curl -X POST https://api.anthropic.com/v1/messages \
  -H "x-api-key: YOUR_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-3-sonnet-20240229","messages":[{"role":"user","content":"test"}],"max_tokens":10}'
```

2. 检查健康检查配置：
```bash
# 查看健康检查配置
curl http://localhost:8080/api/load-balancers/{lb_id}
```

3. 检查网络连接：
```bash
# 测试网络连接
ping api.anthropic.com
telnet api.anthropic.com 443
```

4. 查看健康检查日志：
```bash
# 查看日志
tail -f server.log | grep "health check"
```

**解决方案**：
- 如果节点确实不可用，联系API提供商
- 如果是网络问题，检查防火墙和代理设置
- 如果是配置问题，调整健康检查参数

#### 2. 熔断器频繁触发

**症状**：
- 熔断器状态频繁在open和closed之间切换
- 告警显示熔断器状态变化

**排查步骤**：

1. 检查错误率：
```bash
# 查看统计数据
curl http://localhost:8080/api/load-balancers/{lb_id}/stats
```

2. 查看请求日志：
```bash
# 查看失败的请求
curl http://localhost:8080/api/load-balancers/{lb_id}/logs?success=false
```

3. 检查熔断器配置：
```bash
# 查看熔断器配置
curl http://localhost:8080/api/load-balancers/{lb_id}
```

**解决方案**：
- 如果错误率确实很高，检查节点配置和API密钥
- 如果是暂时性问题，可以调整熔断器阈值
- 如果是负载过高，增加更多节点

#### 3. 响应时间过长

**症状**：
- P99响应时间超过10ms
- 用户反馈响应慢

**排查步骤**：

1. 检查性能指标：
```bash
# 查看响应时间统计
curl http://localhost:8080/api/load-balancers/{lb_id}/stats
```

2. 检查数据库性能：
```bash
# 查看数据库大小
ls -lh data/proxy.db

# 检查是否需要清理
```

3. 检查缓存命中率：
```bash
# 查看缓存统计（需要在代码中添加API）
```

**解决方案**：
- 清理过期数据
- 增加缓存大小
- 优化数据库查询
- 增加连接池大小

#### 4. 内存使用过高

**症状**：
- 系统内存使用持续增长
- 可能出现OOM错误

**排查步骤**：

1. 检查内存使用：
```bash
# 查看进程内存使用
ps aux | grep claude-code-cli-with-openai-api

# 使用pprof分析内存
go tool pprof http://localhost:8080/debug/pprof/heap
```

2. 检查缓存大小：
```bash
# 查看缓存配置
```

3. 检查数据库连接：
```bash
# 检查是否有连接泄漏
```

**解决方案**：
- 减小缓存大小
- 增加数据清理频率
- 检查是否有内存泄漏
- 重启服务

---

## 性能调优

### 健康检查优化

```json
{
  "health_check_interval": 30,  // 增加间隔可以减少开销
  "health_check_timeout": 5,    // 减少超时可以更快发现故障
  "failure_threshold": 3,       // 增加阈值可以减少误判
  "recovery_threshold": 2       // 减少阈值可以更快恢复
}
```

### 重试策略优化

```json
{
  "max_retries": 3,             // 根据业务需求调整
  "initial_retry_delay": 100,   // 初始延迟（毫秒）
  "max_retry_delay": 5000       // 最大延迟（毫秒）
}
```

### 熔断器优化

```json
{
  "error_rate_threshold": 0.5,  // 错误率阈值（0.0-1.0）
  "circuit_breaker_window": 60, // 时间窗口（秒）
  "circuit_breaker_timeout": 30,// 熔断超时（秒）
  "half_open_requests": 3       // 半开状态测试请求数
}
```

### 数据库优化

1. **定期清理过期数据**：
```bash
# 清理30天前的请求日志
sqlite3 data/proxy.db "DELETE FROM load_balancer_request_logs WHERE created_at < datetime('now', '-30 days')"

# 清理90天前的统计数据
sqlite3 data/proxy.db "DELETE FROM load_balancer_stats WHERE created_at < datetime('now', '-90 days')"
```

2. **优化数据库**：
```bash
# 压缩数据库
sqlite3 data/proxy.db "VACUUM"

# 分析查询计划
sqlite3 data/proxy.db "EXPLAIN QUERY PLAN SELECT * FROM load_balancer_request_logs WHERE load_balancer_id = 'xxx'"
```

### 缓存优化

- 增加缓存TTL可以减少数据库查询
- 增加缓存大小可以提高命中率
- 定期清理过期缓存

---

## 备份和恢复

### 数据库备份

```bash
# 备份数据库
cp data/proxy.db data/proxy.db.backup.$(date +%Y%m%d_%H%M%S)

# 使用SQLite备份命令
sqlite3 data/proxy.db ".backup data/proxy.db.backup"

# 定期备份（添加到crontab）
0 2 * * * /path/to/backup.sh
```

### 恢复数据库

```bash
# 停止服务
kill -SIGTERM <pid>

# 恢复数据库
cp data/proxy.db.backup data/proxy.db

# 启动服务
./claude-code-cli-with-openai-api server
```

### 配置备份

```bash
# 导出负载均衡器配置
curl http://localhost:8080/api/load-balancers/{lb_id} > lb_config.json

# 导入配置
curl -X PUT http://localhost:8080/api/load-balancers/{lb_id} \
  -H "Content-Type: application/json" \
  -d @lb_config.json
```

---

## 升级和回滚

### 升级流程

1. **备份数据**：
```bash
cp data/proxy.db data/proxy.db.backup
```

2. **停止服务**：
```bash
kill -SIGTERM <pid>
```

3. **更新二进制文件**：
```bash
cp claude-code-cli-with-openai-api claude-code-cli-with-openai-api.old
cp claude-code-cli-with-openai-api.new claude-code-cli-with-openai-api
```

4. **运行数据库迁移**：
```bash
./claude-code-cli-with-openai-api migrate
```

5. **启动服务**：
```bash
./claude-code-cli-with-openai-api server
```

6. **验证服务**：
```bash
curl http://localhost:8080/health
```

### 回滚流程

1. **停止服务**：
```bash
kill -SIGTERM <pid>
```

2. **恢复二进制文件**：
```bash
cp claude-code-cli-with-openai-api.old claude-code-cli-with-openai-api
```

3. **恢复数据库**：
```bash
cp data/proxy.db.backup data/proxy.db
```

4. **启动服务**：
```bash
./claude-code-cli-with-openai-api server
```

### 灰度发布

1. **部署新版本到部分节点**
2. **监控新版本的性能和错误率**
3. **逐步增加新版本的流量**
4. **如果出现问题，立即回滚**

---

## 安全最佳实践

### API密钥管理

- 使用环境变量存储API密钥
- 定期轮换API密钥
- 限制API密钥的权限
- 监控API密钥的使用情况

### 访问控制

- 使用HTTPS加密通信
- 实施身份验证和授权
- 限制管理API的访问
- 记录所有管理操作的审计日志

### 数据保护

- 加密敏感数据
- 定期备份数据
- 限制数据库访问权限
- 清理过期的敏感数据

### 监控和审计

- 监控异常的API调用
- 记录所有配置变更
- 定期审查访问日志
- 设置告警阈值

---

## 附录

### 常用命令速查

```bash
# 查看服务状态
ps aux | grep claude-code-cli-with-openai-api

# 查看日志
tail -f server.log

# 查看实时指标
curl http://localhost:8080/api/load-balancers/{lb_id}/stats

# 查看健康状态
curl http://localhost:8080/api/load-balancers/{lb_id}/health

# 查看告警
curl http://localhost:8080/api/load-balancers/{lb_id}/alerts

# 备份数据库
cp data/proxy.db data/proxy.db.backup

# 清理日志
sqlite3 data/proxy.db "DELETE FROM load_balancer_request_logs WHERE created_at < datetime('now', '-30 days')"
```

### 联系支持

如果遇到无法解决的问题，请联系技术支持：

- GitHub Issues: https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api/issues
- Email: support@example.com

### 相关文档

- [API文档](./LOAD_BALANCER_ENHANCEMENTS.md)
- [用户使用指南](./LOAD_BALANCER_USAGE.md)
- [性能测试报告](./PERFORMANCE_TEST_REPORT.md)
- [测试覆盖率报告](./TEST_COVERAGE_REPORT.md)
