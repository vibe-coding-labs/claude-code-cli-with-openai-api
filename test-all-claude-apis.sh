#!/bin/bash

# 综合测试所有Claude API接口
# 使用iFlow配置进行测试

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

# 测试结果计数
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 辅助函数
print_header() {
    echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}\n"
}

print_test() {
    echo -e "${YELLOW}► $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
    ((PASSED_TESTS++))
    ((TOTAL_TESTS++))
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
    ((FAILED_TESTS++))
    ((TOTAL_TESTS++))
}

# 测试函数
test_health() {
    print_test "测试健康检查端点"
    
    response=$(curl -s -w "\n%{http_code}" "${BASE_URL}/health")
    http_code=$(echo "$response" | tail -n 1)
    body=$(echo "$response" | head -n -1)
    
    if [ "$http_code" = "200" ]; then
        print_success "健康检查成功"
    else
        print_error "健康检查失败 (HTTP $http_code)"
    fi
}

# ========== Messages API 测试 ==========
test_messages_api() {
    print_header "Messages API 测试"
    
    # 测试1: 创建基本消息
    print_test "测试创建基本消息"
    response=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/v1/messages" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -H "anthropic-version: ${ANTHROPIC_VERSION}" \
        -d '{
            "model": "claude-3-5-sonnet-20241022",
            "max_tokens": 100,
            "messages": [
                {
                    "role": "user",
                    "content": "说一句话证明你在正常工作"
                }
            ]
        }')
    
    http_code=$(echo "$response" | tail -n 1)
    if [ "$http_code" = "200" ]; then
        print_success "创建消息成功"
    else
        print_error "创建消息失败 (HTTP $http_code)"
    fi
    
    # 测试2: 带系统提示的消息
    print_test "测试带系统提示的消息"
    response=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/v1/messages" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -H "anthropic-version: ${ANTHROPIC_VERSION}" \
        -d '{
            "model": "claude-3-5-sonnet-20241022",
            "system": "你是一个友好的助手",
            "max_tokens": 50,
            "messages": [
                {
                    "role": "user",
                    "content": "你好"
                }
            ]
        }')
    
    http_code=$(echo "$response" | tail -n 1)
    if [ "$http_code" = "200" ]; then
        print_success "系统提示消息成功"
    else
        print_error "系统提示消息失败 (HTTP $http_code)"
    fi
    
    # 测试3: 计数tokens
    print_test "测试计数tokens"
    response=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/v1/messages/count_tokens" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -H "anthropic-version: ${ANTHROPIC_VERSION}" \
        -d '{
            "model": "claude-3-5-sonnet-20241022",
            "messages": [
                {
                    "role": "user",
                    "content": "这是一个测试消息，用来计算token数量"
                }
            ]
        }')
    
    http_code=$(echo "$response" | tail -n 1)
    body=$(echo "$response" | head -n -1)
    if [ "$http_code" = "200" ]; then
        print_success "Token计数成功: $body"
    else
        print_error "Token计数失败 (HTTP $http_code)"
    fi
    
    # 测试4: 流式响应
    print_test "测试流式响应"
    response=$(curl -s -w "\n%{http_code}" -N -X POST "${BASE_URL}/v1/messages" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -H "anthropic-version: ${ANTHROPIC_VERSION}" \
        -d '{
            "model": "claude-3-5-sonnet-20241022",
            "max_tokens": 50,
            "stream": true,
            "messages": [
                {
                    "role": "user",
                    "content": "计数到3"
                }
            ]
        }' 2>&1 | head -n 5)
    
    if echo "$response" | grep -q "event:"; then
        print_success "流式响应成功"
    else
        print_error "流式响应失败"
    fi
}

