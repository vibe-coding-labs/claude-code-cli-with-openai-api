#!/bin/bash

# Claude CLI完整兼容性测试
# 测试-p选项和交互式模式的各种场景

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

# 配置
BASE_URL="http://localhost:8083"
CONFIG_ID="8fccf7f4-392d-4351-8382-c7ffc1a9de76"

# 测试结果计数
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 辅助函数
print_test() {
    echo -e "${YELLOW}► $1${NC}"
    ((TOTAL_TESTS++))
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
    ((PASSED_TESTS++))
}

print_failure() {
    echo -e "${RED}✗ $1${NC}"
    ((FAILED_TESTS++))
}

print_info() {
    echo -e "${CYAN}  $1${NC}"
}

# 标题
echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════╗"
echo "║        Claude CLI 完整兼容性测试 (iFlow集成)              ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo -e "${NC}\n"

# 设置环境变量
export ANTHROPIC_BASE_URL="${BASE_URL}/proxy/${CONFIG_ID}"
export ANTHROPIC_API_KEY="${CONFIG_ID}"

echo -e "${CYAN}测试配置:${NC}"
echo "  ANTHROPIC_BASE_URL: $ANTHROPIC_BASE_URL"
echo "  ANTHROPIC_API_KEY: $ANTHROPIC_API_KEY"
echo ""

