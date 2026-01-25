#!/bin/bash
# 生产环境部署验证脚本

set -e

NAMESPACE=${1:-zhaixingren-prod}
DOMAIN=${2:-your-domain.example.com}
API_KEY=${3:-test-api-key}

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 测试结果统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 打印标题
print_header() {
    echo ""
    echo "=========================================="
    echo "$1"
    echo "=========================================="
}

# 打印测试结果
print_test() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    if [ $2 -eq 0 ]; then
        echo -e "${GREEN}✅ PASS${NC}: $1"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}❌ FAIL${NC}: $1"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

# 打印信息
print_info() {
    echo -e "${BLUE}ℹ️  INFO${NC}: $1"
}

# 打印警告
print_warning() {
    echo -e "${YELLOW}⚠️  WARN${NC}: $1"
}

print_header "生产环境部署验证"
echo "命名空间: ${NAMESPACE}"
echo "域名: ${DOMAIN}"
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# ==========================================
# 1. Kubernetes 资源验证
# ==========================================
print_header "1. Kubernetes 资源验证"

# 1.1 检查 Deployment
print_info "检查 Deployment..."
DEPLOYMENT_COUNT=$(kubectl get deployment -n ${NAMESPACE} -l app=claude-proxy --no-headers 2>/dev/null | wc -l)
if [ "$DEPLOYMENT_COUNT" -gt 0 ]; then
    print_test "Deployment 存在" 0
    
    # 检查副本数
    DESIRED=$(kubectl get deployment -n ${NAMESPACE} -l app=claude-proxy -o jsonpath='{.items[0].spec.replicas}' 2>/dev/null)
    READY=$(kubectl get deployment -n ${NAMESPACE} -l app=claude-proxy -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null)
    
    if [ "$DESIRED" = "$READY" ]; then
        print_test "所有副本就绪 (${READY}/${DESIRED})" 0
    else
        print_test "副本未全部就绪 (${READY}/${DESIRED})" 1
    fi
else
    print_test "Deployment 存在" 1
fi

# 1.2 检查 Pod
print_info "检查 Pod 状态..."
POD_COUNT=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy --no-headers 2>/dev/null | wc -l)
RUNNING_COUNT=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)

if [ "$POD_COUNT" -eq "$RUNNING_COUNT" ] && [ "$POD_COUNT" -gt 0 ]; then
    print_test "所有 Pod 运行正常 (${RUNNING_COUNT}/${POD_COUNT})" 0
else
    print_test "部分 Pod 未运行 (${RUNNING_COUNT}/${POD_COUNT})" 1
fi

# 1.3 检查 Service
print_info "检查 Service..."
SERVICE_COUNT=$(kubectl get svc -n ${NAMESPACE} -l app=claude-proxy --no-headers 2>/dev/null | wc -l)
if [ "$SERVICE_COUNT" -gt 0 ]; then
    print_test "Service 存在" 0
else
    print_test "Service 存在" 1
fi

# 1.4 检查 Ingress
print_info "检查 Ingress..."
INGRESS_COUNT=$(kubectl get ingress -n ${NAMESPACE} --no-headers 2>/dev/null | wc -l)
if [ "$INGRESS_COUNT" -gt 0 ]; then
    print_test "Ingress 存在" 0
else
    print_test "Ingress 存在" 1
fi

# ==========================================
# 2. 健康检查验证
# ==========================================
print_header "2. 健康检查验证"

# 2.1 检查健康端点
print_info "检查健康端点..."
HEALTH_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" https://${DOMAIN}/health 2>/dev/null || echo "000")
if [ "$HEALTH_RESPONSE" = "200" ]; then
    print_test "健康端点响应正常 (HTTP 200)" 0
else
    print_test "健康端点响应异常 (HTTP ${HEALTH_RESPONSE})" 1
fi