# ========== Batch API 测试 ==========
test_batch_api() {
    print_header "Batch API 测试"
    
    # 创建批处理
    print_test "测试创建批处理"
    create_response=$(curl -s -X POST "${BASE_URL}/v1/batches" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -d '{
            "requests": [
                {
                    "custom_id": "req-001",
                    "params": {
                        "model": "claude-3-5-sonnet-20241022",
                        "max_tokens": 50,
                        "messages": [
                            {"role": "user", "content": "测试批处理消息1"}
                        ]
                    }
                },
                {
                    "custom_id": "req-002",
                    "params": {
                        "model": "claude-3-5-haiku-20241022",
                        "max_tokens": 30,
                        "messages": [
                            {"role": "user", "content": "测试批处理消息2"}
                        ]
                    }
                }
            ]
        }')
    
    batch_id=$(echo "$create_response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    if [ -n "$batch_id" ]; then
        print_success "创建批处理成功: $batch_id"
        
        # 获取批处理状态
        print_test "测试获取批处理状态"
        status_response=$(curl -s -X GET "${BASE_URL}/v1/batches/${batch_id}" \
            -H "x-api-key: ${API_KEY}")
        if echo "$status_response" | grep -q "$batch_id"; then
            print_success "获取批处理状态成功"
        else
            print_error "获取批处理状态失败"
        fi
        
        # 列出批处理
        print_test "测试列出批处理"
        list_response=$(curl -s -X GET "${BASE_URL}/v1/batches" \
            -H "x-api-key: ${API_KEY}")
        if echo "$list_response" | grep -q '"data"'; then
            print_success "列出批处理成功"
        else
            print_error "列出批处理失败"
        fi
        
        # 获取批处理结果
        print_test "测试获取批处理结果"
        results_response=$(curl -s -X GET "${BASE_URL}/v1/batches/${batch_id}/results" \
            -H "x-api-key: ${API_KEY}")
        if echo "$results_response" | grep -q '"results"'; then
            print_success "获取批处理结果成功"
        else
            print_error "获取批处理结果失败"
        fi
        
        # 取消批处理
        print_test "测试取消批处理"
        cancel_response=$(curl -s -X POST "${BASE_URL}/v1/batches/${batch_id}/cancel" \
            -H "x-api-key: ${API_KEY}")
        if echo "$cancel_response" | grep -q "canceled"; then
            print_success "取消批处理成功"
        else
            print_error "取消批处理失败"
        fi
        
        # 删除批处理
        print_test "测试删除批处理"
        delete_response=$(curl -s -X DELETE "${BASE_URL}/v1/batches/${batch_id}" \
            -H "x-api-key: ${API_KEY}")
        if echo "$delete_response" | grep -q "deleted"; then
            print_success "删除批处理成功"
        else
            print_error "删除批处理失败"
        fi
    else
        print_error "创建批处理失败"
    fi
}

# ========== Files API 测试 ==========
test_files_api() {
    print_header "Files API 测试"
    
    # 创建测试文件
    echo "This is a test file for Claude API" > /tmp/test_claude.txt
    
    # 上传文件
    print_test "测试上传文件"
    upload_response=$(curl -s -X POST "${BASE_URL}/v1/files" \
        -H "x-api-key: ${API_KEY}" \
        -F "file=@/tmp/test_claude.txt" \
        -F "purpose=assistants")
    
    file_id=$(echo "$upload_response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    if [ -n "$file_id" ]; then
        print_success "上传文件成功: $file_id"
        
        # 列出文件
        print_test "测试列出文件"
        list_response=$(curl -s -X GET "${BASE_URL}/v1/files" \
            -H "x-api-key: ${API_KEY}")
        if echo "$list_response" | grep -q '"data"'; then
            print_success "列出文件成功"
        else
            print_error "列出文件失败"
        fi
        
        # 获取文件元数据
        print_test "测试获取文件元数据"
        metadata_response=$(curl -s -X GET "${BASE_URL}/v1/files/${file_id}" \
            -H "x-api-key: ${API_KEY}")
        if echo "$metadata_response" | grep -q "$file_id"; then
            print_success "获取文件元数据成功"
        else
            print_error "获取文件元数据失败"
        fi
        
        # 获取文件内容
        print_test "测试获取文件内容"
        content_response=$(curl -s -X GET "${BASE_URL}/v1/files/${file_id}/content" \
            -H "x-api-key: ${API_KEY}")
        if echo "$content_response" | grep -q "test file"; then
            print_success "获取文件内容成功"
        else
            print_error "获取文件内容失败"
        fi
        
        # 删除文件
        print_test "测试删除文件"
        delete_response=$(curl -s -X DELETE "${BASE_URL}/v1/files/${file_id}" \
            -H "x-api-key: ${API_KEY}")
        if echo "$delete_response" | grep -q "deleted"; then
            print_success "删除文件成功"
        else
            print_error "删除文件失败"
        fi
    else
        print_error "上传文件失败"
    fi
    
    # 清理测试文件
    rm -f /tmp/test_claude.txt
}

# ========== Skills API 测试 ==========
test_skills_api() {
    print_header "Skills API 测试"
    
    # 创建技能
    print_test "测试创建技能"
    create_response=$(curl -s -X POST "${BASE_URL}/v1/skills" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -d '{
            "name": "test_skill",
            "description": "测试技能",
            "instructions": "这是一个测试技能的指令",
            "parameters": {
                "type": "object",
                "properties": {
                    "input": {"type": "string"}
                }
            }
        }')
    
    skill_id=$(echo "$create_response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    if [ -n "$skill_id" ]; then
        print_success "创建技能成功: $skill_id"
        
        # 列出技能
        print_test "测试列出技能"
        list_response=$(curl -s -X GET "${BASE_URL}/v1/skills" \
            -H "x-api-key: ${API_KEY}")
        if echo "$list_response" | grep -q '"data"'; then
            print_success "列出技能成功"
        else
            print_error "列出技能失败"
        fi
        
        # 获取技能详情
        print_test "测试获取技能详情"
        detail_response=$(curl -s -X GET "${BASE_URL}/v1/skills/${skill_id}" \
            -H "x-api-key: ${API_KEY}")
        if echo "$detail_response" | grep -q "$skill_id"; then
            print_success "获取技能详情成功"
        else
            print_error "获取技能详情失败"
        fi
        
        # 创建技能版本
        print_test "测试创建技能版本"
        version_response=$(curl -s -X POST "${BASE_URL}/v1/skills/${skill_id}/versions" \
            -H "Content-Type: application/json" \
            -H "x-api-key: ${API_KEY}" \
            -d '{
                "instructions": "更新的技能指令",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "input": {"type": "string"},
                        "output": {"type": "string"}
                    }
                }
            }')
        
        version_id=$(echo "$version_response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
        if [ -n "$version_id" ]; then
            print_success "创建技能版本成功: $version_id"
            
            # 列出技能版本
            print_test "测试列出技能版本"
            versions_response=$(curl -s -X GET "${BASE_URL}/v1/skills/${skill_id}/versions" \
                -H "x-api-key: ${API_KEY}")
            if echo "$versions_response" | grep -q '"data"'; then
                print_success "列出技能版本成功"
            else
                print_error "列出技能版本失败"
            fi
            
            # 删除技能版本
            print_test "测试删除技能版本"
            delete_version_response=$(curl -s -X DELETE "${BASE_URL}/v1/skills/${skill_id}/versions/${version_id}" \
                -H "x-api-key: ${API_KEY}")
            if echo "$delete_version_response" | grep -q "deleted"; then
                print_success "删除技能版本成功"
            else
                print_error "删除技能版本失败"
            fi
        else
            print_error "创建技能版本失败"
        fi
        
        # 删除技能
        print_test "测试删除技能"
        delete_response=$(curl -s -X DELETE "${BASE_URL}/v1/skills/${skill_id}" \
            -H "x-api-key: ${API_KEY}")
        if echo "$delete_response" | grep -q "deleted"; then
            print_success "删除技能成功"
        else
            print_error "删除技能失败"
        fi
    else
        print_error "创建技能失败"
    fi
}

# ========== Models API 测试 ==========
test_models_api() {
    print_header "Models API 测试"
    
    # 列出模型
    print_test "测试列出模型"
    list_response=$(curl -s -X GET "${BASE_URL}/v1/models" \
        -H "x-api-key: ${API_KEY}")
    
    if echo "$list_response" | grep -q '"data"'; then
        print_success "列出模型成功"
        
        # 获取具体模型信息
        print_test "测试获取模型详情 (claude-3-opus-20240229)"
        model_response=$(curl -s -X GET "${BASE_URL}/v1/models/claude-3-opus-20240229" \
            -H "x-api-key: ${API_KEY}")
        if echo "$model_response" | grep -q "claude-3-opus"; then
            print_success "获取Opus模型详情成功"
        else
            print_error "获取Opus模型详情失败"
        fi
        
        print_test "测试获取模型详情 (claude-3-5-sonnet-20241022)"
        model_response=$(curl -s -X GET "${BASE_URL}/v1/models/claude-3-5-sonnet-20241022" \
            -H "x-api-key: ${API_KEY}")
        if echo "$model_response" | grep -q "claude-3-5-sonnet"; then
            print_success "获取Sonnet模型详情成功"
        else
            print_error "获取Sonnet模型详情失败"
        fi
        
        print_test "测试获取不存在的模型"
        model_response=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/v1/models/invalid-model" \
            -H "x-api-key: ${API_KEY}")
        http_code=$(echo "$model_response" | tail -n 1)
        if [ "$http_code" = "404" ]; then
            print_success "正确返回404错误"
        else
            print_error "错误处理失败"
        fi
    else
        print_error "列出模型失败"
    fi
}

# ========== Admin API 测试 ==========
test_admin_api() {
    print_header "Admin API 测试"
    
    # 获取组织信息（GetMe）
    print_test "测试GetMe接口（Claude CLI身份验证关键）"
    getme_response=$(curl -s -X GET "${BASE_URL}/v1/me" \
        -H "x-api-key: ${API_KEY}")
    
    if echo "$getme_response" | grep -q '"type":"organization"'; then
        print_success "GetMe接口成功"
        org_id=$(echo "$getme_response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
        
        # 获取组织使用情况
        print_test "测试获取组织使用情况"
        usage_response=$(curl -s -X GET "${BASE_URL}/v1/organizations/${org_id}/usage" \
            -H "x-api-key: ${API_KEY}")
        if echo "$usage_response" | grep -q '"object":"usage"'; then
            print_success "获取组织使用情况成功"
        else
            print_error "获取组织使用情况失败"
        fi
    else
        print_error "GetMe接口失败（这会导致Claude CLI无法启动！）"
    fi
}

# ========== 测试特定配置的API ==========
test_proxy_config_api() {
    print_header "测试配置独立路径 (/proxy/:id/v1/*)"
    
    CONFIG_ID="iflow"
    
    # 测试配置独立的Messages API
    print_test "测试配置独立的Messages API"
    response=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/proxy/${CONFIG_ID}/v1/messages" \
        -H "Content-Type: application/json" \
        -H "x-api-key: ${API_KEY}" \
        -H "anthropic-version: ${ANTHROPIC_VERSION}" \
        -d '{
            "model": "claude-3-5-sonnet-20241022",
            "max_tokens": 50,
            "messages": [
                {
                    "role": "user",
                    "content": "测试配置独立路径"
                }
            ]
        }')
    
    http_code=$(echo "$response" | tail -n 1)
    if [ "$http_code" = "200" ] || [ "$http_code" = "404" ]; then
        if [ "$http_code" = "404" ]; then
            print_success "配置独立路径正确返回404（配置不存在）"
        else
            print_success "配置独立路径消息成功"
        fi
    else
        print_error "配置独立路径失败 (HTTP $http_code)"
    fi
    
    # 测试配置独立的GetMe
    print_test "测试配置独立的GetMe接口"
    getme_response=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/proxy/${CONFIG_ID}/v1/me" \
        -H "x-api-key: ${API_KEY}")
    
    http_code=$(echo "$getme_response" | tail -n 1)
    if [ "$http_code" = "200" ] || [ "$http_code" = "404" ]; then
        print_success "配置独立GetMe接口响应正确"
    else
        print_error "配置独立GetMe接口失败"
    fi
}

# ========== 主函数 ==========
main() {
    echo -e "${BLUE}"
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║          Claude API 综合测试套件 (使用iFlow配置)           ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    
    # 检查服务器是否运行
    print_test "检查服务器状态..."
    if ! curl -s "${BASE_URL}/health" > /dev/null 2>&1; then
        print_error "服务器未运行！请先启动服务器: ./claude-with-openai-api"
        exit 1
    fi
    print_success "服务器运行正常"
    
    # 运行各项测试
    test_health
    test_messages_api
    test_batch_api
    test_files_api
    test_skills_api
    test_models_api
    test_admin_api
    test_proxy_config_api
    
    # 输出测试报告
    echo -e "\n${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}                       测试报告                               ${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "总测试数: ${TOTAL_TESTS}"
    echo -e "${GREEN}通过: ${PASSED_TESTS}${NC}"
    echo -e "${RED}失败: ${FAILED_TESTS}${NC}"
    
    if [ ${FAILED_TESTS} -eq 0 ]; then
        echo -e "\n${GREEN}🎉 所有测试通过！${NC}"
        exit 0
    else
        echo -e "\n${RED}⚠️  有 ${FAILED_TESTS} 个测试失败${NC}"
        exit 1
    fi
}

# 运行主函数
main "$@"
