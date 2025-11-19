#!/bin/bash

# Claude CLI的简单替代方案
# 直接用curl调用我们的API

CONFIG_ID="8fccf7f4-392d-4351-8382-c7ffc1a9de76"
PORT="8082"

if [ -z "$1" ]; then
    echo "用法: $0 \"你的问题\""
    echo "示例: $0 \"写一个hello world程序\""
    exit 1
fi

PROMPT="$1"

curl -s -X POST "http://localhost:${PORT}/proxy/${CONFIG_ID}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d "{
    \"model\": \"claude-3-5-sonnet-20241022\",
    \"max_tokens\": 4096,
    \"messages\": [{\"role\": \"user\", \"content\": $(echo "$PROMPT" | jq -R -s .)}]
  }" | jq -r '.content[0].text'
