#!/bin/bash

echo "=================================="
echo "测试 Claude CLI 认证端点"
echo "=================================="
echo ""

BASE_URL="http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76"
API_KEY="test"

echo "1. 测试 /v1/me 端点："
curl -s "$BASE_URL/v1/me" \
  -H "x-api-key: $API_KEY" | jq . 2>/dev/null || curl -s "$BASE_URL/v1/me" -H "x-api-key: $API_KEY"
echo ""
echo ""

echo "2. 测试 /v1/models 端点："
curl -s "$BASE_URL/v1/models" \
  -H "x-api-key: $API_KEY" | jq . 2>/dev/null || curl -s "$BASE_URL/v1/models" -H "x-api-key: $API_KEY"
echo ""
echo ""

echo "3. 测试 /v1/organizations/test/usage 端点："
curl -s "$BASE_URL/v1/organizations/test/usage" \
  -H "x-api-key: $API_KEY" | jq . 2>/dev/null || curl -s "$BASE_URL/v1/organizations/test/usage" -H "x-api-key: $API_KEY"
echo ""
echo ""

echo "=================================="
echo "如果以上端点都返回正常数据，"
echo "Claude CLI 应该不再提示登录！"
echo "=================================="
echo ""
echo "现在请在新终端运行："
echo "  ANTHROPIC_API_KEY=\"test\" ANTHROPIC_BASE_URL=\"$BASE_URL\" claude"
