# 生产环境部署检查清单

## 概述

本文档提供负载均衡器增强功能生产环境部署的完整检查清单。在执行生产部署前，请确保所有项目都已完成并验证。

## 部署前检查

### 1. 代码和构建

- [ ] 所有代码已合并到主分支
- [ ] 代码审查已完成并批准
- [ ] 所有单元测试通过 (覆盖率 > 80%)
- [ ] 所有集成测试通过
- [ ] 所有属性测试通过
- [ ] 性能测试达标 (P99 < 100ms)
- [ ] 压力测试通过 (> 1000 req/s)
- [ ] Docker 镜像已构建并推送到仓库
- [ ] 镜像标签正确 (v2.0.0)
- [ ] 镜像扫描无严重漏洞

### 2. 数据库

- [ ] 数据库迁移脚本已准备
- [ ] 迁移脚本已在测试环境验证
- [ ] 数据库备份已完成
- [ ] 备份已验证可恢复
- [ ] 索引创建脚本已准备
- [ ] 数据清理任务已配置
- [ ] 数据库连接池配置正确

### 3. 配置

- [ ] 环境变量已配置
- [ ] 配置文件已更新
- [ ] 密钥和证书已配置
- [ ] 健康检查参数已设置
- [ ] 重试策略已配置
- [ ] 熔断器参数已设置
- [ ] 日志级别已配置
- [ ] 监控配置已完成

### 4. 基础设施

- [ ] Kubernetes 集群状态正常
- [ ] 节点资源充足 (CPU, 内存, 磁盘)
- [ ] 网络配置正确
- [ ] 负载均衡器配置正确
- [ ] DNS 记录已配置
- [ ] TLS 证书有效
- [ ] 防火墙规则已配置
- [ ] 存储卷已准备

### 5. 监控和告警

- [ ] Prometheus 正常运行
- [ ] Grafana 仪表板已创建
- [ ] 告警规则已配置
- [ ] 告警通知渠道已测试
- [ ] 日志聚合系统正常
- [ ] 指标收集正常
- [ ] 健康检查端点可访问

### 6. 文档

- [ ] API 文档已更新
- [ ] 用户指南已更新
- [ ] 运维手册已更新
- [ ] 故障排查指南已准备
- [ ] 发布说明已编写
- [ ] 回滚方案已文档化

### 7. 团队准备

- [ ] 部署计划已沟通
- [ ] 团队成员已培训
- [ ] 值班安排已确定
- [ ] 紧急联系方式已更新
- [ ] 回滚权限已确认
- [ ] 监控责任已分配

## 灰度发布检查

### 阶段 0：准备

- [ ] Canary 镜像已构建
- [ ] Canary 配置已准备
- [ ] 监控脚本已测试
- [ ] 回滚脚本已测试
- [ ] 流量分配策略已确定

### 阶段 1：10% 流量

- [ ] Canary Deployment 已部署
- [ ] 10% 流量已配置
- [ ] 监控 30 分钟无异常
- [ ] 错误率 < 1%
- [ ] P99 延迟 < 100ms
- [ ] 健康检查通过率 > 99%
- [ ] 功能验证通过

### 阶段 2：30% 流量

- [ ] 流量增加到 30%
- [ ] 监控 1 小时无异常
- [ ] 压力测试通过
- [ ] 数据库性能正常
- [ ] 无内存泄漏
- [ ] 无严重错误日志

### 阶段 3：50% 流量

- [ ] 流量增加到 50%
- [ ] 监控 2 小时无异常
- [ ] 长时间稳定性测试通过
- [ ] 健康检查功能验证
- [ ] 熔断器功能验证
- [ ] 重试功能验证

### 阶段 4：100% 流量

- [ ] 流量切换到 100%
- [ ] 监控 24 小时无异常
- [ ] 所有功能正常
- [ ] 性能指标达标
- [ ] 用户反馈正面
- [ ] 旧版本已下线

## 部署执行

### 1. 数据库迁移

