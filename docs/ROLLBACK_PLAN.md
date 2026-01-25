# 回滚方案

## 概述

本文档描述了负载均衡器增强功能的回滚策略和步骤。当新版本出现问题时，可以按照本文档快速回滚到稳定版本。

**版本信息**:
- **当前版本**: v2.0 (负载均衡器增强版)
- **上一稳定版本**: v1.0 (基础版本)

## 回滚触发条件

在以下情况下应考虑回滚：

### 严重问题（立即回滚）

1. **服务不可用**
   - 所有请求失败率 > 50%
   - 响应时间 > 10秒
   - 服务无法启动

2. **数据丢失或损坏**
   - 数据库损坏
   - 配置丢失
   - 统计数据异常

3. **安全问题**
   - 发现严重安全漏洞
   - 未授权访问
   - 数据泄露

### 一般问题（评估后回滚）

1. **性能下降**
   - P99 延迟 > 100ms（基线 10ms）
   - 吞吐量下降 > 30%
   - 内存使用增加 > 50%

2. **功能异常**
   - 健康检查误报
   - 熔断器误触发
   - 重试逻辑错误

3. **兼容性问题**
   - 与现有配置不兼容
   - API 变更导致客户端失败

## 回滚策略

### 1. 数据库回滚

#### 1.1 检查数据库版本

```bash
# 连接到数据库
sqlite3 data/proxy.db

# 查看迁移版本
SELECT * FROM schema_migrations ORDER BY version DESC LIMIT 5;
```

#### 1.2 回滚数据库迁移

```bash
# 备份当前数据库
cp data/proxy.db data/proxy.db.backup.$(date +%Y%m%d_%H%M%S)

# 回滚到 v1.0 版本（移除增强功能的表和字段）
sqlite3 data/proxy.db <<EOF
-- 删除新增的表
DROP TABLE IF EXISTS health_statuses;
DROP TABLE IF EXISTS circuit_breaker_states;
DROP TABLE IF EXISTS load_balancer_request_logs;
DROP TABLE IF EXISTS load_balancer_stats;
DROP TABLE IF EXISTS node_stats;
DROP TABLE IF EXISTS alerts;

-- 移除新增的字段（SQLite 不支持 DROP COLUMN，需要重建表）
-- 如果需要，可以保留这些字段，它们不会影响 v1.0 的运行

-- 更新迁移版本
DELETE FROM schema_migrations WHERE version >= '006';

EOF
```

#### 1.3 验证数据库

```bash
# 检查表结构
sqlite3 data/proxy.db ".schema"

# 检查数据完整性
sqlite3 data/proxy.db "PRAGMA integrity_check;"
```

### 2. 应用回滚

#### 2.1 Docker 部署回滚

```bash
# 停止当前容器
docker-compose down

# 切换到 v1.0 镜像
docker pull docker.zhaixingren.cn/aigchub/claude-code-cli-openai-api:v1.0

# 更新 docker-compose.yml
sed -i 's/:latest/:v1.0/g' docker-compose.yml

# 启动 v1.0 版本
docker-compose up -d

# 验证服务
curl http://localhost:54988/health
```

#### 2.2 Kubernetes 部署回滚

```bash
# 方法 1: 使用 kubectl rollout undo
kubectl rollout undo deployment/claude-code-cli-openai-api -n zhaixingren-prod

# 方法 2: 回滚到指定版本
kubectl rollout history deployment/claude-code-cli-openai-api -n zhaixingren-prod
kubectl rollout undo deployment/claude-code-cli-openai-api --to-revision=<revision-number> -n zhaixingren-prod

# 方法 3: 更新镜像标签
kubectl set image deployment/claude-code-cli-openai-api \
  claude-code-cli-openai-api=docker.zhaixingren.cn/aigchub/claude-code-cli-openai-api:v1.0 \
  -n zhaixingren-prod

# 监控回滚进度
kubectl rollout status deployment/claude-code-cli-openai-api -n zhaixingren-prod

# 验证 Pod 状态
kubectl get pods -n zhaixingren-prod -l app=claude-code-cli-openai-api
```

#### 2.3 二进制部署回滚

```bash
# 停止当前服务
systemctl stop claude-code-cli

# 恢复 v1.0 二进制文件
cp /opt/claude-code-cli/bin/claude-code-cli.v1.0 /opt/claude-code-cli/bin/claude-code-cli

# 启动服务
systemctl start claude-code-cli

# 检查状态
systemctl status claude-code-cli
```

