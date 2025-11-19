#!/bin/bash

# 测试Claude Code CLI与iFlow API配置
# 配置ID: ff40e638-918a-4556-b3c5-4155d1cc4156

CONFIG_ID="ff40e638-918a-4556-b3c5-4155d1cc4156"

echo "============================================"
echo "Claude Code CLI 测试"
echo "============================================"
echo ""
echo "配置信息:"
echo "  Config ID: ${CONFIG_ID}"
echo "  Base URL: http://localhost:10086/proxy/${CONFIG_ID}"
echo ""

# 设置环境变量
export ANTHROPIC_BASE_URL="http://localhost:10086/proxy/${CONFIG_ID}"
export ANTHROPIC_API_KEY="${CONFIG_ID}"

echo "环境变量已设置:"
echo "  ANTHROPIC_BASE_URL=${ANTHROPIC_BASE_URL}"
echo "  ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}"
echo ""

# 测试基本请求
echo "测试1: 基本API调用"
echo "-------------------------------------------"
curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: ${ANTHROPIC_API_KEY}" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 50,
    "messages": [{"role": "user", "content": "你好，请用一句话介绍你自己"}]
  }' | jq -r '.content[0].text'
echo ""
echo ""

echo "如果上面测试成功，你现在可以使用Claude Code CLI了！"
echo ""
echo "使用命令:"
echo "  claude"
echo ""
echo "或者临时使用："
echo "  ANTHROPIC_BASE_URL=http://localhost:10086/proxy/${CONFIG_ID} ANTHROPIC_API_KEY=${CONFIG_ID} claude"
echo ""
