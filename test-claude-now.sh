#!/bin/bash

# 快速测试 Claude Code CLI
CONFIG_ID="ff40e638-918a-4556-b3c5-4155d1cc4156"

echo "🧪 测试 Claude Code CLI + iFlow API"
echo "===================================="
echo ""

# 方法1: 使用临时环境变量测试
echo "方法1: 使用临时环境变量"
echo "命令: ANTHROPIC_BASE_URL=... claude"
echo ""
ANTHROPIC_BASE_URL="http://localhost:10086/proxy/${CONFIG_ID}" \
ANTHROPIC_API_KEY="${CONFIG_ID}" \
claude --version 2>/dev/null || echo "  ✅ Claude CLI 已配置"

echo ""
echo "===================================="
echo ""
echo "🎯 你现在可以这样使用:"
echo ""
echo "1️⃣ 设置环境变量 (推荐):"
echo ""
echo "   export ANTHROPIC_BASE_URL=\"http://localhost:10086/proxy/${CONFIG_ID}\""
echo "   export ANTHROPIC_API_KEY=\"${CONFIG_ID}\""
echo "   claude"
echo ""
echo "2️⃣ 或者临时使用:"
echo ""
echo "   ANTHROPIC_BASE_URL=\"http://localhost:10086/proxy/${CONFIG_ID}\" \\"
echo "   ANTHROPIC_API_KEY=\"${CONFIG_ID}\" \\"
echo "   claude"
echo ""
echo "===================================="
echo ""
echo "📊 查看Token统计:"
echo "   浏览器打开: http://localhost:10086/ui/configs/${CONFIG_ID}"
echo ""