```bash
# 1. 备份数据库
sqlite3 data/proxy.db ".backup data/proxy.db.backup-$(date +%Y%m%d-%H%M%S)"

# 2. 执行迁移
./scripts/migrate-database.sh

# 3. 验证迁移
sqlite3 data/proxy.db ".schema" | grep -E "health_statuses|circuit_breaker_states"
```

- [ ] 数据库备份完成
- [ ] 迁移脚本执行成功
- [ ] 表结构验证通过
- [ ] 索引创建成功
- [ ] 数据完整性检查通过

### 2. 部署 Canary

```bash
# 1. 应用 Canary Deployment
kubectl apply -f k8s/deployment-canary.yaml

# 2. 等待 Pod 就绪
kubectl wait --for=condition=ready pod -l app=claude-proxy,version=canary -n zhaixingren-prod --timeout=300s

# 3. 验证健康状态
kubectl exec -it <canary-pod> -n zhaixingren-prod -- wget -O- http://localhost:54988/health
```

- [ ] Canary Deployment 创建成功
- [ ] Pod 启动正常
- [ ] 健康检查通过
- [ ] 日志无错误

### 3. 配置流量分配

```bash
# 阶段 1: 10% 流量
kubectl apply -f k8s/service-canary-10.yaml

# 启动监控
./scripts/monitor-canary.sh 10 1800
```

- [ ] Ingress 配置成功
- [ ] 流量分配正确
- [ ] 监控脚本运行正常

### 4. 逐步增加流量

按照灰度发布指南逐步增加流量：

- [ ] 10% → 30% 完成
- [ ] 30% → 50% 完成
- [ ] 50% → 100% 完成

### 5. 清理旧版本

```bash
# 1. 删除 Canary 标签
kubectl label deployment claude-proxy-canary version-

# 2. 更新主 Deployment
kubectl set image deployment/claude-proxy claude-proxy=<new-image>

# 3. 删除旧资源
kubectl delete deployment claude-proxy-stable -n zhaixingren-prod
```

- [ ] 主 Deployment 已更新
- [ ] 旧版本已删除
- [ ] 临时资源已清理

## 部署后验证

### 1. 功能验证

- [ ] 健康检查功能正常
  ```bash
  curl http://your-domain/api/load-balancers/{id}/health-status
  ```

- [ ] 熔断器功能正常
  ```bash
  curl http://your-domain/api/load-balancers/{id}/circuit-breaker-status
  ```

- [ ] 统计数据正常
  ```bash
  curl http://your-domain/api/load-balancers/{id}/stats?window=1h
  ```

- [ ] 告警功能正常
  ```bash
  curl http://your-domain/api/load-balancers/{id}/alerts
  ```

- [ ] 请求日志正常
  ```bash
  curl http://your-domain/api/load-balancers/{id}/logs?limit=100
  ```

### 2. 性能验证

- [ ] P50 延迟 < 50ms
- [ ] P99 延迟 < 100ms
- [ ] 吞吐量 > 1000 req/s
- [ ] 错误率 < 1%
- [ ] CPU 使用正常
- [ ] 内存使用稳定

### 3. 监控验证

- [ ] Prometheus 指标正常收集
- [ ] Grafana 仪表板显示正常
- [ ] 告警规则正常工作
- [ ] 日志正常收集
- [ ] 健康检查端点响应正常

### 4. 端到端测试

- [ ] 创建负载均衡器
- [ ] 添加配置节点
- [ ] 发送测试请求
- [ ] 模拟节点故障
- [ ] 验证故障转移
- [ ] 验证节点恢复
- [ ] 查看监控数据
- [ ] 查看告警记录

## 回滚准备

### 回滚触发条件

立即回滚如果：
- [ ] 错误率 > 1%
- [ ] P99 延迟 > 100ms
- [ ] 健康检查失败率 > 1%
- [ ] 出现数据丢失
- [ ] 出现严重安全问题
- [ ] 出现级联故障

### 回滚步骤