# 2.2 检查 Pod 健康探针
print_info "检查 Pod 健康探针..."
POD_NAME=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy --no-headers 2>/dev/null | head -1 | awk '{print $1}')
if [ -n "$POD_NAME" ]; then
    LIVENESS=$(kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
    if [ "$LIVENESS" = "True" ]; then
        print_test "Pod 健康探针正常" 0
    else
        print_test "Pod 健康探针异常" 1
    fi
else
    print_test "无法找到 Pod" 1
fi

# ==========================================
# 3. 数据库验证
# ==========================================
print_header "3. 数据库验证"

if [ -n "$POD_NAME" ]; then
    # 3.1 检查数据库文件
    print_info "检查数据库文件..."
    DB_EXISTS=$(kubectl exec ${POD_NAME} -n ${NAMESPACE} -- test -f /app/data/proxy.db && echo "yes" || echo "no" 2>/dev/null)
    if [ "$DB_EXISTS" = "yes" ]; then
        print_test "数据库文件存在" 0
    else
        print_test "数据库文件不存在" 1
    fi
    
    # 3.2 检查新表
    print_info "检查新表结构..."
    TABLES=$(kubectl exec ${POD_NAME} -n ${NAMESPACE} -- sqlite3 /app/data/proxy.db ".tables" 2>/dev/null || echo "")
    
    if echo "$TABLES" | grep -q "health_statuses"; then
        print_test "health_statuses 表存在" 0
    else
        print_test "health_statuses 表不存在" 1
    fi
    
    if echo "$TABLES" | grep -q "circuit_breaker_states"; then
        print_test "circuit_breaker_states 表存在" 0
    else
        print_test "circuit_breaker_states 表不存在" 1
    fi
    
    if echo "$TABLES" | grep -q "load_balancer_request_logs"; then
        print_test "load_balancer_request_logs 表存在" 0
    else
        print_test "load_balancer_request_logs 表不存在" 1
    fi
    
    if echo "$TABLES" | grep -q "alerts"; then
        print_test "alerts 表存在" 0
    else
        print_test "alerts 表不存在" 1
    fi
fi

# ==========================================
# 4. API 功能验证
# ==========================================
print_header "4. API 功能验证"

# 4.1 测试基本 API
print_info "测试基本 API..."
API_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" https://${DOMAIN}/api/configs 2>/dev/null || echo "000")
if [ "$API_RESPONSE" = "200" ] || [ "$API_RESPONSE" = "401" ]; then
    print_test "API 端点可访问" 0
else
    print_test "API 端点不可访问 (HTTP ${API_RESPONSE})" 1
fi

# 4.2 测试负载均衡器 API
print_info "测试负载均衡器 API..."
LB_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" https://${DOMAIN}/api/load-balancers 2>/dev/null || echo "000")
if [ "$LB_RESPONSE" = "200" ] || [ "$LB_RESPONSE" = "401" ]; then
    print_test "负载均衡器 API 可访问" 0
else
    print_test "负载均衡器 API 不可访问 (HTTP ${LB_RESPONSE})" 1
fi

# ==========================================
# 5. 性能验证
# ==========================================
print_header "5. 性能验证"

# 5.1 测试响应时间
print_info "测试响应时间..."
RESPONSE_TIME=$(curl -s -o /dev/null -w "%{time_total}" https://${DOMAIN}/health 2>/dev/null || echo "999")
RESPONSE_TIME_MS=$(echo "$RESPONSE_TIME * 1000" | bc -l 2>/dev/null || echo "999")

if (( $(echo "$RESPONSE_TIME_MS < 1000" | bc -l) )); then
    print_test "响应时间正常 (${RESPONSE_TIME_MS} ms)" 0
else
    print_test "响应时间过长 (${RESPONSE_TIME_MS} ms)" 1
fi

# 5.2 检查资源使用
print_info "检查资源使用..."
if command -v kubectl &> /dev/null; then
    CPU_USAGE=$(kubectl top pods -n ${NAMESPACE} -l app=claude-proxy --no-headers 2>/dev/null | awk '{sum+=$2} END {print sum}' || echo "0")
    MEM_USAGE=$(kubectl top pods -n ${NAMESPACE} -l app=claude-proxy --no-headers 2>/dev/null | awk '{sum+=$3} END {print sum}' || echo "0")
    
    print_info "CPU 使用: ${CPU_USAGE}"
    print_info "内存使用: ${MEM_USAGE}"
    
    # 简单检查：CPU 和内存使用不为 0 表示正常
    if [ "$CPU_USAGE" != "0" ] && [ "$MEM_USAGE" != "0" ]; then
        print_test "资源使用正常" 0
    else
        print_test "资源使用异常" 1
    fi
fi

# ==========================================
# 6. 日志验证
# ==========================================
print_header "6. 日志验证"

if [ -n "$POD_NAME" ]; then
    # 6.1 检查错误日志
    print_info "检查错误日志..."
    ERROR_COUNT=$(kubectl logs ${POD_NAME} -n ${NAMESPACE} --tail=100 2>/dev/null | grep -i "error\|fatal\|panic" | wc -l || echo "0")
    
    if [ "$ERROR_COUNT" -eq 0 ]; then
        print_test "无严重错误日志" 0
    else
        print_warning "发现 ${ERROR_COUNT} 条错误日志"
        print_test "日志检查" 1
    fi
    
    # 6.2 检查启动日志
    print_info "检查启动日志..."
    if kubectl logs ${POD_NAME} -n ${NAMESPACE} --tail=100 2>/dev/null | grep -q "Server started\|Listening on"; then
        print_test "服务启动日志正常" 0
    else
        print_test "服务启动日志异常" 1
    fi
fi

# ==========================================
# 7. 监控验证
# ==========================================
print_header "7. 监控验证"

# 7.1 检查 Prometheus 指标
print_info "检查 Prometheus 指标..."
if [ -n "$POD_NAME" ]; then
    METRICS_RESPONSE=$(kubectl exec ${POD_NAME} -n ${NAMESPACE} -- wget -q -O- http://localhost:54988/metrics 2>/dev/null || echo "")
    
    if [ -n "$METRICS_RESPONSE" ]; then
        print_test "Prometheus 指标端点正常" 0
    else
        print_test "Prometheus 指标端点异常" 1
    fi
fi

# ==========================================
# 8. 新功能验证
# ==========================================
print_header "8. 新功能验证"

# 8.1 验证健康检查功能
print_info "验证健康检查功能..."
# 这里需要实际的 API 调用来验证
print_warning "需要手动验证健康检查功能"

# 8.2 验证熔断器功能
print_info "验证熔断器功能..."
print_warning "需要手动验证熔断器功能"

# 8.3 验证重试功能
print_info "验证重试功能..."
print_warning "需要手动验证重试功能"

# 8.4 验证监控功能
print_info "验证监控功能..."
print_warning "需要手动验证监控功能"

# ==========================================
# 总结
# ==========================================
print_header "验证总结"

echo "总测试数: ${TOTAL_TESTS}"
echo -e "通过: ${GREEN}${PASSED_TESTS}${NC}"
echo -e "失败: ${RED}${FAILED_TESTS}${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}=========================================="
    echo "✅ 所有验证通过！"
    echo "==========================================${NC}"
    exit 0
else
    echo -e "${RED}=========================================="
    echo "❌ 部分验证失败！"
    echo "==========================================${NC}"
    echo ""
    echo "请检查失败的测试项并修复问题。"
    exit 1
fi
