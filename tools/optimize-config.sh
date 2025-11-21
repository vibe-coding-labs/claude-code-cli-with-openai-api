#!/bin/bash

# 优化 ClaudeCode 配置脚本
# 用于提升 Agent 稳定性

set -e

DB_PATH="${1:-data/proxy.db}"
CONFIG_ID="${2:-}"

if [ ! -f "$DB_PATH" ]; then
    echo "❌ 数据库文件不存在: $DB_PATH"
    exit 1
fi

echo "🔧 优化 ClaudeCode 配置..."
echo "数据库: $DB_PATH"
echo ""

# 如果没有指定配置 ID，列出所有配置
if [ -z "$CONFIG_ID" ]; then
    echo "📋 现有配置:"
    sqlite3 "$DB_PATH" "SELECT id, name, request_timeout, retry_count, enabled FROM api_configs;" | while IFS='|' read -r id name timeout retry enabled; do
        echo "  ID: $id"
        echo "  名称: $name"
        echo "  超时: ${timeout}s"
        echo "  重试: ${retry}次"
        echo "  启用: $enabled"
        echo "  ---"
    done
    
    echo ""
    echo "用法: $0 [数据库路径] [配置ID]"
    echo "示例: $0 data/proxy.db your-config-id"
    exit 0
fi

echo "配置 ID: $CONFIG_ID"
echo ""

# 获取当前配置
current_timeout=$(sqlite3 "$DB_PATH" "SELECT request_timeout FROM api_configs WHERE id = '$CONFIG_ID';")
current_retry=$(sqlite3 "$DB_PATH" "SELECT retry_count FROM api_configs WHERE id = '$CONFIG_ID';")

if [ -z "$current_timeout" ]; then
    echo "❌ 配置不存在: $CONFIG_ID"
    exit 1
fi

echo "📊 当前配置:"
echo "  超时时间: ${current_timeout}s"
echo "  重试次数: ${current_retry}次"
echo ""

# 推荐配置
RECOMMENDED_TIMEOUT=300
RECOMMENDED_RETRY=5

echo "💡 推荐配置:"
echo "  超时时间: ${RECOMMENDED_TIMEOUT}s (对于重度使用场景)"
echo "  重试次数: ${RECOMMENDED_RETRY}次 (对于不稳定网络)"
echo ""

read -p "是否应用推荐配置? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ 已取消"
    exit 0
fi

echo ""
echo "⚙️  应用配置..."

# 更新配置
sqlite3 "$DB_PATH" "UPDATE api_configs SET request_timeout = $RECOMMENDED_TIMEOUT, retry_count = $RECOMMENDED_RETRY WHERE id = '$CONFIG_ID';"

echo "✅ 配置已更新！"
echo ""
echo "📊 新配置:"
echo "  超时时间: ${RECOMMENDED_TIMEOUT}s"
echo "  重试次数: ${RECOMMENDED_RETRY}次"
echo ""
echo "⚠️  请重启服务以使配置生效:"
echo "  pkill -f claude-with-openai-api"
echo "  nohup ./claude-with-openai-api > server.log 2>&1 &"
