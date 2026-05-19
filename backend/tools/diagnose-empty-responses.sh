#!/bin/bash

# 诊断空响应问题

set -e

DB_PATH="${1:-data/proxy.db}"

if [ ! -f "$DB_PATH" ]; then
    echo "❌ 数据库文件不存在: $DB_PATH"
    exit 1
fi

echo "🔍 诊断空响应问题"
echo "================================"
echo ""

# 统计最近 1 小时的错误
echo "📊 最近 1 小时的请求统计:"
sqlite3 "$DB_PATH" "
SELECT 
    status,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM request_logs WHERE created_at > datetime('now', '-1 hour')), 2) as percentage
FROM request_logs 
WHERE created_at > datetime('now', '-1 hour')
GROUP BY status
ORDER BY count DESC;
" | while IFS='|' read -r status count pct; do
    if [ "$status" = "success" ]; then
        echo "  ✅ $status: $count 次 ($pct%)"
    else
        echo "  ❌ $status: $count 次 ($pct%)"
    fi
done

echo ""
echo "================================"
echo ""

# 查看错误详情
echo "🔍 错误详情 (最近 10 条):"
sqlite3 "$DB_PATH" "
SELECT 
    datetime(created_at, 'localtime') as time,
    config_id,
    model,
    error_message
FROM request_logs 
WHERE status = 'error' 
    AND created_at > datetime('now', '-1 hour')
ORDER BY created_at DESC 
LIMIT 10;
" | while IFS='|' read -r time config model error; do
    echo "  时间: $time"
    echo "  配置: $config"
    echo "  模型: $model"
    echo "  错误: $error"
    echo "  ---"
done

echo ""
echo "================================"
echo ""

# 统计错误类型
echo "📈 错误类型统计 (最近 1 小时):"
sqlite3 "$DB_PATH" "
SELECT 
    CASE 
        WHEN error_message LIKE '%empty%' THEN 'Empty Response'
        WHEN error_message LIKE '%timeout%' THEN 'Timeout'
        WHEN error_message LIKE '%429%' THEN 'Rate Limit'
        WHEN error_message LIKE '%500%' THEN 'Server Error'
        WHEN error_message LIKE '%context deadline%' THEN 'Context Deadline'
        ELSE 'Other'
    END as error_type,
    COUNT(*) as count
FROM request_logs 
WHERE status = 'error' 
    AND created_at > datetime('now', '-1 hour')
GROUP BY error_type
ORDER BY count DESC;
" | while IFS='|' read -r type count; do
    echo "  $type: $count 次"
done

echo ""
echo "================================"
echo ""

# 按配置统计错误率
echo "📊 各配置的错误率:"
sqlite3 "$DB_PATH" "
SELECT 
    ac.name as config_name,
    COUNT(CASE WHEN rl.status = 'error' THEN 1 END) as error_count,
    COUNT(*) as total_count,
    ROUND(COUNT(CASE WHEN rl.status = 'error' THEN 1 END) * 100.0 / COUNT(*), 2) as error_rate
FROM request_logs rl
JOIN api_configs ac ON rl.config_id = ac.id
WHERE rl.created_at > datetime('now', '-1 hour')
GROUP BY ac.id, ac.name
ORDER BY error_rate DESC;
" | while IFS='|' read -r name errors total rate; do
    if [ "$(echo "$rate > 50" | bc -l)" -eq 1 ]; then
        echo "  ❌ $name: $errors/$total ($rate%) - 严重问题"
    elif [ "$(echo "$rate > 20" | bc -l)" -eq 1 ]; then
        echo "  ⚠️  $name: $errors/$total ($rate%) - 需要关注"
    else
        echo "  ✅ $name: $errors/$total ($rate%)"
    fi
done

echo ""
echo "================================"
echo ""

# 建议
echo "💡 问题诊断建议:"
echo ""

# 检查是否是空响应问题
empty_count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM request_logs WHERE status = 'error' AND error_message LIKE '%empty%' AND created_at > datetime('now', '-1 hour');")

if [ "$empty_count" -gt 0 ]; then
    echo "🔴 检测到 $empty_count 次空响应错误"
    echo ""
    echo "   可能原因："
    echo "   1. 上游 OpenAI API 质量问题（最常见）"
    echo "   2. API Key 配额不足或被限流"
    echo "   3. 请求参数不正确"
    echo "   4. 模型不可用或正在维护"
    echo ""
    echo "   建议操作："
    echo "   ✅ 已自动启用：空响应自动重试（5 次）"
    echo "   1. 检查上游 API 状态"
    echo "   2. 验证 API Key 是否有效"
    echo "   3. 尝试更换到更稳定的 API 提供商"
    echo "   4. 如果使用代理，检查代理稳定性"
    echo ""
fi

# 检查超时问题
timeout_count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM request_logs WHERE status = 'error' AND (error_message LIKE '%timeout%' OR error_message LIKE '%deadline%') AND created_at > datetime('now', '-1 hour');")

if [ "$timeout_count" -gt 0 ]; then
    echo "⏱️  检测到 $timeout_count 次超时错误"
    echo ""
    echo "   建议："
    echo "   1. 当前超时配置: 300 秒"
    echo "   2. 如需更长时间，可增加到 600 秒："
    echo "      sqlite3 $DB_PATH \"UPDATE api_configs SET request_timeout = 600 WHERE enabled = 1;\""
    echo ""
fi

# 检查限流问题
ratelimit_count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM request_logs WHERE status = 'error' AND error_message LIKE '%429%' AND created_at > datetime('now', '-1 hour');")

if [ "$ratelimit_count" -gt 0 ]; then
    echo "🚦 检测到 $ratelimit_count 次限流错误"
    echo ""
    echo "   建议："
    echo "   1. 降低请求频率"
    echo "   2. 升级 API Key 配额"
    echo "   3. 配置多个 API 实现负载均衡"
    echo ""
fi

echo "================================"
