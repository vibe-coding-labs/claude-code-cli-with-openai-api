# 灰度发布测试指南

## 概述

本文档描述负载均衡器增强功能的灰度发布测试流程。灰度发布（Canary Deployment）是一种渐进式部署策略，通过逐步将流量切换到新版本来降低风险。

## 灰度发布策略

### 阶段划分

1. **阶段 0：准备阶段** - 验证环境和配置
2. **阶段 1：10% 流量** - 部署到 10% 的实例
3. **阶段 2：30% 流量** - 扩展到 30% 的实例
4. **阶段 3：50% 流量** - 扩展到 50% 的实例
5. **阶段 4：100% 流量** - 完全切换到新版本

### 每个阶段的验证标准

- 错误率 < 1%
- P99 延迟 < 100ms
- 健康检查通过率 > 99%
- 无严重告警
- 监控指标正常

## 前置条件

### 1. 环境准备

```bash
# 确保有两个环境：生产环境和灰度环境
# 生产环境：运行当前稳定版本
# 灰度环境：运行新版本（负载均衡器增强功能）

# 检查 Docker 镜像
docker images | grep claude-proxy

# 检查 Kubernetes 集群状态
kubectl cluster-info
kubectl get nodes
```

### 2. 监控系统

确保以下监控系统正常运行：

- Prometheus - 指标收集
- Grafana - 可视化监控
- 日志聚合系统（可选）

### 3. 回滚准备

```bash
# 备份当前数据库
sqlite3 data/proxy.db ".backup data/proxy.db.backup-$(date +%Y%m%d-%H%M%S)"

# 记录当前版本
kubectl get deployment claude-proxy -o yaml > deployment-backup.yaml
```

## 灰度发布步骤

### 阶段 0：准备阶段

#### 1. 构建新版本镜像

```bash
# 构建包含负载均衡器增强功能的镜像
docker build -t ${DOCKER_REGISTRY}/claude-proxy:v2.0-canary .

# 推送到镜像仓库
docker push ${DOCKER_REGISTRY}/claude-proxy:v2.0-canary
```

#### 2. 创建灰度部署配置

```bash
# 创建 Canary Deployment
kubectl apply -f k8s/deployment-canary.yaml
```

#### 3. 验证灰度环境

```bash
# 检查 Pod 状态
kubectl get pods -l version=canary

# 检查健康状态
kubectl exec -it <canary-pod> -- wget -O- http://localhost:54988/health

# 检查日志
kubectl logs -f <canary-pod>
```

### 阶段 1：10% 流量（观察期：30 分钟）

#### 1. 调整流量分配

```bash
# 更新 Service 权重，将 10% 流量导向 Canary
kubectl apply -f k8s/service-canary-10.yaml
```

#### 2. 监控关键指标

```bash
# 使用提供的监控脚本
./scripts/monitor-canary.sh 10
```

监控以下指标：

- **请求成功率**：应 > 99%
- **P99 延迟**：应 < 100ms
- **健康检查通过率**：应 > 99%
- **熔断器触发次数**：应为 0 或极少
- **错误日志数量**：应无严重错误

#### 3. 功能验证

```bash
# 测试健康检查功能
curl http://your-domain/api/load-balancers/{id}/health-status

# 测试统计数据
curl http://your-domain/api/load-balancers/{id}/stats?window=1h

# 测试告警功能
curl http://your-domain/api/load-balancers/{id}/alerts

# 测试请求日志
curl http://your-domain/api/load-balancers/{id}/logs?limit=100
```

#### 4. 决策点

**继续条件**：
- 所有监控指标正常
- 无严重错误或告警
- 功能测试全部通过

**回滚条件**：
- 错误率 > 1%
- P99 延迟 > 100ms
- 出现严重告警
- 功能测试失败

### 阶段 2：30% 流量（观察期：1 小时）

#### 1. 扩展流量

```bash
# 将流量增加到 30%
kubectl apply -f k8s/service-canary-30.yaml

# 增加 Canary 副本数
kubectl scale deployment claude-proxy-canary --replicas=3
```

#### 2. 持续监控

```bash
./scripts/monitor-canary.sh 30
```

#### 3. 压力测试

```bash
# 使用 wrk 进行压力测试
wrk -t4 -c100 -d60s --latency http://your-domain/v1/chat/completions \
  -H "Authorization: Bearer test-key" \
  -H "Content-Type: application/json" \
  -s scripts/wrk-test.lua

# 预期结果：
# - 吞吐量 > 1000 req/s
# - P99 延迟 < 100ms
# - 错误率 < 1%
```

#### 4. 数据库性能验证

```bash
# 检查数据库大小
ls -lh data/proxy.db

# 检查慢查询
sqlite3 data/proxy.db "EXPLAIN QUERY PLAN SELECT * FROM health_statuses;"

# 验证索引使用
sqlite3 data/proxy.db ".schema health_statuses"
```

### 阶段 3：50% 流量（观察期：2 小时）

#### 1. 平衡流量

```bash
# 将流量增加到 50%
kubectl apply -f k8s/service-canary-50.yaml

# 调整副本数
kubectl scale deployment claude-proxy-canary --replicas=5
kubectl scale deployment claude-proxy-stable --replicas=5
```

#### 2. 长时间稳定性测试

```bash
# 运行 2 小时的持续负载测试
./scripts/long-running-test.sh 2h
```

#### 3. 验证新功能

##### 健康检查功能

```bash
# 模拟节点故障
# 1. 停止一个配置节点
# 2. 观察健康检查器是否正确标记为 unhealthy
# 3. 验证选择器是否排除该节点
# 4. 恢复节点
# 5. 验证节点是否恢复为 healthy
```