```bash
# 1. 执行回滚脚本
./scripts/rollback-canary.sh

# 2. 验证回滚
kubectl get pods -n zhaixingren-prod -l app=claude-proxy

# 3. 恢复数据库（如需要）
cp data/proxy.db.backup-latest data/proxy.db
kubectl rollout restart deployment/claude-proxy -n zhaixingren-prod

# 4. 验证服务
curl http://your-domain/health
```

- [ ] 回滚脚本已准备
- [ ] 回滚权限已确认
- [ ] 回滚流程已演练
- [ ] 数据恢复方案已准备

## 监控和观察

### 第一周监控重点

- [ ] 每天检查错误率
- [ ] 每天检查性能指标
- [ ] 每天检查告警记录
- [ ] 每天检查资源使用
- [ ] 每天检查日志异常
- [ ] 收集用户反馈

### 持续监控

- [ ] 设置每日监控报告
- [ ] 配置异常自动告警
- [ ] 定期审查性能趋势
- [ ] 定期检查数据库大小
- [ ] 定期清理历史数据

## 文档更新

- [ ] 更新 README.md
- [ ] 更新 API 文档
- [ ] 更新部署指南
- [ ] 更新运维手册
- [ ] 更新故障排查指南
- [ ] 创建发布公告
- [ ] 更新版本号

## 团队沟通

- [ ] 向团队通报部署完成
- [ ] 分享部署经验
- [ ] 更新知识库
- [ ] 安排培训会议
- [ ] 收集反馈意见

## 后续优化

- [ ] 分析性能数据
- [ ] 识别优化机会
- [ ] 规划下一步改进
- [ ] 更新技术债务清单
- [ ] 安排代码重构

## 签字确认

### 部署前确认

- **开发负责人**: _____________ 日期: _______
  - 确认代码质量和测试覆盖
  
- **测试负责人**: _____________ 日期: _______
  - 确认所有测试通过
  
- **运维负责人**: _____________ 日期: _______
  - 确认基础设施就绪
  
- **技术负责人**: _____________ 日期: _______
  - 批准生产部署

### 部署后确认

- **部署执行人**: _____________ 日期: _______
  - 确认部署成功完成
  
- **验证负责人**: _____________ 日期: _______
  - 确认功能和性能验证通过
  
- **监控负责人**: _____________ 日期: _______
  - 确认监控系统正常
  
- **项目负责人**: _____________ 日期: _______
  - 确认项目交付完成

## 附录

### 关键命令参考

```bash
# 查看 Pod 状态
kubectl get pods -n zhaixingren-prod -l app=claude-proxy

# 查看 Pod 日志
kubectl logs -f <pod-name> -n zhaixingren-prod

# 查看 Deployment 状态
kubectl get deployment -n zhaixingren-prod

# 查看 Service 状态
kubectl get svc -n zhaixingren-prod

# 查看 Ingress 状态
kubectl get ingress -n zhaixingren-prod

# 执行数据库查询
kubectl exec -it <pod-name> -n zhaixingren-prod -- sqlite3 /app/data/proxy.db

# 查看资源使用
kubectl top pods -n zhaixingren-prod -l app=claude-proxy

# 重启 Deployment
kubectl rollout restart deployment/claude-proxy -n zhaixingren-prod

# 查看滚动更新状态
kubectl rollout status deployment/claude-proxy -n zhaixingren-prod

# 回滚到上一个版本
kubectl rollout undo deployment/claude-proxy -n zhaixingren-prod
```

### 紧急联系方式

- **技术负责人**: [姓名] - [电话] - [邮箱]
- **运维团队**: [电话] - [邮箱]
- **值班工程师**: [电话] - [邮箱]
- **紧急热线**: [电话]

### 相关文档

- [灰度发布指南](./CANARY_DEPLOYMENT_GUIDE.md)
- [部署指南](../DEPLOYMENT.md)
- [运维手册](./OPERATIONS_MANUAL.md)
- [故障排查指南](./troubleshooting-empty-choices.md)
- [API 文档](./CLAUDE_API_REFERENCE.md)

---

**注意**: 本检查清单应在每次生产部署前完整执行。任何未完成的项目都应在部署前解决或获得明确的豁免批准。
