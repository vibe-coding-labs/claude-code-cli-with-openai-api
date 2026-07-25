#!/bin/bash
# 监控代理服务通信质量
# 用法: ./scripts/monitor-quality.sh [db_path]

# 先确定数据库路径（在 cd 之前）
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

echo "数据库路径: $DB_PATH"

echo "========================================"
echo "代理服务通信质量监控报告"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"
echo ""

# 1. 总体统计
echo "## 1. 总体统计"
sqlite3 "$DB_PATH" "
SELECT
    '总请求数' as metric,
    COUNT(*) as value
FROM request_logs
UNION ALL
SELECT
    '成功请求数',
    COUNT(*)
FROM request_logs
WHERE status = 'success'
UNION ALL
SELECT
    '失败请求数',
    COUNT(*)
FROM request_logs
WHERE status = 'error'
UNION ALL
SELECT
    '失败率(%)',
    ROUND(COUNT(CASE WHEN status = 'error' THEN 1 END) * 100.0 / COUNT(*), 2)
FROM request_logs;
" | column -t -s '|'
echo ""

# 2. 最近1小时统计
echo "## 2. 最近1小时统计"
sqlite3 "$DB_PATH" "
SELECT
    '最近1小时请求数' as metric,
    COUNT(*) as value
FROM request_logs
WHERE created_at >= datetime('now', '-1 hour')
UNION ALL
SELECT
    '最近1小时成功数',
    COUNT(*)
FROM request_logs
WHERE created_at >= datetime('now', '-1 hour') AND status = 'success'
UNION ALL
SELECT
    '最近1小时失败数',
    COUNT(*)
FROM request_logs
WHERE created_at >= datetime('now', '-1 hour') AND status = 'error'
UNION ALL
SELECT
    '最近1小时失败率(%)',
    ROUND(COUNT(CASE WHEN status = 'error' THEN 1 END) * 100.0 / COUNT(*), 2)
FROM request_logs
WHERE created_at >= datetime('now', '-1 hour');
" | column -t -s '|'
echo ""

# 3. 按模型失败率（最近24小时）
echo "## 3. 各模型失败率（最近24小时，请求数>=5）"
sqlite3 "$DB_PATH" "
SELECT
    model,
    COUNT(*) as total,
    SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as errors,
    ROUND(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) || '%' as error_rate
FROM request_logs
WHERE created_at >= datetime('now', '-24 hours')
GROUP BY model
HAVING total >= 5
ORDER BY error_rate DESC;
" | column -t -s '|'
echo ""

# 4. 主要错误类型（最近24小时）
echo "## 4. 主要错误类型（最近24小时）"
sqlite3 "$DB_PATH" "
SELECT
    SUBSTR(error_message, 1, 60) as error_type,
    COUNT(*) as count
FROM request_logs
WHERE status = 'error'
  AND created_at >= datetime('now', '-24 hours')
GROUP BY error_type
ORDER BY count DESC
LIMIT 5;
" | column -t -s '|'
echo ""

# 5. 请求耗时分布（最近24小时）
echo "## 5. 请求耗时分布（最近24小时）"
sqlite3 "$DB_PATH" "
SELECT
    CASE
        WHEN duration_ms < 1000 THEN '0-1s'
        WHEN duration_ms < 5000 THEN '1-5s'
        WHEN duration_ms < 10000 THEN '5-10s'
        WHEN duration_ms < 30000 THEN '10-30s'
        WHEN duration_ms < 60000 THEN '30-60s'
        ELSE '>60s'
    END as duration,
    COUNT(*) as count,
    ROUND(AVG(duration_ms)) as avg_ms
FROM request_logs
WHERE created_at >= datetime('now', '-24 hours')
GROUP BY duration
ORDER BY avg_ms;
" | column -t -s '|'
echo ""

# 6. 超时请求警告
echo "## 6. 超时请求警告（最近24小时，>30s）"
TIMEOUT_COUNT=$(sqlite3 "$DB_PATH" "
SELECT COUNT(*)
FROM request_logs
WHERE created_at >= datetime('now', '-24 hours')
  AND duration_ms > 30000
  AND status = 'success';
")

if [ "$TIMEOUT_COUNT" -gt 0 ]; then
    echo "⚠️  发现 $TIMEOUT_COUNT 个超过30秒的成功请求"
    sqlite3 "$DB_PATH" "
    SELECT
        datetime(created_at, 'localtime') as time,
        model,
        ROUND(duration_ms / 1000.0, 1) as 'duration(s)'
    FROM request_logs
    WHERE created_at >= datetime('now', '-24 hours')
      AND duration_ms > 30000
      AND status = 'success'
    ORDER BY duration_ms DESC
    LIMIT 5;
    " | column -t -s '|'
else
    echo "✅ 没有发现超时请求"
fi
echo ""

# 7. 代理错误统计
echo "## 7. 代理错误统计（最近24小时）"
sqlite3 "$DB_PATH" "
SELECT
    error_category,
    COUNT(*) as count
FROM proxy_errors
WHERE created_at >= datetime('now', '-24 hours')
GROUP BY error_category
ORDER BY count DESC;
" 2>/dev/null | column -t -s '|' || echo "(无代理错误数据)"
echo ""

echo "========================================"
echo "报告生成完毕"