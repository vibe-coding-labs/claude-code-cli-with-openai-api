#!/bin/bash
# 监控代理服务客户端中断问题
# 用法: ./scripts/monitor-client-disconnections.sh [db_path]
# 自动检测高频客户端中断并尝试修复

set -e

# 确定数据库路径
if [ -n "$1" ]; then
    DB_PATH="$1"
elif [ -f "/home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/data/proxy.db" ]; then
    DB_PATH="/home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/data/proxy.db"
elif [ -f "data/proxy.db" ]; then
    DB_PATH="data/proxy.db"
elif [ -f "../data/proxy.db" ]; then
    DB_PATH="../data/proxy.db"
else
    echo "错误: 找不到 proxy.db 数据库文件"
    exit 1
fi

# 代理服务名称
SERVICE_NAME="claude-openai-proxy.service"
# 中断阈值：最近15分钟内超过此数量视为高频
DISCONNECTION_THRESHOLD=10
# 检查时间窗口（分钟）
WINDOW_MINUTES=15

echo "========================================"
echo "客户端中断监控"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"
echo ""

# 1. 检查服务状态
echo "## 1. 服务状态检查"
if systemctl --user is-active "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "✅ 服务运行正常"
    systemctl --user status "$SERVICE_NAME" --no-pager 2>&1 | head -5 || true
else
    echo "❌ 服务未运行，尝试启动..."
    systemctl --user start "$SERVICE_NAME" 2>&1 || true
    sleep 2
    if systemctl --user is-active "$SERVICE_NAME" >/dev/null 2>&1; then
        echo "✅ 服务启动成功"
    else
        echo "❌ 服务启动失败"
    fi
fi
echo ""

# 2. 检查最近客户端中断错误
echo "## 2. 客户端中断错误分析（最近 ${WINDOW_MINUTES} 分钟）"
DISCONNECTION_COUNT=$(sqlite3 "$DB_PATH" "
SELECT COUNT(*)
FROM proxy_errors
WHERE created_at >= datetime('now', '-${WINDOW_MINUTES} minutes')
  AND (error_message LIKE '%client disconnected%'
       OR error_message LIKE '%socket connection was closed%'
       OR error_message LIKE '%cancelled%'
       OR error_category = 'client_cancelled');
" 2>/dev/null || echo "0")

echo "最近 ${WINDOW_MINUTES} 分钟客户端中断次数: $DISCONNECTION_COUNT"

if [ "$DISCONNECTION_COUNT" -gt "$DISCONNECTION_THRESHOLD" ]; then
    echo "⚠️  客户端中断次数超过阈值 ($DISCONNECTION_THRESHOLD)，可能需要修复"
    echo ""
    echo "最近的中断错误:"
    sqlite3 "$DB_PATH" "
    SELECT
        datetime(created_at, 'localtime') as time,
        config_name,
        model,
        substr(error_message, 1, 80) as error_preview
    FROM proxy_errors
    WHERE created_at >= datetime('now', '-${WINDOW_MINUTES} minutes')
      AND (error_message LIKE '%client disconnected%'
           OR error_message LIKE '%socket connection was closed%'
           OR error_message LIKE '%cancelled%'
           OR error_category = 'client_cancelled')
    ORDER BY created_at DESC
    LIMIT 5;
    " 2>/dev/null | column -t -s '|' || true
else
    echo "✅ 客户端中断次数在正常范围内"
fi
echo ""

# 3. 检查错误趋势
echo "## 3. 错误趋势分析（最近1小时）"
ERROR_TREND=$(sqlite3 "$DB_PATH" "
SELECT
    strftime('%Y-%m-%d %H:%M', created_at) as time_bucket,
    COUNT(*) as error_count
FROM proxy_errors
WHERE created_at >= datetime('now', '-1 hour')
GROUP BY time_bucket
ORDER BY time_bucket DESC
LIMIT 10;
" 2>/dev/null || echo "")

if [ -n "$ERROR_TREND" ]; then
    echo "$ERROR_TREND" | column -t -s '|' || true
else
    echo "✅ 最近1小时无错误记录"
fi
echo ""

# 4. 自动修复
echo "## 4. 自动修复"
if [ "$DISCONNECTION_COUNT" -gt "$DISCONNECTION_THRESHOLD" ]; then
    echo "检测到高频客户端中断，尝试重启服务..."
    systemctl --user restart "$SERVICE_NAME" 2>&1 || true
    sleep 3
    if systemctl --user is-active "$SERVICE_NAME" >/dev/null 2>&1; then
        echo "✅ 服务重启成功"
    else
        echo "❌ 服务重启失败，请手动检查"
    fi
else
    echo "✅ 无需修复"
fi
echo ""

# 5. 建议
echo "## 5. 建议"
if [ "$DISCONNECTION_COUNT" -gt "$DISCONNECTION_THRESHOLD" ]; then
    echo "⚠️  高频客户端中断可能原因："
    echo "  1. 上游服务不稳定（网络问题、过载等）"
    echo "  2. 客户端超时设置过短"
    echo "  3. 代理服务配置需要优化"
    echo ""
    echo "建议操作："
    echo "  - 检查上游服务状态"
    echo "  - 增加客户端超时时间"
    echo "  - 查看详细日志: tail -f logs/proxy.log"
fi

echo ""
echo "========================================"
echo "监控完成"
echo "========================================"
