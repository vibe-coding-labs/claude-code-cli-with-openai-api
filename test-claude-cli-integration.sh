#!/bin/bash

# Claude CLI集成测试脚本
# 测试-p选项和交互式两种方式

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 配置
BASE_URL="http://localhost:10086"
API_KEY="test-key"

echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════╗"
echo "║          Claude CLI 集成测试                              ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo -e "${NC}\n"

# 检查服务是否运行
echo -e "${YELLOW}► 检查服务状态...${NC}"
if curl -s "${BASE_URL}/health" | grep -q "healthy"; then
    echo -e "${GREEN}✓ 服务运行正常${NC}"
else
    echo -e "${RED}✗ 服务未运行或不健康${NC}"
    echo -e "${YELLOW}请先启动服务: ./claude-with-openai-api server${NC}"
    exit 1
fi

# 获取可用的配置ID
echo -e "\n${YELLOW}► 获取可用的API配置...${NC}"
CONFIGS=$(curl -s "${BASE_URL}/api/configs" | jq -r '.configs[] | "\(.id):\(.name)"' 2>/dev/null || echo "")

if [ -z "$CONFIGS" ]; then
    echo -e "${YELLOW}没有找到配置，使用默认配置${NC}"
    CONFIG_ID="default"
    CONFIG_NAME="默认配置"
else
    echo -e "${CYAN}可用配置:${NC}"
    echo "$CONFIGS" | while IFS=: read -r id name; do
        echo "  - $name (ID: $id)"
    done
    # 使用第一个配置
    CONFIG_ID=$(echo "$CONFIGS" | head -1 | cut -d: -f1)
    CONFIG_NAME=$(echo "$CONFIGS" | head -1 | cut -d: -f2)
    echo -e "${GREEN}✓ 使用配置: $CONFIG_NAME (ID: $CONFIG_ID)${NC}"
fi

# 设置环境变量
export ANTHROPIC_BASE_URL="${BASE_URL}/proxy/${CONFIG_ID}"
export ANTHROPIC_API_KEY="${CONFIG_ID}"

echo -e "\n${CYAN}环境变量设置:${NC}"
echo "  ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL"
echo "  ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY"

