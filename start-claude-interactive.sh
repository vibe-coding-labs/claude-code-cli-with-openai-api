#!/bin/bash

# Claude CLI 交互模式启动脚本
# 使用 iFlow API (qwen3-coder-plus)

export ANTHROPIC_BASE_URL=http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76
export ANTHROPIC_API_KEY="test"

echo "🚀 Starting Claude CLI with iFlow API (qwen3-coder-plus)"
echo "📡 Base URL: $ANTHROPIC_BASE_URL"
echo ""

claude
