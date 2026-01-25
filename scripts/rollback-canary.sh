#!/bin/bash
# 快速回滚灰度部署

set -e

NAMESPACE=${1:-zhaixingren-prod}
BACKUP_DIR="./backups/$(date +%Y%m%d-%H%M%S)"

echo "=========================================="
echo "开始回滚 Canary 部署"
echo "=========================================="
echo "命名空间: ${NAMESPACE}"
echo "备份目录: ${BACKUP_DIR}"
echo "=========================================="
echo ""

# 创建备份目录
mkdir -p ${BACKUP_DIR}

# 1. 备份当前 Canary 配置
echo "📦 备份 Canary 配置..."
kubectl get deployment claude-proxy-canary -n ${NAMESPACE} -o yaml > ${BACKUP_DIR}/deployment-canary.yaml 2>/dev/null || true
kubectl get ingress claude-proxy-canary-ingress -n ${NAMESPACE} -o yaml > ${BACKUP_DIR}/ingress-canary.yaml 2>/dev/null || true
echo "✅ 配置已备份到 ${BACKUP_DIR}"
echo ""

# 2. 将所有流量切回稳定版本
echo "🔄 将流量切回稳定版本..."
kubectl delete ingress claude-proxy-canary-ingress -n ${NAMESPACE} 2>/dev/null || true
echo "✅ Canary Ingress 已删除"
echo ""

# 3. 缩减 Canary 副本数为 0
echo "📉 缩减 Canary 副本数..."
kubectl scale deployment claude-proxy-canary -n ${NAMESPACE} --replicas=0 2>/dev/null || true
echo "✅ Canary 副本数已设置为 0"
echo ""

# 4. 等待 Pod 终止
echo "⏳ 等待 Canary Pod 终止..."
kubectl wait --for=delete pod -l app=claude-proxy,version=canary -n ${NAMESPACE} --timeout=60s 2>/dev/null || true
echo "✅ Canary Pod 已终止"
echo ""

# 5. 验证稳定版本状态
echo "🔍 验证稳定版本状态..."
STABLE_PODS=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy,version=stable --no-headers 2>/dev/null | wc -l)
STABLE_RUNNING=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy,version=stable --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)

if [ "$STABLE_PODS" -eq "$STABLE_RUNNING" ] && [ "$STABLE_PODS" -gt 0 ]; then
    echo "✅ 稳定版本运行正常: ${STABLE_RUNNING}/${STABLE_PODS} Pods Running"
else
    echo "⚠️  稳定版本状态异常: ${STABLE_RUNNING}/${STABLE_PODS} Pods Running"
fi
echo ""

# 6. 显示当前 Pod 状态
echo "📊 当前 Pod 状态:"
echo "----------------------------------------"
kubectl get pods -n ${NAMESPACE} -l app=claude-proxy -o wide
echo ""

# 7. 显示当前 Service 状态
echo "🌐 当前 Service 状态:"
echo "----------------------------------------"
kubectl get svc -n ${NAMESPACE} -l app=claude-proxy
echo ""

# 8. 检查最近的错误日志
echo "📝 检查稳定版本日志..."
STABLE_POD=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy,version=stable --no-headers 2>/dev/null | head -1 | awk '{print $1}')
if [ -n "$STABLE_POD" ]; then
    echo "最近的日志 (${STABLE_POD}):"
    kubectl logs -n ${NAMESPACE} ${STABLE_POD} --tail=20 2>/dev/null || echo "无法获取日志"
else
    echo "⚠️  未找到稳定版本 Pod"
fi
echo ""

# 9. 可选：恢复数据库备份
echo "=========================================="
echo "数据库恢复选项"
echo "=========================================="
echo "如需恢复数据库备份，请手动执行:"
echo ""
echo "1. 查看可用备份:"
echo "   ls -lh data/proxy.db.backup-*"
echo ""
echo "2. 恢复备份 (替换 TIMESTAMP):"
echo "   cp data/proxy.db.backup-TIMESTAMP data/proxy.db"
echo ""
echo "3. 重启 Pod 以加载新数据库:"
echo "   kubectl rollout restart deployment/claude-proxy-stable -n ${NAMESPACE}"
echo ""

# 10. 清理建议
echo "=========================================="
echo "清理建议"
echo "=========================================="
echo "回滚完成后，可以考虑以下清理操作:"
echo ""
echo "1. 删除 Canary Deployment (保留配置用于分析):"
echo "   kubectl delete deployment claude-proxy-canary -n ${NAMESPACE}"
echo ""
echo "2. 删除 Canary PVC (释放存储空间):"
echo "   kubectl delete pvc claude-proxy-data-canary -n ${NAMESPACE}"
echo ""
echo "3. 删除 Canary Service:"
echo "   kubectl delete svc claude-proxy-canary-service -n ${NAMESPACE}"
echo ""

# 11. 总结
echo "=========================================="
echo "✅ 回滚完成"
echo "=========================================="
echo "备份位置: ${BACKUP_DIR}"
echo "当前状态: 所有流量已切回稳定版本"
echo ""
echo "后续步骤:"
echo "1. 监控稳定版本运行状况"
echo "2. 分析 Canary 失败原因"
echo "3. 修复问题后重新部署"
echo "=========================================="
