#!/bin/bash

# 测试 Claude CLI 交互模式

export ANTHROPIC_BASE_URL=http://localhost:8082/proxy/8fccf7f4-392d-4351-8382-c7ffc1a9de76
export ANTHROPIC_API_KEY="test"

echo "🧪 Testing Claude CLI Interactive Mode..."
echo ""

# 用一个简单的命令测试
echo "用一句话介绍你自己" | claude

echo ""
echo "✅ Test completed"
