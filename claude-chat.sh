#!/bin/bash

# Claude CLI 伪交互模式
# 通过循环提示实现类似交互效果

echo "🤖 Claude Code (iFlow API - qwen3-coder-plus)"
echo "📝 输入 'exit' 或 'quit' 退出"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

while true; do
    echo -n "You: "
    read -r input
    
    # 检查退出命令
    if [[ "$input" == "exit" || "$input" == "quit" ]]; then
        echo "👋 Goodbye!"
        break
    fi
    
    # 跳过空输入
    if [[ -z "$input" ]]; then
        continue
    fi
    
    # 调用 Claude CLI
    echo ""
    echo "Claude:"
    echo "$input" | claude --print
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
done
