#!/bin/bash

# 测试 iFlow API 和代理服务器
# 使用方法: ./test-iflow-api.sh

IFLOW_API_KEY="sk-d35ba48d260a51054982b9a6794ca2d9"
IFLOW_BASE_URL="https://apis.iflow.cn/v1"
PROXY_URL="http://localhost:10086"

echo "============================================"
echo "iFlow API 测试脚本"
echo "============================================"
echo ""

# 测试1: 直接测试 iFlow API
echo "📡 测试 1: 直接调用 iFlow API"
echo "-------------------------------------------"
curl -s -X POST ${IFLOW_BASE_URL}/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${IFLOW_API_KEY}" \
  -d '{
    "model": "qwen3-coder",
    "messages": [{"role": "user", "content": "你好"}],
    "max_tokens": 50
  }' | jq -r '.choices[0].message.content'
echo ""
echo ""

# 测试2: 通过代理服务器测试 Claude API 格式
echo "🔄 测试 2: 通过代理转换 Claude API 格式"
echo "-------------------------------------------"
curl -s -X POST ${PROXY_URL}/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 50,
    "messages": [{"role": "user", "content": "你好"}]
  }' | jq -r '.content[0].text'
echo ""
echo ""

# 测试3: 测试流式响应
echo "📺 测试 3: 测试流式响应"
echo "-------------------------------------------"
curl -s -N -X POST ${PROXY_URL}/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-haiku-20240307",
    "max_tokens": 30,
    "stream": true,
    "messages": [{"role": "user", "content": "数到3"}]
  }' | grep "content_block_delta" | head -5
echo ""
echo ""

echo "============================================"
echo "✅ 测试完成"
echo "============================================"
