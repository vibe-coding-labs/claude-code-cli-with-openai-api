#!/bin/bash

# 测试 Claude CLI 交互模式

echo "========================================="
echo "Claude CLI 交互模式调试脚本"
echo "========================================="
echo ""

# 配置
export ANTHROPIC_BASE_URL="http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76"
export ANTHROPIC_API_KEY="test"

echo "1. 检查环境变量设置："
echo "   ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL"
echo "   ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY"
echo ""

echo "2. 测试 -p 模式（应该成功）："
claude -p "hi" 2>&1 | head -3
echo ""

echo "3. 测试服务器健康检查："
curl -s http://localhost:8082/health | jq . 2>/dev/null || curl -s http://localhost:8082/health
echo ""

echo "4. 测试连接测试端点（模拟 Claude CLI 的测试请求）："
curl -s -X POST http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1,
    "messages": [{"role": "user", "content": "test"}]
  }' | jq . 2>/dev/null || echo "请求失败"
echo ""

echo "5. 检查 Claude CLI 是否有缓存的登录状态："
if [ -d ~/.config/claude ]; then
  echo "   ~/.config/claude 目录存在"
  ls -la ~/.config/claude/ 2>/dev/null
elif [ -d ~/.claude ]; then
  echo "   ~/.claude 目录存在"
  ls -la ~/.claude/ 2>/dev/null
else
  echo "   没有找到 Claude 配置目录"
fi
echo ""

echo "========================================="
echo "如果 -p 模式成功但交互模式失败，可能的原因："
echo "1. Claude CLI 需要额外的认证端点"
echo "2. 环境变量在交互模式下没有正确传递"
echo "3. Claude CLI 缓存了旧的登录状态，需要 /logout"
echo ""
echo "建议操作："
echo "  - 尝试：claude logout"
echo "  - 然后重新测试交互模式"
echo "========================================="
