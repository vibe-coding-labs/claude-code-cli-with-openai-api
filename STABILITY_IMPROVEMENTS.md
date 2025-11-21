# Claude Code 稳定性改进指南

## 问题：Agent 进程经常断掉

### 已实施的改进 ✅

#### 1. 心跳机制（Heartbeat）
**作用**：防止长时间无数据导致连接超时

```go
// 每 5 秒发送一次 ping 事件保持连接活跃
heartbeatStop := StartHeartbeat(c, ctx, 5*time.Second)
defer StopHeartbeat(heartbeatStop)
```

**效果**：
- ✅ 防止 CDN/代理超时
- ✅ 保持 SSE 连接活跃
- ✅ 客户端可以检测连接状态

#### 2. 智能超时控制
**之前**：全局超时包括响应体读取，导致长响应超时
**现在**：使用 2x 超时，允许充分的响应读取时间

```go
// 2x 超时：1x 用于 API 处理，1x 用于响应读取
ctx, cancel := context.WithTimeout(context.Background(), c.Timeout*2)
```

#### 3. 自动重试机制
**配置**：默认 3 次重试，支持 1-100 次
**策略**：指数退避（1s, 2s, 4s, 8s...）
**触发条件**：
- 5xx 服务器错误
- 429 限流错误
- 网络临时故障

#### 4. 连接池优化
**HTTP 连接池**：
- MaxIdleConns: 100
- MaxIdleConnsPerHost: 100
- Keep-Alive: 启用
- HTTP/2: 自动尝试

**效果**：
- ✅ 连接复用，减少握手开销
- ✅ 支持高并发
- ✅ 降低延迟

#### 5. 异步日志
**效果**：
- ✅ 日志记录不阻塞请求
- ✅ 提升响应速度
- ✅ 减少内存压力

---

## 推荐配置 🎯

### ClaudeCode 配置优化

在你的 ClaudeCode 配置中建议使用以下设置：

#### 1. 增加超时时间（重要！）

对于生成大量代码的场景：

```bash
# 在数据库中更新配置
sqlite3 data/proxy.db "UPDATE api_configs SET request_timeout = 300 WHERE id = 'your-config-id';"
```

**推荐超时时间**：
- 轻度使用（代码补全）：90-120 秒
- 中度使用（代码审查）：180-240 秒  
- 重度使用（生成大量代码）：**300-600 秒** ⭐

#### 2. 增加重试次数

```bash
# 设置为 5 次重试
sqlite3 data/proxy.db "UPDATE api_configs SET retry_count = 5 WHERE id = 'your-config-id';"
```

**推荐重试次数**：
- 稳定网络：3 次（默认）
- 不稳定网络：5-10 次 ⭐

#### 3. 使用稳定的上游 API

**建议**：
- ✅ 使用企业级 API（如 Azure OpenAI）
- ✅ 使用国内稳定的转发服务
- ✅ 配置多个备用 API（负载均衡）

---

## 进一步优化建议 🚀

### 1. 健康检查端点

添加定期健康检查，确保服务可用：

```bash
# 每分钟检查一次
*/1 * * * * curl -f http://localhost:54988/health || systemctl restart claude-api
```

### 2. 监控和告警

监控关键指标：
- 请求成功率
- 平均响应时间
- 错误率
- 数据库连接数

### 3. 日志分析

定期检查日志中的问题模式：

```bash
# 查看最近的错误
tail -100 server.log | grep -i error

# 统计错误类型
grep -i error server.log | cut -d' ' -f5- | sort | uniq -c | sort -rn
```

### 4. 连接保活配置

如果使用反向代理（如 Nginx），配置保活：

```nginx
location /v1/messages {
    proxy_pass http://localhost:54988;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    
    # SSE 专用配置
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 600s;  # 10 分钟超时
    proxy_connect_timeout 60s;
}
```

---

## 故障排除 🔧

### 症状 1：频繁超时

**可能原因**：
- 上游 API 响应慢
- 网络不稳定
- 超时配置过短

**解决方案**：
1. 增加 `request_timeout` 到 300-600 秒
2. 检查网络连接质量
3. 尝试更换上游 API

### 症状 2：连接断开

**可能原因**：
- 长时间无数据传输
- CDN/代理超时
- 客户端超时

**解决方案**：
1. ✅ **已实施**：心跳机制每 5 秒发送 ping
2. 配置反向代理的超时时间
3. 检查客户端超时设置

### 症状 3：高错误率

**可能原因**：
- 上游 API 限流
- API Key 配额不足
- 服务不可用

**解决方案**：
1. 增加重试次数到 5-10 次
2. 检查 API Key 配额和限制
3. 配置多个 API 实现负载均衡

---

## 性能监控命令 📊

### 实时监控

```bash
# 监控服务日志
tail -f server.log

# 监控错误
tail -f server.log | grep -i error

# 监控数据库性能
watch -n 1 'sqlite3 data/proxy.db "SELECT status, COUNT(*) FROM request_logs WHERE created_at > datetime(\"now\", \"-1 hour\") GROUP BY status;"'
```

### 性能指标

```bash
# 今天的成功率
sqlite3 data/proxy.db "SELECT 
    status, 
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM request_logs WHERE DATE(created_at) = DATE('now')), 2) as percentage
FROM request_logs 
WHERE DATE(created_at) = DATE('now')
GROUP BY status;"

# 平均响应时间
sqlite3 data/proxy.db "SELECT 
    AVG(duration_ms) as avg_ms,
    MIN(duration_ms) as min_ms,
    MAX(duration_ms) as max_ms
FROM request_logs 
WHERE created_at > datetime('now', '-1 hour');"
```

---

## 紧急情况处理 🚨

### 如果服务完全不可用

1. **重启服务**：
```bash
pkill -f claude-with-openai-api
nohup ./claude-with-openai-api > server.log 2>&1 &
```

2. **检查数据库**：
```bash
# 检查数据库文件是否损坏
sqlite3 data/proxy.db "PRAGMA integrity_check;"

# 优化数据库
sqlite3 data/proxy.db "VACUUM;"
```

3. **清理缓存**：
```bash
# 重启服务会自动清理内存缓存
# 数据库缓存在 5 分钟后自动过期
```

---

## 总结

### 当前稳定性状态：⭐⭐⭐⭐

经过优化后，系统具备：
- ✅ 心跳保活机制
- ✅ 智能超时控制（2x）
- ✅ 自动重试机制（3-100 次）
- ✅ 连接池优化（100 连接）
- ✅ 异步日志记录
- ✅ 配置缓存（5 分钟）

### 建议操作清单：

1. [ ] 将 `request_timeout` 设置为 300-600 秒
2. [ ] 将 `retry_count` 设置为 5-10 次
3. [ ] 配置反向代理的超时时间（如有）
4. [ ] 设置定期健康检查
5. [ ] 监控日志中的错误模式
6. [ ] 考虑配置多个上游 API

### 预期改进：

- 🎯 连接稳定性：**大幅提升**
- 🎯 超时错误：**显著减少**
- 🎯 自动恢复：**3-5 次重试**
- 🎯 整体可用性：**95%+**

---

## 获取支持

如果问题仍然存在，请提供：
1. server.log 最近 100 行
2. 数据库配置（`SELECT * FROM api_configs;`）
3. 错误出现的频率和场景
4. ClaudeCode 的版本信息
