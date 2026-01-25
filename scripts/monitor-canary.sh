#!/bin/bash
# 监控灰度发布的关键指标

set -e

PERCENTAGE=${1:-10}
DURATION=${2:-1800}  # 默认监控 30 分钟
NAMESPACE=${3:-zhaixingren-prod}
PROMETHEUS_URL=${PROMETHEUS_URL:-http://localhost:9090}

echo "=========================================="
echo "开始监控 Canary 部署"
echo "=========================================="
echo "流量比例: ${PERCENTAGE}%"
echo "监控时长: ${DURATION} 秒 ($(($DURATION / 60)) 分钟)"
echo "命名空间: ${NAMESPACE}"
echo "=========================================="
echo ""

START_TIME=$(date +%s)
END_TIME=$((START_TIME + DURATION))
ITERATION=0

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查阈值
check_threshold() {
    local value=$1
    local threshold=$2
    local comparison=$3  # "gt" for greater than, "lt" for less than
    
    if [ "$comparison" = "gt" ]; then
        if (( $(echo "$value > $threshold" | bc -l) )); then
            echo -e "${RED}FAIL${NC}"
            return 1
        fi
    elif [ "$comparison" = "lt" ]; then
        if (( $(echo "$value < $threshold" | bc -l) )); then
            echo -e "${RED}FAIL${NC}"
            return 1
        fi
    fi
    echo -e "${GREEN}PASS${NC}"
    return 0
}

while [ $(date +%s) -lt $END_TIME ]; do
    ITERATION=$((ITERATION + 1))
    CURRENT_TIME=$(date '+%Y-%m-%d %H:%M:%S')
    ELAPSED=$(($(date +%s) - START_TIME))
    REMAINING=$((END_TIME - $(date +%s)))
    
    clear
    echo "=========================================="
    echo "Canary 监控 - 迭代 #${ITERATION}"
    echo "=========================================="
    echo "当前时间: ${CURRENT_TIME}"
    echo "已运行: $(($ELAPSED / 60)) 分钟"
    echo "剩余: $(($REMAINING / 60)) 分钟"
    echo "=========================================="
    echo ""
    
    # 1. Pod 状态
    echo "📦 Pod 状态:"
    echo "----------------------------------------"
    kubectl get pods -n ${NAMESPACE} -l app=claude-proxy -o wide
    echo ""
    
    # 检查 Canary Pod 是否全部 Running
    CANARY_PODS=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy,version=canary --no-headers 2>/dev/null | wc -l)
    CANARY_RUNNING=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy,version=canary --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
    
    if [ "$CANARY_PODS" -eq "$CANARY_RUNNING" ] && [ "$CANARY_PODS" -gt 0 ]; then
        echo -e "Canary Pods: ${GREEN}${CANARY_RUNNING}/${CANARY_PODS} Running${NC}"
    else
        echo -e "Canary Pods: ${RED}${CANARY_RUNNING}/${CANARY_PODS} Running${NC}"
    fi
    echo ""
    
    # 2. 错误率
    echo "❌ 错误率 (最近 5 分钟):"
    echo "----------------------------------------"
    
    # 查询 Canary 错误率
    CANARY_ERROR_RATE=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=rate(http_requests_total{namespace=\"${NAMESPACE}\",version=\"canary\",status=~\"5..\"}[5m])/rate(http_requests_total{namespace=\"${NAMESPACE}\",version=\"canary\"}[5m])" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "0"' 2>/dev/null || echo "0")
    
    CANARY_ERROR_RATE_PCT=$(echo "$CANARY_ERROR_RATE * 100" | bc -l 2>/dev/null || echo "0")
    printf "Canary: %.2f%% " "$CANARY_ERROR_RATE_PCT"
    check_threshold "$CANARY_ERROR_RATE_PCT" "1.0" "gt"
    
    # 查询 Stable 错误率
    STABLE_ERROR_RATE=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=rate(http_requests_total{namespace=\"${NAMESPACE}\",version=\"stable\",status=~\"5..\"}[5m])/rate(http_requests_total{namespace=\"${NAMESPACE}\",version=\"stable\"}[5m])" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "0"' 2>/dev/null || echo "0")
    
    STABLE_ERROR_RATE_PCT=$(echo "$STABLE_ERROR_RATE * 100" | bc -l 2>/dev/null || echo "0")
    printf "Stable: %.2f%%\n" "$STABLE_ERROR_RATE_PCT"
    echo ""
    
    # 3. 延迟
    echo "⏱️  P99 延迟 (最近 5 分钟):"
    echo "----------------------------------------"
    
    # 查询 Canary P99 延迟
    CANARY_P99=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket{namespace=\"${NAMESPACE}\",version=\"canary\"}[5m]))" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "0"' 2>/dev/null || echo "0")
    
    CANARY_P99_MS=$(echo "$CANARY_P99 * 1000" | bc -l 2>/dev/null || echo "0")
    printf "Canary: %.2f ms " "$CANARY_P99_MS"
    check_threshold "$CANARY_P99_MS" "100" "gt"
    
    # 查询 Stable P99 延迟
    STABLE_P99=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket{namespace=\"${NAMESPACE}\",version=\"stable\"}[5m]))" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "0"' 2>/dev/null || echo "0")
    
    STABLE_P99_MS=$(echo "$STABLE_P99 * 1000" | bc -l 2>/dev/null || echo "0")
    printf "Stable: %.2f ms\n" "$STABLE_P99_MS"
    echo ""
    
    # 4. 健康检查通过率
    echo "💚 健康检查通过率 (最近 5 分钟):"
    echo "----------------------------------------"
    
    CANARY_HEALTH_RATE=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=rate(health_check_success_total{namespace=\"${NAMESPACE}\",version=\"canary\"}[5m])/rate(health_check_total{namespace=\"${NAMESPACE}\",version=\"canary\"}[5m])" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "1"' 2>/dev/null || echo "1")
    
    CANARY_HEALTH_PCT=$(echo "$CANARY_HEALTH_RATE * 100" | bc -l 2>/dev/null || echo "100")
    printf "Canary: %.2f%% " "$CANARY_HEALTH_PCT"
    check_threshold "$CANARY_HEALTH_PCT" "99" "lt"
    echo ""
    
    # 5. 请求吞吐量
    echo "📊 请求吞吐量 (req/s):"
    echo "----------------------------------------"
    
    CANARY_RPS=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=rate(http_requests_total{namespace=\"${NAMESPACE}\",version=\"canary\"}[1m])" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "0"' 2>/dev/null || echo "0")
    
    STABLE_RPS=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=rate(http_requests_total{namespace=\"${NAMESPACE}\",version=\"stable\"}[1m])" 2>/dev/null | \
      jq -r '.data.result[0].value[1] // "0"' 2>/dev/null || echo "0")
    
    printf "Canary: %.2f req/s\n" "$CANARY_RPS"
    printf "Stable: %.2f req/s\n" "$STABLE_RPS"
    echo ""
    
    # 6. 资源使用
    echo "💻 资源使用:"
    echo "----------------------------------------"
    
    # CPU 使用
    CANARY_CPU=$(kubectl top pods -n ${NAMESPACE} -l app=claude-proxy,version=canary --no-headers 2>/dev/null | awk '{sum+=$2} END {print sum}' || echo "0")
    STABLE_CPU=$(kubectl top pods -n ${NAMESPACE} -l app=claude-proxy,version=stable --no-headers 2>/dev/null | awk '{sum+=$2} END {print sum}' || echo "0")
    
    echo "CPU:"
    echo "  Canary: ${CANARY_CPU}"
    echo "  Stable: ${STABLE_CPU}"
    
    # 内存使用
    CANARY_MEM=$(kubectl top pods -n ${NAMESPACE} -l app=claude-proxy,version=canary --no-headers 2>/dev/null | awk '{sum+=$3} END {print sum}' || echo "0")
    STABLE_MEM=$(kubectl top pods -n ${NAMESPACE} -l app=claude-proxy,version=stable --no-headers 2>/dev/null | awk '{sum+=$3} END {print sum}' || echo "0")
    
    echo "Memory:"
    echo "  Canary: ${CANARY_MEM}"
    echo "  Stable: ${STABLE_MEM}"
    echo ""
    
    # 7. 最近的错误日志
    echo "📝 最近的错误日志 (最近 1 分钟):"
    echo "----------------------------------------"
    
    CANARY_POD=$(kubectl get pods -n ${NAMESPACE} -l app=claude-proxy,version=canary --no-headers 2>/dev/null | head -1 | awk '{print $1}')
    if [ -n "$CANARY_POD" ]; then
        kubectl logs -n ${NAMESPACE} ${CANARY_POD} --since=1m 2>/dev/null | grep -i "error\|fatal\|panic" | tail -5 || echo "无错误日志"
    else
        echo "无 Canary Pod"
    fi
    echo ""
    
    # 8. 决策建议
    echo "=========================================="
    echo "🎯 决策建议:"
    echo "=========================================="
    
    FAIL_COUNT=0
    
    # 检查各项指标
    if (( $(echo "$CANARY_ERROR_RATE_PCT > 1.0" | bc -l) )); then
        echo -e "${RED}⚠️  错误率超过阈值 (1%)${NC}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    if (( $(echo "$CANARY_P99_MS > 100" | bc -l) )); then
        echo -e "${YELLOW}⚠️  P99 延迟超过阈值 (100ms)${NC}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    if (( $(echo "$CANARY_HEALTH_PCT < 99" | bc -l) )); then
        echo -e "${RED}⚠️  健康检查通过率低于阈值 (99%)${NC}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    if [ "$CANARY_PODS" -ne "$CANARY_RUNNING" ]; then
        echo -e "${RED}⚠️  部分 Canary Pod 未运行${NC}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    if [ $FAIL_COUNT -eq 0 ]; then
        echo -e "${GREEN}✅ 所有指标正常，可以继续灰度发布${NC}"
    elif [ $FAIL_COUNT -le 1 ]; then
        echo -e "${YELLOW}⚠️  发现 ${FAIL_COUNT} 个问题，建议继续观察${NC}"
    else
        echo -e "${RED}❌ 发现 ${FAIL_COUNT} 个问题，建议回滚${NC}"
        echo ""
        echo "执行回滚命令:"
        echo "  ./scripts/rollback-canary.sh"
    fi
    
    echo "=========================================="
    echo ""
    
    # 等待 60 秒后继续下一次检查
    sleep 60
done

echo ""
echo "=========================================="
echo "✅ 监控完成"
echo "=========================================="
echo "总监控时长: $(($DURATION / 60)) 分钟"
echo "总迭代次数: ${ITERATION}"
echo ""