### 3. 配置回滚

#### 3.1 恢复配置文件

```bash
# 恢复环境变量配置
cp .env.v1.0 .env

# 恢复 Kubernetes ConfigMap
kubectl apply -f k8s/configmap.v1.0.yaml

# 重启 Pod 以应用新配置
kubectl rollout restart deployment/claude-code-cli-openai-api -n zhaixingren-prod
```

#### 3.2 恢复负载均衡器配置

```bash
# 通过 API 禁用增强功能
curl -X PUT http://localhost:54988/api/load-balancers/{id} \
  -H "Content-Type: application/json" \
  -d '{
    "health_check_enabled": false,
    "circuit_breaker_enabled": false,
    "max_retries": 0
  }'
```

### 4. 前端回滚

#### 4.1 回滚前端代码

```bash
# 切换到 v1.0 分支
cd frontend
git checkout v1.0

# 重新构建
npm run build

# 部署
cp -r build/* /var/www/html/
```

#### 4.2 清除浏览器缓存

通知用户清除浏览器缓存或使用硬刷新（Ctrl+F5）。

## 回滚验证

### 1. 功能验证

```bash
# 健康检查
curl http://localhost:54988/health

# API 测试
curl http://localhost:54988/api/configs

# 负载均衡器测试
curl http://localhost:54988/api/load-balancers
```

### 2. 性能验证

```bash
# 运行性能测试
go test -bench=. -benchtime=10s ./handler

# 检查响应时间
ab -n 1000 -c 10 http://localhost:54988/api/configs
```

### 3. 数据验证

```bash
# 检查配置数量
sqlite3 data/proxy.db "SELECT COUNT(*) FROM api_configs;"

# 检查负载均衡器数量
sqlite3 data/proxy.db "SELECT COUNT(*) FROM load_balancers;"

# 检查数据完整性
sqlite3 data/proxy.db "PRAGMA integrity_check;"
```

### 4. 日志检查

```bash
# 检查错误日志
tail -f logs/app.log | grep ERROR

# Kubernetes 日志
kubectl logs -n zhaixingren-prod -l app=claude-code-cli-openai-api --tail=100
```

## 回滚后清理

### 1. 清理增强功能数据

```bash
# 如果确定不再需要增强功能的数据，可以清理
sqlite3 data/proxy.db <<EOF
DROP TABLE IF EXISTS health_statuses;
DROP TABLE IF EXISTS circuit_breaker_states;
DROP TABLE IF EXISTS load_balancer_request_logs;
DROP TABLE IF EXISTS load_balancer_stats;
DROP TABLE IF EXISTS node_stats;
DROP TABLE IF EXISTS alerts;
EOF
```

### 2. 清理 Kubernetes 资源

```bash
# 删除增强功能相关的资源
kubectl delete -f k8s/hpa.yaml
kubectl delete -f k8s/servicemonitor.yaml
kubectl delete configmap claude-code-cli-config -n zhaixingren-prod
```

### 3. 清理监控配置

```bash
# 删除 Prometheus 告警规则
kubectl delete prometheusrule claude-code-cli-alerts -n monitoring

# 删除 Grafana 仪表板
# 手动从 Grafana UI 删除
```

## 部分回滚

如果只有某些功能有问题，可以进行部分回滚：

### 禁用健康检查

```bash
# 通过 API 禁用
curl -X PUT http://localhost:54988/api/load-balancers/{id} \
  -H "Content-Type: application/json" \
  -d '{"health_check_enabled": false}'

# 或通过环境变量
export LB_HEALTH_CHECK_ENABLED=false
systemctl restart claude-code-cli
```

### 禁用熔断器

```bash
curl -X PUT http://localhost:54988/api/load-balancers/{id} \
  -H "Content-Type: application/json" \
  -d '{"circuit_breaker_enabled": false}'
```

### 禁用重试

```bash
curl -X PUT http://localhost:54988/api/load-balancers/{id} \
  -H "Content-Type: application/json" \
  -d '{"max_retries": 0}'
```

## 回滚时间线

