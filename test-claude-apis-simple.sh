#!/bin/bash

# 简单的Claude API测试脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
BASE_URL="http://localhost:10086"
API_KEY="test-key"
ANTHROPIC_VERSION="2023-06-01"

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}   Claude API 基础功能测试${NC}"
echo -e "${BLUE}======================================${NC}\n"

# 1. 测试健康检查
echo -e "${YELLOW}► 测试健康检查${NC}"
curl -s "${BASE_URL}/health" | jq '.' || echo -e "${RED}✗ 健康检查失败${NC}"
echo ""

# 2. 测试GetMe接口（Claude CLI关键接口）
echo -e "${YELLOW}► 测试GetMe接口 (Claude CLI身份验证)${NC}"
curl -s -X GET "${BASE_URL}/v1/me" \
    -H "x-api-key: ${API_KEY}" | jq '.' || echo -e "${RED}✗ GetMe接口失败${NC}"
echo ""

# 3. 测试Models API
echo -e "${YELLOW}► 测试列出模型${NC}"
curl -s -X GET "${BASE_URL}/v1/models" \
    -H "x-api-key: ${API_KEY}" | jq '.data[0]' || echo -e "${RED}✗ 列出模型失败${NC}"
echo ""

# 4. 测试Messages API - 计数tokens
echo -e "${YELLOW}► 测试计数tokens${NC}"
curl -s -X POST "${BASE_URL}/v1/messages/count_tokens" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${API_KEY}" \
    -H "anthropic-version: ${ANTHROPIC_VERSION}" \
    -d '{
        "model": "claude-3-5-sonnet-20241022",
        "messages": [
            {
                "role": "user",
                "content": "Hello, how are you?"
            }
        ]
    }' | jq '.' || echo -e "${RED}✗ Token计数失败${NC}"
echo ""

# 5. 测试Batch API - 创建批处理
echo -e "${YELLOW}► 测试创建批处理${NC}"
BATCH_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/batches" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${API_KEY}" \
    -d '{
        "requests": [
            {
                "custom_id": "test-001",
                "params": {
                    "model": "claude-3-5-sonnet-20241022",
                    "max_tokens": 50,
                    "messages": [
                        {"role": "user", "content": "Say hello"}
                    ]
                }
            }
        ]
    }')
echo "$BATCH_RESPONSE" | jq '.' || echo -e "${RED}✗ 创建批处理失败${NC}"
BATCH_ID=$(echo "$BATCH_RESPONSE" | jq -r '.id')
echo ""

# 6. 测试Files API
echo -e "${YELLOW}► 测试Files API${NC}"
echo "Test file content" > /tmp/test_api.txt
FILE_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/files" \
    -H "x-api-key: ${API_KEY}" \
    -F "file=@/tmp/test_api.txt" \
    -F "purpose=assistants")
echo "$FILE_RESPONSE" | jq '.' || echo -e "${RED}✗ 上传文件失败${NC}"
FILE_ID=$(echo "$FILE_RESPONSE" | jq -r '.id')
echo ""

# 7. 测试Skills API
echo -e "${YELLOW}► 测试创建技能${NC}"
SKILL_RESPONSE=$(curl -s -X POST "${BASE_URL}/v1/skills" \
    -H "Content-Type: application/json" \
    -H "x-api-key: ${API_KEY}" \
    -d '{
        "name": "test_skill",
        "description": "A test skill",
        "instructions": "This is a test",
        "parameters": {
            "type": "object",
            "properties": {
                "input": {"type": "string"}
            }
        }
    }')
echo "$SKILL_RESPONSE" | jq '.' || echo -e "${RED}✗ 创建技能失败${NC}"
echo ""

# 清理
if [ -n "$FILE_ID" ] && [ "$FILE_ID" != "null" ]; then
    echo -e "${YELLOW}► 清理：删除测试文件${NC}"
    curl -s -X DELETE "${BASE_URL}/v1/files/${FILE_ID}" \
        -H "x-api-key: ${API_KEY}" | jq '.'
fi

if [ -n "$BATCH_ID" ] && [ "$BATCH_ID" != "null" ]; then
    echo -e "${YELLOW}► 清理：删除测试批处理${NC}"
    curl -s -X DELETE "${BASE_URL}/v1/batches/${BATCH_ID}" \
        -H "x-api-key: ${API_KEY}" | jq '.'
fi

rm -f /tmp/test_api.txt

echo -e "\n${GREEN}✅ 测试完成！${NC}"