##### 熔断器功能

```bash
# 模拟高错误率
# 1. 配置一个节点返回大量错误
# 2. 观察熔断器是否触发 open 状态
# 3. 验证该节点是否被隔离
# 4. 等待超时后验证 half_open 状态
# 5. 验证恢复到 closed 状态
```

##### 重试功能

```bash
# 测试重试机制
# 1. 模拟临时网络错误
# 2. 验证请求是否自动重试
# 3. 验证重试使用不同节点
# 4. 验证指数退避策略
```

### 阶段 4：100% 流量（观察期：24 小时）

#### 1. 完全切换

```bash
# 将所有流量切换到新版本
kubectl apply -f k8s/service-canary-100.yaml

# 或者直接更新主部署
kubectl set image deployment/claude-proxy \
  claude-proxy=${DOCKER_REGISTRY}/claude-proxy:v2.0-canary

# 等待滚动更新完成
kubectl rollout status deployment/claude-proxy
```

#### 2. 移除旧版本

```bash
# 等待 24 小时观察期后，如果一切正常
kubectl delete deployment claude-proxy-stable
```

#### 3. 更新镜像标签

```bash
# 将 canary 标签更新为 latest
docker tag ${DOCKER_REGISTRY}/claude-proxy:v2.0-canary \
  ${DOCKER_REGISTRY}/claude-proxy:latest

docker push ${DOCKER_REGISTRY}/claude-proxy:latest
```

## 监控脚本

### monitor-canary.sh

```bash
#!/bin/bash
# 监控灰度发布的关键指标

PERCENTAGE=$1
DURATION=${2:-1800}  # 默认监控 30 分钟

echo "开始监控 Canary 部署 (${PERCENTAGE}% 流量)"
echo "监控时长: ${DURATION} 秒"
echo "=========================================="

START_TIME=$(date +%s)
END_TIME=$((START_TIME + DURATION))

while [ $(date +%s) -lt $END_TIME ]; do
    echo ""
    echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "----------------------------------------"
    
    # 检查 Pod 状态
    echo "Pod 状态:"
    kubectl get pods -l app=claude-proxy
    
    # 检查错误率
    echo ""
    echo "错误率:"
    curl -s "http://localhost:9090/api/v1/query?query=rate(http_requests_total{status=~\"5..\"}[5m])" | \
      jq -r '.data.result[] | "\(.metric.pod): \(.value[1])"'
    
    # 检查延迟
    echo ""
    echo "P99 延迟:"
    curl -s "http://localhost:9090/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket[5m]))" | \
      jq -r '.data.result[] | "\(.metric.pod): \(.value[1])s"'
    
    # 检查健康检查
    echo ""
    echo "健康检查通过率:"
    curl -s "http://localhost:9090/api/v1/query?query=rate(health_check_success_total[5m])/rate(health_check_total[5m])" | \
      jq -r '.data.result[] | "\(.metric.pod): \(.value[1])"'
    
    sleep 60
done

echo ""
echo "=========================================="
echo "监控完成"
```

### rollback-canary.sh

```bash
#!/bin/bash
# 快速回滚灰度部署

echo "开始回滚 Canary 部署..."

# 1. 将流量切回稳定版本
kubectl apply -f k8s/service-stable.yaml

# 2. 删除 Canary 部署
kubectl delete deployment claude-proxy-canary

# 3. 恢复数据库（如果需要）
if [ -f "data/proxy.db.backup-latest" ]; then
    echo "恢复数据库备份..."
    cp data/proxy.db.backup-latest data/proxy.db
fi

# 4. 验证回滚
kubectl get pods -l app=claude-proxy
kubectl get svc claude-proxy

echo "回滚完成"
```

## 验证清单

### 功能验证

- [ ] 健康检查功能正常工作
- [ ] 熔断器正确触发和恢复
- [ ] 重试机制按预期工作
- [ ] 故障转移自动进行
- [ ] 动态权重调整生效
- [ ] 监控数据正确收集
- [ ] 告警正确触发
- [ ] 请求日志正确记录

### 性能验证

- [ ] P99 延迟 < 100ms
- [ ] 吞吐量 > 1000 req/s
- [ ] 错误率 < 1%
- [ ] 内存使用稳定
- [ ] CPU 使用正常
- [ ] 数据库查询性能良好

### 稳定性验证

- [ ] 长时间运行无内存泄漏
- [ ] 无 goroutine 泄漏
- [ ] 数据库连接正常
- [ ] 日志无异常错误
- [ ] 无死锁或竞态条件

## 回滚决策矩阵

| 指标 | 阈值 | 严重程度 | 操作 |
|------|------|----------|------|
| 错误率 | > 1% | 高 | 立即回滚 |
| P99 延迟 | > 100ms | 中 | 观察 15 分钟，未改善则回滚 |
| 健康检查失败率 | > 1% | 高 | 立即回滚 |
| 内存泄漏 | 持续增长 | 高 | 立即回滚 |
| 严重错误日志 | > 10/分钟 | 高 | 立即回滚 |
| 数据库错误 | 任何 | 高 | 立即回滚 |

## 成功标准

灰度发布被认为成功，当且仅当：

1. 所有阶段的观察期内指标正常
2. 功能验证清单全部通过
3. 性能验证清单全部通过
4. 稳定性验证清单全部通过
5. 无需回滚
6. 用户反馈正面

## 后续步骤

灰度发布成功后：

1. 更新文档标记新版本为稳定版
2. 归档旧版本镜像
3. 清理临时资源
4. 总结经验教训
5. 更新运维手册

## 联系方式

如遇问题，请联系：

- 技术负责人：[姓名]
- 运维团队：[联系方式]
- 紧急热线：[电话]
