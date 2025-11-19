#!/bin/bash

echo "=================================="
echo "启动 Claude CLI with Proxy"
echo "=================================="
echo ""

export ANTHROPIC_API_KEY="test"
export ANTHROPIC_BASE_URL="http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76"

echo "环境变量设置："
echo "  ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY"
echo "  ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL"
echo ""

echo "测试连接..."
curl -s "$ANTHROPIC_BASE_URL/v1/me" -H "x-api-key: $ANTHROPIC_API_KEY" | jq -r '.display_name' 2>/dev/null && echo "✅ 连接成功" || echo "❌ 连接失败"
echo ""

echo "启动 Claude CLI（如果提示登录，说明环境变量未生效）..."
echo ""

exec claude "$@"