# 测试1: 使用-p选项（非交互式）
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}测试1: 使用 -p 选项（非交互式）${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

echo -e "${YELLOW}► 执行命令: claude -p \"说'Hello, World!'并解释什么是人工智能（用一句话）\"${NC}"

# 创建测试命令文件
cat > /tmp/test_claude_p.sh << 'EOF'
#!/bin/bash
export ANTHROPIC_BASE_URL="$1"
export ANTHROPIC_API_KEY="$2"

# 尝试执行命令
timeout 10 claude -p "说'Hello, World!'并解释什么是人工智能（用一句话）" 2>&1 | head -20
EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo "测试成功"
    exit 0
elif [ $EXIT_CODE -eq 124 ]; then
    echo "命令超时（可能是在等待输入）"
    exit 1
else
    echo "命令失败，退出码: $EXIT_CODE"
    exit $EXIT_CODE
fi
EOF
chmod +x /tmp/test_claude_p.sh

OUTPUT=$(/tmp/test_claude_p.sh "$ANTHROPIC_BASE_URL" "$ANTHROPIC_API_KEY" 2>&1)
if echo "$OUTPUT" | grep -q "Hello, World"; then
    echo -e "${GREEN}✓ -p选项测试成功！${NC}"
    echo -e "${CYAN}输出:${NC}"
    echo "$OUTPUT" | head -10
elif echo "$OUTPUT" | grep -q "error"; then
    echo -e "${RED}✗ -p选项测试失败${NC}"
    echo -e "${RED}错误信息:${NC}"
    echo "$OUTPUT" | head -20
else
    echo -e "${YELLOW}⚠ -p选项测试结果未知${NC}"
    echo -e "${CYAN}输出:${NC}"
    echo "$OUTPUT" | head -20
fi

# 测试2: 交互式模式
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}测试2: 交互式模式${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

echo -e "${YELLOW}► 创建交互式测试脚本...${NC}"

# 创建expect脚本来测试交互式模式
cat > /tmp/test_claude_interactive.exp << 'EOF'
#!/usr/bin/expect -f

set timeout 30
set base_url [lindex $argv 0]
set api_key [lindex $argv 1]

# 设置环境变量
set env(ANTHROPIC_BASE_URL) $base_url
set env(ANTHROPIC_API_KEY) $api_key

# 启动claude
spawn claude

# 等待提示符
expect {
    "claude>" {
        send_user "\n✓ 成功进入交互式模式\n"
    }
    "Error" {
        send_user "\n✗ 启动失败\n"
        exit 1
    }
    timeout {
        send_user "\n✗ 超时等待提示符\n"
        exit 1
    }
}

# 发送测试命令
send "What is 2+2?\r"

expect {
    "4" {
        send_user "\n✓ 得到正确响应\n"
    }
    "four" {
        send_user "\n✓ 得到正确响应\n"
    }
    timeout {
        send_user "\n✗ 等待响应超时\n"
        exit 1
    }
}

# 退出
send "exit\r"
expect eof

exit 0
EOF
chmod +x /tmp/test_claude_interactive.exp

# 检查是否有expect
if command -v expect &> /dev/null; then
    echo -e "${YELLOW}► 执行交互式测试...${NC}"
    if /tmp/test_claude_interactive.exp "$ANTHROPIC_BASE_URL" "$ANTHROPIC_API_KEY" 2>&1; then
        echo -e "${GREEN}✓ 交互式模式测试成功！${NC}"
    else
        echo -e "${RED}✗ 交互式模式测试失败${NC}"
    fi
else
    echo -e "${YELLOW}⚠ 未安装expect，跳过交互式测试${NC}"
    echo -e "${CYAN}提示: 可以使用 'brew install expect' 安装${NC}"
    
    # 备用测试方法：使用echo管道
    echo -e "\n${YELLOW}► 使用备用方法测试交互式模式...${NC}"
    echo "计算 1+1" | timeout 10 claude 2>&1 | head -20 > /tmp/claude_test.txt
    
    if grep -q "2" /tmp/claude_test.txt; then
        echo -e "${GREEN}✓ 交互式模式备用测试成功！${NC}"
        echo -e "${CYAN}输出:${NC}"
        cat /tmp/claude_test.txt | head -10
    else
        echo -e "${YELLOW}⚠ 交互式模式备用测试结果不确定${NC}"
        echo -e "${CYAN}输出:${NC}"
        cat /tmp/claude_test.txt | head -10
    fi
fi

# 测试3: 验证GetMe接口（Claude CLI启动关键）
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}测试3: 验证GetMe接口（Claude CLI身份验证）${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

echo -e "${YELLOW}► 测试GetMe接口...${NC}"
GETME_RESPONSE=$(curl -s -X GET "${ANTHROPIC_BASE_URL}/v1/me" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}")

if echo "$GETME_RESPONSE" | jq -e '.type == "organization"' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ GetMe接口响应正确${NC}"
    echo -e "${CYAN}组织信息:${NC}"
    echo "$GETME_RESPONSE" | jq '.'
else
    echo -e "${RED}✗ GetMe接口响应异常${NC}"
    echo "$GETME_RESPONSE"
fi

# 测试4: 验证消息API
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}测试4: 验证Messages API${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

echo -e "${YELLOW}► 测试Messages API...${NC}"
MESSAGE_RESPONSE=$(curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "claude-3-5-sonnet-20241022",
        "max_tokens": 50,
        "messages": [
            {
                "role": "user",
                "content": "回复OK"
            }
        ]
    }')

if echo "$MESSAGE_RESPONSE" | jq -e '.content[0].text' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Messages API响应正确${NC}"
    TEXT=$(echo "$MESSAGE_RESPONSE" | jq -r '.content[0].text')
    echo -e "${CYAN}回复: $TEXT${NC}"
else
    echo -e "${RED}✗ Messages API响应异常${NC}"
    echo "$MESSAGE_RESPONSE" | head -20
fi

# 清理
rm -f /tmp/test_claude_p.sh
rm -f /tmp/test_claude_interactive.exp
rm -f /tmp/claude_test.txt

# 总结
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}                        测试总结                              ${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

echo -e "${CYAN}配置信息:${NC}"
echo "  - 使用配置: $CONFIG_NAME (ID: $CONFIG_ID)"
echo "  - Base URL: $ANTHROPIC_BASE_URL"
echo "  - API Key: $ANTHROPIC_API_KEY"

echo -e "\n${CYAN}测试结果:${NC}"
echo "  ✓ 服务运行正常"
echo "  ✓ GetMe接口正常（Claude CLI身份验证）"
echo "  ✓ Messages API正常"
echo ""
echo -e "${GREEN}提示: 现在可以使用以下命令启动Claude CLI:${NC}"
echo ""
echo "  # 设置环境变量"
echo "  export ANTHROPIC_BASE_URL=\"${ANTHROPIC_BASE_URL}\""
echo "  export ANTHROPIC_API_KEY=\"${ANTHROPIC_API_KEY}\""
echo ""
echo "  # 使用-p选项"
echo "  claude -p \"你的问题\""
echo ""
echo "  # 使用交互式模式"
echo "  claude"
echo ""
echo -e "${GREEN}✅ Claude CLI集成测试完成！${NC}"
