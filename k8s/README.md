# Kubernetes 部署指南

本目录包含 Claude Code CLI with OpenAI API 项目的 Kubernetes 部署配置。

## 文件说明

### 核心配置

- `deployment-enhanced.yaml` - 增强版部署配置（推荐）
  - 包含健康检查探针
  - 资源限制和请求
  - 持久化存储
  - 安全上下文
  
- `deployment.yaml` - 基础部署配置（生产环境）
  - 简化配置
  - 适用于快速部署

- `configmap.yaml` - 配置映射
  - 应用程序配置
  - 负载均衡器配置
  - 性能参数

- `hpa.yaml` - 水平Pod自动扩缩容
  - 基于CPU和内存的自动扩缩容
  - 最小2个副本，最大10个副本

- `servicemonitor.yaml` - Prometheus 监控配置
  - 自动服务发现
  - 指标采集配置

- `ingress.yaml` - Ingress 配置
  - 外部访问配置
  - TLS 证书配置

## 部署步骤

### 1. 创建命名空间（如果不存在）

```bash
kubectl create namespace zhaixingren-prod
```

### 2. 创建 ConfigMap

```bash
kubectl apply -f k8s/configmap.yaml
```

### 3. 创建 Secret（可选）

如果需要存储敏感信息（如 API 密钥），创建 Secret：

```bash
kubectl create secret generic claude-code-cli-secrets \
  --from-literal=ENCRYPTION_KEY=your-encryption-key \
  --namespace=zhaixingren-prod
```

### 4. 部署应用

#### 使用增强版配置（推荐）

```bash
kubectl apply -f k8s/deployment-enhanced.yaml
```

#### 使用基础配置

```bash
kubectl apply -f k8s/deployment.yaml
```

### 5. 配置自动扩缩容（可选）

```bash
kubectl apply -f k8s/hpa.yaml
```

### 6. 配置 Ingress（可选）

编辑 `ingress.yaml` 文件，设置您的域名和 TLS 证书，然后应用：

```bash
kubectl apply -f k8s/ingress.yaml
```

### 7. 配置监控（可选）

如果您使用 Prometheus Operator：

```bash
kubectl apply -f k8s/servicemonitor.yaml
```

## 验证部署

### 检查 Pod 状态

```bash
kubectl get pods -n zhaixingren-prod -l app=claude-code-cli-openai-api
```

### 检查服务状态

```bash
kubectl get svc -n zhaixingren-prod -l app=claude-code-cli-openai-api
```

### 查看 Pod 日志

```bash
kubectl logs -n zhaixingren-prod -l app=claude-code-cli-openai-api --tail=100 -f
```

### 检查健康状态

```bash
# 端口转发
kubectl port-forward -n zhaixingren-prod svc/claude-code-cli-openai-api-service 8080:80

# 在另一个终端测试
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## 配置说明

### 资源配置

默认资源配置：

- **Requests**: 128Mi 内存, 100m CPU
- **Limits**: 512Mi 内存, 500m CPU

根据实际负载调整这些值。

### 健康检查

- **Liveness Probe**: 检查应用是否存活
  - 路径: `/health`
  - 初始延迟: 30秒
  - 检查间隔: 10秒
  
- **Readiness Probe**: 检查应用是否就绪
  - 路径: `/ready`
  - 初始延迟: 10秒
  - 检查间隔: 5秒

- **Startup Probe**: 检查应用是否启动完成
  - 路径: `/health`
  - 最大等待时间: 60秒 (12次 × 5秒)

### 自动扩缩容

HPA 配置：

- **最小副本数**: 2
- **最大副本数**: 10
- **扩容条件**: CPU > 70% 或 内存 > 80%
- **缩容策略**: 稳定窗口 5分钟，最多缩减 50%
- **扩容策略**: 立即扩容，最多增加 100%

### 持久化存储

应用使用 PersistentVolumeClaim 存储数据：

- **存储类**: standard
- **访问模式**: ReadWriteOnce
- **容量**: 10Gi

根据需要调整存储大小。

## 更新部署

### 滚动更新

```bash
# 更新镜像
kubectl set image deployment/claude-code-cli-openai-api \
  claude-code-cli-openai-api=docker.zhaixingren.cn/aigchub/claude-code-cli-openai-api:v2.0 \
  -n zhaixingren-prod

# 查看更新状态
kubectl rollout status deployment/claude-code-cli-openai-api -n zhaixingren-prod
```

### 回滚部署

```bash
# 查看历史版本
kubectl rollout history deployment/claude-code-cli-openai-api -n zhaixingren-prod

# 回滚到上一个版本
kubectl rollout undo deployment/claude-code-cli-openai-api -n zhaixingren-prod

# 回滚到指定版本
kubectl rollout undo deployment/claude-code-cli-openai-api --to-revision=2 -n zhaixingren-prod
```

## 故障排查

### Pod 无法启动

```bash
# 查看 Pod 详情
kubectl describe pod <pod-name> -n zhaixingren-prod

# 查看事件
kubectl get events -n zhaixingren-prod --sort-by='.lastTimestamp'
```

### 健康检查失败

```bash
# 进入 Pod
kubectl exec -it <pod-name> -n zhaixingren-prod -- /bin/sh

# 手动测试健康检查端点
curl http://localhost:54988/health
```

### 存储问题

```bash
# 查看 PVC 状态
kubectl get pvc -n zhaixingren-prod

# 查看 PV 状态
kubectl get pv
```

### 查看资源使用

```bash
# 查看 Pod 资源使用
kubectl top pods -n zhaixingren-prod -l app=claude-code-cli-openai-api

# 查看节点资源使用
kubectl top nodes
```

## 监控和告警

### Prometheus 指标

应用暴露以下 Prometheus 指标：

- `http_requests_total` - 总请求数
- `http_request_duration_seconds` - 请求延迟
- `lb_health_status` - 负载均衡器健康状态
- `lb_circuit_breaker_state` - 熔断器状态
- `lb_request_retries_total` - 重试次数

### Grafana 仪表板

导入预配置的 Grafana 仪表板：

```bash
# 仪表板 JSON 文件位于 docs/grafana-dashboard.json
```

## 安全最佳实践

1. **使用非 root 用户运行**
   - 配置中已设置 `runAsUser: 1000`

2. **只读根文件系统**
   - 考虑启用 `readOnlyRootFilesystem: true`

3. **网络策略**
   - 创建 NetworkPolicy 限制 Pod 间通信

4. **RBAC**
   - 为应用创建专用的 ServiceAccount
   - 授予最小权限

5. **Secret 管理**
   - 使用 Kubernetes Secrets 或外部 Secret 管理工具
   - 启用 Secret 加密

## 性能优化

1. **资源限制**
   - 根据实际负载调整 CPU 和内存限制
   - 使用 VPA (Vertical Pod Autoscaler) 自动调整

2. **副本数**
   - 生产环境至少 2 个副本
   - 使用 HPA 自动扩缩容

3. **亲和性和反亲和性**
   - 配置 Pod 反亲和性，分散到不同节点
   - 配置节点亲和性，选择合适的节点

4. **持久化存储**
   - 使用高性能存储类（如 SSD）
   - 考虑使用本地存储提高性能

## 参考文档

- [Kubernetes 官方文档](https://kubernetes.io/docs/)
- [负载均衡器增强文档](../docs/LOAD_BALANCER_ENHANCEMENTS.md)
- [运维手册](../docs/OPERATIONS_MANUAL.md)
- [Docker 部署指南](../docs/DOCKER_DEPLOYMENT.md)