| 步骤 | 预计时间 | 负责人 |
|------|---------|--------|
| 1. 决策回滚 | 5 分钟 | 技术负责人 |
| 2. 通知团队 | 2 分钟 | 运维负责人 |
| 3. 备份数据 | 5 分钟 | 运维工程师 |
| 4. 回滚应用 | 10 分钟 | 运维工程师 |
| 5. 回滚数据库 | 15 分钟 | 数据库管理员 |
| 6. 验证功能 | 10 分钟 | 测试工程师 |
| 7. 监控观察 | 30 分钟 | 全体 |
| **总计** | **~77 分钟** | |

## 回滚检查清单

### 回滚前

- [ ] 确认回滚原因和影响范围
- [ ] 通知相关团队成员
- [ ] 备份当前数据库
- [ ] 备份当前配置文件
- [ ] 记录当前版本号和镜像标签
- [ ] 准备回滚脚本和命令

### 回滚中

- [ ] 停止当前服务
- [ ] 回滚数据库（如需要）
- [ ] 回滚应用代码/镜像
- [ ] 回滚配置文件
- [ ] 启动服务
- [ ] 检查服务状态

### 回滚后

- [ ] 验证核心功能
- [ ] 验证 API 端点
- [ ] 检查性能指标
- [ ] 检查错误日志
- [ ] 监控系统稳定性（至少 30 分钟）
- [ ] 通知用户（如需要）
- [ ] 记录回滚原因和过程
- [ ] 制定修复计划

## 预防措施

为了减少回滚的需要，建议：

1. **充分测试**
   - 在测试环境完整测试所有功能
   - 进行压力测试和性能测试
   - 模拟故障场景

2. **灰度发布**
   - 先在小范围用户中测试
   - 逐步扩大发布范围
   - 监控关键指标

3. **功能开关**
   - 使用配置开关控制新功能
   - 可以快速禁用有问题的功能
   - 无需重新部署

4. **监控告警**
   - 设置完善的监控指标
   - 配置告警规则
   - 及时发现问题

5. **自动化回滚**
   - 配置自动回滚条件
   - 当关键指标异常时自动回滚
   - 减少人工干预时间

## 联系方式

### 紧急联系人

- **技术负责人**: [姓名] - [电话] - [邮箱]
- **运维负责人**: [姓名] - [电话] - [邮箱]
- **数据库管理员**: [姓名] - [电话] - [邮箱]

### 支持渠道

- **Slack**: #claude-code-cli-support
- **邮件**: support@example.com
- **电话**: +86-xxx-xxxx-xxxx

## 参考文档

- [部署指南](./DOCKER_DEPLOYMENT.md)
- [Kubernetes 部署](../k8s/README.md)
- [数据库迁移指南](./DATABASE_MIGRATION_GUIDE.md)
- [运维手册](./OPERATIONS_MANUAL.md)
- [故障排查指南](./TROUBLESHOOTING.md)

## 版本历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|---------|
| 1.0 | 2026-01-24 | AI Assistant | 初始版本 |

## 附录

### A. 回滚脚本

```bash
#!/bin/bash
# rollback.sh - 自动回滚脚本

set -e

VERSION=${1:-v1.0}
NAMESPACE=${2:-zhaixingren-prod}

echo "开始回滚到版本 $VERSION..."

# 1. 备份当前数据库
echo "备份数据库..."
cp data/proxy.db data/proxy.db.backup.$(date +%Y%m%d_%H%M%S)

# 2. 回滚 Kubernetes 部署
echo "回滚 Kubernetes 部署..."
kubectl set image deployment/claude-code-cli-openai-api \
  claude-code-cli-openai-api=docker.zhaixingren.cn/aigchub/claude-code-cli-openai-api:$VERSION \
  -n $NAMESPACE

# 3. 等待回滚完成
echo "等待回滚完成..."
kubectl rollout status deployment/claude-code-cli-openai-api -n $NAMESPACE

# 4. 验证服务
echo "验证服务..."
kubectl get pods -n $NAMESPACE -l app=claude-code-cli-openai-api

echo "回滚完成！"
```

### B. 验证脚本

```bash
#!/bin/bash
# verify.sh - 验证脚本

set -e

ENDPOINT=${1:-http://localhost:54988}

echo "验证服务 $ENDPOINT..."

# 健康检查
echo "1. 健康检查..."
curl -f $ENDPOINT/health || exit 1

# API 测试
echo "2. API 测试..."
curl -f $ENDPOINT/api/configs || exit 1

# 性能测试
echo "3. 性能测试..."
ab -n 100 -c 10 $ENDPOINT/api/configs > /dev/null 2>&1 || exit 1

echo "验证通过！"
```
