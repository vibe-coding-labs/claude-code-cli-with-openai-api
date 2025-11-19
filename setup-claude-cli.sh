#!/bin/bash

# Claude Code CLI 配置脚本 - iFlow API
# 自动生成时间: 2025-11-18

CONFIG_ID="ff40e638-918a-4556-b3c5-4155d1cc4156"

echo "========================================"
echo "Claude Code CLI - iFlow API 配置"
echo "========================================"
echo ""
echo "配置ID: ${CONFIG_ID}"
echo "配置名称: iFlow API"
echo ""

# 设置环境变量
export ANTHROPIC_BASE_URL="http://localhost:10086/proxy/${CONFIG_ID}"
export ANTHROPIC_API_KEY="${CONFIG_ID}"

echo "✅ 环境变量已设置:"
echo ""
echo "export ANTHROPIC_BASE_URL=\"http://localhost:10086/proxy/${CONFIG_ID}\""
echo "export ANTHROPIC_API_KEY=\"${CONFIG_ID}\""
echo ""
echo "========================================"
echo "现在可以使用 Claude Code CLI 了！"
echo "========================================"
echo ""
echo "测试命令:"
echo "  claude"
echo ""
echo "或者使用临时变量:"
echo "  ANTHROPIC_BASE_URL=http://localhost:10086/proxy/${CONFIG_ID} ANTHROPIC_API_KEY=${CONFIG_ID} claude"
echo ""