# ========== 基础API测试 ==========
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}1. 基础API测试${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

# 测试GetMe接口
print_test "GetMe接口 (Claude CLI身份验证必需)"
GETME_RESPONSE=$(curl -s -X GET "${ANTHROPIC_BASE_URL}/v1/me" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}")

if echo "$GETME_RESPONSE" | jq -e '.type == "organization"' > /dev/null 2>&1; then
    print_success "GetMe接口正常"
    print_info "组织: $(echo "$GETME_RESPONSE" | jq -r '.name')"
else
    print_failure "GetMe接口失败"
    print_info "$GETME_RESPONSE"
fi

# 测试Messages API
print_test "Messages API基本功能"
MSG_RESPONSE=$(curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "claude-3-5-sonnet-20241022",
        "max_tokens": 50,
        "messages": [{"role": "user", "content": "回复OK"}]
    }')

if echo "$MSG_RESPONSE" | jq -e '.content[0].text' > /dev/null 2>&1; then
    print_success "Messages API正常"
    print_info "响应: $(echo "$MSG_RESPONSE" | jq -r '.content[0].text' | head -c 50)..."
else
    print_failure "Messages API失败"
fi

# ========== -p选项测试 ==========
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}2. 测试-p选项（非交互式模式）${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

# 测试简单问题
print_test "简单数学问题"
P_RESULT=$(ANTHROPIC_BASE_URL="$ANTHROPIC_BASE_URL" ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" claude -p "直接回答：2+2等于几？只说数字" 2>&1 || true)
if echo "$P_RESULT" | grep -q "4"; then
    print_success "简单问题响应正确"
    print_info "回答: $(echo "$P_RESULT" | head -1)"
else
    print_failure "简单问题响应异常"
    print_info "$(echo "$P_RESULT" | head -2)"
fi

# 测试中文支持
print_test "中文支持"
P_CHINESE=$(ANTHROPIC_BASE_URL="$ANTHROPIC_BASE_URL" ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" claude -p "用一句话介绍你自己" 2>&1 || true)
if echo "$P_CHINESE" | grep -qE "助手|AI|人工智能|Claude"; then
    print_success "中文响应正常"
    print_info "$(echo "$P_CHINESE" | head -1)"
else
    print_failure "中文响应异常"
fi

# 测试代码生成
print_test "代码生成能力"
P_CODE=$(ANTHROPIC_BASE_URL="$ANTHROPIC_BASE_URL" ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" claude -p "写一个Python的Hello World，只要代码" 2>&1 || true)
if echo "$P_CODE" | grep -q "print"; then
    print_success "代码生成正常"
else
    print_failure "代码生成失败"
fi

# ========== 交互式模式测试 ==========
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}3. 测试交互式模式${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

# 测试单个问题
print_test "单个问题输入"
INTERACTIVE_SINGLE=$(echo "3+3等于几？" | ANTHROPIC_BASE_URL="$ANTHROPIC_BASE_URL" ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" claude 2>&1 | head -20 || true)
if echo "$INTERACTIVE_SINGLE" | grep -q "6"; then
    print_success "单问题交互成功"
else
    print_failure "单问题交互失败"
fi

# 测试多行输入
print_test "多行输入"
MULTI_INPUT=$(printf "第一个问题：1+1\n第二个问题：2+2" | ANTHROPIC_BASE_URL="$ANTHROPIC_BASE_URL" ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" claude 2>&1 | head -30 || true)
if echo "$MULTI_INPUT" | grep -qE "2.*4|第一.*第二"; then
    print_success "多行输入处理正常"
else
    print_failure "多行输入处理异常"
fi

# ========== 高级功能测试 ==========
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}4. 高级功能测试${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

# 测试流式响应
print_test "流式响应支持"
STREAM_RESPONSE=$(curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "claude-3-5-sonnet-20241022",
        "max_tokens": 50,
        "stream": true,
        "messages": [{"role": "user", "content": "数到3"}]
    }' 2>&1 | head -5)

if echo "$STREAM_RESPONSE" | grep -q "event:"; then
    print_success "流式响应支持正常"
else
    print_failure "流式响应不支持"
fi

# 测试Token计数
print_test "Token计数功能"
TOKEN_RESPONSE=$(curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages/count_tokens" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "claude-3-5-sonnet-20241022",
        "messages": [{"role": "user", "content": "测试消息"}]
    }')

if echo "$TOKEN_RESPONSE" | jq -e '.input_tokens' > /dev/null 2>&1; then
    print_success "Token计数功能正常"
    print_info "Tokens: $(echo "$TOKEN_RESPONSE" | jq -r '.input_tokens')"
else
    print_failure "Token计数功能异常"
fi

# ========== 错误处理测试 ==========
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}5. 错误处理测试${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

# 测试无效API Key
print_test "无效API Key处理"
INVALID_KEY_RESPONSE=$(curl -s -X GET "${ANTHROPIC_BASE_URL}/v1/me" \
    -H "x-api-key: invalid-key" 2>&1)

if echo "$INVALID_KEY_RESPONSE" | grep -qE "error|unauthorized|invalid"; then
    print_success "无效API Key正确拒绝"
else
    print_failure "无效API Key处理异常"
fi

# 测试无效模型
print_test "无效模型处理"
INVALID_MODEL=$(curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "invalid-model-xyz",
        "max_tokens": 50,
        "messages": [{"role": "user", "content": "test"}]
    }')

if echo "$INVALID_MODEL" | grep -qE "error|invalid"; then
    print_success "无效模型正确处理"
else
    print_failure "无效模型处理异常"
fi

# ========== 总结 ==========
echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}                     测试报告总结                            ${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"

SUCCESS_RATE=$(echo "scale=2; $PASSED_TESTS * 100 / $TOTAL_TESTS" | bc)

echo -e "${CYAN}测试统计:${NC}"
echo "  总测试数: $TOTAL_TESTS"
echo -e "  ${GREEN}通过: $PASSED_TESTS${NC}"
echo -e "  ${RED}失败: $FAILED_TESTS${NC}"
echo "  成功率: ${SUCCESS_RATE}%"

echo -e "\n${CYAN}兼容性结果:${NC}"
if [ $PASSED_TESTS -ge 8 ]; then
    echo -e "  ${GREEN}✓ Claude CLI基本兼容${NC}"
    echo -e "  ${GREEN}✓ -p选项（非交互式）支持${NC}"
    echo -e "  ${GREEN}✓ 交互式模式支持${NC}"
    echo -e "  ${GREEN}✓ iFlow API集成成功${NC}"
else
    echo -e "  ${YELLOW}⚠ 部分功能可能存在兼容性问题${NC}"
fi

echo -e "\n${CYAN}使用建议:${NC}"
echo "1. 对于快速单次查询，使用-p选项："
echo "   claude -p \"你的问题\""
echo ""
echo "2. 对于多轮对话，使用交互式模式："
echo "   claude"
echo ""
echo "3. 对于脚本集成，使用管道："
echo "   echo \"问题\" | claude"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "\n${GREEN}🎉 完美！所有测试通过，Claude CLI完全兼容。${NC}"
elif [ $FAILED_TESTS -le 2 ]; then
    echo -e "\n${GREEN}✅ 良好！Claude CLI基本兼容，可以正常使用。${NC}"
else
    echo -e "\n${YELLOW}⚠️  注意：有${FAILED_TESTS}个测试失败，可能影响部分功能。${NC}"
fi
